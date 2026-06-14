package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfig writes content to a config file in a fresh temp dir and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), configFilename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv(configEnvVar, "")

	c := NewConfig()
	sources, err := c.load(nil, false)
	require.NoError(t, err)

	assert.Equal(t, ":7777", c.Server.Listen)
	assert.False(t, c.Ping.Privileged)
	assert.Empty(t, c.Machines)
	// The broadcast target defaults to the limited broadcast on port 9.
	assert.Equal(t, "255.255.255.255", c.Broadcast.Address)
	assert.Equal(t, 9, c.Broadcast.Port)
	// Defaults alone are not a config "source".
	assert.Empty(t, sources)
}

func TestBroadcastForResolution(t *testing.T) {
	c := &Config{Broadcast: Broadcast{Address: "255.255.255.255", Port: 9}}

	tests := []struct {
		name    string
		machine Machine
		want    Broadcast
	}{
		{
			name:    "no override inherits global",
			machine: Machine{Name: "a"},
			want:    Broadcast{Address: "255.255.255.255", Port: 9},
		},
		{
			name:    "full override",
			machine: Machine{Name: "b", Broadcast: &Broadcast{Address: "192.168.1.255", Port: 7}},
			want:    Broadcast{Address: "192.168.1.255", Port: 7},
		},
		{
			name:    "address-only override inherits port",
			machine: Machine{Name: "c", Broadcast: &Broadcast{Address: "192.168.20.255"}},
			want:    Broadcast{Address: "192.168.20.255", Port: 9},
		},
		{
			name:    "port-only override inherits address",
			machine: Machine{Name: "d", Broadcast: &Broadcast{Port: 7}},
			want:    Broadcast{Address: "255.255.255.255", Port: 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.BroadcastFor(tt.machine)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBroadcastAddr(t *testing.T) {
	assert.Equal(t, "192.168.1.255:9", Broadcast{Address: "192.168.1.255", Port: 9}.Addr())
}

func TestMachineByName(t *testing.T) {
	c := &Config{Machines: []Machine{
		{Name: "alpha", Mac: "01:02:03:04:05:06"},
		{Name: "beta", Mac: "0a:0b:0c:0d:0e:0f"},
	}}

	tests := []struct {
		name      string
		lookup    string
		wantName  string
		wantError bool
	}{
		{name: "exact match", lookup: "alpha", wantName: "alpha"},
		{name: "case-insensitive match", lookup: "ALPHA", wantName: "alpha"},
		{name: "not found", lookup: "ghost", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine, err := c.MachineByName(tt.lookup)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, machine.Name)
		})
	}
}

func TestMachineHardwareAddr(t *testing.T) {
	mac, err := Machine{Mac: "01:02:03:04:05:06"}.HardwareAddr()
	require.NoError(t, err)
	assert.Equal(t, "01:02:03:04:05:06", mac.String())

	_, err = Machine{Mac: "not-a-mac"}.HardwareAddr()
	require.Error(t, err)
}

func TestLoadBroadcastFromFile(t *testing.T) {
	t.Setenv(configEnvVar, "")

	file := writeConfig(t, `
broadcast:
  address: "192.168.0.255"
  port: 7
machines:
  - name: alpha
    mac: "01:02:03:04:05:06"
    broadcast:
      address: "192.168.20.255"
`)

	c := NewConfig()
	_, err := c.load([]string{file}, false)
	require.NoError(t, err)

	// Global broadcast comes from the file.
	assert.Equal(t, "192.168.0.255", c.Broadcast.Address)
	assert.Equal(t, 7, c.Broadcast.Port)

	// The per-machine override sets only the address, inheriting the global port.
	require.Len(t, c.Machines, 1)
	assert.Equal(t, "192.168.20.255:7", c.BroadcastFor(c.Machines[0]).Addr())
}

func TestLoadGlobalBroadcastPartialKeepsDefaultPort(t *testing.T) {
	t.Setenv(configEnvVar, "")

	// Only the address is set globally; the port must fall back to the default 9
	// (relies on config layers merging per key rather than replacing the block).
	file := writeConfig(t, `
broadcast:
  address: "192.168.0.255"
`)

	c := NewConfig()
	_, err := c.load([]string{file}, false)
	require.NoError(t, err)

	assert.Equal(t, "192.168.0.255", c.Broadcast.Address)
	assert.Equal(t, 9, c.Broadcast.Port)
}

func TestLoadFilePrecedence(t *testing.T) {
	t.Setenv(configEnvVar, "")

	lower := writeConfig(t, `
server:
  listen: ":1111"
ping:
  privileged: true
machines:
  - name: alpha
    mac: "01:02:03:04:05:06"
`)
	higher := writeConfig(t, `
server:
  listen: ":2222"
machines:
  - name: beta
    mac: "0a:0b:0c:0d:0e:0f"
`)

	c := NewConfig()
	sources, err := c.load([]string{lower, higher}, false)
	require.NoError(t, err)

	// Later file overrides the listen address...
	assert.Equal(t, ":2222", c.Server.Listen)
	// ...while a key only the earlier file set is preserved.
	assert.True(t, c.Ping.Privileged)
	// Slices are replaced wholesale, not merged: the higher-precedence
	// machines list wins entirely.
	require.Len(t, c.Machines, 1)
	assert.Equal(t, "beta", c.Machines[0].Name)
	// Both files are reported as sources, in load order.
	assert.Equal(t, []string{lower, higher}, sources)
}

func TestLoadWOLConfigOverride(t *testing.T) {
	file := writeConfig(t, `
server:
  listen: ":1111"
`)
	t.Setenv(configEnvVar, `
server:
  listen: ":9999"
`)

	c := NewConfig()
	sources, err := c.load([]string{file}, false)
	require.NoError(t, err)

	// In search mode WOL_CONFIG is layered on top, so it overrides config files.
	assert.Equal(t, ":9999", c.Server.Listen)
	assert.Equal(t, []string{file, configEnvVar + " environment variable"}, sources)
}

func TestLoadExplicitIgnoresEnv(t *testing.T) {
	// File sets listen and omits ping; env sets both.
	file := writeConfig(t, `
server:
  listen: ":1234"
`)
	t.Setenv(configEnvVar, `
server:
  listen: ":9999"
ping:
  privileged: true
`)

	c := NewConfig()
	sources, err := c.load([]string{file}, true)
	require.NoError(t, err)

	// An explicit file is used on its own: WOL_CONFIG is ignored entirely, so the
	// file's value wins and a key the file omits falls back to the default.
	assert.Equal(t, ":1234", c.Server.Listen)
	assert.False(t, c.Ping.Privileged)
	// Only the file is reported as a source.
	assert.Equal(t, []string{file}, sources)
}

func TestLoadMissingFilesIgnored(t *testing.T) {
	t.Setenv(configEnvVar, "")

	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	c := NewConfig()
	// Missing search-path files are skipped rather than failing the load.
	sources, err := c.load([]string{missing}, false)
	require.NoError(t, err)
	assert.Equal(t, ":7777", c.Server.Listen)
	assert.Empty(t, sources)
}

func TestLoadRequiredMissingErrors(t *testing.T) {
	t.Setenv(configEnvVar, "")

	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	c := NewConfig()
	// An explicit (required) file that is missing is an error.
	_, err := c.load([]string{missing}, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestLoadMalformedYAML(t *testing.T) {
	t.Setenv(configEnvVar, "")

	bad := writeConfig(t, "machines: [unterminated\n")

	c := NewConfig()
	_, err := c.load([]string{bad}, false)
	require.Error(t, err)
}

func TestLoadDoesNotShareState(t *testing.T) {
	t.Setenv(configEnvVar, "")

	withMachine := writeConfig(t, `
machines:
  - name: alpha
    mac: "01:02:03:04:05:06"
`)

	first := NewConfig()
	_, err := first.load([]string{withMachine}, false)
	require.NoError(t, err)
	require.Len(t, first.Machines, 1)

	// A second load with no files must not inherit the first load's machines.
	// Guards against regressing to a shared package-global koanf instance.
	second := NewConfig()
	_, err = second.load(nil, false)
	require.NoError(t, err)
	assert.Empty(t, second.Machines)
	assert.Equal(t, ":7777", second.Server.Listen)
}

func TestLoadExplicitPath(t *testing.T) {
	t.Setenv(configEnvVar, "")

	// A non-empty path is used as-is, so this stays hermetic (no $HOME / /etc).
	file := writeConfig(t, `
server:
  listen: ":4242"
`)

	c := NewConfig()
	err := c.Load(file)
	require.NoError(t, err)
	assert.Equal(t, ":4242", c.Server.Listen)
	assert.Equal(t, []string{file}, c.Sources())
}

func TestLoadExplicitMissing(t *testing.T) {
	t.Setenv(configEnvVar, "")

	missing := filepath.Join(t.TempDir(), "nope.yaml")

	c := NewConfig()
	err := c.Load(missing)
	require.Error(t, err)
}

func TestLoadSchedules(t *testing.T) {
	t.Setenv(configEnvVar, "")

	file := writeConfig(t, `
machines:
  - name: nas
    mac: "01:02:03:04:05:06"
schedules:
  - name: weekend-backup
    machine: nas
    cron: "0 2 * * 6"
  - machine: nas
    cron: "@daily"
`)

	c := NewConfig()
	_, err := c.load([]string{file}, true)
	require.NoError(t, err)

	require.Len(t, c.Schedules, 2)
	assert.Equal(t, Schedule{Name: "weekend-backup", Machine: "nas", Cron: "0 2 * * 6"}, c.Schedules[0])
	// The optional name may be omitted.
	assert.Equal(t, Schedule{Machine: "nas", Cron: "@daily"}, c.Schedules[1])
}

func TestLoadSchedulesAbsent(t *testing.T) {
	t.Setenv(configEnvVar, "")

	// A config without a schedules section leaves Schedules empty (disabled).
	file := writeConfig(t, `
machines:
  - name: nas
    mac: "01:02:03:04:05:06"
`)

	c := NewConfig()
	_, err := c.load([]string{file}, true)
	require.NoError(t, err)
	assert.Empty(t, c.Schedules)
}
