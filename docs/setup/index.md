---
title: Setup guide
---

This section describes how to set up all the software needed for a precision time server using SatPulse.

For a complete PTP/NTP time server on Linux, follow these steps.

1. Install Linux. The goal of this stage is to get Linux installed and set up to the point where you can login with SSH.
   For Raspberry Pi, I recommend [installing Raspberry Pi OS]({% link setup/rpi-os.md %})
   (note that kernel version 6.12.34 does not work for SatPulse or PTP).
   It is also possible to use Fedora. I wrote a guide for [installing Fedora 41]({% link setup/fedora-rpi.md %}).
   In either case you probably want to [set a static IP address]({% link setup/network.md %}).
2. [Install SatPulse]({% link setup/satpulse-install.md %}).
   After this, you can use [satpulsetool]({%link man/satpulsetool.1.md%}) without any additional configuration.
3. Identify the devices connected to the GPS and verify that data is being received. This can most easily be done with satpulsetool.
   There are two devices involved:
   * a [serial]({% link setup/gps-serial.md %}) device; configuration also needs to know the baud-rate;
   * a [network interface with PTP hardware clock]({% link setup/phc.md %}); configuration also needs to know what pin on the ethernet card is connected to the GPS PPS output.
4. [Configure SatPulse]({% link setup/satpulse-config.md %}. This will install a service that runs the satpulsed daemon, which will synchronize the PHC with the GPS.
5. [Setup chrony]({% link setup/chrony.md %}). This:
   * synchronizes the system clock to the PHC;
   * runs an NTP client; this provides an important check that the PHC time is correct; and
   * runs an NTP server, if desired.
6. [Setup a PTP server]({% link setup/ptp4l.md %}). This uses the ptp4l daemon, which is part of LinuxPTP.
