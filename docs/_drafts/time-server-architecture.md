---
title: "Time server architecture"
---

It can be hard to understand how everything fits together with a PTP/NTP time server. This post explains how things work when using SatPulse.

## Kernel devices

There are four key kernel devices involved.

- System clock. There is an underlying raw hardware counter. The kernel exposes three kinds of clock based on this.
   - the raw monotonic clock corresponds to this hardware counter, and is not adjusted for phase or frequency
   - the monotonic clock is adjusted by NTP for frequency, so that it can accurately measure time intervals
   - the realtime clock is adjusted by NTP for frequency and phase, so that it corresponds closely to UTC (this implies it can jump)

- PTP Hardware Clock (PHC). This is a hardware clock within the ethernet controller. It is important to understand that this is completely independent of the system clock. It has a software-defined pin (SDP) connected to the GPS receiver's PPS output. It has the capability to timestamp a pulse received on the SDP, i.e. record the time of the PHC at which the pulse occurred. The PHC can be adjusted for phase and frequency.

- Network interface. This is provided by an ethernet controller and is responsible for sending and receiving network packets with PTP or NTP messages. Crucially, it has an associated PHC and can timestamp ingoing and outgoing packets using the PHC i.e. the hardware records the time of the PHC at which each packet is sent or received.

- Serial port. This is connected to the serial or USB interface of the GPS receiver. The kernel TTY subsystem exposes this as a /dev/tty*X* device.

## Processes

Apart from satpulsed, which is the main daemon program in SatPulse, there are two other processes involved.

- ptp4l, which is part of LinuxPTP, implements the PTP standard. The PTP standard focuses on transferring time over the network from the PHC of one device to the PHC of another device. It does not concern itself with the system clock nor does it specify a mechanism for transferring time from an external source such as a GPS receiver into a PHC. When used in a time server with satpulsed, it is satpulsed rather than ptp4l that adjusts the PHC. Ptp4l interacts with the PHC primarily via hardware timestamping: when it sends and receives packets containing PTP messages it instructs the network interface to record the time of the PHC at which the packets were sent or received and to provide it with those times.

- chronyd, which is part of chrony, implements the NTP standard. Whereas PTP is focused on the PHC, NTP is focused on the system clock. Chrony accepts information from NTP servers and from reference clocks and uses this to update the system clock and to provide information to NTP clients.

The fundamental role of satpulsed is to transfer time from the GPS receiver to the PHC. This involves the following.

- Reading timestamp events from the PHC. The PHC hardware can record the time of the PHC at which a time pulse occurs on a PHC SDP. The GPS emits time pulses that are precisely aligned with the beginning of a second. So if the timestamp says the pulse is 45 nanoseconds after the beginning of the second, this indicates that the PHC differs from the true time by 45 nanoseconds plus some integral number of seconds.
- Reading messages from the GPS receiver over the serial port. These messages give the approximate current time. By correlating these messages with the timestamp events, it becomes possible to determine which second the pulse is marking the beginning of, and thus calculate exactly how much the PHC differs from the true time.
- Adjusting the PHC. From the timestamp events and GPS messages, satpulsed derives a measurement once per second indicating how much the PHC differs from the true time. When the process starts up, this information is used to set the PHC time. After that, it continually adjusts the frequency of the PHC to keep the difference between the PHC and true time as small as possible. Note that setting the PHC time is relatively imprecise, because this involves separate operations to read and write the PHC time. To get the PHC time precisely right, frequency adjustment is necessary.

The PTP standard requires that when time is transferred in from an external source, certain metadata relating to the time is also transferred in. With ptp4l, this metadata is provided using a specific PTP management message defined by ptp4l (called GRANDMASTER_SETTINGS_NP, NP for non-portable). To support this, satpulsed implements a PTP management client and uses it to send GRANDMASTER_SETTINGS_NP messages to ptp4l over a socket. The kinds of metadata that PTP requires can be divided into two.

The first kind of metadata relates to the offset between TAI time and UTC time. The time of the PHC uses the PTP timescale which is based on TAI. TAI differs from UTC time by an integral number of seconds, which changes when leap seconds occur. It turns out that all GNSS systems (other than GLONASS) work in a similar way: their clocks use a timescale based on TAI, but they also provide metadata about the offset between TAI and UTC time. Usually GPS receivers hide all this complexity and output messages with UTC time, which is suitable for NTP. But for PTP, the GPS receiver should output messages with TAI time (or rather GNSS time, which is a fixed number of seconds from TAI time) and also messages about leap seconds. During start-up, satpulsed sends configuration messages to the GPS to make it do this, and then it uses these messages to provide the appropriate metadata to ptp4l.

The second kind of metadata relates to the clock quality. This says whether the PHC is synchronized to GNSS at all, and if it is, how accurately it is synchronized. A PTP server broadcasts the quality of its clock. PTP clients use this information to choose the best PTP server. So if there's a problem with the GPS receiver which prevents satpulsed from keeping the PHC synchronized with the GPS receiver time, then satpulsed needs to tell ptp4l about this promptly, so that ptp4l can change the metadata that it provides to clients about its clock quality. This will allow clients to switch over to an alternative PTP server. To implement this, satpulsed has to monitor the synchronization state.

When used with chrony, satpulsed in addition takes readings of the PHC and the realtime clock that are as simultaneous as the hardware allows. When the PHC is in sync with the GPS receiver time, then these readings can be used to generate a sample saying how much the realtime clock is off from the true time; satpulsed will send these samples to chrony over a socket using the refclock SOCK protocol. Chrony can use this as a reference clock.

## Comparison to classic NTP server

This architecture differs from a classic NTP server, using an NTP server such as ntpd or chrony together with gpsd. Apart from the addition of a PTP implementation, the key difference is in the presence of a PHC. In a classic NTP architecture, the GPS receiver is attached to a serial port pin or GPIO pin. When a GPS receiver emits a pulse, an interrupt will occur and the kernel will record the time of the pulse. The kernel makes these timestamps available to applications via a PPS kernel device, which will be read by either gpsd or the NTP server.

The crucial difference is that with a PPS signal connected to a PHC, the hardware records the time of the PHC at which each pulse occurs without the intervention of the kernel. This enables the timestamps to be accurate to a few nanoseconds, whereas with a PPS device the accuracy is several orders of magnitude worse.
