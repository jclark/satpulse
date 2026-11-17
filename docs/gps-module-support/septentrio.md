---
title: Septentrio
toc: false
classes: wide
---

SatPulse supports modules from [Septentrio](https://www.septentrio.com/).
The vendor name used with the `--vendor` option and `vendor` key is `septentrio`.

Septentrio modules use the Septentrio Binary Format (SBF) for periodic data.
Septentrio refers to individual messages as *blocks*.
Configuration uses line-oriented ASCII commands.

SatPulse supports:

- decoding of the SBF packet format (packet format tag is `SBF`)
- conversion of blocks into the SatPulse device-independent data model
- high-level configuration; this is experimental and is enabled only when a vendor is explicitly specified {% include new-in.html version="0.4" %}
- low-level configuration
  - message files
  - a `septentrio` response pattern in message files, for correlation of responses
- [conversion]({%link man/satpulsetool-convobs.1.md%}) of raw observation (MeasEpoch and MeasExtra) blocks into RINEX

SatPulse has been tested with the mosaic-G5 P3.

## Supported blocks

SatPulse decodes the following SBF blocks.
The last column says whether high-level configuration can automatically enable output of the block.

| Block | Number | Used for | Automatically enabled |
|-------|--------|----------|-----------------------|
| MeasExtra | 4000 | raw observations | yes |
| DOP | 4001 | solution quality | yes |
| PVTCartesian | 4006 | TAI time, ECEF position, ECEF velocity, solution quality, survey | yes |
| PVTGeodetic | 4007 | TAI time, geodetic position, geodetic velocity, solution quality | yes |
| SatVisibility | 4012 | decode only | no |
| ChannelStatus | 4013 | satellites | yes |
| ReceiverStatus | 4014 | decode only | no |
| MeasEpoch | 4027 | satellite signals, raw observations | yes |
| GALUtc | 4031 | leap second | yes |
| QualityInd | 4082 | decode only | no |
| RFStatus | 4092 | decode only | no |
| BDSUtc | 4121 | leap second | yes |
| GALAuthStatus | 4245 | decode only | no |
| GPSUtc | 5894 | leap second | yes |
| ReceiverSetup | 5902 | decode only | no |
| PosCovCartesian | 5905 | solution quality | no |
| PosCovGeodetic | 5906 | solution quality | no |
| VelCovCartesian | 5907 | solution quality | no |
| VelCovGeodetic | 5908 | solution quality | no |
| xPPSOffset | 5911 | time pulse | yes |
| ReceiverTime | 5914 | TAI time, UTC time | no |
| DiffCorrIn | 5919 | corrections usage | yes |
| EndOfPVT | 5921 | navigation epoch | yes |
| EndOfMeas | 5922 | decode only | no |
| BaseStation | 5949 | corrections usage | yes |
