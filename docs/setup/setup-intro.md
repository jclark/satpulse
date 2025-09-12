---
title: Setup introduction
---

This section provides step-by-step guidance for installing and configuring SatPulse to create a precision time server.

## Stage 1: Install OS

**Goal:** Get to the point where OS boots and you can login with SSH

This stage is not described for Intel PCs (assumed already running Linux). For Raspberry Pi, choose one:

* [Raspberry Pi OS installation]({% link setup/rpi-os.md %})
* [Fedora on Raspberry Pi]({% link setup/fedora-rpi.md %})

Network setup (optional but recommended):

* [Network configuration]({% link setup/network.md %}) - Set up static IP address

## Stage 2: Install SatPulse

**Goal:** SatPulse installed; satpulsetool can run

* [Installing SatPulse]({% link setup/satpulse-install.md %})

## Stage 3: Identify and verify hardware

**Goal:** Identify hardware devices needed for configuration, verify basic functionality

Two substages (can be done in either order):

* [Serial connection setup]({% link setup/gps-serial.md %}) - Correct serial device and baud found; GNSS confirmed via satpulsetool gps
  * [Raspberry Pi serial specifics]({% link setup/rpi-serial.md %}) - Reference for Pi UART complexities
* [PTP hardware clock]({% link setup/phc.md %}) - PPS network interface and pin identified; pulses confirmed via satpulsetool sdp

## Stage 4: Configure SatPulse

**Goal:** Edit satpulse.toml correctly; service enabled and running

* [SatPulse configuration guide]({% link setup/satpulse-config.md %})

## Stage 5: Get system clock synced and NTP running

**Goal:** Chrony installed/configured; system clock synced with PHC; NTP client/server working

* [Chrony setup]({% link setup/chrony.md %})

## Stage 6: Get PTP running

**Goal:** PTP server installed/configured; time distributed on the network

* [PTP server setup]({% link setup/ptp4l.md %})