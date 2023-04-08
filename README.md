The gps4ptp project provides software that allows a GPS receiver to be used as a source of time for the Precision Time Protocol (PTP). The goal is to make it easy and inexpensive to run a PTP grandmaster, which can provide a high-precision source of time for a network. This project does not provide an implementation of PTP. Instead it is designed to work with the ptp4l program from the LinuxPTP project.

It is written in the Go programming language. It works only on Linux, since only Linux provides the necessary APIs.

The project is currently at a pre-alpha stage of development. It is not yet ready for use.

## Basics of how it works

Gps4ptp is designed to make two pieces of hardware work together:

- a GPS receiver (or GPS disciplined oscillator) that has a PPS (pulse per second) output pin
- a NIC (network interface card) that includes a PTP hardware clock and both
   - supports PTP hardware timestamping, and
   - has a PPS input pin (more precisely, supports using the PTP hardware clock to timestamp pulses received on the pin)

The PPS output from the GPS needs to be connected to the PPS input on the NIC.

gps4ptp is a daemon, written in the Go language, which:

- talks to a GPS receiver over a serial port
- uses the Linux kernel's PTP hardware clock infrastructure to
   - read the timestamps of pulses from the GPS
   - adjust the time of the PTP hardware clock

The pulse from the GPS receiver is precisely aligned with the start of a second; the information over the serial port is used to determine which second it is.

The ptp4l daemon (part of the LinuxPTP project) then uses PTP to distribute the time of the PTP hardware clock to other PTP nodes.

## Quick start

This assumes a Linux system that uses systemd, such as Ubuntu or Fedora.

1. Ensure you have suitable hardware. See the [What hardware to get](#what-hardware-to-get) section below.
2. [Install Go](https://go.dev/doc/install).
3. Make sure you have git installed: `sudo apt install git`
4. Clone the gps4ptp repository: `git clone https://github.com/jclark/gps4ptp.git`
5. Change into the gps4ptp directory: `cd gps4ptp`
6. Build it: `./build.sh`
7. Install it: `sudo ./install.sh` (this will install it as `/usr/local/sbin/gps4ptp`)
8. Edit the configuration file: `sudo nano /usr/local/etc/gps4ptp.toml`. In particular, you may need to change:
    * the serial port device
    * the serial port speed
    * the network interface that the PPS input is connected to
9. Start it: `sudo systemctl start gps4ptp.service`
10. Check that it started ok: `sudo systemctl status gps4ptp.service`
12. Check the logs: `journalctl -u gps4ptp`
13. Enable it at boot: `sudo systemctl enable gps4ptp.service`
14. Install and configure ptp4l
    * `sudo apt install linuxptp`
    * Modify `/etc/linuxptp/ptp4l.conf`; use [configs/ptp4l.conf](configs/ptp4l.conf) as a starting point
    * `sudo systemctl start ptp4l@eth0.service`; replace `eth0` by the appropriate network interface
    * `sudo systemctl enable ptp4l@eth0.service`

## Features

Gps4ptp provides the following features:

- It filters the pulses received from the GPS receiver to eliminate those that appear to be spurious. This prevents spurious pulses causing spikes in the PTP hardware clock.

- It automatically handles NICs that generate timestamps for both edges of a pulse (some Intel NICs, including the i210, do this).

- It can talk to U-blox receivers (or clones) using the UBX protocol, which is the native binary protocol of U-blox receivers. This gives gps4ptp access to the full capabilities of U-blox receivers. UBX is a bidirectonal protocol, which allows gps4ptp to make changes to the receiver's configuration. It can also use the ASCII NMEA protocol, which is universally supported, but provides limited capabilities.

- By using the UBX protocol, it can take advantage of the sawtooth correction (sometimes called quantization error) information that is available from some U-blox receivers. This enables more precise synchronization of the PTP hardware clock.

- It allows TCP connections with the GPS receiver attached to the serial port (similar to ser2net). This means you can run gps4ptp on a Linux box (such as a Raspberry Pi) and then connect back to the GPS receiver over TCP from a Windows PC, using a program like u-center (from U-blox), to monitor or configure the GPS receiver, *at the same time as* it is being used for PTP. (This provides plenty of opportunity to break things, but is quite handy.)
 
- By using the UBX protocol, it can get the current time from the GPS receiver as atomic time, and use that directly to set the PTP hardware clock. Both GPS and PTP work natively using atomic time (without leap seconds) rather than UTC. With the NMEA protocol, time has to be converted to UTC and then back to atomic time.

- It provides the PTP grandmaster instance with the data it needs about an external source of time. With ptp4l, this is done using the PTP management protocol.

    - It provides information about whether the PTP hardware clock is synchronized with GPS receiver. This information will be provided by ptp4l to its clients, which can then use this in selecting the best grandmaster.

    - It provides information about how UTC time can be derived from atomic time. While GPS and PTP both work natively in atomic time, which does not have leap seconds, they also provide information about leap seconds to enable UTC time to be derived from atomic time.

The following features are in the process of being implemented:
- It can examine the configuration of the U-blox receiver and reconfigure as necessary so it works well for timing purposes.

- The PTP hardware clock on the CM4 does not work properly when the network cable is unplugged; gps4ptp detects this and handles it automatically.

The following features are planned:
 
- Allow monitoring of the GPS receiver using HTTP.

## What hardware to get

There are very few inexpensive NICs that support PPS input. At the time of writing (2023Q2), the best options are:

- the Intel i210, specifically the i210-T1 card; this can be used with any PC
- Raspberry PI Compute Module 4 (CM4), combined with the official CM4 IO board

There are plenty of suitable GPS receivers. I recommend getting a U-blox receiver.  

More information specifically for the CM4 option (including suitable GPS receivers) is available in my [rpi-cm4-ptp-guide](https://github.com/jclark/rpi-cm4-ptp-guide) project.

For PTP to work well, clients need to have NICs with PTP hardware timestamping support. This is a common feature of modern NICs. The PTP features also need to be supported by the driver. Intel NICs generally have PTP hardware timestamping with Linux driver support.

For best results the network switches should also have PTP support. For a low-cost switch, I recommend the FS.com IES3110 series.

## Relationship to other software

There are some existing, well-established open source projects that provide similar functionality, but they are don't do quite what gps4ptp does.

It would probably be possible to evolve these, but they are both written in C, and I personally want to work in more modern, safer languages.

### LinuxPTP

The [LinuxPTP](https://linuxptp.sourceforge.net/) project provides a number of programs. The main one is ptp4l, which implements the PTP protocol. gps4ptp is intended to work in conjunction with ptp4l. LinuxPTP also includes the ts2phc program, which with the `-s nmea` options performs the same basic function as gps4ptp. However, it does not have the extra features that gps4ptp provides. In particular, it understands only the NMEA protocol; this is simple, and universally supported, but the only time-related information it provides is the current UTC time.

### gpsd

The [gpsd](https://gpsd.gitlab.io/gpsd/) project provides a daemon that talks to GPS receivers and understands the UBX protocol, as well as several others, but it lacks PTP-specific functionality, and its protocol is not a good fit for the needs of PTP:

- it assumes the PPS signal connected to a serial line, rather than to a NIC; the sawtooth correction information is linked to the PPS information, so isn't available when the PPS is connected to the NIC
- it provides time in UTC not atomic time, and doesn't provide leap second information






