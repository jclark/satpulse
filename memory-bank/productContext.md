# Product Context: SatPulse

## Why This Project Exists

SatPulse exists to make high-precision time synchronization accessible and affordable for local networks. Traditional NTP-based time synchronization typically achieves millisecond-level precision, which is insufficient for many modern applications. While PTP (Precision Time Protocol) can achieve much higher precision, setting up a PTP time server with a GPS reference has traditionally been complex and expensive.

SatPulse bridges this gap by:

1. Simplifying the setup and configuration of a high-precision time server
2. Leveraging low-cost hardware (under $150) to achieve nanosecond-level precision
3. Providing a complete solution that works with both PTP and NTP clients
4. Automating the configuration of GPS receivers for optimal timing performance

## Problems It Solves

1. **Complexity of High-Precision Timing**: Setting up a high-precision time server traditionally requires specialized knowledge and complex configuration. SatPulse simplifies this process with clear documentation and automated configuration.

2. **Cost Barrier**: Commercial high-precision time servers can cost thousands of dollars. SatPulse works with affordable hardware like the Raspberry Pi Compute Module 4/5 or Intel i210 NICs.

3. **Integration Challenges**: Existing solutions like LinuxPTP's ts2phc and gpsd don't fully address the needs of a PTP time server with GPS reference. SatPulse integrates seamlessly with LinuxPTP (ptp4l) and chrony to provide a complete solution.

4. **Hardware Limitations**: Traditional NTP setups connect GPS PPS to GPIO or serial pins, limiting precision due to interrupt latency. SatPulse connects GPS PPS directly to the ethernet controller's PPS input, enabling nanosecond-level precision.

5. **Protocol Limitations**: The NMEA protocol used by many GPS solutions provides limited timing information. SatPulse supports the UBX protocol for U-blox receivers, enabling access to advanced timing features like sawtooth correction and atomic time.

## Real-World Applications

SatPulse enables precise time synchronization for various applications:

1. **Telecom & 5G**: Coordinating basestation transmissions and supporting split architecture
2. **Power & Energy**: Synchronizing synchrophasors for grid stability monitoring
3. **Audio-Visual**: Synchronizing multiple audio/video devices for recording and playback
4. **Industrial Automation**: Coordinating sensors, material handling, and actuation systems
5. **Automotive**: Supporting Advanced Driver Assistance Systems and entertainment systems
6. **Finance**: Meeting regulatory requirements for transaction timestamping
7. **Datacenters**: Improving efficiency in distributed systems and network monitoring
8. **Scientific Research**: Supporting distributed data acquisition and processing

## How It Should Work

SatPulse operates as a daemon (satpulsed) that:

1. Communicates with a GPS receiver over a serial port
2. Reads timestamps of pulses from the GPS receiver
3. Adjusts the PTP hardware clock (PHC) to match the GPS time
4. Provides metadata to the PTP server (ptp4l) about clock accuracy and leap seconds
5. Generates samples for the NTP server (chrony) to synchronize the system clock

The user connects a GPS receiver to their computer via:
- Serial connection for data
- PPS connection to the ethernet controller's PPS input pin

After installation and configuration, SatPulse runs as a systemd service, continuously maintaining synchronization between the GPS time and the PTP hardware clock.

## User Experience Goals

1. **Simple Setup**: Users should be able to set up a high-precision time server with minimal effort, following clear documentation.

2. **Minimal Configuration**: The configuration file should be simple and well-documented, with sensible defaults.

3. **Reliable Operation**: Once configured, SatPulse should operate reliably without requiring frequent user intervention.

4. **Informative Monitoring**: Users should be able to monitor the status of their time server through the HTTP interface and system logs.

5. **Flexible Integration**: Users should be able to use SatPulse with different GPS receivers, ethernet controllers, and time server configurations.

6. **Transparent Operation**: Users should understand how SatPulse works and how it integrates with other components like ptp4l and chrony.

7. **Graceful Error Handling**: SatPulse should recover from common errors like occasional GPS signal issues without requiring user intervention.

## Target Users

1. **Network Administrators**: Setting up time servers for organizational networks
2. **Industrial Automation Engineers**: Implementing precise timing for industrial systems
3. **Hobbyists and Enthusiasts**: Experimenting with high-precision timing
4. **Researchers**: Implementing timing solutions for scientific applications
5. **Developers**: Building applications that require precise timing
