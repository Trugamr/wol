package cmd

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trugamr/wol/config"
)

func TestResolveSchedules(t *testing.T) {
	machines := []config.Machine{
		{Name: "nas", Mac: "01:02:03:04:05:06"},
		{Name: "badmac", Mac: "not-a-mac"},
	}
	newCfg := func(schedules ...config.Schedule) *config.Config {
		return &config.Config{
			Broadcast: config.Broadcast{Address: "255.255.255.255", Port: 9},
			Machines:  machines,
			Schedules: schedules,
		}
	}

	t.Run("valid schedules resolve to MAC and broadcast", func(t *testing.T) {
		resolved, err := resolveSchedules(newCfg(
			config.Schedule{Name: "weekend-backup", Machine: "nas", Cron: "0 2 * * 6"},
			config.Schedule{Machine: "nas", Cron: "@daily"},
		))
		require.NoError(t, err)
		require.Len(t, resolved, 2)

		want, err := net.ParseMAC("01:02:03:04:05:06")
		require.NoError(t, err)

		assert.Equal(t, "weekend-backup", resolved[0].label)
		assert.Equal(t, want, resolved[0].mac)
		assert.Equal(t, "255.255.255.255:9", resolved[0].addr)
		assert.NotNil(t, resolved[0].schedule)

		// An omitted name falls back to the machine name in the label.
		assert.Equal(t, "nas", resolved[1].label)
	})

	t.Run("per-machine broadcast override is honored", func(t *testing.T) {
		cfg := &config.Config{
			Broadcast: config.Broadcast{Address: "255.255.255.255", Port: 9},
			Machines: []config.Machine{
				{Name: "nas", Mac: "01:02:03:04:05:06", Broadcast: &config.Broadcast{Address: "192.168.1.255"}},
			},
			Schedules: []config.Schedule{{Machine: "nas", Cron: "@daily"}},
		}
		resolved, err := resolveSchedules(cfg)
		require.NoError(t, err)
		require.Len(t, resolved, 1)
		// Address overridden per machine, port inherited from the global default.
		assert.Equal(t, "192.168.1.255:9", resolved[0].addr)
	})

	t.Run("empty input yields no schedules", func(t *testing.T) {
		resolved, err := resolveSchedules(newCfg())
		require.NoError(t, err)
		assert.Empty(t, resolved)
	})

	t.Run("unknown machine fails fast", func(t *testing.T) {
		_, err := resolveSchedules(newCfg(
			config.Schedule{Name: "ghost-wake", Machine: "ghost", Cron: "@daily"},
		))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ghost-wake")
		assert.Contains(t, err.Error(), "ghost")
	})

	t.Run("invalid cron fails fast", func(t *testing.T) {
		_, err := resolveSchedules(newCfg(
			config.Schedule{Name: "bad-cron", Machine: "nas", Cron: "not a cron"},
		))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad-cron")
		assert.Contains(t, err.Error(), "not a cron")
	})

	t.Run("unparseable machine MAC fails fast", func(t *testing.T) {
		_, err := resolveSchedules(newCfg(
			config.Schedule{Machine: "badmac", Cron: "@daily"},
		))
		require.Error(t, err)
	})
}

func TestWakeJob(t *testing.T) {
	want, err := net.ParseMAC("01:02:03:04:05:06")
	require.NoError(t, err)

	var gotMac net.HardwareAddr
	var gotAddr string
	wake := func(mac net.HardwareAddr, addr string) error {
		gotMac = mac
		gotAddr = addr
		return nil
	}

	wakeJob(wake, resolvedSchedule{label: "nas", mac: want, addr: "192.168.1.255:9"})()

	assert.Equal(t, want, gotMac)
	assert.Equal(t, "192.168.1.255:9", gotAddr)
}

func TestNewSchedulerInvalidConfig(t *testing.T) {
	cfg := &config.Config{
		Machines:  []config.Machine{{Name: "nas", Mac: "01:02:03:04:05:06"}},
		Schedules: []config.Schedule{{Name: "ghost-wake", Machine: "ghost", Cron: "@daily"}},
	}
	_, err := newScheduler(cfg, func(net.HardwareAddr, string) error { return nil })
	require.Error(t, err)
}
