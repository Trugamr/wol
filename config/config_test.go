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
	require.NoError(t, c.load(nil))

	assert.Equal(t, ":7777", c.Server.Listen)
	assert.False(t, c.Ping.Privileged)
	assert.Empty(t, c.Machines)
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
	require.NoError(t, c.load([]string{lower, higher}))

	// Later file overrides the listen address...
	assert.Equal(t, ":2222", c.Server.Listen)
	// ...while a key only the earlier file set is preserved.
	assert.True(t, c.Ping.Privileged)
	// Slices are replaced wholesale, not merged: the higher-precedence
	// machines list wins entirely.
	require.Len(t, c.Machines, 1)
	assert.Equal(t, "beta", c.Machines[0].Name)
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
	require.NoError(t, c.load([]string{file}))

	// WOL_CONFIG overrides values from config files.
	assert.Equal(t, ":9999", c.Server.Listen)
}

func TestLoadMissingFilesIgnored(t *testing.T) {
	t.Setenv(configEnvVar, "")

	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	c := NewConfig()
	// Missing files are skipped rather than failing the load.
	require.NoError(t, c.load([]string{missing}))
	assert.Equal(t, ":7777", c.Server.Listen)
}

func TestLoadMalformedYAML(t *testing.T) {
	t.Setenv(configEnvVar, "")

	bad := writeConfig(t, "machines: [unterminated\n")

	c := NewConfig()
	require.Error(t, c.load([]string{bad}))
}

func TestLoadDoesNotShareState(t *testing.T) {
	t.Setenv(configEnvVar, "")

	withMachine := writeConfig(t, `
machines:
  - name: alpha
    mac: "01:02:03:04:05:06"
`)

	first := NewConfig()
	require.NoError(t, first.load([]string{withMachine}))
	require.Len(t, first.Machines, 1)

	// A second load with no files must not inherit the first load's machines.
	// Guards against regressing to a shared package-global koanf instance.
	second := NewConfig()
	require.NoError(t, second.load(nil))
	assert.Empty(t, second.Machines)
	assert.Equal(t, ":7777", second.Server.Listen)
}
