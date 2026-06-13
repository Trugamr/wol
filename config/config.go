package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	// Broadcast optionally overrides the global broadcast target for this
	// machine. Zero-value fields inherit the global setting. This is what lets a
	// machine on a different subnet/VLAN use its own directed broadcast address.
	Broadcast *Broadcast `koanf:"broadcast"`
}

// HardwareAddr parses and returns the machine's MAC address.
func (m Machine) HardwareAddr() (net.HardwareAddr, error) {
	mac, err := net.ParseMAC(m.Mac)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MAC address: %w", err)
	}
	return mac, nil
}

// Broadcast is the destination a magic packet is sent to. It is reused for the
// global default and for per-machine overrides; on an override, a zero-value
// field (empty Address or zero Port) inherits the global value.
type Broadcast struct {
	// Address is the broadcast address to send the magic packet to. Use the
	// subnet-directed broadcast (e.g. 192.168.1.255) to reliably reach a device
	// on a specific subnet from a multi-homed host, rather than the limited
	// broadcast 255.255.255.255.
	Address string `koanf:"address"`
	// Port is the UDP port to send the magic packet to (typically 9, sometimes 7).
	Port int `koanf:"port"`
}

// Addr returns the host:port string the magic packet is dialed to,
// e.g. "192.168.1.255:9".
func (b Broadcast) Addr() string {
	return net.JoinHostPort(b.Address, strconv.Itoa(b.Port))
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
	// Broadcast represents the default broadcast target for magic packets.
	Broadcast Broadcast `koanf:"broadcast"`

	// sources records which inputs contributed to the loaded config, in load
	// order. It is unexported so koanf's reflection-based providers ignore it.
	sources []string
}

// NewConfig creates a new Config instance
func NewConfig() *Config {
	return &Config{}
}

// Sources returns the inputs that contributed to the last Load, in load order
// (e.g. config file paths and the WOL_CONFIG environment variable).
func (c *Config) Sources() []string {
	return c.sources
}

// MachineByName returns the configured machine with the given name
// (case-insensitive).
func (c *Config) MachineByName(name string) (Machine, error) {
	for _, m := range c.Machines {
		if strings.EqualFold(m.Name, name) {
			return m, nil
		}
	}

	return Machine{}, fmt.Errorf("machine with name %q not found", name)
}

// BroadcastFor returns the effective broadcast target for a machine: the global
// broadcast setting with any non-zero per-machine override fields applied on top.
func (c *Config) BroadcastFor(m Machine) Broadcast {
	b := c.Broadcast
	if m.Broadcast != nil {
		if m.Broadcast.Address != "" {
			b.Address = m.Broadcast.Address
		}
		if m.Broadcast.Port != 0 {
			b.Port = m.Broadcast.Port
		}
	}
	return b
}

// Load loads the configuration. The contributing sources are recorded and can be
// retrieved with Sources (in load order, later values overriding earlier ones).
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
func (c *Config) Load(path string) error {
	// An explicit config file replaces the default search locations and must exist.
	paths := []string{path}
	explicit := true

	if path == "" {
		explicit = false
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		// Order here matters as later values will override earlier ones
		paths = []string{
			filepath.Join("/etc", "wol", configFilename),
			filepath.Join(home, ".wol", configFilename),
			filepath.Join(".", configFilename),
		}
	}

	sources, err := c.load(paths, explicit)
	if err != nil {
		return err
	}
	c.sources = sources
	return nil
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
		Broadcast: Broadcast{
			Address: "255.255.255.255",
			Port:    9,
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

	// WOL_CONFIG overrides the discovered files; an explicit --config file is
	// authoritative, so the env var is ignored in that mode.
	if ec := os.Getenv(configEnvVar); !explicit && ec != "" {
		if err := k.Load(rawbytes.Provider([]byte(ec)), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", configEnvVar, err)
		}
		sources = append(sources, configEnvVar+" environment variable")
	}

	if err := k.Unmarshal("", c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return sources, nil
}
