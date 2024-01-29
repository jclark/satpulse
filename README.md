SatPulse allows the time pulse from a Global Navigation Satellite System (GNSS) receiver to be used as a source of precision time.
The initial focus is on supporting the Precision Time Protocol (PTP): the goal is to make it easy and inexpensive to run a PTP grandmaster, which can provide a high-precision source of time for a network. This project does not provide an implementation of PTP. Instead it is designed to work with the ptp4l program from the LinuxPTP project.

The first and most well-known GNSS constellation is the Global Positioning System (GPS), operated by the USA.
But there are other GNSS constellations with global coverage: Galileo, GLONASS and BeiDou operated by the European Union, Russia and China respectively.
SatPulse can work with whichever constellations are supported by the GNSS receiver.

SatPulse is written in the Go programming language. It works only on Linux, since only Linux provides the necessary APIs.

The project is currently at a pre-alpha stage of development. It is not yet ready for use.

## Basics of how it works

SatPulse is designed to make two pieces of hardware work together:

- a GNSS receiver (or GNSS disciplined oscillator) that has a PPS (pulse per second) output pin
- a NIC (network interface card) that includes a PTP hardware clock and both
   - supports PTP hardware timestamping, and
   - has a PPS input pin (more precisely, supports using the PTP hardware clock to timestamp pulses received on the pin)

The PPS output from the GNSS receiver needs to be connected to the PPS input on the NIC.

SatPulse is a daemon, written in the Go language, which:

- talks to a GNSS receiver over a serial port
- uses the Linux kernel's PTP hardware clock infrastructure to
   - read the timestamps of pulses from the GNSS receiver
   - adjust the time of the PTP hardware clock

The pulse from the GNSS receiver is precisely aligned with the start of a second; the information over the serial port is used to determine which second it is.

The ptp4l daemon (part of the LinuxPTP project) then uses PTP to distribute the time of the PTP hardware clock to other PTP nodes.

## Quick start

Before you start, ensure you have suitable hardware. See the [What hardware to get](#what-hardware-to-get) section below.

This assumes a Linux distribution that uses systemd. Instructions differ slightly between:

* Debian-based distributions: Debian, Raspberry Pi OS, Ubuntu
* RPM-based distributions: Fedora, CentOS, RHEL

### Install satpulse

1. [Install Go](https://go.dev/doc/install).
2. Make sure you have `git`` installed
   * On Debian: `sudo apt install git`
   * On Fedora: `sudo dnf install git`
3. Clone the satpulse repository: `git clone https://github.com/jclark/satpulse.git`
4. Change into the satpulse directory: `cd satpulse`
5. Build it: `make`
6. Install it: `sudo make install`

After this, you will have

* the SatPulse daemon installed `/usr/local/sbin/satpulsed`
* the configuration file for the daemon installed as `/usr/local/etc/satpulse.toml`
* the systemd unit file for the daemon installed as `/etc/systemd/system/satpulse@.service`
* the SatPulse command line tool installed as `/usr/local/sbin/satpulsetool`

### Configure satpulse

1. Edit the configuration file: `sudo nano /usr/local/etc/satpulse.toml`. In particular, you may need to change:
    * the serial port speed
    * the network interface that the PPS input is connected to
2. Start it: `sudo systemctl start satpulse.service@ttyX` where `/dev/ttyX` is the serial device connected to your GPS receiver 
3. Check that it started ok: `sudo systemctl status satpulse.service@ttyX`
4. Check the logs: `journalctl -u satpulse@ttyX`
5. Enable it at boot: `sudo systemctl enable satpulse.service@ttyX`

### Install and configure ptp4l

1. `sudo apt install linuxptp`
2. Install a ptp4l service
   * On Debian: `sudo cp configs/ptp4l.service /etc/systemd/system/`
   * On Fedora: there's nothing needed; the service provided by the RPM is fine
3. Modify the ptp4l config file; use [configs/ptp4l.conf](configs/ptp4l.conf) as a starting point
   * On Debian: the file is `/etc/linuxptp/ptp4l.conf`
   * On Fedora: the file is `/etc/ptp4l.conf`
4. `sudo systemctl enable --now ptp4l`

## Features

SatPulse provides the following features:

- It provides the PTP grandmaster instance with the data it needs about an external source of time. With ptp4l, this is done using the PTP management protocol.

    - It provides information about whether the PTP hardware clock is synchronized with GNSS receiver. This information will be provided by ptp4l to its clients, which can then use this in selecting the best grandmaster.

    - It provides information about how UTC time can be derived from atomic time. While GNSS and PTP both work natively in atomic time, which does not have leap seconds, they also provide information about leap seconds to enable UTC time to be derived from atomic time.

- It can act as a chrony SOCK refclock (similar to `phc2sys -E refclock_sock` in LinuxPTP 4.x).

- It can talk to U-blox receivers (or clones) using the UBX protocol, which is the native binary protocol of U-blox receivers. This gives SatPulse access to the full capabilities of U-blox receivers. 
It can also use the ASCII NMEA protocol, which is universally supported, but provides limited capabilities.

    - It can determine the receiver's configuration, and automatically make changes so that it works optimally for timing purposes.
      (Changes are made only to the receiver's RAM configuration, so will be undone if the receiver is power cycled.)

    - It can take advantage of the sawtooth correction (sometimes called quantization error) information that is available from some U-blox receivers.
      This enables more precise synchronization of the PTP hardware clock.

    - It can get the current time from the GNSS receiver as atomic time, and use that directly to set the PTP hardware clock.
      Both GNSS and PTP work natively using atomic time (without leap seconds) rather than UTC. With the NMEA protocol, time has to be converted to UTC and then back to atomic time.

    - It can get information about upcoming leap seconds from GNSS and provide that information to the PTP grandmaster.

- It automatically handles NICs that generate timestamps for both edges of a pulse (Intel NICs, including the i210, do this). (Using the UBX protocol we can get the pulse width.)

- It is aware of the quirks of the Raspberry CM4 and has code to cleanly work around them.

- It allows TCP connections with the GNSS receiver attached to the serial port (similar to ser2net). This means you can run SatPulse on a Linux box (such as a Raspberry Pi) and then connect back to the GNSS receiver over TCP from a Windows PC, using a program like u-center (from U-blox), to monitor or configure the GNSS receiver, *at the same time as* it is being used for PTP. (This provides plenty of opportunity to break things, but is quite handy.)

- It provides an HTTP interface for monitoring.

## What hardware to get

There are very few inexpensive NICs that support PPS input. At the time of writing (2023Q3), the best options are:

- the Intel i210, specifically the i210-T1 card; this can be used with any PC;
- Raspberry Pi Compute Module 4 (CM4), combined with the official CM4 IO board

For more information (including suitable GNSS receivers)

- for the i210 and other PC-based options, see my [pc-ptp-ntp-guide](https://github.com/jclark/pc-ptp-ntp-guide) project
- for the CM4 option, see my [rpi-cm4-ptp-guide](https://github.com/jclark/rpi-cm4-ptp-guide) project 

When choosing a GNSS receiver for use with SatPulse, I recommend using a u-blox receiver.

For PTP to work well, clients need to have NICs with PTP hardware timestamping support. This is a common feature of modern NICs. The PTP features also need to be supported by the driver. Intel NICs generally have PTP hardware timestamping with Linux driver support.

For best results the network switches should also have PTP support. For a low-cost switch, I recommend the FS.com IES3110 series.

## Relationship to other software

There are some existing, well-established open source projects that provide similar functionality, but they are don't do quite what SatPulse does.

It would probably be possible to evolve these, but they are both written in C, and I personally want to work in more modern, safer languages.

### LinuxPTP

The [LinuxPTP](https://linuxptp.sourceforge.net/) project provides a number of programs. The main one is ptp4l, which implements the PTP protocol. SatPulse is intended to work in conjunction with ptp4l. LinuxPTP also includes the ts2phc program, which with the `-s nmea` options performs the same basic function as SatPulse. However, it does not have the extra features that SatPulse provides. In particular, it understands only the NMEA protocol; this is simple, and universally supported, but the only time-related information it provides is the current UTC time.

### gpsd

The [gpsd](https://gpsd.gitlab.io/gpsd/) project provides a daemon that talks to GNSS receivers and understands the UBX protocol, as well as several others, but it lacks PTP-specific functionality, and its protocol is not a good fit for the needs of PTP:

- it assumes the PPS signal connected to a serial line, rather than to a NIC; the sawtooth correction information is linked to the PPS information, so isn't available when the PPS is connected to the NIC
- it provides time in UTC not atomic time, and doesn't provide leap second information




