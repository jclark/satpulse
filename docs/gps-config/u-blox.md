---
title: u-blox
sitemap: false
---


Both high-level and low-level configuration are fully supported for u-blox.

TODO: Explain that configuration differs a lot between generations and families

## High-level configuration

What is supported will depend on the product category. Standard precision modules do not support:
* time mode
* RTCM output
* raw message output

u-blox receivers allow the time pulse to be aligned to either a UTC realization or to a GNSS system time.
For best timing performance, u-blox recommends aligning to a GNSS system time,
and this is what the timeGNSS property supports. If a receiver has its time pulse aligned to UTC,
this will be represented as not having a timeGNSS property. Using `--pps` option will configure the time pulse to be aligned to a GNSS system time; which GNSS depends on which constellations are enabled.

u-blox supports selectively saving configuration to non-volatile memory (e.g. `--save` as opposed to `--save-all` in satpulsetool gps).
In gen 9, you can save precisely the configuration settings that were changed.
In gen 8, settings are saved in groups; if any setting in a group is changed, then selective save will save the whole group.

u-blox has limitations about what combinations of signals can be enabled. In particular:

* every constellation must have an L1 signal enabled
* with multi-band receivers, often L2 or L5 is required in addition to L1 on an enabled constellation
* QZSS requires GPS
* SBAS requires a major constellation other then GLONASS
* some receivers limit the number of major constellations that may be enabled

Some more recent receivers support a native USB interface.
This will show up as `/dev/ttyACM*` on Linux.
When using this interface, the receiver does not have a speed
(like 9600 baud), and you cannot set the receiver speed.
The serial device has a speed, but on Linux the speed makes no difference.
You can use `satpulsetool gps --show-port` to show what receiver port you are using.

## Low-level configuration

### Message files

### Message types

There are several u-blox-specific message types.

#### ubx message type
The most general type is `ubx` which allows you to specify and arbitrary UBX message.


Suppose you want a to enable UBX-NAV-CLOCK message at 1Hz.
If you consult the relevant manual (u-blox M8 Receiver description),
it will tell you that you need send UBX-CFG-MSG (class 0x06, id 0x01), with the following payload

| Byte Offset | Format | Scaling | Name | Unit | Description |
|------------:|--------|---------|------|------|-------------|
| 0 | `U1` | – | `msgClass` | – | Message class |
| 1 | `U1` | – | `msgID` | – | Message identifier |
| 2 | `U1` | – | `rate` | – | Send rate on current port |


The message class and id of UBX-NAV-CLOCK are 0x01 and 0x22, and the rate for 1Hz is 1.

This turns into the following entry in the TOML file:

```
[[ubx]]
tag = "ubx-nav-clock"
description = "Enable NAV-CLOCK"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0x01, 0x22, 1]
```

The `payload.types` and `payload.values` entries are parallel.

SatPulse knows about how acknowledgements work on UBX, so if you send this it can report if the appropriate acknowledgement message is received.

#### ubxval message type

#### ubxvalport message types

