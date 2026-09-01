---
title: Quectel
toc: false
classes: wide
---

SatPulse supports the LG290P and LC29H series of modules from [Quectel](https://www.quectel.com/).
The vendor name used with the `--vendor` option and `vendor` key is `quectel`.

Rather than a completely separate protocol,
Quectel makes use of the NMEA proprietary sentence extension mechanism, which uses NMEA sentences starting with `P`.
Quectel has defined a set of sentences starting with `PQTM` (`P` together with `QTM`, which is the 3-character mnemonic for Quectel).
These are used for both periodic data and configuration.

Some Quectel modules also expose the protocol used by the chipset.
In the case of the LC29H, which uses the Airoha AG3335 chipset,
the chipset also uses NMEA proprietary sentences, but starting with `PAIR` (where `AIR` is the mnemonic for Airoha).
These sentences are used only for configuration.
The LG290P uses only PQTM.

SatPulse supports:

- conversion of PQTM sentences into the SatPulse device-independent data model
- low-level configuration
  - message files for configuration of LG290P and LC29H
  - correlation of responses to PQTM and PAIR messages

High-level configuration for the LG290P is [under development](https://github.com/jclark/satpulse/pull/355).

SatPulse has been tested with the LG290P and the LC29H.
The LG580P and LG680P use the same protocol as the LG290P.

## Supported PQTM sentences

SatPulse decodes the following PQTM sentences.

| Sentence | Used for |
|---------|----------|
| PQTMANTENNASTATUS | decode only |
| PQTMDOP | solution quality |
| PQTMEOE | navigation epoch |
| PQTMEPE | solution quality |
| PQTMGEOFENCESTATUS | decode only |
| PQTMNAV | UTC time, TAI time, geodetic position, geodetic velocity, solution quality |
| PQTMODO | decode only |
| PQTMPL | decode only |
| PQTMPPPNAV | UTC time, TAI time, geodetic position, geodetic velocity, solution quality |
| PQTMPVT | UTC time, TAI offset, geodetic position, geodetic velocity, solution quality |
| PQTMSVINSTATUS | survey |
| PQTMTXT | decode only |
| PQTMVEL | geodetic velocity, solution quality |
