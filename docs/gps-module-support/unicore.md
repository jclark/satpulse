---
title: Unicore
toc: false
classes: wide
---

SatPulse supports the NebulasIV series of high precision modules (UM980, UM981, UM982 and UM960) from [Unicore](https://en.unicore.com/).
Unicore's standard precision modules, such as the UM620, use a completely different protocol and are not supported;
nor is the UT986 timing receiver, which also uses a different protocol.
The vendor name used with the `--vendor` option and `vendor` key is `unicore`.

The Unicore protocol is similar to the [NovAtel OEM6/OEM7 protocol]({% link gps-module-support/novatel.md %}).
It treats periodic data, which it calls *logs*, differently from configuration:
the logs have a dual ASCII/binary syntax,
whereas configuration uses line-oriented ASCII commands.

For these modules, SatPulse supports:

- decoding of the ASCII and binary packet formats used for logs (packet format tags are `UNCA` and `UNCB`)
- conversion of logs into the SatPulse device-independent data model
- high-level configuration
- low-level configuration
  - message files for configuration that high-level configuration does not cover
  - a `unicore` response pattern in message files, for correlation of responses
- [conversion]({%link man/satpulsetool-convobs.1.md%}) of raw observation (OBSVM) logs into RINEX

These modules also have an undocumented capability for emitting NovAtel OEM6/OEM7 compatible logs,
and SatPulse supports this as well.
There are some minor implementation differences in these logs between Unicore and NovAtel:
for example, Unicore uses a different number for the IONUTC log.
The `vendor` must be specified to enable correct handling of these Unicore differences.

SatPulse has been tested with the UM980, UM982 and UM960.

## Supported logs

SatPulse decodes the following logs.
The log name is given without the A or B suffix that selects the ASCII or binary form;
the number is the message number used by the binary form.
The use is described in terms of the SatPulse device-independent data model;
*decode only* means the log is decoded but does not contribute to the model.
The last column says whether high-level configuration can automatically enable output of the log.

| Log | Number | Used for | Automatically enabled |
|-----|--------|----------|--------------------------|
| RECTIME | 102 | UTC time, TAI time | yes |
| GPSUTC | 19 | leap second | yes |
| GALUTC | 20 | leap second | yes |
| BD3UTC | 22 | leap second | yes |
| BDSUTC | 2012 | leap second | no |
| BESTNAV | 2118 | geodetic position, geodetic velocity, solution quality | yes |
| BESTNAVXYZ | 240 | ECEF position, ECEF velocity, solution quality | yes |
| PPPNAV | 1026 | geodetic position, solution quality | no |
| SATSINFO | 2124 | satellites | yes |
| BESTSAT | 1041 | satellite usage | yes |
| STADOP | 954 | solution quality | yes |
| OBSVM | 12 | raw observations | yes |
| PPSSTATUS | 9000 | decode only | no |
| VERSION | 37 | receiver identification | no |
