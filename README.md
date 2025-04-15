# wol 🦭

A CLI tool to send Wake-On-LAN (WOL) magic packets to wake up devices on your
network. Features both CLI commands and a web interface.

<img src="assets/images/web.png" alt="Web Interface" />

## Features

- Send WOL magic packets via CLI or web interface
- Configure multiple machines with names for easy access
- List configured machines
- Web interface for easy wake-up
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
            ip: "192.168.1.100"
          - name: server
            mac: "AA:BB:CC:DD:EE:FF"
            ip: "192.168.1.101"
        server:
          listen: ":7777"
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

Alternatively, you can provide the configuration via the `WOL_CONFIG` environment variable:

```sh
export WOL_CONFIG='
machines:
  - name: desktop
    mac: "00:11:22:33:44:55"
    ip: "192.168.1.100" # Optional, for status checking
  - name: server
    mac: "AA:BB:CC:DD:EE:FF"
    ip: "192.168.1.101" # Optional, for status checking

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
    schedule: "0 12 * * *" #Optional, to wake on cron schedule when running the serve command
  - name: server
    mac: "AA:BB:CC:DD:EE:FF"
    ip: "192.168.1.101" # Optional, for status checking
    schedule: "0 0 * * 3" #Optional, to wake on cron schedule when running the serve command


server:
  listen: ":7777" # Optional, defaults to :7777
```

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
- Optionally scheduled machine wake up

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

For more details, see [issue #12](https://github.com/Trugamr/wol/issues/12).

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE.md)
file for details.

## Contributing

Contributions are welcome! Feel free to open issues or submit pull requests.
