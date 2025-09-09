---
title: Hardware for precision network timing
toc: false
---

The pages in this section aim to explain everything you need to choose, buy and assemble the hardware for a precision time server, which can run SatPulse. The choice of GNSS modules is somewhat specific to SatPulse, in that SatPulse will work better using a GNSS receiver that uses a protocol it supports, but apart from that, this information here is not SatPulse-specific.

This covers only fairly inexpensive hardware: nothing above $1000, and mostly much cheaper than that.

The key hardware requirement for a precision time server is an ethernet controller with a PPS input pin. At the time of writing, 2025Q3, there are very few such controllers available at low cost, and these can be divided into two categories. For each category, there is a separate page describing how to build a system.

- The [RPi CM4/CM5 build]({%link hardware/cm-build.md %}) page describes how to build a system using the ethernet controller in the Raspberry Pi (RPi) Compute Module 4 and 5 (CM4/CM5) (note that the ethernet controller in the RPi 5 and previous models does not have this capability); 
- The [Intel build]({%link hardware/intel-build.md %}) page describes how to build a system using Intel NICs, specifically the i210 and the i226 (which has replaced the i225). Typically these use a x86 PC, but there is also a hybrid option that uses a Intel NIC in a hat that attaches to a RPi 5 (not a CM5).

In both cases, you will need a GNSS board, receiver or disciplined oscillaor. I have divided this up into four pages:

- [GNSS modules]({%link hardware/gnss-modules.md %}) explains about the various kinds of GNSS module you can get; a module is the main component of a GNSS board or receiver, and determines most of its capabilities.
- [GNSS boards]({%link hardware/gnss-boards.md %}) explains the key requirements for a GNSS boards or cards that will live inside the computer case and lists some specific suitable products. Carrier boards for the CM4/CM5 provide pins that make it easy to connect boards. This does not work so well with an PC using an Intel NIC. However, there is an M.2 card that works inside a PC.
- [GNSS receivers]({%link hardware/gnss-receivers.md %}) explains the key requirements for GNSS receivers with their own enclosure and lists some specific suitable products, which use the modules described on the previous page. These work equally well with CM4/CM5 or Intel systems.
- [GNSSDOs]({%link hardware/gnssdos.md %}) covers GNSS disciplined oscillators. I have only found one suitable model, and it works equally well with CM4/CM5 or Intel systems.

Your GNSS receiver or board will need an antenna: the [Antennas]({%link hardware/antennas.md %}) page provides information about GNSS antennas.
It also deals with antenna splitters.

The [PTM]({%link hardware/ptm.md %}) page explains Precision Time Measurement, a PCIe feature that enables precise synchronization between a PHC and the system clock. This is important when a server is acting as a NTP server as well as PTP server, and for clients. SatPulse can take advantage of PTM.

The [Clients]({%link hardware/clients.md %}) pages provides information about hardware for PTP clients.

The [Switches]({%link hardware/switches.md %}) page provides information about PTP-aware switches, which hugely improve how accurately a client can synchronize: the improvements from a PTP-aware switch are much greater than those from a better GNSS receiver.

The [Vendors]({%link hardware/vendors.md %}) page lists vendors you can buy the above hardware from. This includes both smaller, specialist vendors
that make their own branded products, and larger distributors that just sell products made by others.
