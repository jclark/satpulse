---
title: "Precision network timing"
---

The focus of the SatPulse project is using GNSS receivers with computer systems.
The starting point is a GNSS receiver directly connected to a host computer.
For precision timing, two connections to the receiver are required.
The first is a pulse-per-second (PPS) connection: the GNSS receiver emits a pulse that marks the start of each second very accurately.
The second is a serial connection: the GNSS receiver emits messages shortly after each second,
which include the current time.

For timing, we are trying to solve two problems:
how to transfer time from the GNSS receiver to the host computer,
and how to transfer time from the host computer to other computers over the network.
The classic approach to both these problems is purely software-based, in the sense that only general-purpose hardware is used.
The PPS signal is attached to a GPIO or serial port pin.
The pulse edge causes an interrupt, and the kernel's interrupt handler records the system clock time when it was called.
This information, together with the messages from the serial connection, is used
by an NTP server to synchronize the host's system clock with the GNSS receiver's time.
An NTP client exchanges packets with NTP servers over the network.
The kernel timestamps each packet, recording the system clock time it sent or received the packet.
These timestamps are used by the NTP client to synchronize its system clock with the server.
This works very well, but the achievable accuracy is limited to about 1 microsecond.
This is well short of the accuracy of a GPS receiver,
which is about 5ns for a high-end model, or about 30ns for an inexpensive model.

In the last few years, inexpensive hardware has become available that makes it possible to do much better than this.
A precision in the low tens of nanoseconds is achievable with [hardware with a total cost in the low hundreds of dollars]({% link hardware/index.md %}#hardware-for-timing-applications).
This hardware is designed to support the Precision Time Protocol (PTP), but its use is not restricted to PTP,
and, in fact, the hardware can be used to improve the performance of NTP.

## PTP hardware clocks

The main hardware component into which PTP hardware support is incorporated is the ethernet controller.
An ethernet controller that supports PTP has its own clock, called the PTP hardware clock (PHC), which is completely independent
of the computer's system clock. The principal function of the PHC is to timestamp incoming and outgoing ethernet packets.
Each packet is timestamped by the controller hardware, without going through the operating system; this allows the timestamp to be
accurate to within a few nanoseconds.

For GNSS time transfer, the ethernet controller must also have a PPS (pulse-per-second)
input pin, sometimes called an SDP (software-defined pin).
Pulses on the pin are timestamped in hardware by the ethernet controller.
As with the packets, the timestamp of a pulse is with respect to the PHC.
Although many ethernet controllers have PTP support,
few of those provide a suitable input pin, and even fewer of those are inexpensive. Linux is also required,
because only Linux has [APIs](https://docs.kernel.org/driver-api/ptp.html) that provide the necessary access
to the ethernet controller's PTP support. The ethernet controller must also have Linux drivers that support these APIs.

Note that the transfer of time from the GNSS receiver to the PHC using a PHC's SDP is not handled by the PTP daemon.
A separate component is needed to do this.
This is different from NTP, where the NTP daemon is responsible for disciplining the server's system clock using timing information provided by reference clocks
together with timing information from other NTP servers.

## Synchronizing the system clock

Although Linux provides APIs that allow a PHC to be used as a POSIX clock,
almost all computer software is written to make use of the computer's system clock rather than a PHC.
This means that it is necessary to synchronize the system clock from the PHC.

With hardware support for timestamping packets and PPS pulses, it turns out that this step is the least accurate part of the whole
synchronization chain.
A relatively new technology called Precision Time Measurement (PTM) has been developed to address this problem.
PTM is a PCI Express (PCIe) feature that enables devices on the PCIe bus to synchronize their clocks.
In particular, it provides hardware support for precise synchronization of the
PHC and the system clock.
At a high level, it's like PTP, but for the PCIe bus rather than the network.
PTM requires [support]({% link hardware/ptm.md %}) from both the network card and the computer (specifically the motherboard chipset's PCIe subsystem).

The Linux kernel enables applications to take advantage
of PTM using a system call that performs *cross timestamping*, which
means that a single system call fetches both the system clock time and the PHC time simultaneously.
(The system call is the PTP_SYS_OFFSET_PRECISE ioctl.)

There are also a few on-motherboard Intel ethernet controllers, such as the I219-V, that support cross timestamping using a device-specific mechanism unrelated to PTM.

The PTP daemon deals only with transferring time from the PHC of one system to the PHC of another system.
Synchronizing the system clock with the PHC needs to be handled by another component.
One way to do this is to use an NTP daemon in conjunction with a reference clock that supplies timing information based on the PHC.

## Time scales

System clocks, at least on Unix-like systems, conventionally use the UTC time scale,
with the start of 1970 as the epoch.

PTP uses the TAI time scale, which is a continuous time scale not affected by leap seconds.
TAI is currently ahead of UTC by 37 seconds.
On Linux, the PHC is conventionally in TAI time, also with the start of 1970 as the epoch.

GNSS constellations with the exception of GLONASS use continuous time scales, which are a fixed offset from TAI.
They periodically broadcast the current offset between the GNSS system time and UTC.
This allows GNSS receivers to report time in UTC.
However, after a receiver cold starts, while it is waiting for the broadcast, which can take up to 12.5 minutes with GPS,
it relies on the offset compiled into its firmware.
The last leap second was at the end of 2016.
This means that receivers with firmware older than that can report the wrong UTC time for up to 12.5 minutes after a cold start.

The NMEA protocol reports time only in UTC.
However, vendor-specific protocols can also report time in the GNSS time scale.
This allows the GNSS system time to be converted directly to the PTP time scale,
without conversion into and out of UTC.

The GLONASS system time scale is based on UTC, which makes GLONASS a slightly less good fit for PTP.

This issue is becoming less important.
The responsible international organizations are moving strongly in the direction of abolishing leap seconds,
and the relevant technical committee has recommended that this happen in 2027.
If this is approved, as is likely, TAI will remain a constant 37 seconds ahead of UTC for all dates from 2017 onwards.

## Time source selection

PTP and NTP take different approaches to selecting time sources.
Typically, an NTP daemon is configured with multiple sources of time,
and the daemon decides how to select and combine them to produce a single time estimate.

The normal approach with PTP is that each PTP server on the network broadcasts its availability, along with metadata about its clock quality, such as its accuracy.
A PTP client then selects from the available servers using the Best Master Clock Algorithm (BMCA), which is defined in the PTP standard.
(PTP has traditionally used the term "grandmaster" to refer to a server,
but this term is being phased out in favour of more inclusive language.)

This means that it is crucial that the component that transfers time from the GNSS receiver to the PHC provide metadata to the PTP daemon about the clock quality.
In particular, it is important that the PTP daemon is notified promptly if the
GNSS receiver loses its lock, so that clients can switch over to an alternative server.

## Switches

The other main kind of component that has PTP hardware support is the [network switch]({% link hardware/switches.md %}).
The accuracy of the algorithm PTP uses to synchronize PHCs depends on path delay being symmetric:
in other words, that the time taken for a packet to travel from A to B will be the same as the time taken for a packet to travel from B to A.
But this will not usually be the case when there are switches in between.
A switch can reduce accuracy by hundreds of nanoseconds.
PTP has the concept of a transparent clock, which solves this by allowing
the switch to insert a field into the packet that says how long the packet has spent in the switch.

Adding a switch with PTP support typically improves synchronization quality more than changing from a low-end GNSS receiver to a high-end GNSS receiver.

## Chain of synchronization

Distributing GNSS time over a network involves a chain of synchronization:
GNSS receiver to PHC to switch to PHC to system clock.
To preserve GNSS accuracy, each link in the chain needs hardware support:
- hardware timestamping of the PPS signal
- hardware timestamping of the network packets
- switch with PTP transparent clock support
- PTM for cross timestamping

With all of these in place, end-to-end accuracy in the tens of nanoseconds is achievable.

## Timing receivers

Several vendors, notably u-blox, sell receivers designed specifically for timing use.
These receivers typically have three features that are significant for timing use.

First, they provide a timing mode.
Normally, a GNSS receiver uses information from at least four satellites to compute its 3D position and the time.
In timing mode, it assumes a fixed position and then uses information from one satellite to compute the time.
There are usually two possibilities for determining the fixed position to be used.
* It can be explicitly specified by the user, typically done using PPP.
* The receiver can perform a survey-in process, where it computes its position once a second over a user-specified period of time, and then uses the average of the positions as the fixed position.

Second, they offer the ability to report raw observations.
This can be used to determine the fixed position to use with timing mode.
This is described in more detail in the [Precision positioning]({% link intro/positioning.md %}#raw-observations) page.

Third, they provide reporting of quantization or sawtooth error.
When GNSS receivers generate a pulse to mark the start of a second, the pulse is constrained to be aligned to a tick of the receiver's internal clock, and so will not usually be precisely aligned to the true start of the second as determined by the receiver.
The error in the pulse caused by this constraint is called a quantization error
or sawtooth error.
This error is reported in a message generated shortly before or after the pulse,
which allows the host computer to correct for it.

The first two features are also provided in receivers designed for precision positioning.
Only quantization error reporting is unique to timing receivers.
But modern receivers tend to have higher clock speeds which make this error less significant than it used to be:
a typical figure is 8ns peak-to-peak.

Note that ionospheric error corrected by dual-band receivers is greater than quantization error,
particularly in locations close to the magnetic equator.

