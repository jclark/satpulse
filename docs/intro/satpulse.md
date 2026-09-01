---
title: SatPulse
---

The SatPulse software consists of three programs:

- [`satpulsed`]({%link man/satpulsed.8.md%}) - an integrated daemon, which connects to a GPS receiver over a serial port; the functions it performs are controlled by a configuration file in [TOML format]({%link man/satpulse.toml.5.md%})
- [`satpulsewb`]({%link man/satpulsewb.1.md%}) - a web-based GUI for GPS receiver configuration and monitoring, called SatPulse Workbench {% include new-in-03.html %}
- [`satpulsetool`]({%link man/satpulsetool.1.md%}) - a suite of command-line tools, usable with or without the daemon; there is a subcommand for each tool

All three programs are written in Go and use a common Go library.

## Platform support

All three programs run on Linux, macOS and Windows. {% include new-in-03.html %}
On Linux, SatPulse is packaged for deb-based and rpm-based distributions;
on macOS, it installs from the Homebrew tap;
on Windows, it is distributed as a zip file.
See [Installing SatPulse]({% link setup/satpulse-install.md %}).

Timing with a PHC is supported only on Linux.
Timing based on a serial PPS signal is supported on Linux and macOS.

## Timing

SatPulse can be used for timing both with and without a PHC.
See [Precision timing](timing.md) for the concepts behind these features.

When used without a PHC, SatPulse can provide timing information to an NTP daemon.
There are two different approaches depending on how the PPS signal is wired up:

- based on serial messages alone: the NTP daemon reads PPS timestamps itself, and uses the timing information from SatPulse to identify which second each pulse corresponds to
- based on serial PPS: when the PPS signal is wired to a modem control line of the serial port, satpulsed timestamps the pulses itself, and sends samples with sufficient information for the NTP daemon to synchronize the system clock without any additional input {% include new-in-03.html %}

SatPulse supports two protocols for communicating with an NTP daemon:
- the refclock SOCK protocol used by chrony, and now also supported by ntpd-rs
- the traditional shared memory protocol (driver type 28) used by the reference NTP implementation {% include new-in-03.html %}

`satpulsetool` provides the [`serial`]({%link man/satpulsetool-serial.1.md%}) tool for working with serial ports:
it can detect the speed of a connected GPS receiver, and can also detect PPS pulses on a modem control line. {% include new-in-03.html %}

Most of SatPulse's timing functionality is designed to support use of a PHC. `satpulsed`:

- has a robust, highly configurable subsystem for synchronizing a PHC with a GPS receiver
- can send PTP management protocol messages to the LinuxPTP `ptp4l` daemon providing metadata relating to clock quality and TAI-UTC offsets
- can provide timing information to NTP daemons based on the synchronized PHC
- can support cross-timestamping with PHCs that support it, such as the Intel i225/i226
- can apply sawtooth corrections provided by the GPS receiver
- is aware of the numerous quirks of the Raspberry Pi CM4/CM5 and has code to cleanly work around them
- can automatically handle PHCs that timestamp both edges of a pulse, such as the Intel i210 and i225/i226

`satpulsetool` provides two PHC-related tools:

- the [`sdp`]({%link man/satpulsetool-sdp.1.md%}) tool provides a convenient way for working with PHC SDPs
- the [`syncsim`]({%link man/satpulsetool-syncsim.1.md%}) tool simulates synchronization and can be used to tune configuration parameters

## Positioning

SatPulse is designed to support the use of hardware RTK. {% include new-in-03.html %}
`satpulsed` can

- act as an Ntrip caster, serving RTCM corrections from the GPS receiver to Ntrip clients
- act as an Ntrip server, pushing RTCM corrections from the GPS receiver to an Ntrip caster
- pull RTCM corrections from an Ntrip caster or a TCP server, feeding them to the GPS receiver
- convert RTCM MSM7 packets to MSM4 when acting as an Ntrip caster or server

`satpulsetool` provides the `ntrip` tool for fetching correction data from an Ntrip caster.

`satpulsetool` provides the [`convobs`]({%link man/satpulsetool-convobs.1.md%}) tool for converting raw observation data,
in either RTCM MSM7 or vendor-specific formats, into RINEX. {% include new-in-03.html %}
RINEX files can be sent to a post-processing service such as CSRS-PPP,
in order to get the most accurate possible position estimate.

SatPulse also provides access to position data; `satpulsed` can

- generate a track log, which can be converted to GPX
- provide an HTTP endpoint exposing the current position

## GPS receiver configuration

`satpulsetool` provides the [`gps`]({%link man/satpulsetool-gps.1.md%}) tool for GPS receiver configuration.
It supports two styles of configuration:
- high-level configuration is expressed in device-independent terms; it can be used without having any knowledge of vendor-specific protocols
- low-level configuration is based on message files, which contain named collections of messages

High-level configuration can be used to configure:
- PPS output
- antenna cable delay
- enabled GNSS signals, expressed in terms of constellations and bands
- time mode, including fixed position and survey-in
- output of NMEA messages
- output of RTCM messages
- output of vendor-specific messages, expressed in terms of the information they contain
- serial speed
- operations affecting non-volatile memory, such as saving, reloading and factory resetting

It can also be used to show information about the receiver's capabilities and current configuration.

Message files can be used both to support vendor protocols for which high-level configuration has not yet been implemented,
and to support device-specific functionality that high-level configuration does not support.

Message files can describe messages not just as raw bytes, but using protocol-specific message types.
This allows messages for binary protocols to be expressed in human-readable form, and allows
`satpulsetool gps` to correlate responses from the GPS receiver with sent messages,
so that the user can tell whether a message was accepted by the receiver.

`satpulsed` uses its configuration file to intelligently perform certain non-disruptive kinds of configuration.
For example, if `satpulsed` is configured to synchronize a PHC, it will ensure PPS output and the needed timing messages are enabled.
Configuration changes made by `satpulsed` are made only in RAM, and will be undone if the receiver is power cycled.

SatPulse Workbench is a third frontend to the same configuration model (see [satpulsewb(1)]({%link man/satpulsewb.1.md%})). {% include new-in-03.html %}
Its Configuration tab is a graphical view of high-level configuration, edited as a form.
Its Message file tab sends message files chosen from the installed library.

## Observability

SatPulse has a rich device-independent observability model, which includes

- timing: GNSS time and PHC synchronization state, accuracy
- positioning: position, velocity, accuracy and reports of corrections consumed
- satellites: satellite positions and signal strengths
- solution quality: solution type, types of correction used, DOP

`satpulsed` exposes this model through:

- a built-in Web GUI
- Prometheus metrics
- an event log in JSONL format

## Packet processing

SatPulse has a layered processing model.
The lowest-level layer divides a byte stream up into packets in one of the protocols that SatPulse supports.
A single byte stream can mix protocols.

`satpulsed` can act as a proxy relaying packets to and from the GPS receiver:
- the packets can be filtered by protocol
- the proxy can use either TCP or Unix domain sockets
- the proxy can be read-only or read-write
- the `gps` tool can use Unix domain sockets and a read-write proxy to enable configuration while `satpulsed` is running
- a read-write proxy using TCP can allow programs like `u-center` or Lady Heather to access the GPS receiver

This is similar to what the existing `ser2net` program does, but is protocol-aware.

SatPulse defines a JSONL format for capturing packets, which includes the content of the packet together with metadata.
Packet logs can be captured by `satpulsed` or by the `gps` tool.

`satpulsetool` includes several tools for working with packet byte streams and packet logs:

- [`scan`]({%link man/satpulsetool-scan.1.md%}) converts a packet byte stream into a JSONL packet log
- [`pack`]({%link man/satpulsetool-pack.1.md%}) converts a JSONL packet log back into a packet byte stream
- `decode` decodes an individual packet into a JSON object
- `annotate` adds decoded fields to a JSONL packet log
- `replay` converts a packet log into the same JSONL event log format used by `satpulsed`

## Receiver protocol support

All GPS receivers support the NMEA protocol, which is vendor-independent.
SatPulse can support a broad range of functionality using just NMEA.
SatPulse also supports RTCM, which is also vendor-independent.

SatPulse also supports vendor-defined protocols.
These protocols are used by receivers to
- provide periodic data about the ongoing operation of the receiver; these are conceptually similar to NMEA, but provide richer information
- allow configuration of the receiver; there are no vendor-independent protocols for this

See [GPS module support]({% link gps-module-support/index.md %}) for details of support for vendor-defined protocols.
