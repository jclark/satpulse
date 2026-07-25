---
title: Setup guide
toc: false
---

This section describes how to set up all the software needed for a precision time server using SatPulse.

For a complete PTP/NTP time server on Linux, follow these steps.

1. Install Linux. The goal of this stage is to get Linux installed and set up to the point where you can login with SSH.
   For Raspberry Pi, I recommend [installing Raspberry Pi OS]({% link setup/rpi-os.md %})
   (note that kernel version 6.12.34 does not work for SatPulse or PTP).
   It is also possible to [install Fedora]({% link setup/fedora-rpi.md %}).
   In either case you probably want to [set a static IP address]({% link setup/network.md %}).
2. On a Raspberry Pi, if you are connecting the TX/RX pins on the GPS board to the 40-pin HAT connector on the carrier board,
   then you need to [configure the UARTs]({%link setup/rpi-uart.md %}).
3. [Install SatPulse]({% link setup/satpulse-install.md %}).
   After this, you can use [satpulsetool]({%link man/satpulsetool.1.md%}) without any additional configuration.
4. Identify the devices connected to the GPS and verify that data is being received. This can most easily be done with satpulsetool.
   There are two devices involved:
   * a [serial]({% link setup/gps-serial.md %}) device; configuration also needs to know the baud-rate;
   * a [network interface with PTP hardware clock]({% link setup/phc.md %}); configuration also needs to know what pin on the ethernet card is connected to the GPS PPS output.
5. [Configure and run satpulsed]({% link setup/satpulsed.md %}). This will install a service that runs the satpulsed daemon, which will synchronize the PHC with the GPS.
6. [Setup chrony]({% link setup/chrony.md %}). This:
   * synchronizes the system clock to the PHC;
   * runs an NTP client; this provides an important check that the PHC time is correct; and
   * runs an NTP server, if desired.
7. [Setup a PTP server]({% link setup/ptp4l.md %}). This uses the ptp4l daemon, which is part of LinuxPTP.
8. Once you have the time server running, you can [monitor]({% link setup/monitor.md %}) it in a variety of ways. 

Although use of SatPulse for a PTP time server requires a PHC,
SatPulse can feed an NTP server and has GPS-related functionality that can be used [without a PHC]({%link setup/without-phc.md %}).