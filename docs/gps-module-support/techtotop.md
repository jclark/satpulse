---
title: Techtotop/Taidou
toc: false
classes: wide
---

SatPulse supports modules from [Techtotop](https://www.techtotop.com/enindex.aspx), known in Chinese as Taidou.
The vendor name used with the `--vendor` option and `vendor` key is `techtotop` or `taidou`.

These modules all use the binary SDBP protocol,
which is similar in style to the UBX protocol
and handles both periodic data and configuration.

For these modules, SatPulse supports:

- decoding of the SDBP packet format (packet format tag is `SDBP`)
- conversion of messages into the SatPulse device-independent data model
- low-level configuration
  - message files for configuration
  - a `sdbp` message type in message files, with correlation of responses

SatPulse has been tested with the T303-5D, which is an L1/L5 timing module.

## Supported messages

SatPulse decodes the following SDBP messages.

| Message | Class/ID | Used for |
|---------|----------|----------|
| SDBP-PUB-ACK | 0x01 0x01 | configuration acknowledgement |
| SDBP-PUB-NAK | 0x01 0x02 | configuration acknowledgement |
| SDBP-CTL-RESTART | 0x02 0x01 | receiver reset |
| SDBP-CTL-CONFIG | 0x02 0x02 | non-volatile memory operations |
| SDBP-CTL-STANDBY | 0x02 0x04 | decode only |
| SDBP-CFG-GNSS | 0x03 0x11 | signal configuration |
| SDBP-CFG-UART | 0x03 0x21 | serial port configuration |
| SDBP-CFG-RATE | 0x03 0x36 | navigation rate configuration |
| SDBP-CFG-PPS | 0x03 0x41 | time pulse configuration |
| SDBP-CFG-TMODE | 0x03 0x43 | time mode configuration |
| SDBP-CFG-DELAY2 | 0x03 0x44 | time pulse configuration |
| SDBP-CFG-TMODE2 | 0x03 0x45 | time mode configuration |
| SDBP-CFG-NMEA | 0x03 0x51 | message configuration |
| SDBP-CFG-SDBP | 0x03 0x52 | message configuration |
| SDBP-QUE-VER | 0x05 0x01 | receiver identification |
| SDBP-DAT-DOP | 0x06 0x13 | solution quality |
| SDBP-DAT-BDST | 0x06 0x16 | TAI time |
| SDBP-DAT-GPST | 0x06 0x17 | TAI time |
| SDBP-DAT-GALT | 0x06 0x19 | TAI time |
| SDBP-DAT-ECEF2 | 0x06 0x1B | ECEF position, ECEF velocity |
| SDBP-DAT-LLA3 | 0x06 0x1D | geodetic position, geodetic velocity |
| SDBP-DAT-NED3 | 0x06 0x1E | geodetic velocity |
| SDBP-DAT-UTCT2 | 0x06 0x1F | UTC time, leap second |
| SDBP-DAT-BDSU | 0x06 0x2C | leap second |
| SDBP-DAT-GPSU | 0x06 0x2D | leap second |
| SDBP-DAT-GALU | 0x06 0x2E | leap second |
| SDBP-DAT-SAT | 0x06 0x30 | satellites |
| SDBP-DAT-TSURV | 0x06 0x40 | survey |
| SDBP-DAT-TPPS | 0x06 0x41 | time pulse |
