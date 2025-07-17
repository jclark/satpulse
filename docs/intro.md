---
title: Introduction to SatPulse
---
The purpose of SatPulse is to make it easy to run a time server for your local network that enables much more
precise synchronization than is possible in a typical NTP-based setup, without having to spend a lot of money.
A precision in the low tens of nanoseconds is achievable with hardware with a total cost in the low hundreds of dollars.
SatPulse does not by itself provide a complete time server: rather it is designed to combine with the
PTP (Precision Time Protocol) server provided by the [LinuxPTP](https://linuxptp.nwtime.org/) project, which is called ptp4l,
and the NTP server provided by the [chrony](https://chrony-project.org/) project.
The role of SatPulse is to use the GPS receiver as a source of time for ptp4l and chrony.

A time server based on SatPulse achieves its high precision by taking advantage of hardware support for PTP.
The central piece of hardware required by SatPulse is an ethernet controller with a PPS (pulse-per-second)
input pin, sometimes called an SDP (software-defined pin). Although many ethernet controllers have PTP support,
few of those provide a suitable input pin, and even fewer of those are inexpensive. SatPulse runs only on Linux,
since only Linux has [APIs](https://docs.kernel.org/driver-api/ptp.html) that provide the necessary access
to the ethernet controller's PTP support. The ethernet controller must also have Linux drivers that support these APIs. 
The main requirement for the GPS receiver is that it provide a PPS output signal that is electrically compatible with the PPS
input pin on the ethernet controller. See [What hardware to get]({%link hardware.md %}) for more details.
In a typical NTP setup, the PPS output of the GPS receiver is connected to either a serial port or a GPIO pin; SatPulse does
not support this, since it cannot achieve the precision that SatPulse is intended to enable.

GPS (Global Positioning System) technically refers to the satellite constellation operated by the USA. There
are three other similar constellations with global coverage: Galileo, BeiDou and GLONASS operated by the
European Union, China and Russia respectively. The technically correct term for such a constellation is
GNSS (Global Navigation Satellite System). SatPulse is designed to work any GNSS.
The term GPS is used informally to refer to any GNSS system, and we will use it in that sense.

## Basics of how it works

An ethernet controller that supports PTP has its own clock, called the PTP hardware clock (PHC), which is completely independent
of the computer's system clock. The principal function of the PHC is to timestamp incoming and outgoing ethernet packets.
Each packet is timestamped by the controller hardware, without going through the operating system; this allows the timestamp to be
accurate to within a few nanoseconds. When the controller has a PPS input pin, the controller can also timestamp the pulses
detected by the pin. As with the packets, the timestamp of a pulse is with respect to the PHC.

SatPulse provides two programs, written in the [Go](https://go.dev/) programming language: `satpulsed` and `satpulsetool`.
The main one is `satpulsed`; it is a daemon, which

- talks to a GPS receiver over a serial port
- reads the timestamps of pulses from the GPS receiver
- adjusts the time of the PHC to match the GPS: the pulse from the GPS receiver is precisely aligned with the start of a second;
  the messages received over the serial port are used to determine which second it is

PTP does not concern itself with the system clock at all. The PTP server and client work together to transfer the
time from the server's PHC to the client's PHC. A PTP server also provides time-related metadata to its clients:

- a key idea in PTP is that there can be multiple servers and the client chooses the best; the metadata provides information about the clock
  accuracy, which the client uses in making its choice
- a PTP server typically expects the PHC to use the PTP timescale, which is based on atomic time (TAI); the metadata provides information about leap seconds to
  allow the client to derive UTC time from TAI

SatPulse communicates with ptp4l using the PTP management protocol and provides it with this metadata.

NTP works a bit differently: the ultimate purpose of NTP is to keep the system clock of NTP clients in sync with UTC.
SatPulse takes simultaneous samples of the system clock and the PHC. When the PHC is in sync with the GPS,
SatPulse derives samples giving the system clock time and corresponding true UTC time, and sends them to chrony using
chrony's refclock SOCK interface. Chrony uses these samples to adjust the system clock,
thus keeping the system clock in sync with UTC. Chrony can also act as an NTP server,
allowing a NTP client to sync its system clock to UTC. The chrony interface thus enables the time from
the GPS receiver to be used in two distinct ways: to synchronize the system clock and to provide an NTP service.

## Features

SatPulse provides the following features:

- It can talk to U-blox GPS receivers (or clones) using the UBX protocol, which is the native binary protocol of U-blox receivers. This gives SatPulse access to the full capabilities of U-blox receivers. (It can also use the ASCII NMEA protocol, which is universally supported, but provides limited capabilities.)

    - It can determine the receiver's configuration, and automatically make changes so that it works optimally for timing purposes.
      (Changes are made only to the receiver's RAM configuration, so will be undone if the receiver is power cycled.)

    - It can take advantage of the sawtooth correction (sometimes called quantization error) information that is available from some U-blox receivers. This enables more precise synchronization of the PTP hardware clock.

    - It can get the current time from the GPS receiver as atomic time, and use that directly to set the PTP hardware clock.
      Both GPS and PTP work natively using atomic time (without leap seconds) rather than UTC. With the NMEA protocol, time has to be converted to UTC and then back to atomic time.

    - It can get information about upcoming leap seconds from GPS and provide that information to the PTP grandmaster.

- It automatically handles NICs that generate timestamps for both edges of a pulse (Intel NICs, including the i210, do this); this is enabled partly by using the UBX protocol, which allows us to get the pulse width.

- It is aware of the quirks of the Raspberry Pi CM4/CM5 and has code to cleanly work around them.

- It can recover from occasional errors in the PPS signal from the GPS receiver.

- It allows TCP connections with the GPS receiver attached to the serial port (similar to ser2net).
This means you can run SatPulse on a Linux box (such as a Raspberry Pi) and then monitor or configure the GPS receiver over the network from a Windows PC,
using a program like [u-center](https://www.u-blox.com/en/product/u-center) or [Lady Heather](http://www.ke5fx.com/heather/readme.htm),
*at the same time as* it is being used for PTP. (This provides plenty of opportunity to break things, but is quite handy.)

- It provides an HTTP interface for monitoring, including a graphical web interface and a Prometheus `/metrics` endpoint.


## Relationship to other software

There are some existing, well-established open source projects that provide similar functionality, but they don't quite do what SatPulse does.

It would probably be possible to evolve these, but they are both written in C, and I personally want to work in more modern, safer languages.

### LinuxPTP

The [LinuxPTP](https://linuxptp.sourceforge.net/) project provides a number of programs. The main one is ptp4l, which implements the PTP protocol. SatPulse is intended to work in conjunction with ptp4l. LinuxPTP also includes the ts2phc program, which with the `-s nmea` options performs the same basic function as SatPulse. It also includes the phc2sys program, which, with the -E refclock_sock option, can generate samples for chrony.

 However, it does not have the extra features that SatPulse provides. In particular, it understands only the NMEA protocol; this is simple, and universally supported, but the only time-related information it provides is the current UTC time.

### gpsd

The [gpsd](https://gpsd.gitlab.io/gpsd/) project provides a daemon that talks to GPS receivers and understands the UBX protocol, as well as several others, but it lacks PTP-specific functionality, and its protocol is not a good fit for the needs of PTP:

- it assumes the PPS signal connected to a serial or GPIO pin, using the kernel's PPS infrastructure, rather than to a NIC; the sawtooth correction information is linked to the PPS information, so isn't available when the PPS is connected to the NIC
- it provides time in UTC not atomic time, and doesn't provide leap second information
