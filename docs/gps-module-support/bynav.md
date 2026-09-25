---
title: ByNav
toc: false
classes: wide
---

SatPulse supports the M20 and M10 series of modules from [ByNav](https://www.bynav.com/en/).
The vendor name used with the `--vendor` option and `vendor` key is `bynav`.

These modules use the [NovAtel OEM6/OEM7 protocol]({% link gps-module-support/novatel.md %}) for their logs:
ByNav supports some logs defined by NovAtel and defines some of its own.
Configuration uses line-oriented ASCII commands, which are different from NovAtel's, although similar in style.

For these modules, SatPulse supports:

- decoding of the ASCII and binary packet formats used for logs (packet format tags are `NOVA` and `NOVB`)
- decoding of the abbreviated ASCII packet format used for command responses (packet format tag is `NOVAA`)
- conversion of logs into the SatPulse device-independent data model
- low-level configuration
  - a message file for configuration
  - a `novatel` response pattern in message files, for correlation of responses

SatPulse has been tested with the M20 and the M10.

## Low-level configuration

A command like the following will configure a receiver suitably for `satpulsed`:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m /usr/share/satpulse/gpsmsg/bynav/bynav.toml \
    -t msg-all-off,nov-timeb,nov-bestposb,nov-bestvelb,nov-psrdopb,nov-ionutcb,nmea-talker-auto,nmea-gsa,nmea-gsv,pps
```

Add `save` to the comma-separated list of tags to make it persistent.

## Supported logs

SatPulse decodes the following logs.
The log name is given without the A or B suffix that selects the ASCII or binary form;
the number is the message ID used by the binary form.
ByNav's interface protocol manual does not document BESTVEL or PSRDOP,
but ByNav receivers output them.

| Log | Number | Used for |
|-----|--------|----------|
| IONUTC | 8 | leap second |
| BESTPOS | 42 | geodetic position, solution quality |
| BESTVEL | 99 | geodetic velocity |
| PSRVEL | 100 | geodetic velocity |
| TIME | 101 | UTC time, TAI time |
| PSRDOP | 174 | solution quality |
| BESTGNSSPOS | 1429 | geodetic position, solution quality |
| BESTGNSSVEL | 1430 | geodetic velocity |
| BYCHECK | 42272 | decode only |
