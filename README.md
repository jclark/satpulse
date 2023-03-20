The gps4ptp project provides software that allows a GPS receiver to be used as a source of time for the Precision Time Protocol (PTP). The goal is to make it easy and inexpensive to run a PTP grandmaster, which can provide a high-precision source of time for a network. This project does not provide an implementation of PTP. Instead it is designed to work with the ptp4l program from the LinuxPTP project.

It is written in the Go programming language. It works only on Linux, since only Linux provides the necessary APIs.

The project is currently in still at a pre-alpha stage of development. It is not yet ready for use.

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

## Features

Gps4ptp provides the following features:

- It filters the pulses received from the GPS receiver to eliminate those that appear to be spurious. This prevents spurious pulses causing spikes in the PTP hardware clock.

- It can take advantage of the sawtooth correction information provided by some U-blox receivers. This enables more precise synchronization of the PTP hardware clock.

- The most common Intel NICs generate timestamps for both edges of a pulse: gps4ptp deals with this automatically.

- It can talk to U-blox receivers (or clones) using UBX, the native U-blox binary protocol. This gives gps4ptp access to the full capabilities of the receiver. UBX is a bidirectonal protocol, which allows gps4ptp to make changes to the receiver's configuration.

- XXX ser2net

- Using the UBX protocol it can get the current atomic time from the GPS receiver, and use that directly to set the PTP hardware clock. Both GPS and PTP work natively using atomic time rather than UTC. With UBX, time has to be converted to UTC and then back to atomic time.

The following features are in progress:

- It includes a PTP management client that is used to provide information to ptp4l. PTP requires that a time source provide the PTP instance with not only the current time, but also data about the time. Gps2ptp uses the PTP management protocol to provide the following information:

    - It informs ptp4l when it no longer has a time fix from GPS. This information will be provided by ptp4l to its clients, which will then have the opportunity to select an alternative grandmaster.

    - It provides information about how UTC time can be derived from atomic time. While GPS and PTP both work natively in atomic time, which does not have leap seconds, they also provide information about leap seconds to enable the current UTC time to be derived from the atomic time.

- The PTP hardware clock on the CM4 does not work properly when the network cable is unplugged; gps4ptp detects this and handles it automatically.

The following features are planned:
 
- XXX http

## What hardware to get

There are very few inexpensive NICs that support PPS input. At the time of writing (2023Q2), the best options are:

- the Intel i210, specifically the i210-T1 card; this can be used with any PC
- Raspberry PI Compute Module 4 (CM4), combined with the official CM4 IO board

There are plenty of suitable GPS receivers. I recommend getting a U-blox receiver.  

More information specifically for the CM4 option (including suitable GPS receivers) is available in my [rpi-cm4-ptp-guide](https://github.com/jclark/rpi-cm4-ptp-guide) project.

For PTP to work well, clients need to have NICs with PTP hardware timestamping support. This is a common feature of modern NICs. The PTP features also need to be supported by the driver. Intel NICs generally have PTP hardware timestamping with kernel support.

For best results the network switches should also have PTP support. For a low-cost switch, I recommend the FS.com IES3110 series.

## Relationship to other software

### LinuxPTP

The LinuxPTP project provides a number of programs. The main one is ptp4l, which implements the PTP protocol. gps4ptp is intended to work in conjunction with ptp4l. LinuxPTP also includes the ts2phc program, which with the `-s nmea` options performs the same basic function as gps4ptp. However, it does not have the extra features that gps4ptp provides. In particular, it uses the NMEA protocol, rather than UBX; this is simple, and universally supported, but the only information provided is the current UTC time.

### gpsd

XXX




