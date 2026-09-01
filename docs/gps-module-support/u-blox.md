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
