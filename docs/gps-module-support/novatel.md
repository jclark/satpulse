---
title: NovAtel
toc: false
classes: wide
---

SatPulse supports the OEM6/OEM7 protocol defined by [NovAtel](https://novatel.com/) for its GPS modules.
The vendor name used with the `--vendor` option and `vendor` key is `novatel`.

This protocol treats periodic data, which it calls *logs*, differently from configuration.
Periodic data can be encoded in either an ASCII or a binary packet format,
whereas configuration uses line-oriented ASCII commands.
The protocol makes the set of logs extensible:
each log has an identifier, which is a name in the ASCII format and a number in the binary format,
and a structured payload.
The protocol defines how to encode a structured payload as either an ASCII or binary packet.

For this protocol, SatPulse supports:

- decoding of the ASCII and binary packet formats used for logs (packet format tags are `NOVA` and `NOVB`)
- decoding of the abbreviated ASCII packet format used for command responses (packet format tag is `NOVAA`)
- conversion of logs into the SatPulse device-independent data model

SatPulse does not include a message file for configuring NovAtel modules.

SatPulse has not yet been tested with a NovAtel receiver;
support for the protocol has been validated with ByNav, SinoGNSS and Unicore modules.

## Supported logs

SatPulse decodes the following logs.
The log name is given without the A or B suffix that selects the ASCII or binary form;
the number is the message ID used by the binary form.

| Log | Number | Used for |
|-----|--------|----------|
| IONUTC | 8 | leap second |
| BESTPOS | 42 | geodetic position, solution quality |
| PSRPOS | 47 | geodetic position, solution quality |
| BESTVEL | 99 | geodetic velocity |
| PSRVEL | 100 | geodetic velocity |
| TIME | 101 | UTC time, TAI time |
| PSRDOP | 174 | solution quality |
| BESTXYZ | 241 | ECEF position, ECEF velocity, solution quality |
| BESTGNSSPOS | 1429 | geodetic position, solution quality |
| BESTGNSSVEL | 1430 | geodetic velocity |
