package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

const (
	koanfDelimiter = "."
	koanfTag       = "koanf"
	configFilename = "config.yaml"
	configEnvVar   = "WOL_CONFIG"
)

// Machine represents a machine to wake up
type Machine struct {
	// Name of the machine
	Name string `koanf:"name"`
	// MAC address of the machine
	Mac string `koanf:"mac"`
	// Hostname or IP address of the machine (optional)
	IP *string `koanf:"ip"`
}

// Server represents the server configuration
type Server struct {
	// Listen address for the server
	Listen string `koanf:"listen"`
}

// Ping represents the ping configuration
type Ping struct {
	// Privileged determines if privileged ping should be used
	Privileged bool `koanf:"privileged"`
}

// Config represents the configuration for the application
type Config struct {
	// Machines represents the list of machines to wake up
	Machines []Machine `koanf:"machines"`
	// Server represents the server configuration
	Server Server `koanf:"server"`
	// Ping represents the ping configuration
	Ping Ping `koanf:"ping"`
}

// NewConfig creates a new Config instance
func NewConfig() *Config {
	return &Config{}
}

// Load loads the configuration and returns the sources that contributed to it,
// in load order (later values override earlier ones).
//
// If path is non-empty it is used as the sole config file and must exist (a
// missing file is an error). An explicit --config file is authoritative: the
// default search locations and the WOL_CONFIG environment variable are ignored.
//
// If path is empty, the default locations are searched and any that are missing
// are skipped, with WOL_CONFIG layered on top as the highest-precedence source:
//   - /etc/wol/config.yaml
//   - ~/.wol/config.yaml
//   - ./config.yaml
//
// In all cases the built-in defaults are the lowest-priority layer.
func (c *Config) Load(path string) ([]string, error) {
	// An explicit config file replaces the default search locations and must exist.
	if path != "" {
		return c.load([]string{path}, true)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Order here matters as later values will override earlier ones
	paths := []string{
		filepath.Join("/etc", "wol", configFilename),
		filepath.Join(home, ".wol", configFilename),
		filepath.Join(".", configFilename),
	}

	return c.load(paths, false)
}

// load reads configuration into c from the given file paths (in precedence order,
// later overriding earlier) layered on top of the defaults. When explicit is
// false it then layers the WOL_CONFIG environment variable on top. It returns the
// sources that actually contributed, in load order, so callers can report which
// configuration was used.
//
// explicit selects the behavior for an explicit --config file: the file must
// exist (a missing file is an error) and it is used on its own — WOL_CONFIG is
// ignored. With auto-discovery (explicit false) missing files are skipped and
// WOL_CONFIG is layered on top.
//
// It uses a fresh koanf instance per call so repeated loads don't accumulate
// state, and takes its search paths as an argument so tests can point it at
// temporary files instead of the real /etc/wol or ~/.wol.
func (c *Config) load(paths []string, explicit bool) ([]string, error) {
	k := koanf.New(koanfDelimiter)

	// Defaults are the lowest-priority layer.
	defaults := &Config{
		Server: Server{
			Listen: ":7777",
		},
		Ping: Ping{
			Privileged: false,
		},
	}
	if err := k.Load(structs.Provider(defaults, koanfTag), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	var sources []string
	for _, path := range paths {
		err := k.Load(file.Provider(path), yaml.Parser())
		if err != nil {
			if os.IsNotExist(err) {
				// A missing explicit file is an error; a missing search-path
				// location is simply skipped.
				if explicit {
					return nil, fmt.Errorf("config file %q does not exist", path)
				}
				continue
			}
			return nil, fmt.Errorf("failed to load config file %q: %w", path, err)
		}
		sources = append(sources, path)
	}

	// An explicit --config file is authoritative and used on its own; WOL_CONFIG
	// only applies when discovering configuration automatically.
	if !explicit {
		if ec := os.Getenv(configEnvVar); ec != "" {
			if err := k.Load(rawbytes.Provider([]byte(ec)), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("failed to load config from %s: %w", configEnvVar, err)
			}
			sources = append(sources, configEnvVar+" environment variable")
		}
	}

	if err := k.Unmarshal("", c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return sources, nil
}
