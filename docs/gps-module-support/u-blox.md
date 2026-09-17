---
title: u-blox
toc: false
classes: wide
---

SatPulse supports a wide range of modules from [u-blox](https://www.u-blox.com/),
from the LEA-6T through to the ZED-X20P,
covering their standard precision, high precision and timing product categories.
The vendor name used with the `--vendor` option and `vendor` key is `u-blox` or `ublox`.

All u-blox modules use the binary UBX protocol.
The UBX protocol handles both periodic data and configuration.
Different versions of the protocol differ in which messages they support:
the messages used by the LEA-6T and the ZED-X20P are mostly different.
In particular, generation 9 introduced a new configuration system,
which is completely different that used by earlier generations.
SatPulse supports both the new and the old configuration system.

For u-blox modules, SatPulse supports:

- decoding of the UBX packet format (packet format tag is `UBX`)
- conversion of messages into the SatPulse device-independent data model
- high-level configuration
- low-level configuration
  - message files for configuration that high-level configuration does not cover
  - `ubx`, `ubxval` and `ubxvalport` message types in message files, with correlation of responses
- [conversion]({%link man/satpulsetool-convobs.1.md%}) of raw observation (UBX-RXM-RAWX) messages into RINEX

SatPulse has been tested with the following modules:

- ZED-X20P
- NEO-F10T
- NEO-F10N
- MAX-F10S
- MAX-M10S
- UBX-M10050-KB
- ZED-F9P
- ZED-F9T
- LEA-F9T
- NEO-M9N
- LEA-M8T
- LEA-M8F
- UBX-M8030
- UBX-G7020
- LEA-6T

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
* SBAS requires a major constellation other than GLONASS
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
The most general type is `ubx` which allows you to specify an arbitrary UBX message.


Suppose you want to enable UBX-NAV-CLOCK message at 1Hz.
If you consult the relevant manual (u-blox M8 Receiver description),
it will tell you that you need to send UBX-CFG-MSG (class 0x06, id 0x01), with the following payload

| Byte Offset | Format | Scaling | Name | Unit | Description |
|------------:|--------|---------|------|------|-------------|
| 0 | `U1` | - | `msgClass` | - | Message class |
| 1 | `U1` | - | `msgID` | - | Message identifier |
| 2 | `U1` | - | `rate` | - | Send rate on current port |


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

## Supported messages

SatPulse decodes the following UBX messages.
The last column says whether high-level configuration can automatically enable output of the message.

| Message | Class/ID | Used for | Automatically enabled |
|---------|----------|----------|-----------------------|
| UBX-NAV-POSECEF | 0x01 0x01 | ECEF position | yes |
| UBX-NAV-POSLLH | 0x01 0x02 | geodetic position | yes |
| UBX-NAV-DOP | 0x01 0x04 | solution quality | yes |
| UBX-NAV-SOL | 0x01 0x06 | decode only | no |
| UBX-NAV-PVT | 0x01 0x07 | UTC time, geodetic position, geodetic velocity, solution quality | yes |
| UBX-NAV-VELECEF | 0x01 0x11 | ECEF velocity | yes |
| UBX-NAV-VELNED | 0x01 0x12 | geodetic velocity | yes |
| UBX-NAV-HPPOSECEF | 0x01 0x13 | ECEF position | yes |
| UBX-NAV-HPPOSLLH | 0x01 0x14 | geodetic position | yes |
| UBX-NAV-TIMEGPS | 0x01 0x20 | TAI time, UTC offset | yes |
| UBX-NAV-TIMEUTC | 0x01 0x21 | UTC time | yes |
| UBX-NAV-CLOCK | 0x01 0x22 | decode only | no |
| UBX-NAV-TIMEGLO | 0x01 0x23 | TAI time | no |
| UBX-NAV-TIMEBDS | 0x01 0x24 | TAI time, UTC offset | no |
| UBX-NAV-TIMEGAL | 0x01 0x25 | TAI time, UTC offset | no |
| UBX-NAV-TIMELS | 0x01 0x26 | leap second | yes |
| UBX-NAV-TIMEQZSS | 0x01 0x27 | TAI time, UTC offset | no |
| UBX-NAV-SVINFO | 0x01 0x30 | satellites | yes |
| UBX-NAV-SAT | 0x01 0x35 | satellites | yes |
| UBX-NAV-SVIN | 0x01 0x3B | survey | yes |
| UBX-NAV-SIG | 0x01 0x43 | satellite signals | yes |
| UBX-NAV-EOE | 0x01 0x61 | navigation epoch | yes |
| UBX-NAV-TIMETRUSTED | 0x01 0x64 | decode only | no |
| UBX-RXM-RAWX | 0x02 0x15 | raw observations | yes |
| UBX-RXM-COR | 0x02 0x34 | corrections usage | no |
| UBX-INF-ERROR | 0x04 0x00 | logging | no |
| UBX-INF-WARNING | 0x04 0x01 | logging | no |
| UBX-INF-NOTICE | 0x04 0x02 | logging | no |
| UBX-INF-TEST | 0x04 0x03 | logging | no |
| UBX-INF-DEBUG | 0x04 0x04 | logging | no |
| UBX-ACK-NAK | 0x05 0x00 | configuration acknowledgement | - |
| UBX-ACK-ACK | 0x05 0x01 | configuration acknowledgement | - |
| UBX-CFG-PRT | 0x06 0x00 | communications port configuration | - |
| UBX-CFG-MSG | 0x06 0x01 | message configuration | - |
| UBX-CFG-INF | 0x06 0x02 | decode only | - |
| UBX-CFG-RST | 0x06 0x04 | receiver reset | - |
| UBX-CFG-RATE | 0x06 0x08 | navigation rate configuration | - |
| UBX-CFG-CFG | 0x06 0x09 | non-volatile memory operations | - |
| UBX-CFG-TMODE | 0x06 0x1D | time mode configuration | - |
| UBX-CFG-NAV5 | 0x06 0x24 | navigation model configuration | - |
| UBX-CFG-TP5 | 0x06 0x31 | time pulse configuration | - |
| UBX-CFG-TMODE2 | 0x06 0x3D | time mode configuration | - |
| UBX-CFG-GNSS | 0x06 0x3E | signal configuration | - |
| UBX-CFG-TMODE3 | 0x06 0x71 | time mode configuration | - |
| UBX-CFG-VALSET | 0x06 0x8A | changing configuration | - |
| UBX-CFG-VALGET | 0x06 0x8B | getting configuration | - |
| UBX-CFG-VALDEL | 0x06 0x8C | decode only | - |
| UBX-MON-VER | 0x0A 0x04 | receiver identification | no |
| UBX-MON-MSGPP | 0x0A 0x06 | decode only | no |
| UBX-MON-HW | 0x0A 0x09 | decode only | no |
| UBX-MON-GNSS | 0x0A 0x28 | signal capabilities | no |
| UBX-MON-COMMS | 0x0A 0x36 | port identification, logging | no |
| UBX-TIM-TP | 0x0D 0x01 | time pulse | yes |
| UBX-TIM-SVIN | 0x0D 0x04 | survey | yes |
| UBX-TIM-TOS | 0x0D 0x12 | time pulse | yes |
| UBX-MGA-GAL | 0x13 0x02 | OSNMA Merkle tree root | - |
| UBX-MGA-INI | 0x13 0x40 | time assistance | - |
| UBX-SEC-OSNMA | 0x27 0x0A | decode only | no |
