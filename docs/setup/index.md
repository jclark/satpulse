---
title: Setup guide
toc: false
---

This section describes how to set up SatPulse and the software that works with it.

Before installing SatPulse, you obviously need a working OS.
SatPulse runs on Unix-like systems, including Linux and macOS.
When running headless, you should get to the point where you can log in with SSH.

On a Raspberry Pi,
I recommend [installing Raspberry Pi OS]({% link setup/rpi-os.md %})
(note that kernel version 6.12.34 does not work for SatPulse or PTP).
It is also possible to [install Fedora]({% link setup/fedora-rpi.md %}).
On a CM4 or CM5, if you are connecting the TX/RX pins on the GPS board to the 40-pin HAT connector on the carrier board,
then you also need to [configure the UARTs]({% link setup/rpi-uart.md %}).

If the machine will act as a server, you will probably want a static IP address;
the [network configuration]({% link setup/network.md %}) page describes how to do this on Linux.

The first stage is the baseline setup,
which gets you to the point where the GPS receiver is connected, satpulsed is running, and monitoring is working:

1. [Install SatPulse]({% link setup/satpulse-install.md %}).
   After this, you can use [satpulsetool]({%link man/satpulsetool.1.md%}) without any additional configuration.
2. Identify the [serial]({% link setup/gps-serial.md %}) device connected to the GPS and verify that data is being received.
   This can most easily be done with satpulsetool.
   Configuration also needs to know the baud rate.
3. [Configure and run satpulsed]({% link setup/satpulsed.md %}).
4. [Monitor]({% link setup/monitor.md %}) satpulsed in a variety of ways.

On top of the baseline, you can add support for positioning with RTK and/or timing.

For positioning, [RTK setup]({% link setup/rtk.md %}) shows how to configure SatPulse
for the base side and the rover side of an [RTK]({% link intro/positioning.md %}#real-time-kinematic) setup.

For timing, there are two levels, depending on your hardware:

* [Basic use with NTP]({% link setup/ntp.md %}) builds an NTP server on general-purpose, widely available hardware,
  such as a Raspberry Pi, with accuracy in the microsecond range.
* [Precision timing with a PHC]({% link setup/phc.md %}) uses a network interface
  with a [PTP Hardware Clock]({% link intro/timing.md %}) that timestamps the GPS PPS signal in hardware.
  This supports a PTP server as well as NTP.
  It can achieve accuracy in the tens of nanoseconds, but needs very specific hardware.

The timing pages apply to Linux only: PHC support is Linux-specific,
and macOS does not yet have a good way to read a PPS signal.
