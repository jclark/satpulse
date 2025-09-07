---
title: Hardware for precision network timing
toc: false
---

The pages in this section aim to explain everything you need to choose, buy and assemble a hardware system that can run SatPulse.

This covers only fairly inexpensive hardware: nothing above $1000, and mostly much cheaper than that.

The hardware systems on which SatPulse runs can be divided into two based on the ethernet controller into which the PPS signal is being input

- [CM4/CM5 build]({%link cm-build.md %}) describes how to build a system that uses the Raspberry Pi Compute Module 4 or 5 (CM4/CM5) and its ethernet 
- [Intel build]({%link intel-build.md %}) describes how to build a system that uses an Intel NIC. Most of these use a PC, but there is also a hybrid option that uses a Intel NIC in a hat that attaches to a Raspberry Pi 5 (not a CM5).

In both cases, you will need a GNSS board or receiver. I have divided this up into three pages:

- [GNSS modules]({%link gnss-modules.md %}) explains about the various kinds of GNSS module you can get; a module is the main component of a GNSS receiver, and determines most of its capabilities.
- [GNSS boards]({%link external-gnss.md %}) explains the key requirements for a GNSS boards or cards that will live inside the computer case and lists some specific suitable products. Carrier boards for the CM4/CM5 provide pins that make it easy to connect boards. This does not work so well with an PC using an Intel NIC. However, there is an M.2 card that works inside a PC.
- [GNSS receivers]({%link external-gnss.md %}) explains the key requirements for external GNSS receivers with their own enclosure and lists some specific suitable products, which use the modules described on the previous page. These work equally well with CM4/CM5 or Intel systems.
This also covers GNSS disciplined oscillators.

Your GNSS receiver or board will need an antenna: the [Antennas]({%link antennas.md}) page provides information about GNSS antennas.
It also deals with antenna splitters.

The [Switches]({%link switches.md}) page provides information about PTP-aware switches, which hugely improve how accurately a client can synchronize: the improvements from a PTP-aware switch are much greater than those from a better GNSS receiver.

The [Vendors]({%link vendors.md}) page lists vendors you can buy the above hardware from. This includes both smaller, specialist vendors
that make their own branded products, and larger distributors that just sell products made by others.
