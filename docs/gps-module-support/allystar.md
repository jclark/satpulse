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
- high-level configuration; this is experimental and is enabled only when a vendor is explicitly specified {% include new-in-03.html %}
- low-level configuration
  - message files for configuration of TAU1201, TAU13xx and TAU951M modules
  - an `asbin` message type in message files, with correlation of responses

SatPulse has been tested with the TAU1201 and TAU1202, the L1/L2 TAU1302,
the L1/L5 TAU1312 and the more recent TAU951M-P200.

## High-level configuration

High-level configuration has been tested with the TAU1201, TAU1202, TAU1302,
TAU1312 and TAU951M-P200.
It supports:

- selection of GNSS constellations and signals
- configuration of the minimum elevation and time pulse
- mobile, survey-in and fixed-position modes
- configuration of PVT, satellite, NMEA, raw and RTCM message output
- serial speed changes
- selective and complete saves, reloads, cold resets and factory resets

The available RTCM output depends on the module.
The TAU13xx and TAU951M-P200 support MSM4, MSM7 and 1005 output,
whereas the TAU1201/TAU1202 family does not support RTCM output.
Raw output is controlled by RXM-DUMPRAW, but the resulting RXM-RAW payload is undocumented and is not decoded by SatPulse.

Allystar receivers do not expose a time-pulse timestamp, leap-second announcements,
an end-of-epoch message or per-signal satellite information.
They also do not expose an RTCM base-station ID, fixed-position accuracy,
antenna cable delay, time-pulse time scale or the identity of the active serial port.
Fixed positions are stored as ECEF coordinates with one-centimetre resolution;
geodetic positions are converted to ECEF when configured.

## Low-level configuration

The message files under `configs/gpsmsg/allystar/` cover the TAU1201,
TAU13xx and TAU951M module families.
The `asbin` message type sends arbitrary Allystar binary messages
and correlates their data and acknowledgement responses.

## Supported messages

SatPulse decodes the following messages of the Allystar binary protocol.
The last column says whether high-level configuration can automatically enable output of the message.

| Message | Class/ID | Used for | Automatically enabled |
|---------|----------|----------|-----------------------|
| NAV-POSECEF | 0x01 0x01 | ECEF position | yes |
| NAV-POSLLH | 0x01 0x02 | geodetic position | yes |
| NAV-DOP | 0x01 0x04 | solution quality | yes |
| NAV-TIME | 0x01 0x05 | TAI time, UTC offset | yes |
| NAV-VELECEF | 0x01 0x11 | ECEF velocity | yes |
| NAV-VELNED | 0x01 0x12 | geodetic velocity | yes |
| NAV-TIMEUTC | 0x01 0x21 | UTC time | yes |
| NAV-CLOCK | 0x01 0x22 | decode only | no |
| NAV-SVINFO | 0x01 0x30 | satellites | yes |
| NAV-SVIN | 0x01 0x31 | survey | yes |
| NAV-AUTO | 0x01 0xC0 | geodetic position, geodetic velocity, solution quality | yes |
| RXM-DUMPRAW | 0x02 0x01 | raw message configuration | - |
| ACK-NAK | 0x05 0x00 | configuration acknowledgement | - |
| ACK-ACK | 0x05 0x01 | configuration acknowledgement | - |
| CFG-PRT | 0x06 0x00 | serial port configuration | - |
| CFG-MSG | 0x06 0x01 | message configuration | - |
| CFG-PPS | 0x06 0x07 | time pulse configuration | - |
| CFG-CFG | 0x06 0x09 | non-volatile memory operations | - |
| CFG-ELEV | 0x06 0x0B | navigation model configuration | - |
| CFG-NAVSAT | 0x06 0x0C | signal configuration | - |
| CFG-SURVEY | 0x06 0x12 | time mode configuration | - |
| CFG-FIXEDLLA | 0x06 0x13 | time mode configuration | - |
| CFG-FIXEDECEF | 0x06 0x14 | time mode configuration | - |
| CFG-SIMPLERST | 0x06 0x40 | receiver reset | - |
| CFG-NMEAVER | 0x06 0x43 | decode only | - |
| MON-VER | 0x0A 0x04 | receiver identification | no |
