# System Patterns: SatPulse

## System Architecture

SatPulse follows a layered architecture with clear separation of concerns. The system is organized into the following layers:

1. **Command-line Layer**: User interface components
2. **Application Layer**: Core application logic and orchestration
3. **Domain Layer**: Domain-specific abstractions
4. **Library Layer**: Reusable utilities and low-level functionality

### Key Components

1. **satpulsed**: The main daemon that runs continuously to maintain time synchronization
2. **satpulsetool**: Command-line tool for configuration and monitoring
3. **GPS Interface**: Communicates with GPS receivers using UBX or NMEA protocols
4. **PHC Interface**: Interacts with the PTP Hardware Clock in the ethernet controller
5. **PTP Integration**: Communicates with ptp4l using the PTP management protocol
6. **NTP Integration**: Provides samples to chrony using the refclock SOCK interface
7. **HTTP Interface**: Provides monitoring capabilities via a web interface
8. **Proxy Service**: Allows network access to the GPS receiver

## Key Technical Decisions

1. **Go Programming Language**: SatPulse is implemented in Go, which provides:
   - Strong typing and memory safety
   - Excellent concurrency support through goroutines and channels
   - Good performance for system-level programming
   - Cross-compilation capabilities for different architectures

2. **Linux-Only Support**: SatPulse relies on Linux-specific APIs for:
   - Accessing the PTP hardware clock
   - Interacting with network interfaces
   - Utilizing the terminal interface for serial communication

3. **Protocol Support**:
   - Primary support for UBX protocol (U-blox GPS receivers)
   - Secondary support for NMEA protocol (universal compatibility)
   - PTP management protocol for communicating with ptp4l
   - Chrony refclock SOCK protocol for NTP integration

4. **Configuration Approach**:
   - TOML configuration file for human-readable configuration
   - Systemd integration for service management
   - Command-line flags for runtime options

5. **Hardware Abstraction**:
   - Abstraction for PTP hardware clock access
   - Abstraction for GPS protocol implementations
   - Abstraction for serial and network communication

## Design Patterns in Use

1. **Layered Architecture**: Clear separation between command-line, application, domain, and library layers

2. **Dependency Injection**: Components receive their dependencies rather than creating them

3. **Goroutines and Channels**: Concurrent processing with message passing
   - GPS packet reading and processing
   - Timestamp reading and processing
   - Event handling and dispatching

4. **Event-Driven Architecture**: System components communicate through events
   - GPS events (messages, pulses)
   - Timestamp events
   - Synchronization events

5. **Adapter Pattern**: Adapting between different interfaces
   - GPS protocol adapters (UBX, NMEA)
   - PTP management client adapter
   - Chrony refclock adapter
   - Protocol-agnostic packet processors (see [design-protocol-agnostic-processing.md](design-protocol-agnostic-processing.md))

6. **Factory Pattern**: Creating instances of protocol implementations

7. **Observer Pattern**: Components observe and react to events
   - Monitoring synchronization status
   - Updating PTP grandmaster
   - Generating samples for chrony

8. **Command Pattern**: Encapsulating operations as objects
   - GPS commands
   - PTP management commands

## Component Relationships

1. **GPS Communication Flow**:
   ```
   gpsio → scan → gpsprot → (ubx or nmea) → gpsevent → combine → mon → servo
   ```
   
   A planned refactoring ([design-protocol-agnostic-processing.md](design-protocol-agnostic-processing.md)) will improve this flow by decoupling both the `gpsevent.Dispatcher` and `gpscfg` package from specific protocols:
   ```
   gpsio → scan → [protocol-specific processors] → gpsevent → combine → mon → servo
   ```

2. **Timestamp Flow**:
   ```
   ts → gpsevent → combine → mon → servo
   ```

3. **PTP Integration Flow**:
   ```
   mon → pmc → ptp4l
   ```

4. **NTP Integration Flow**:
   ```
   mon → sockrefclock → chrony
   ```

5. **HTTP Monitoring Flow**:
   ```
   mon → web interface
   ```

6. **GPS Proxy Flow**:
   ```
   gpsio → proxy → TCP/Unix socket clients
   ```

## Critical Implementation Paths

1. **GPS Configuration Path**:
   - Read GPS receiver configuration
   - Determine optimal configuration for timing
   - Apply configuration changes
   - Verify configuration success

2. **Synchronization Path**:
   - Receive GPS time pulse (PPS)
   - Timestamp pulse with PHC
   - Receive GPS time message
   - Combine pulse timestamp with time message
   - Calculate offset between PHC and GPS time
   - Adjust PHC frequency to minimize offset
   - Monitor synchronization status

3. **PTP Integration Path**:
   - Monitor synchronization status
   - Determine PTP clock class and accuracy
   - Update PTP grandmaster via management protocol
   - Provide leap second information

4. **NTP Integration Path**:
   - Take simultaneous samples of system clock and PHC
   - Calculate offset between system clock and true UTC time
   - Send samples to chrony via SOCK interface

## Error Handling and Recovery

1. **GPS Signal Issues**:
   - Detect missing or irregular pulses
   - Filter out outliers in pulse timestamps
   - Maintain PHC stability during temporary GPS signal loss

2. **Hardware Quirks**:
   - Handle NICs that generate timestamps for both edges of a pulse
   - Work around Raspberry Pi CM4/CM5 specific issues

3. **Configuration Errors**:
   - Validate configuration parameters
   - Provide clear error messages for misconfiguration
   - Fall back to defaults when appropriate

4. **Communication Failures**:
   - Retry failed communications with GPS receiver
   - Reconnect to PTP and NTP services if connection is lost
   - Log communication failures for troubleshooting

## Performance Considerations

1. **Timing Precision**:
   - Minimize processing latency for timestamp handling
   - Use hardware timestamping capabilities of ethernet controller
   - Apply sawtooth correction for improved precision

2. **Resource Usage**:
   - Efficient goroutine management
   - Minimal memory allocation in critical paths
   - Appropriate buffer sizes for channels

3. **Scalability**:
   - Support for multiple HTTP listeners
   - Support for multiple GPS proxy connections
   - Efficient handling of concurrent client requests
