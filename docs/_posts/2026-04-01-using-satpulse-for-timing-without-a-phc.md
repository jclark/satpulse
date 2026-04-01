---
title: Using SatPulse for timing without a PHC
date: 2026-04-01 16:45:00 +0700
---

Up to now, using SatPulse for timing has required some [very specialized hardware]({%link hardware/index.md %}).
Over the last couple of days, I have implemented a feature that removes this requirement.
It works very simply. SatPulse already has the ability to talk to an NTP server using chrony's refclock SOCK protocol;
this is configured by the `[ntp]` section of `satpulse.toml`.
SatPulse also has the ability to work without a PTP hardware clock;
this is configured simply by leaving out the `[phc]` section of `satpulse.toml`.
In this mode, you can use SatPulse for monitoring and for GNSS packet distribution.
What I've implemented is to make the `[ntp]` section work without a `[phc]` section.
In this situation, SatPulse will use the timing of serial messages from the receiver
to generate samples to send to the NTP server.
On its own, the accuracy from serial timing will be very poor: worse than you would typically get from NTP over a network.
But this becomes useful when combined with an NTP server that has the ability to read a PPS signal.
For example chrony has a PPS refclock that uses the kernel PPS subsystem.
Chrony can also read a PPS signal from a PHC using the PHC refclock with the extpps option.
In this case the NTP server uses the much more accurate PPS signal to determine when a second starts,
and will use the information from SatPulse to determine which second it is.

I have hooked this up to SatPulse's GPS auto-configuration system (enabled with `config=true` in the `[gps]` section).
So when you have an `[ntp]` and no `[phc]` section, it will configure the GPS appropriately (e.g enable a PPS,
enable time mode, enable messages reporting UTC time).
I think this will provide quite a nice way of running an NTP server: as well as auto-configuration, you get the
other SatPulse conveniences like a Web dashboard and Prometheus metrics.

I have only tested this very lightly. I used the chrony PHC extpps option to test,
since all my machines are currently set up with the PPS connected to a PHC.
I used the following as the satpulsed configuration: 

```
[serial]
device = "/dev/ttyACM0"
speed = 38400
[gps]
config = true
vendor = "u-blox"
[ntp]
sock.path = "/var/run/chrony.satpulse.clk.sock"
```

and then used this as my chrony configuration:

```
refclock PHC /dev/ptp0:extpps:pin=1 width 0.1 poll 2 lock NMEA refid PPS
refclock SOCK /var/run/chrony.satpulse.clk.sock offset 0.1 delay 0.2 refid NMEA noselect
```

The `lock NMEA` option tells chrony to use the samples from SatPulse to complete the PHC samples;
the `noselect` option tells chrony not to use the SatPulse samples as an independent time source.

Using chrony to read the PHC like this has one advantage over using SatPulse with a `[phc]` section:
SatPulse will adjust the phase and frequency of the PHC, which is what you want for PTP,
whereas chrony will leave it free-running.
This makes it easy to use the hardware-timestamping feature of chrony, which requires
a free-running PHC.
With SatPulse, you would need to use a virtual PHC, which I have not yet implemented support for.
So using chrony like this is right now the best approach if you want NTP hardware timestamping
and are not interested in PTP.

