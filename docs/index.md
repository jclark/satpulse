---
layout: home
title: "SatPulse"
---
SatPulse is an open source program which makes it easy to run a time server for your local network that enables much more precise synchronization than is possible in a typical NTP-based setup.

A typical NTP stratum-1 server, running on, for example, a Raspberry Pi, connects the PPS (pulse-per-second) output of a GPS receiver to a GPIO or a serial port pin.
SatPulse is designed to work with PTP (Precision Time Protocol) and requires a different hardware setup, where the PPS output of the GPS is connected to a PPS input pin *on the ethernet controller*.
The precision comes from taking advantage of the hardware support for PTP in the ethernet controllers on both the server and the client.
For more details, please read the [Introduction]({% link intro.md %}).

There are only a few suitable ethernet controllers with a PPS input pin available at low cost. One notable example is the ethernet controller included in the Raspberry Pi Compute Module 4 and 5, when combined with a IO board that exposes the SYNC_OUT pin. Note that the Raspberry Pi 4 and 5 do not have this capability. For more on supported hardware, please see the [Hardware]({%link hardware/index.md%}) section. It is possible to buy all the necessary hardware for less than $150.

A time server using SatPulse runs on Linux and combines SatPulse with the PTP (Precision Time Protocol) server provided by the [LinuxPTP](https://linuxptp.nwtime.org/) project, which is called ptp4l. It can also work with the NTP server provided by the [chrony](https://chrony-project.org/) project. This enables a time server that supports both NTP and PTP.

Once you have the necessary hardware see the [Setup guide]({%link setup/index.md %}) to get started.


