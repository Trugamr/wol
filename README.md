# wol 🦭

A CLI tool to send Wake-On-LAN (WOL) magic packets to wake up devices on your
network. Features both CLI commands and a web interface.

<img src="assets/images/web.png" alt="Web Interface" />

## Features

- Send WOL magic packets via CLI or web interface
- Configure multiple machines with names for easy access
- List configured machines
- Web interface for easy wake-up
- Scheduled wake-ups on a cron schedule
- Docker support

## Installation

### Pre-built binaries

Download the latest release for your platform from the
[releases page](https://github.com/trugamr/wol/releases).

Available for:

- Linux (x86_64, arm64, armv7)
- macOS (x86_64, arm64)
- Windows (x86_64)

### Using Go

```sh
go install github.com/trugamr/wol@latest
```

### Using Docker

```sh
docker run --network host -v $(pwd)/config.yaml:/etc/wol/config.yaml ghcr.io/trugamr/wol:latest
```

Or using docker-compose:

```yaml
# Method 1: Using bind mount
services:
  wol:
    image: ghcr.io/trugamr/wol:latest
    command: serve # To start the web interface
    network_mode: "host"
    volumes:
      - ./config.yaml:/etc/wol/config.yaml

# Method 2: Using environment variables
services:
  wol:
    image: ghcr.io/trugamr/wol:latest
    command: serve # To start the web interface
    network_mode: "host"
    environment:
      WOL_CONFIG: |
        machines:
          - name: desktop
            mac: "00:11:22:33:44:55"
            ip: "192.168.1.100" # Optional, for status checking
          - name: server
            mac: "AA:BB:CC:DD:EE:FF"
            ip: "server.local"
        server:
          listen: ":7777" # Optional, defaults to :7777
        ping:
          privileged: false # Optional, set to true to use privileged ping
```

Check out `examples/reverse-proxy.yml` for an example of running wol behind
reverse proxy with basic auth, https etc.

> [!NOTE]
> The config file should be mounted to `/etc/wol/config.yaml` inside the
> container. Host networking is recommended for Wake-on-LAN packets to work
> properly on your local network.

## Configuration

Create a `config.yaml` file in one of these locations (in order of precedence):

- `./config.yaml` (current directory)
- `~/.wol/config.yaml` (home directory)
- `/etc/wol/config.yaml` (system-wide)

To load a config file from an arbitrary path, pass `-c`/`--config`:

```sh
wol serve --config /etc/wol/config.yaml
```

An explicit `--config` file is authoritative: it must exist (wol exits with an
error if it doesn't), and the default search locations above and the `WOL_CONFIG`
environment variable are ignored. On startup `serve` logs which config source it
loaded.

Alternatively, you can provide the configuration via the `WOL_CONFIG` environment variable:

```sh
export WOL_CONFIG='
machines:
  - name: desktop
    mac: "00:11:22:33:44:55"
    ip: "192.168.1.100" # Optional, for status checking
  - name: server
    mac: "AA:BB:CC:DD:EE:FF"
    ip: "server.local"

server:
  listen: ":7777" # Optional, defaults to :7777
'
```

Example configuration:

```yaml
machines:
  - name: desktop
    mac: "00:11:22:33:44:55"
    ip: "192.168.1.100" # Optional, for status checking
  - name: server
    mac: "AA:BB:CC:DD:EE:FF"
    ip: "server.local"

server:
  listen: ":7777" # Optional, defaults to :7777

ping:
  privileged: false # Optional, set to true if you need privileged ping

schedules: # Optional, cron wake-ups run by `wol serve`
  - name: weekend-backup # Optional label, shown in logs
    machine: server # Must match a machine name above
    cron: "0 2 * * 6" # Saturday 02:00
```

### Scheduled wake-ups

The optional `schedules` section makes `wol serve` wake machines automatically on
a cron schedule. Omit it (or leave it empty) and scheduling is disabled — `serve`
behaves exactly as before.

Each schedule has:

- `machine` (required) — the name of a machine defined under `machines`.
- `cron` (required) — when to wake it. Standard five-field cron expressions
  (`minute hour day-of-month month day-of-week`) are supported, as are the
  `@hourly`, `@daily`, `@weekly`, `@monthly` and `@every 1h30m` descriptors.
- `name` (optional) — a label used in log output; defaults to the machine name.

Cron expressions are evaluated in the **server's local time**. When running in a
container, set the `TZ` environment variable (e.g. `TZ=America/New_York`) to
control the timezone.

Schedules are validated when `serve` starts: a schedule that references an unknown
machine or carries an invalid cron expression makes `wol serve` exit with an error,
so misconfiguration is caught immediately rather than silently never firing.

## Usage

### CLI Commands

```sh
# List all configured machines
wol list

# Wake up a machine by name
wol send --name desktop

# Wake up a machine by MAC address
wol send --mac "00:11:22:33:44:55"

# Start the web interface
wol serve

# Start the web interface with an explicit config file
wol serve --config /etc/wol/config.yaml

# Show version information
wol version
```

### Web Interface

The web interface is available at `http://localhost:7777` when running the serve
command. It provides:

- List of all configured machines
- One-click wake up buttons
- Real-time machine status monitoring (when IP is configured)
- Version information
- Links to documentation and support

## Building from Source

```sh
# Clone the repository
git clone https://github.com/trugamr/wol.git
cd wol

# Build
go build

# Run
./wol
```

## Known Issues

### Docker Container Ping Permissions

When running in a Docker container, the machine status feature that uses ping may not work due to permission issues. This is because the application uses [pro-bing](https://github.com/prometheus-community/pro-bing) for sending pings, which requires specific Linux kernel settings.

To fix this issue, you need to set the following sysctl parameter on your host system:

```sh
sysctl -w net.ipv4.ping_group_range="0 2147483647"
```

To make this change persistent, add it to your `/etc/sysctl.conf` file.

You can also try experimenting with setting `ping.privileged: true` in your configuration as an alternative solution.

For more details, see [issue #12](https://github.com/Trugamr/wol/issues/12).

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE.md)
file for details.

## Contributing

Contributions are welcome! Feel free to open issues or submit pull requests.
