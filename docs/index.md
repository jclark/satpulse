---
layout: home
title: "SatPulse"
---
The goal of the SatPulse project is to provide a suite of open-source software that enables precision timing and positioning using modern GPS receivers on Linux and other general-purpose operating systems.
SatPulse has especially deep support for the Raspberry Pi, from the Pi Zero to the Pi 5.

The initial focus of the project was on precision timing, specifically making it easy to run a time server for your local network that enables much more precise synchronization than is possible in a typical NTP-based setup.

But since the initial 0.1 release, SatPulse has been developing rapidly.
With the 0.2 release and upcoming 0.3 release, SatPulse provides a broad range of capabilities related to precision timing and positioning.
Unfortunately, the tutorial documentation linked to from the navigation bar on the left does not yet cover everything that has been added since the 0.1 release.
However, the man pages are fully up-to-date with the current capabilities of the software.
A [0.3 prerelease](https://github.com/jclark/satpulse/releases/tag/v0.3-pre-20260824) is available.
The changes since 0.1 are described in detail in [recent changes]({% link recent-changes.md %}).
The [blog]({% link blog.md %}) also has many posts about how SatPulse has evolved since 0.1.

Precision timing remains the most mature part of SatPulse.
A typical NTP stratum-1 server, running on, for example, a Raspberry Pi, connects the PPS (pulse-per-second) output of a GPS receiver to a GPIO or a serial port pin.
SatPulse can take advantage of hardware designed for PTP (Precision Time Protocol).
The key difference is that the PPS output of the GPS is connected to a PPS input pin *on the ethernet controller*.
The Raspberry Pi CM4 and CM5 have this capability when used with a suitable IO board.
For more details, please read the [Introduction]({% link intro/index.md %}).

In release 0.1, SatPulse required this special kind of ethernet controller.
But since 0.2, this is no longer the case. In particular, since 0.2 SatPulse can supply timing information to an NTP server without any special hardware.
It can also be used as an RTK base station and it provides extensive capabilities for GPS receiver configuration.

SatPulse does not itself provide an NTP or PTP implementation.
Its job is to act as a source of time for NTP or PTP.
It is intended to work with the PTP server provided by the [LinuxPTP](https://linuxptp.nwtime.org/) project, which is called ptp4l,
and with the NTP server provided by the [chrony](https://chrony-project.org/) project.

The [Setup guide]({% link setup/index.md %}) describes how to get started with SatPulse.


