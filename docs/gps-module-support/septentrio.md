---
title: Septentrio
toc: false
classes: wide
---

SatPulse supports modules from [Septentrio](https://www.septentrio.com/). {% include new-in-03.html %}
The vendor name used with the `--vendor` option and `vendor` key is `septentrio`.

Septentrio modules use the Septentrio Binary Format (SBF) for periodic data.
Septentrio refers to individual messages as *blocks*.
Configuration uses line-oriented ASCII commands.

SatPulse supports:

- decoding of the SBF packet format (packet format tag is `SBF`)
- conversion of blocks into the SatPulse device-independent data model
- low-level configuration
  - message files for configuration
  - a `septentrio` response pattern in message files, for correlation of responses

[High-level configuration](https://github.com/jclark/satpulse/pull/354)
and [conversion of raw observations into RINEX](https://github.com/jclark/satpulse/pull/356)
are under development.

SatPulse has been tested with the mosaic-G5 P3.

## Supported blocks

SatPulse decodes the following SBF blocks.

| Block | Number | Used for |
|-------|--------|----------|
| MeasExtra | 4000 | decode only |
| DOP | 4001 | solution quality |
| PVTCartesian | 4006 | TAI time, ECEF position, ECEF velocity, solution quality, survey |
| PVTGeodetic | 4007 | TAI time, geodetic position, geodetic velocity, solution quality |
| SatVisibility | 4012 | decode only |
| ChannelStatus | 4013 | satellites |
| ReceiverStatus | 4014 | decode only |
| MeasEpoch | 4027 | satellite signals |
| GALUtc | 4031 | leap second |
| QualityInd | 4082 | decode only |
| RFStatus | 4092 | decode only |
| BDSUtc | 4121 | leap second |
| GALAuthStatus | 4245 | decode only |
| GPSUtc | 5894 | leap second |
| ReceiverSetup | 5902 | decode only |
| PosCovCartesian | 5905 | solution quality |
| PosCovGeodetic | 5906 | solution quality |
| VelCovCartesian | 5907 | solution quality |
| VelCovGeodetic | 5908 | solution quality |
| xPPSOffset | 5911 | time pulse |
| ReceiverTime | 5914 | TAI time, UTC time |
| DiffCorrIn | 5919 | corrections usage |
| EndOfPVT | 5921 | navigation epoch |
| EndOfMeas | 5922 | decode only |
| BaseStation | 5949 | corrections usage |
