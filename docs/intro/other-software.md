---
title: Other related software
---

This page describes other software that SatPulse works closely with or addresses similar problems to what SatPulse addresses.

These are all open source, unless otherwise stated.

## Timing

### LinuxPTP

The [LinuxPTP](https://linuxptp.nwtime.org/) is the leading PTP implementation for Linux.
It is written in C.
It provides a number of programs. The main one is ptp4l, which implements the PTP protocol.
SatPulse is intended to work in conjunction with ptp4l. It also includes the ts2phc program, which, with the `-s nmea` option, can transfer time from a GNSS receiver to a PHC
using NMEA messages and PPS timestamps on an SDP,
and the phc2sys program, which, with the -E refclock_sock option, can generate samples for chrony.

It is included in Debian and Fedora.

### statime

[statime](https://github.com/pendulum-project/statime) provides a PTP daemon for Linux written in Rust.
It can also be used as a library to implement PTP on other platforms, including embedded platforms.

### NTP/NTPsec

[NTP](https://www.ntp.org/) is the original implementation of NTP,
which has now mostly been supplanted by the [NTPsec](https://www.ntpsec.org/) fork.

SatPulse can provide timing information to NTPsec using the SHM reference clock protocol.

It is included in Debian and Fedora.

### chrony

[chrony](https://chrony-project.org/) is an implementation of NTP with advanced features lacking in NTP and NTPsec,
notably support for using a PHC as a reference clock.

SatPulse can provide timing information to chrony using the SOCK reference clock protocol.
SatPulse's NTP support is mostly tested with chrony.

It is the default NTP implementation for Fedora.

### ntpd-rs

[ntpd-rs](https://github.com/pendulum-project/ntpd-rs) is a relatively new implementation of NTP
in Rust.

ntpd-rs supports the SOCK reference clock protocol defined by chrony, and
SatPulse also works with it.

It has been adopted by Ubuntu.

## Positioning

### RTKLIB

[RTKLIB](https://www.rtklib.com/) is the pioneering implementation of software RTK.
It includes a number of versatile command-line utilities that are very useful when working with RTK,
notably str2str and convbin.
The original author of RTKLIB no longer maintains it.
[RTKLIB Explorer](https://github.com/rtklibexplorer/RTKLIB) is an actively maintained fork.

It is included in Debian.

### BKG Ntrip Caster

[BKG Professional NtripCaster](https://igs.bkg.bund.de/ntrip/bkgcaster) is the leading open source Ntrip caster.
BKG is the German Federal Agency for Cartography and Geodesy,
which was one of the main developers of the Ntrip protocol.
It works on Linux.

SatPulse has been tested with it.

### BNC

[BNC](https://igs.bkg.bund.de/ntrip/bnc), the BKG Ntrip Client, is a cross-platform graphical Ntrip client.
It can perform a variety of realtime GNSS processing.
Notably, it includes a realtime implementation of PPP,
which combines raw observations from a GNSS receiver with RTCM-SSR corrections fetched over Ntrip.
It can also work in batch mode.

### SNIP

[SNIP](https://www.use-snip.com/) is a proprietary Ntrip caster,
used to operate the [RTK2go](https://rtk2go.com) free public Ntrip service.

## GNSS

### gpsd

[gpsd](https://gpsd.gitlab.io/gpsd/) comprises a daemon and associated clients and tools.
The daemon provides a socket interface through which clients can discover and receive information from
GPS receivers using a device-independent JSON-based protocol.
It is designed to work without a configuration file.

It is very widely used and included in Debian and Fedora.

This [blog post]({% link _posts/2026-04-11-design-of-satpulse-compared-with-gpsd.md %}) compares the design of SatPulse and gpsd.

### PyGPSClient and pygnssutils

[PyGPSClient](https://github.com/semuconsulting/PyGPSClient) is a cross-platform GNSS monitoring and configuration GUI.
[pygnssutils](https://github.com/semuconsulting/pygnssutils) is a collection of command-line GNSS tools
(gnssreader, gnssstreamer, gnssserver, gnssntripclient, gnssmqttclient, pyrinexconv).
They are both written in Python and use a common set of Python libraries for parsing various GNSS protocol formats.

There is a lot of overlap of functionality with SatPulse.

### Lady Heather

[Lady Heather](https://www.ke5fx.com/heather/readme.htm) is a timing-oriented monitoring and control program
for GNSS disciplined oscillators and receivers.
There is also a useful GitHub [repo](https://github.com/stargo/LadyHeatherGPS),
whose status is not completely clear.

It was originally written for Windows, but has been ported to X11 on macOS and Linux.

### Geoclue

[GeoClue](https://gitlab.freedesktop.org/geoclue/geoclue) is the standard geolocation service for the Linux desktop.
It distributes geolocation information over D-Bus.
It can get geolocation information in NMEA format from a TCP port.

### Vendor tools

Vendors all have their own proprietary programs for working with their receivers. These are all Windows-only.

- u-blox: u-center/u-center2
- Unicore: UPrecise
- Quectel: QGNSS
- Zhongke: GnssToolkit3
- Allystar: Satrack
- Techtotop/Taidou: TDMonitor
- ByNav: BY_Connect
- ComNav/SinoGNSS: CRU