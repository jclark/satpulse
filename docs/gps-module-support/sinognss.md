---
title: SinoGNSS/ComNav
toc: false
classes: wide
---

SatPulse supports the K9 series of modules from [SinoGNSS](https://www.sinognss.com/),
which brands itself as [ComNav Technology](https://www.comnavtech.com/) for Western audiences.
The vendor name used with the `--vendor` option and `vendor` key is `sinognss` or `comnav`.

These modules use the [NovAtel OEM6/OEM7 protocol]({% link gps-module-support/novatel.md %}) for their logs, which SinoGNSS calls *messages*.
There are some minor implementation differences in the logs between SinoGNSS and NovAtel:
for example, SinoGNSS uses different position type values.
The `vendor` must be specified to enable correct handling of these SinoGNSS differences.
Configuration uses line-oriented ASCII commands, which are different from NovAtel's, although similar in style.

For these modules, SatPulse supports:

- decoding of the ASCII and binary packet formats used for periodic messages (packet format tags are `NOVA` and `NOVB`)
- decoding of the abbreviated ASCII packet format used for command responses (packet format tag is `NOVAA`)
- conversion of messages into the SatPulse device-independent data model
- a message file for low-level configuration

SatPulse has been tested with the K901 and the K902.

## Supported messages

SatPulse decodes the following messages.
The message name is given without the A or B suffix that selects the ASCII or binary form;
the number is the message ID used by the binary form.

| Message | Number | Used for |
|---------|--------|----------|
| IONUTC | 8 | leap second |
| BESTPOS | 42 | geodetic position, solution quality |
| PSRPOS | 47 | geodetic position, solution quality |
| BESTVEL | 99 | geodetic velocity |
| PSRVEL | 100 | geodetic velocity |
| TIME | 101 | UTC time, TAI time |
| PSRDOP | 174 | solution quality |
| BESTXYZ | 241 | ECEF position, ECEF velocity, solution quality |
