# Progress: SatPulse

## What Works

### Core Functionality

1. **GPS Communication**
   - UBX protocol support for U-blox GPS receivers
   - NMEA protocol support for universal compatibility
   - Automatic configuration of GPS receivers for timing purposes
   - Reading and processing of GPS time messages

2. **PTP Hardware Clock Synchronization**
   - Reading timestamps of pulses from the GPS receiver
   - Adjusting the PTP hardware clock to match GPS time
   - Handling NICs that generate timestamps for both edges of a pulse
   - Sawtooth correction for improved precision

3. **PTP Integration**
   - Communication with ptp4l using the PTP management protocol
   - Updating PTP grandmaster with clock class and accuracy
   - Providing leap second information to PTP grandmaster

4. **NTP Integration**
   - Generating samples for chrony using the refclock SOCK interface
   - Synchronizing the system clock with GPS time

5. **Monitoring and Management**
   - HTTP interface for monitoring
   - Command-line tool for configuration and monitoring
   - Logging through systemd and custom log files
   - TCP/IP and Unix domain socket proxying to GPS receiver

### Hardware Support

1. **Ethernet Controllers**
   - Intel i210 NICs
   - Raspberry Pi CM4/CM5 ethernet controller

2. **GPS Receivers**
   - U-blox receivers (primary support)
   - Other receivers via NMEA protocol

### Deployment

1. **Installation Methods**
   - Debian packages (.deb)
   - RPM packages (.rpm)
   - Source installation

2. **Configuration**
   - TOML configuration file
   - Systemd service integration
   - Command-line flags

3. **Documentation**
   - User documentation (installation, configuration, usage)
   - Developer documentation (architecture, APIs, protocols)
   - Blog posts for updates and use cases

## What's Left to Build

### Feature Enhancements

1. **Additional Protocol Support**
   - Support for other GPS protocols beyond UBX and NMEA
   - Enhanced RTCM support for differential GPS

2. **Monitoring Improvements**
   - More detailed statistics in the web interface
   - Graphical visualization of synchronization performance
   - Enhanced alerting for synchronization issues

3. **Configuration Enhancements**
   - More flexible configuration options
   - Better validation and error reporting for configuration
   - Dynamic reconfiguration without restart

### Hardware Support

1. **Additional Ethernet Controllers**
   - Investigation of other controllers with PPS input pins
   - Support for more specialized timing hardware

2. **Platform Expansion**
   - Better support for different Linux distributions
   - Testing on a wider range of hardware combinations

### Integration

1. **Enhanced PTP Integration**
   - Support for PTP profiles
   - Integration with other PTP implementations

2. **Enhanced NTP Integration**
   - Support for other NTP servers beyond chrony
   - More flexible NTP configuration options

### Testing and Quality

1. **Automated Testing**
   - More comprehensive unit tests
   - Integration tests for hardware-specific functionality
   - Performance benchmarking

2. **Documentation**
   - More examples and use cases
   - Troubleshooting guides
   - Performance tuning recommendations

## Current Status

The project is in a stable state with core functionality working reliably. It was recently made public on GitHub (March 11, 2025) and is now focusing on documentation, user feedback, and incremental improvements.

### Stability

- Core synchronization functionality is stable and reliable
- Integration with ptp4l and chrony is working well
- Configuration and deployment processes are well-documented

### Performance

- Achieving precision in the low tens of nanoseconds as targeted
- Performance varies depending on hardware quality and configuration
- Some edge cases still need optimization

### User Adoption

- Early adopters are using the system successfully
- Feedback is being collected and incorporated
- Documentation is being improved based on user questions

## Known Issues

1. **Hardware Compatibility**
   - Limited options for ethernet controllers with PPS input pins
   - Some hardware combinations may require specific configuration
   - Variations in behavior between different hardware implementations

2. **Configuration Complexity**
   - Some configuration options may be confusing for new users
   - Default values may not be optimal for all hardware combinations
   - Schema validation could be improved

3. **Integration Challenges**
   - Different versions of ptp4l and chrony may require different configuration
   - Some Linux distributions may have specific requirements
   - Network configuration can impact performance

4. **Performance Variability**
   - Precision can vary depending on hardware quality
   - Environmental factors can impact GPS signal quality
   - Network load can affect PTP performance

5. **Documentation Gaps**
   - Some advanced configuration options need better documentation
   - Troubleshooting guides could be more comprehensive
   - Performance tuning recommendations could be expanded

## Evolution of Project Decisions

### Protocol Support

**Initial Decision**: Focus on UBX protocol for U-blox receivers with NMEA as a fallback.

**Evolution**: This decision has proven effective, as UBX provides access to advanced timing features while NMEA ensures universal compatibility. No changes to this approach are currently planned.

### Hardware Support

**Initial Decision**: Target Intel i210 NICs and Raspberry Pi CM4/CM5 as the primary supported hardware.

**Evolution**: These remain the best options for affordable hardware with PPS input pins. Future work may explore additional hardware options if they become available.

### Integration Approach

**Initial Decision**: Work alongside existing tools (ptp4l, chrony) rather than replacing them.

**Evolution**: This approach has been successful, allowing users to leverage existing knowledge and infrastructure. The integration points (PTP management protocol, chrony refclock SOCK) have proven effective.

### Configuration Format

**Initial Decision**: Use TOML for configuration due to its human-readability.

**Evolution**: TOML has worked well for configuration, and the addition of schema validation has improved the user experience. Future work may focus on better error reporting and validation.

### Deployment Strategy

**Initial Decision**: Provide both package (.deb, .rpm) and source installation options.

**Evolution**: This approach has made installation accessible to a wide range of users. Future work may focus on improving the installation experience on different Linux distributions.

### Documentation Approach

**Initial Decision**: Focus on comprehensive documentation covering installation, configuration, and usage.

**Evolution**: Documentation has been expanded to include more detailed information about internals, hardware requirements, and real-world applications. Future work will continue to improve documentation based on user feedback.

## Next Milestones

1. **Enhanced Monitoring**: Improve the web interface with more detailed statistics and visualizations.

2. **Performance Optimization**: Further improve the precision of time synchronization, aiming for even lower jitter.

3. **User Feedback Integration**: Collect and incorporate feedback from early adopters to improve usability and features.

4. **Documentation Expansion**: Add more examples, use cases, and troubleshooting guides.

5. **Testing Framework**: Develop more comprehensive testing, particularly for hardware-specific edge cases.
