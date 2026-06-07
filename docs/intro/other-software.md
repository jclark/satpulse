---
title: Other related software
---

This page describes other software that SatPulse works closely with or addresses similar problems to what SatPulse addresses.

## Timing

### LinuxPTP

The [LinuxPTP](https://linuxptp.sourceforge.net/) project provides a number of programs. The main one is ptp4l, which implements the PTP protocol. SatPulse is intended to work in conjunction with ptp4l. LinuxPTP also includes the ts2phc program, which with the `-s nmea` options performs the same basic function as SatPulse. It also includes the phc2sys program, which, with the -E refclock_sock option, can generate samples for chrony.

 However, it does not have the extra features that SatPulse provides. In particular, it understands only the NMEA protocol; this is simple, and universally supported, but the only time-related information it provides is the current UTC time.

### statime

### chrony

### ntpd-rs

### NTP/NTPSec

## Positioning

### RTKLib

### BKG Ntrip Caster

### BNC

Ntrip client.

### SNIP

Ntrip caster.

## GNSS

### gpsd

The [gpsd](https://gpsd.gitlab.io/gpsd/) project provides a daemon that talks to GPS receivers and understands the UBX protocol, as well as several others, but it lacks PTP-specific functionality, and its protocol is not a good fit for the needs of PTP:

- it assumes the PPS signal connected to a serial or GPIO pin, using the kernel's PPS infrastructure, rather than to a NIC; the sawtooth correction information is linked to the PPS information, so isn't available when the PPS is connected to the NIC
- it provides time in UTC not atomic time, and doesn't provide leap second information

### PyGPSClient

pygnssutils etc

### Lady Heather

### Geoclue

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