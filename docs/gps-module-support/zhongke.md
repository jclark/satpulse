---
title: Zhongke Microelectronics
toc: false
classes: wide
---

SatPulse supports modules from [Zhongke Microelectronics](https://icofchina.com/),
including the ATGM332D and ATGM336H series and the AT632-6T-30 timing module.
The vendor name used with the `--vendor` option and `vendor` key is `zhongke` or `casic`.

These modules all use the CASIC binary protocol,
which is similar in style to the UBX protocol
and handles both periodic data and configuration.
There are two generations of this protocol:
the first is used by ATGM332D-5N and ATGM336H-5N series modules using the AT6558 chipset,
and the second by subsequent modules.
SatPulse supports both generations.

For these modules, SatPulse supports:

- decoding of the CASIC packet format (packet format tag is `CASBIN`)
- conversion of messages into the SatPulse device-independent data model
- high-level configuration; this is experimental and is enabled only when a vendor is explicitly specified {% include new-in-03.html %}
- low-level configuration
  - message files for configuration that high-level configuration does not cover
  - a `casbin` message type in message files, with correlation of responses

SatPulse has been tested with the ATGM332D-5N series, ATGM332D-6N series, ATGM332D-F8N series, AT372-6P-34 and AT632-6T-30.
The ATGM336H modules differ from the corresponding ATGM332D modules only in form factor.
For ATGM332D or ATGM336H modules, the last two digits in the full module name e.g. ATGM332D-5N-71 indicate the supported constellations.
Accordingly, SatPulse should work with any ATGM332D or ATGM336H module.

## Supported messages

SatPulse decodes the following CASIC messages.
The two generations of the protocol use disjoint message classes:
NAV and TIM belong to the first generation,
and NAV2, TIM2 and RXM2 to the second;
the ACK, CFG, MSG and MON classes are used by both.
The last column says whether high-level configuration can automatically enable output of the message.

| Message | Class/ID | Used for | Automatically enabled |
|---------|----------|----------|-----------------------|
| NAV-DOP | 0x01 0x01 | solution quality | yes |
| NAV-SOL | 0x01 0x02 | TAI time, ECEF position, ECEF velocity | yes |
| NAV-PV | 0x01 0x03 | geodetic position, geodetic velocity | yes |
| NAV-TIMEUTC | 0x01 0x10 | UTC time | yes |
| NAV-CLOCK | 0x01 0x11 | decode only | no |
| NAV-GPSINFO | 0x01 0x20 | satellites | yes |
| NAV-BDSINFO | 0x01 0x21 | satellites | yes |
| NAV-GLNINFO | 0x01 0x22 | satellites | yes |
| TIM-TP | 0x02 0x00 | time pulse | yes |
| ACK-NAK | 0x05 0x00 | configuration acknowledgement | - |
| ACK-ACK | 0x05 0x01 | configuration acknowledgement | - |
| CFG-PRT | 0x06 0x00 | serial port configuration | - |
| CFG-MSG | 0x06 0x01 | message configuration | - |
| CFG-RST | 0x06 0x02 | receiver reset | - |
| CFG-TP | 0x06 0x03 | time pulse configuration | - |
| CFG-RATE | 0x06 0x04 | navigation rate configuration | - |
| CFG-CFG | 0x06 0x05 | non-volatile memory operations | - |
| CFG-TMODE | 0x06 0x06 | time mode configuration | - |
| CFG-NAVX | 0x06 0x07 | signal configuration, navigation model configuration | - |
| CFG-NAVLIMIT | 0x06 0x0A | navigation model configuration | - |
| CFG-NAVBAND | 0x06 0x0F | signal configuration | - |
| CFG-NMEA | 0x06 0x12 | decode only | - |
| CFG-RTCM | 0x06 0x14 | decode only | - |
| CFG-TMODE2 | 0x06 0x16 | time mode configuration | - |
| MSG-BDSUTC | 0x08 0x00 | decode only | no |
| MSG-GPSUTC | 0x08 0x05 | decode only | yes |
| MON-VER | 0x0A 0x04 | receiver identification | no |
| NAV2-DOP | 0x11 0x01 | solution quality | yes |
| NAV2-SOL | 0x11 0x02 | TAI time, ECEF position, ECEF velocity | yes |
| NAV2-PVH | 0x11 0x03 | geodetic position, geodetic velocity | yes |
| NAV2-TIMEUTC | 0x11 0x05 | UTC time, TAI offset | yes |
| NAV2-SIG | 0x11 0x06 | satellites | yes |
| TIM2-TPX | 0x12 0x00 | time pulse | yes |
| TIM2-TIMEGPS | 0x12 0x01 | TAI time, UTC offset, leap second | no |
| TIM2-TIMEBDS | 0x12 0x02 | TAI time, UTC offset, leap second | no |
| TIM2-TIMEGLN | 0x12 0x03 | UTC time | no |
| TIM2-TIMEGAL | 0x12 0x04 | TAI time, UTC offset, leap second | no |
| TIM2-TIMEIRN | 0x12 0x05 | decode only | no |
| TIM2-TIMEPOS | 0x12 0x06 | survey | yes |
| TIM2-LS | 0x12 0x07 | leap second | yes |
| TIM2-LY | 0x12 0x08 | decode only | no |
| TIM2-TCXO | 0x12 0x09 | decode only | no |
| RXM2-MEASX | 0x13 0x00 | decode only | no |
