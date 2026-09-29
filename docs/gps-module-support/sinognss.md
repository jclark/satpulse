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

In their factory default configuration, these modules output no messages and use a speed of 115200 baud.

For these modules, SatPulse supports:

- decoding of the ASCII and binary packet formats used for periodic messages (packet format tags are `NOVA` and `NOVB`)
- decoding of the abbreviated ASCII packet format used for command responses (packet format tag is `NOVAA`)
- conversion of messages into the SatPulse device-independent data model
- a message file for low-level configuration

SatPulse has been tested with the K803, the K901 and the K902.

## Low-level configuration

A command like the following will configure a receiver suitably for `satpulsed`:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m /usr/share/satpulse/gpsmsg/sinognss/sinognss.toml \
    -t msg-all-off,nov-timeb,nov-bestposb,nov-bestvelb,nov-psrdopb,nov-ionutcb,nmea-ver-411,nmea-gsa,nmea-gsv,pps
```

Add `save` to the comma-separated list of tags to make it persistent.

### Notes on tags in the message file

- The receiver does not support simultaneous output of the ASCII and binary versions of the same log,
  so a tag such as `nov-timeb` implicitly does `nov-timea-off`.
- `msg-all-off` stops output on every port.
- The `fix-rate-*` tags do not change the output rate:
  the tags that enable logs and NMEA sentences output them once a second.
- `nov-ionutcb` and `nov-ionutca` do not output IONUTC periodically:
  they output it once, and after that only when the parameters change.
- `nov-rangeb-30` generates raw observations every 30 seconds aligned with GPS time,
  which are suitable for generating RINEX for submission to a PPP service such as CSRS-PPP.
- `rtcm-msm4`, `rtcm-msm7` and `rtcm-eph` work in rover mode as well as base mode.
- `rtcm-eph` outputs the ephemerides of all satellites once, then one satellite each second, in turn.
- `rtcm-arp` puts the receiver in base mode, fixed at its current position,
  and puts it back in base mode each time it outputs RTCM 1005.
  To use a known position instead, send `fixed-pos-example`, edited with your position, first.
- To put the receiver into rover mode, first do `rtcm-off` if `rtcm-arp` has been enabled,
  and then do `fixed-pos-off`.
- On the K902, `ppp-b2b` did not work with `gnss-bds`; it needed `gnss-all`.

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
