---
title: Hardware
toc: false
---

The pages in this section provide information about hardware suitable for use with SatPulse.
This covers only fairly inexpensive hardware: nothing above $1000, and mostly much cheaper than that.

## General purpose hardware

You will need a GNSS receiver of some kind.
The capabilities of a GNSS receiver are mostly determined by the module it uses.
The [GNSS modules]({%link hardware/gnss-modules.md %}) page explains the various kinds of GNSS module you can get.

Your GNSS receiver will need an antenna: the [Antennas]({%link hardware/antennas.md %}) page provides information about GNSS antennas.
It also deals with antenna splitters.

## Hardware for timing applications

Timing applications have very specific hardware requirements.

The minimum requirement for the host computer is a way to connect the PPS output of the GNSS receiver.
The typical way to do this is to use a GPIO pin on an ARM-based SBC, such as Raspberry Pi.
An alternative is to use a pin on a serial port,
but this is a less good fit for modern hardware trends.

But the best possible timing precision requires an ethernet controller with a PPS input pin. At the time of writing, 2026Q2, there are very few such controllers available at low cost, and these can be divided into two categories. For each category, there is a separate page describing how to build a system.

- The [RPi CM4/CM5 build]({%link hardware/cm-build.md %}) page describes how to build a system using the ethernet controller in the Raspberry Pi (RPi) Compute Module 4 and 5 (CM4/CM5) (note that the ethernet controller in the RPi 5 and previous models does not have this capability); 
- The [Intel build]({%link hardware/intel-build.md %}) page describes how to build a system using Intel NICs, specifically the i210 and the i226 (which has replaced the i225). Typically these use a x86 PC, but there is also a hybrid option that uses a Intel NIC in a hat that attaches to a RPi 5 (not a CM5).

You will also need a suitable GNSS receiver. You can buy a receiver in three forms.

- a board is intended to go inside the host PC's case; the [GNSS boards]({%link hardware/gnss-boards.md %}) page explains the key requirements for boards and lists some suitable products. It emphasizes boards that work well with a Raspberry Pi CM4/CM5. Carrier boards for the CM4/CM5 provide pins that make it easy to connect boards. This does not work so well with an PC using an Intel NIC. However, there is an M.2 card that works inside a PC.

- an enclosed GNSS receiver has its own case; the [Enclosed GNSS receivers]({%link hardware/gnss-enclosed.md %}) page explains the key requirements for such receivers and lists some specific suitable products. These work equally well with CM4/CM5 or Intel systems.

- a GNSS disciplined oscillator combines a GNSS receiver and an oscillator; the [GNSSDOs]({%link hardware/gnssdos.md %}) page explains the key requirements for a GNSSDO to be suitable for computer timing applications and describes the only suitable model I have found. It works equally well with CM4/CM5 or Intel systems.

The [PTM]({%link hardware/ptm.md %}) page describes hardware that supports Precision Time Measurement.
See [Synchronizing the system clock]({% link intro/timing.md %}#synchronizing-the-system-clock) for what this is.
This is important when a server is acting as a NTP server as well as PTP server, and for clients.

The [Clients]({%link hardware/clients.md %}) page provides information about hardware for PTP clients.

The [Switches]({%link hardware/switches.md %}) page provides information about PTP-aware switches, which hugely improve how accurately a client can synchronize: the improvements from a PTP-aware switch are much greater than those from a better GNSS receiver.

## Vendors

The [Vendors]({%link hardware/vendors.md %}) page lists vendors you can buy the above hardware from. This includes both smaller, specialist vendors
that make their own branded products, and larger distributors that just sell products made by others.
