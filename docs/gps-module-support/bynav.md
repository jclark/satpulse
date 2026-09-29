---
title: Bynav
toc: false
classes: wide
---

SatPulse supports the M10 and M2 series (M20/M21/M22) modules from [Bynav](https://www.bynav.com/en/).
The vendor name used with the `--vendor` option and `vendor` key is `bynav`.

These modules use the [NovAtel OEM6/OEM7 protocol]({% link gps-module-support/novatel.md %}) for their logs:
Bynav supports some logs defined by NovAtel and defines some of its own.
Configuration uses line-oriented ASCII commands, which are different from NovAtel's, although similar in style.

For these modules, SatPulse supports:

- decoding of the ASCII and binary packet formats used for logs (packet format tags are `NOVA` and `NOVB`)
- decoding of the abbreviated ASCII packet format used for command responses (packet format tag is `NOVAA`)
- conversion of logs into the SatPulse device-independent data model
- low-level configuration
  - a message file for configuration
  - a `novatel` response pattern in message files, for correlation of responses

SatPulse has been tested with the M10 and the M20.

## Low-level configuration

A command like the following will configure a receiver suitably for `satpulsed`:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m /usr/share/satpulse/gpsmsg/bynav/m10-m20.toml \
    -t msg-all-off,nov-timeb,nov-bestposb,nov-bestvelb,nov-psrdopb,nov-ionutcb,nmea-talker-auto,nmea-gsa,nmea-gsv,pps
```

Add `save` to the comma-separated list of tags to make it persistent.

### Notes on tags in the message file

- `fix-rate-N` also limits the output rate of logs to N a second.
- The effect of `fix-rate-*`, `mode-base` and `mode-rover` persists across `reload`.
- `nmea-gsa` outputs a GSA only for GPS, unless `nmea-talker-auto` has been used.
- The `gnss-*` tags take effect only after `save` and `reload`.
- BeiDou cannot be disabled.
- The RTCM tags work only in base mode.
- `mode-base` uses the receiver's current position as the base position, unless a fixed position has been set.
- The receiver can output only one kind of RTCM observation message for each system,
  so only one of `rtcm-legacy`, `rtcm-msm4` and `rtcm-msm7` can be in effect at a time.

## Supported logs

SatPulse decodes the following logs.
The log name is given without the A or B suffix that selects the ASCII or binary form;
the number is the message ID used by the binary form.
Bynav's interface protocol manual does not document BESTVEL or PSRDOP,
but Bynav receivers output them.

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
