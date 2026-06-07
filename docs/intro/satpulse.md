---
title: SatPulse
---

The SatPulse software consists of two programs:

- `satpulsed` - an integrated daemon, which connects to a GPS receiver over a serial port; the functions it performs are controlled by a configuration file in TOML format
- `satpulsetool` - a suite of command-line tools, usable with or without the daemon; there is a subcommand for each tool

Both programs are written in Go and use a common Go library.

## Platform support

TODO

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
Configuration changes made by `satpulsed` are made only in RAM, and will be undone if the receiver is power cycled.

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

- `scan` converts a packet byte stream into a JSONL packet log
- `pack` converts a JSONL packet log back into a packet byte stream
- `decode` decodes an individual packet into a JSON object
- `annotate` adds decoded fields to a JSONL packet log
- `replay` converts a packet log into the same JSONL event log format used by `satpulsed`

## Receiver protocol support

NMEA and RTCM are vendor-independent protocols.
SatPulse can support a broad range of functionality using just these protocols.

SatPulse also supports vendor-defined protocols.
These protocols are used by receivers to
- provide periodic data about the ongoing operation of the receiver; these are conceptually similar to NMEA, but provide richer information
- allow configuration of the receiver; there are no vendor-independent protocols for this

Supporting a protocol involves:
- recognizing the protocol's packet formats; protocols differ in whether they use a single packet format for both periodic data and configuration
- decoding the packet wire formats
- for periodic data, mapping the decoded packets into the device-independent model that drives observability and timing
- providing protocol-specific message types for conveniently describing protocol messages in message files
- handling request/response correlation when sending messages defined in message files
- providing message files to perform configuration
- supporting high-level configuration
- supporting conversion of raw observation messages into RINEX
- validating the implementation with real hardware

SatPulse defines two tiers of protocol support. These differ mainly as regards configuration:

- for tier 1, the primary configuration mechanism is high-level configuration; the message files provide support for configuration features that are not covered by the device-independent configuration model
- for tier 2, all configuration is handled by message files; high-level configuration is not supported

Message files can provide full access to a receiver's functionality, but high-level configuration is more convenient and requires no device-specific knowledge from the user.
The message files provided for tier 2 receivers follow device-independent tag naming conventions to reduce
the device-specific knowledge needed to use them.

In addition:

- conversion of raw observation messages has currently only been implemented for tier 1 protocols
- more extensive hardware validation has been performed for tier 1 protocols than for tier 2

There is tier 1 support for the following protocols:

- u-blox UBX protocol; support covers a wide range of receivers from LEA-6T through to ZED-X20P,
  including standard precision, high precision and timing receivers
- Unicore protocol used by the NebulasIV family of high precision boards and receivers,
  i.e. the UM980 series (UM980, UM981, UM982 and UM960)

The protocols with tier 2 support can be grouped as follows.
There are binary, UBX-like protocols:

- Allystar; support has been validated with the TAU1201 using the Cynosure III chip,
  and the more recent TAU951M-P200 using the Cynosure IV chip
- CASIC binary protocol used by Zhongke Microelectronics; there are two generations of this protocol, the first used by ATGM332D-5N and ATGM336H-5N series modules using the AT6558, and the second used by subsequent modules
- SDBP protocol used by Techtotop/Taidou; support has been validated with the T303-5D, which is an L1/L5 timing module

There are vendors using the NovAtel OEM6/OEM7 protocol. These protocols treat periodic data, which they call *logs*, differently from configuration. The logs have a dual ASCII/binary syntax and are very similar between vendors: in particular, the packet formats are indistinguishable. Configuration follows a similar style of line-oriented ASCII commands, but is not interoperable between vendors. Unicore UM980 protocol is similar to this, but the packet format is slightly different. There is tier 2 support for:

- ByNav, validated on the M10 and M20
- SinoGNSS, validated on the K901

The Unicore UM980 series also has undocumented support for emitting OEM6/OEM7 compatible logs, and SatPulse supports this as well.

Finally, there are vendors using protocols that use NMEA proprietary sentences starting with `P`.

- PQTM protocol defined by Quectel, which has been validated with the LG290P and LC29H
- PAIR protocol defined by Airoha, which has been validated on the Quectel LC29H (which uses the Airoha AG3335 chipset)

