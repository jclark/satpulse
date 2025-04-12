# Project Brief: SatPulse

## Overview
SatPulse is an open-source program that enables precise time synchronization on local networks. It achieves this by connecting a GPS receiver's PPS (pulse-per-second) output to an ethernet controller's PPS input pin, leveraging hardware support for PTP (Precision Time Protocol) to achieve synchronization precision in the low tens of nanoseconds.

## Core Requirements

1. **High Precision Time Synchronization**
   - Enable time synchronization with precision in the low tens of nanoseconds
   - Utilize hardware support for PTP in ethernet controllers
   - Connect GPS PPS output to ethernet controller PPS input

2. **Integration with Existing Time Services**
   - Work with PTP server (ptp4l) from LinuxPTP project
   - Work with NTP server from chrony project
   - Provide time source for both PTP and NTP clients

3. **GPS Receiver Support**
   - Primary support for U-blox GPS receivers using UBX protocol
   - Secondary support for universal NMEA protocol
   - Configure GPS receivers for optimal timing performance

4. **System Architecture**
   - Daemon (satpulsed) for continuous operation
   - Command-line tool (satpulsetool) for configuration and monitoring
   - HTTP interface for monitoring
   - Support for TCP/IP and Unix domain socket proxying to GPS receiver

5. **Platform Support**
   - Linux only (requires Linux-specific APIs)
   - Support for specific hardware: Intel i210 NICs, Raspberry Pi CM4/CM5 with IO board

## Project Goals

1. **Ease of Use**
   - Make it easy to run a high-precision time server
   - Provide clear documentation for setup and configuration
   - Automate GPS receiver configuration for timing purposes

2. **Affordability**
   - Enable high-precision time synchronization with low-cost hardware
   - Total hardware cost target: less than $150

3. **Reliability**
   - Recover from occasional errors in GPS PPS signal
   - Handle quirks of specific hardware (e.g., Raspberry Pi CM4/CM5)
   - Monitor synchronization status and report to PTP grandmaster

4. **Flexibility**
   - Support both PTP and NTP protocols
   - Allow monitoring and configuration of GPS receiver over network
   - Support different GNSS systems (GPS, Galileo, BeiDou, GLONASS)

## Non-Goals

1. Not intended to replace LinuxPTP or chrony, but to complement them
2. Not designed to work with GPS PPS connected to GPIO or serial port
3. Not intended to support operating systems other than Linux
4. Not designed to work with all ethernet controllers, only those with PPS input pins

## Success Criteria

1. Achieve time synchronization precision in the low tens of nanoseconds
2. Successfully integrate with LinuxPTP (ptp4l) and chrony
3. Support both Intel i210 NICs and Raspberry Pi CM4/CM5 with IO board
4. Provide comprehensive documentation for setup and configuration
5. Enable real-world applications requiring precise time synchronization
