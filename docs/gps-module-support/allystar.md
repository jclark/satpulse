---
title: Allystar
toc: false
classes: wide
---

SatPulse supports modules from [Allystar](https://www.allystar.com/en).
The vendor name used with the `--vendor` option and `vendor` key is `allystar`.

These modules all use the Allystar binary protocol,
which is similar in style to the UBX protocol
and handles both periodic data and configuration.

For these modules, SatPulse supports:

- decoding of the Allystar binary packet format (packet format tag is `ASBIN`)
- conversion of messages into the SatPulse device-independent data model
- low-level configuration
  - message files for configuration
  - an `asbin` message type in message files, with correlation of responses

High-level configuration is [under development](https://github.com/jclark/satpulse/pull/349).

SatPulse has been tested with the TAU1201 and the more recent TAU951M-P200.

## Supported messages

SatPulse decodes the following messages of the Allystar binary protocol.

| Message | Class/ID | Used for |
|---------|----------|----------|
| NAV-POSECEF | 0x01 0x01 | ECEF position |
| NAV-POSLLH | 0x01 0x02 | geodetic position |
| NAV-DOP | 0x01 0x04 | solution quality |
| NAV-TIME | 0x01 0x05 | TAI time, UTC offset |
| NAV-VELECEF | 0x01 0x11 | ECEF velocity |
| NAV-VELNED | 0x01 0x12 | geodetic velocity |
| NAV-TIMEUTC | 0x01 0x21 | UTC time |
| NAV-CLOCK | 0x01 0x22 | decode only |
| NAV-SVINFO | 0x01 0x30 | satellites |
| NAV-SVIN | 0x01 0x31 | survey |
| NAV-AUTO | 0x01 0xC0 | geodetic position, geodetic velocity, solution quality |
| ACK-NAK | 0x05 0x00 | configuration acknowledgement |
| ACK-ACK | 0x05 0x01 | configuration acknowledgement |
| CFG-PRT | 0x06 0x00 | serial port configuration |
| CFG-MSG | 0x06 0x01 | message configuration |
| CFG-PPS | 0x06 0x07 | time pulse configuration |
| CFG-CFG | 0x06 0x09 | non-volatile memory operations |
| CFG-ELEV | 0x06 0x0B | navigation model configuration |
| CFG-NAVSAT | 0x06 0x0C | signal configuration |
| CFG-SURVEY | 0x06 0x12 | time mode configuration |
| CFG-FIXEDECEF | 0x06 0x14 | time mode configuration |
| CFG-SIMPLERST | 0x06 0x40 | receiver reset |
| CFG-NMEAVER | 0x06 0x43 | decode only |
| MON-VER | 0x0A 0x04 | receiver identification |
