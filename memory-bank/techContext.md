# Technical Context: SatPulse

## Technologies Used

### Programming Languages

1. **Go (Golang)**
   - Primary implementation language
   - Version: Go 1.23.0 (as of 2025)
   - Used for all backend components, including daemon and command-line tool

2. **TypeScript**
   - Used for the web interface
   - Transpiled to JavaScript for browser compatibility

3. **JavaScript**
   - Used in the web interface
   - Preact library for UI components

### Protocols

1. **UBX Protocol**
   - Binary protocol for U-blox GPS receivers
   - Provides access to advanced timing features
   - Supports configuration, monitoring, and data retrieval

2. **NMEA Protocol**
   - ASCII protocol for GPS receivers
   - Universal compatibility
   - Limited timing capabilities

3. **PTP (Precision Time Protocol)**
   - IEEE 1588 standard for precise time synchronization
   - Hardware timestamping for nanosecond-level precision
   - Management protocol for configuration and monitoring

4. **NTP (Network Time Protocol)**
   - Protocol for time synchronization over packet-switched networks
   - Integration via chrony's refclock SOCK interface

5. **HTTP/HTML/CSS**
   - Used for the monitoring web interface

### System Integration

1. **Systemd**
   - Service management
   - Logging via journald
   - Socket activation

2. **Linux Kernel APIs**
   - PTP hardware clock access
   - Network interface management
   - Serial port communication

3. **Unix Domain Sockets**
   - Communication with ptp4l
   - Communication with chrony
   - GPS proxy functionality

## Development Setup

### Build System

1. **Make**
   - Primary build tool
   - Targets for build, install, test, clean

2. **Go Modules**
   - Dependency management
   - Version control for Go packages

3. **Node.js/npm**
   - Web interface development
   - TypeScript transpilation
   - JavaScript bundling

### Testing

1. **Go Testing Framework**
   - Unit tests for Go packages
   - Integration tests for system components

2. **Jest**
   - Testing for TypeScript/JavaScript components

### Packaging

1. **Debian Packaging**
   - .deb packages for Debian-based distributions
   - Includes systemd service files and configuration

2. **RPM Packaging**
   - .rpm packages for Fedora-based distributions
   - Includes systemd service files and configuration

## Technical Constraints

### Hardware Constraints

1. **Ethernet Controller Requirements**
   - Must have a PPS input pin
   - Must have Linux drivers that support PTP hardware clock
   - Must support external timestamping
   - Limited options: Intel i210, Raspberry Pi CM4/CM5 ethernet controller

2. **GPS Receiver Requirements**
   - Must provide a PPS output signal
   - Must be electrically compatible with the ethernet controller's PPS input
   - Recommended: U-blox receivers for UBX protocol support

3. **Platform Requirements**
   - Linux operating system
   - Systemd for service management
   - Access to PTP hardware clock APIs

### Software Constraints

1. **Linux-Only Support**
   - Relies on Linux-specific APIs
   - No support for Windows, macOS, or other operating systems

2. **Protocol Limitations**
   - UBX protocol: Limited to U-blox receivers or compatible clones
   - NMEA protocol: Limited timing capabilities

3. **Integration Requirements**
   - Requires LinuxPTP (ptp4l) for PTP server functionality
   - Requires chrony for NTP server functionality

## Dependencies

### External Go Dependencies

1. **golang.org/x/exp**: Experimental Go packages
2. **golang.org/x/sys**: System-level Go packages
3. **github.com/jclark/crc24q**: CRC-24Q implementation
4. **github.com/mdlayher/netlink**: Netlink protocol implementation
5. **gopkg.in/yaml.v3**: YAML parsing
6. **github.com/spf13/pflag**: Command-line flag parsing
7. **github.com/pelletier/go-toml/v2**: TOML configuration parsing

### External System Dependencies

1. **LinuxPTP (ptp4l)**: PTP server implementation
2. **chrony**: NTP server implementation
3. **systemd**: Service management

### JavaScript Dependencies

1. **Preact**: Lightweight React alternative for UI components
2. **TypeScript**: Type-safe JavaScript
3. **Jest**: JavaScript testing framework

## Tool Usage Patterns

### Development Workflow

1. **Code Organization**
   - Command-line layer: `cmd/` directory
   - Application layer: Top-level packages in `internal/`
   - Domain layer: Protocol-specific packages in `internal/`
   - Library layer: Utility packages in `internal/`

2. **Testing Approach**
   - Unit tests for individual packages
   - Integration tests for system components
   - System tests using Ansible for end-to-end testing

3. **Documentation**
   - Markdown documentation in `docs/` directory
   - Jekyll for website generation
   - Code comments for API documentation

### Deployment Workflow

1. **Installation Methods**
   - Package installation (.deb, .rpm)
   - Source installation (make install)

2. **Configuration**
   - TOML configuration file
   - Systemd service template
   - Command-line flags for runtime options

3. **Monitoring**
   - HTTP interface for status monitoring
   - Systemd journal for logging
   - Custom log files for detailed analysis

## Development Practices

1. **Code Style**
   - Go standard formatting (gofmt)
   - Go linting tools
   - TypeScript/JavaScript linting

2. **Version Control**
   - Git for source control
   - GitHub for hosting and collaboration
   - Semantic versioning for releases

3. **Release Process**
   - Tagged releases on GitHub
   - Binary packages for different platforms
   - Release notes with changes and improvements

4. **Documentation**
   - User documentation (installation, configuration, usage)
   - Developer documentation (architecture, APIs, protocols)
   - Blog posts for updates and use cases
