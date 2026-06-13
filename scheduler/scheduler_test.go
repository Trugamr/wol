package scheduler

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

	t.Run("valid schedules resolve to machine MACs", func(t *testing.T) {
		resolved, err := resolveSchedules(machines, []config.Schedule{
			{Name: "weekend-backup", Machine: "nas", Cron: "0 2 * * 6"},
			{Machine: "nas", Cron: "@daily"},
		})
		require.NoError(t, err)
		require.Len(t, resolved, 2)

		want, err := net.ParseMAC("01:02:03:04:05:06")
		require.NoError(t, err)

		assert.Equal(t, "weekend-backup", resolved[0].label)
		assert.Equal(t, want, resolved[0].mac)
		assert.NotNil(t, resolved[0].schedule)

		// An omitted name falls back to the machine name in the label.
		assert.Equal(t, "nas", resolved[1].label)
	})

	t.Run("empty input yields no schedules", func(t *testing.T) {
		resolved, err := resolveSchedules(machines, nil)
		require.NoError(t, err)
		assert.Empty(t, resolved)
	})

	t.Run("unknown machine fails fast", func(t *testing.T) {
		_, err := resolveSchedules(machines, []config.Schedule{
			{Name: "ghost-wake", Machine: "ghost", Cron: "@daily"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ghost-wake")
		assert.Contains(t, err.Error(), "ghost")
	})

	t.Run("invalid cron fails fast", func(t *testing.T) {
		_, err := resolveSchedules(machines, []config.Schedule{
			{Name: "bad-cron", Machine: "nas", Cron: "not a cron"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad-cron")
		assert.Contains(t, err.Error(), "not a cron")
	})

	t.Run("unparseable machine MAC fails fast", func(t *testing.T) {
		_, err := resolveSchedules(machines, []config.Schedule{
			{Machine: "badmac", Cron: "@daily"},
		})
		require.Error(t, err)
	})
}

func TestWakeJob(t *testing.T) {
	want, err := net.ParseMAC("01:02:03:04:05:06")
	require.NoError(t, err)

	var gotMac net.HardwareAddr
	waker := func(mac net.HardwareAddr) error {
		gotMac = mac
		return nil
	}

	wakeJob(waker, "nas", want)()

	assert.Equal(t, want, gotMac)
}

func TestNewInvalidConfig(t *testing.T) {
	machines := []config.Machine{{Name: "nas", Mac: "01:02:03:04:05:06"}}

	_, err := New(machines, []config.Schedule{
		{Name: "ghost-wake", Machine: "ghost", Cron: "@daily"},
	}, func(net.HardwareAddr) error { return nil })
	require.Error(t, err)
}
