---
title: SatPulse
---

The SatPulse software consists of two programs:

- `satpulsed` - an integrated daemon, which connects to a GPS receiver over a serial port; the functions it performs are controlled by a configuration file in TOML format
- `satpulsetool` - a suite of command-line tools, usable with or without the daemon; there is a subcommand for each tool

Both programs are written in Go and use a common Go library.

## Timing

SatPulse can be used for timing both with and without a PHC.
See [Precision timing](timing.md) for the concepts behind these features.

When used without a PHC, SatPulse can provide timing information to an NTP daemon based on serial messages alone.
The NTP daemon will typically read PPS timestamps itself, and use the timing information from SatPulse to identify which second each pulse corresponds to.
SatPulse supports two protocols for communicating with an NTP daemon:
- the refclock SOCK protocol used by chrony, and now also supported by ntpd-rs
- the traditional shared memory protocol (driver type 28) used by the reference NTP implementation

Most of SatPulse's timing functionality is designed to support use of a PHC. `satpulsed`:

- has a robust, highly configurable subsystem for synchronizing a PHC with a GPS receiver
- can send PTP management protocol messages to the LinuxPTP `ptp4l` daemon providing metadata relating to clock quality and TAI-UTC offsets
- can provide timing information to NTP daemons based on the synchronized PHC
- can support cross-timestamping with PHCs that support it, such as the Intel i225/i226
- can apply sawtooth corrections provided by the GPS receiver
- is aware of the numerous quirks of the Raspberry Pi CM4/CM5 and has code to cleanly work around them
- can automatically handle PHCs that timestamp both edges of a pulse, such as the Intel i210 and i225/i226

`satpulsetool` provides two PHC-related tools:

- the `sdp` tool provides a convenient way for working with PHC SDPs
- the `syncsim` tool simulates synchronization and can be used to tune configuration parameters

## Positioning

SatPulse is designed to support the use of hardware RTK. These features are new in 0.3.
`satpulsed` can

- act as a Ntrip caster, serving RTCM corrections from the GPS receiver to Ntrip clients
- act as an Ntrip server, pushing RTCM corrections from the GPS receiver to an Ntrip caster
- pull RTCM corrections from an Ntrip caster or a TCP server, feeding them to the GPS receiver
- convert RTCM MSM7 packets to MSM4 when acting as a Ntrip caster or server

`satpulsetool` provides the `ntrip` tool for fetching correction data from an Ntrip caster.

Also new in 0.3, `satpulsetool` provides the `convobs` tool for converting raw observation data,
in either RTCM MSM7 or vendor-specific formats, into RINEX.
RINEX files can be sent to a post-processing service such as CSRS-PPP,
in order to get the most accurate possible position estimate.

SatPulse also provides access to position data; `satpulsed` can

- generate a track log, which can be converted to GPX
- provide an HTTP endpoint exposing the current position

## GPS receiver configuration

`satpulsetool` provides the `gps` tool for GPS receiver configuration.
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
- a read-write proxy using TCP can allow programs like `u-center` to access the GPS receiver

SatPulse defines a JSONL format for capturing packets, which includes the content of the packet together with metadata.
Packet logs can be captured by `satpulsed` or by the `gps` tool.

`satpulsetool` includes several tools for working with packet byte streams and packet logs:

- `scan` converts a packet byte stream into a JSONL packet log
- `pack` converts a JSONL packet log back into a packet byte stream
- `decode` decodes an individual packet into a JSON object
- `annotate` adds decoded fields to a JSONL packet log
- `replay` converts a packet log into the same JSONL event log format used by `satpulsed`

## Receiver protocol support

- SatPulse reads native receiver packet streams, rather than requiring every receiver to be reduced to basic NMEA output.
- It understands standard NMEA and RTCM, plus vendor protocols including u-blox UBX, Unicore, Quectel, CASIC/Zhongke, Allystar, Bynav, SinoGNSS/ComNav and Techtotop/Taidou.
- Vendor protocols expose information that basic NMEA does not, including timing quality, survey state, solution quality, correction use, raw receiver status and receiver-specific configuration responses.
- SatPulse converts supported protocol messages into common events for time, position, velocity, solution quality, satellite and signal status, survey state, leap seconds and correction status.
- Those common events are what make the same timing, positioning, monitoring, logs and metrics work across different receivers.

## Where SatPulse fits

- SatPulse is strongest when a GNSS receiver is part of a computer system that needs receiver configuration, timing, correction-stream routing, packet access and observability.
- The related software page should explain the boundaries between SatPulse, PTP/NTP daemons, GPSd, RTK tools, Ntrip tools and vendor tools.
