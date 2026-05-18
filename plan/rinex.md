# RINEX observation conversion

The goal is to generate RINEX observation files from RTCM streams with MSM7
data or from UBX streams with UBX-RXM-RAWX data.

As part of this, define a modern JSONL format for RINEX observation conversion.
The format should be semantically close to RINEX observation data, but without
the fixed-column syntax and header-dependent record layout. It should be usable
as an intermediate format in conversion workflows.

The JSONL format uses `.obsj` files. Records are either `SignalObservation` or
`Metadata`. There is no envelope or discriminator: if a JSON object has a `t`
field, it is a `SignalObservation`; otherwise it is `Metadata`.

`SignalObservation` represents one satellite signal at one epoch. It carries
the RINEX-style satellite identifier, RINEX signal identifier, observation time,
and the observation values needed to produce RINEX C/L/D/S records. GLONASS
FDMA frequency channel is carried on the observation when known.

`Metadata` carries scraps of information that correspond to RINEX observation
header fields. Metadata records may appear anywhere in the file. A RINEX writer
will read the whole `.obsj` file, merge metadata with facts derived from the
observations, and then write the RINEX observation header and data records.

Generating RINEX output is not streamable because the header depends on the
whole set of observations and metadata. Generating `.obsj` should be streamable
from RTCM MSM7 and UBX-RXM-RAWX with only limited buffering.

The intermediate format definition lives in `gps/lib/rinex` so it can be reused
outside the SatPulse domain layer. It must not depend on `gpsprot`, but should
use compatible concepts for satellite identifiers, time handling, and
observation values.

The conversion code from RTCM streams and UBX streams may need domain-layer
packages. That code can live outside `gps/lib/rinex`; the reusable library
package should define the `.obsj` records and low-level formatting helpers, not
own the whole conversion pipeline.

## Next steps

Add an `.obsj` reader in `gps/lib/rinex`. It should take an `io.Reader`, read
JSONL records containing a mixture of `SignalObservation` and `Metadata`, and
return separate slices of observations and metadata.

Add metadata merge support. The input should be a slice of `Metadata` records;
the output should be a single merged metadata value suitable for driving RINEX
header generation.

Add a RINEX observation writer. It should take an `io.Writer`, the merged metadata,
and a slice of `SignalObservation`, then write a
RINEX observation file or return an error. The writer will derive observation
code lists, epoch ordering, satellite ordering, and other header facts from the
complete observation slice.

Add a sink interface in `gps/lib/rinex` for conversion output. It should have
methods for `Metadata` and `SignalObservation`, and probably a `Flush` or close
method for final output and error reporting.

Provide one sink implementation that writes `.obsj` JSONL to an `io.Writer`.
This implementation can write records as they arrive.

Provide another sink implementation that writes RINEX observation output to an
`io.Writer`. This implementation should merge metadata as it arrives and buffer
observations until flush/finalization, then call the RINEX observation writer
described above. RINEX output needs whole-file facts for the header,
observation code lists, epoch ordering, and satellite ordering.

Add conversion from receiver stream formats outside `gps/lib/rinex`. The
converter code may depend on domain-layer packages and protocol-specific packet
handling. Converters should write their output to the `gps/lib/rinex` sink
interface, so conversion can be used for streaming `.obsj` generation, direct
RINEX generation, and tests.

Start with UBX-RXM-RAWX. It maps naturally to `SignalObservation`: one RAWX
measurement group becomes one signal observation, with the RAWX message epoch
providing the GPS-scale `Time`. The main missing piece is the mapping from UBX
`gnssId`/`sigId` to RINEX satellite and signal identifiers, plus state for
deriving LLI from carrier-phase validity and lock-time changes.

Add RTCM MSM7 conversion after that. MSM7 provides pseudorange, carrier phase,
Doppler, carrier-to-noise density, lock time, half-cycle ambiguity, and for
GLONASS the FDMA frequency channel. The converter takes a stream of RTCM
messages: MSM7 messages emit `SignalObservation` records, while ARP, receiver,
and antenna-related messages emit `Metadata` records. The converter will need
to assemble MSM fragments for the same epoch/reference station and map RTCM
satellite and signal IDs to RINEX identifiers.

## UBX gnssId/sigId to RINEX signal mapping

Sources: u-blox X20-HPG-2.00 section 1.5.4 Table 4 (signal identifiers); RINEX
4.02 Tables 10-16 (observation codes).

Each row maps a `(gnssId, sigId)` pair from `UBX-RXM-RAWX` to a RINEX satellite
system letter and a two-character RINEX signal suffix. The system letter
combined with the RINEX-numbered SV gives the `SatelliteID`; the signal suffix
prefixed with the observation type (`C`, `L`, `D`, `S`) gives the
`ObservationCode`.

| gnssId | sigId | UBX signal name        | RINEX sys | RINEX sig | RINEX band   |
| -----: | ----: | ---------------------- | :-------: | :-------: | ------------ |
|      0 |     0 | GPS L1C/A              |     G     |    1C     | L1 1575.42   |
|      0 |     3 | GPS L2 CL              |     G     |    2L     | L2 1227.60   |
|      0 |     4 | GPS L2 CM              |     G     |    2S     | L2 1227.60   |
|      0 |     6 | GPS L5 I               |     G     |    5I     | L5 1176.45   |
|      0 |     7 | GPS L5 Q               |     G     |    5Q     | L5 1176.45   |
|      1 |     0 | SBAS L1C/A             |     S     |    1C     | L1 1575.42   |
|      2 |     0 | Galileo E1 C (pilot)   |     E     |    1C     | E1 1575.42   |
|      2 |     1 | Galileo E1 B (data)    |     E     |    1B     | E1 1575.42   |
|      2 |     3 | Galileo E5 aI          |     E     |    5I     | E5a 1176.45  |
|      2 |     4 | Galileo E5 aQ          |     E     |    5Q     | E5a 1176.45  |
|      2 |     5 | Galileo E5 bI          |     E     |    7I     | E5b 1207.140 |
|      2 |     6 | Galileo E5 bQ          |     E     |    7Q     | E5b 1207.140 |
|      2 |     8 | Galileo E6 B           |     E     |    6B     | E6 1278.75   |
|      2 |     9 | Galileo E6 C           |     E     |    6C     | E6 1278.75   |
|      2 |    10 | Galileo E6 A (PRS)     |     E     |    6A     | E6 1278.75   |
|      3 |     0 | BeiDou B1I D1          |     C     |    2I     | B1I 1561.098 |
|      3 |     1 | BeiDou B1I D2          |     C     |    2I     | B1I 1561.098 |
|      3 |     2 | BeiDou B2I D1          |     C     |    7I     | B2 1207.140  |
|      3 |     3 | BeiDou B2I D2          |     C     |    7I     | B2 1207.140  |
|      3 |     4 | BeiDou B3I D1          |     C     |    6I     | B3 1268.52   |
|      3 |    10 | BeiDou B3I D2          |     C     |    6I     | B3 1268.52   |
|      3 |     5 | BeiDou B1 Cp (pilot)   |     C     |    1P     | B1C 1575.42  |
|      3 |     6 | BeiDou B1 Cd (data)    |     C     |    1D     | B1C 1575.42  |
|      3 |     7 | BeiDou B2 ap (pilot)   |     C     |    5P     | B2a 1176.45  |
|      3 |     8 | BeiDou B2 ad (data)    |     C     |    5D     | B2a 1176.45  |
|      5 |     0 | QZSS L1C/A             |     J     |    1C     | L1 1575.42   |
|      5 |     1 | QZSS L1S               |     J     |    1Z     | L1 1575.42   |
|      5 |     4 | QZSS L2 CM             |     J     |    2S     | L2 1227.60   |
|      5 |     5 | QZSS L2 CL             |     J     |    2L     | L2 1227.60   |
|      5 |     8 | QZSS L5 I              |     J     |    5I     | L5 1176.45   |
|      5 |     9 | QZSS L5 Q              |     J     |    5Q     | L5 1176.45   |
|      5 |    12 | QZSS L1C/B             |     J     |    1E     | L1 1575.42   |
|      6 |     0 | GLONASS L1 OF (C/A)    |     R     |    1C     | G1 1602+k*9/16 |
|      6 |     2 | GLONASS L2 OF (C/A)    |     R     |    2C     | G2 1246+k*7/16 |
|      7 |     0 | NavIC L5 A (SPS)       |     I     |    5A     | L5 1176.45   |

Notes:

- BeiDou D1 and D2 are different navigation message variants on the same
  physical signal (B1I or B3I). RINEX has no separate code for the data
  variant, so `sigId` 0 and 1 both map to `2I` and `sigId` 4 and 10 both map
  to `6I`. The converter must treat the two variants as the same RINEX signal
  and merge observations accordingly.
- BeiDou B1I sits at 1561.098 MHz but is assigned RINEX band 2, not band 1.
  RINEX band 1 on BeiDou is reserved for the B1C signals (1575.42 MHz).
- For SBAS, the RINEX satellite letter is always `S`; the `svId` to RINEX
  PRN conversion follows the standard rules and is independent of the signal
  mapping.
- GLONASS observations need the FDMA frequency channel `k` (range -7..+6) on
  the `SignalObservation`. UBX `RXM-RAWX` carries it as `freqId`, with
  `k = freqId - 7`.
- The RINEX `1Z` code is used for both the legacy QZSS L1-SAIF signal and the
  updated L1S signal (per the RINEX 4.02 QZSS table note).
- UBX-RXM-RAWX only supplies `sigId` from protocol version 27 onward. Earlier
  protocol versions emit the subset marked "no explicit sigId" in u-blox
  Table 4; for the X20 (protocol >= 50) the field is always present and the
  full mapping above applies.
- gnssId values 4 (IMES) and any others not listed have no UBX-RXM-RAWX
  signals defined in the X20 spec and need not be mapped.

## UBX-RXM-RAWX LLI derivation

The RINEX loss-of-lock indicator (LLI) byte that follows each phase
observation is not provided directly by UBX-RXM-RAWX; it is derived from
per-measurement flags and from per-(sat, sig) state carried across
epochs. The rules below follow rtklib-ex's UBX decoder
(`src/rcv/ublox.c`, function `decode_rxmrawx`).

### LLI bit 1: half-cycle ambiguity unresolved

Set when a phase value is present at this epoch and the receiver has not
resolved half-cycle ambiguity:

- Default: set when `trkStat.halfCycleValid == 0`.
- SBAS exception: the receiver does not expose `halfCycleValid` for SBAS,
  so the bit is set while `locktime <= 8000 ms` and cleared once
  `locktime > 8000 ms`.

### LLI bit 0: cycle slip

A slip is detected for this (sat, sig) whenever any of the following is
true:

1. `locktime == 0` (no continuous tracking).
2. `locktime` decreased since the previous epoch on this signal.
3. `trkStat.halfCycleSubtracted` flipped since the previous epoch.
4. `cpStdev` (the carrier-phase σ index, bits 3..0) is at or above the
   slip threshold. The default threshold is 15, which is u-blox's
   "max quantization noise / invalid" sentinel; the threshold should be
   configurable.

When a slip is detected, the converter remembers it. The slip bit is
stamped on the next epoch that actually reports a phase value for this
(sat, sig); the pending-slip flag is then cleared. This guarantees that
slips are never lost across gaps in phase reporting.

### Converter state

To compute these flags the converter must keep, per (sat, sig):

- The previous epoch's `locktime`.
- The previous epoch's `halfCycleSubtracted` bit.
- A pending-slip flag carried forward across phase-absent epochs.

State is established lazily on first observation of a (sat, sig).

## SSI

RINEX C/L/D/S records reserve a one-character SSI (signal-strength
indicator) digit immediately after each LLI byte. RTKLIB leaves this
column blank in its RINEX output, relying on the separate `S<band><attr>`
observation type to carry signal strength as dBHz. We follow RTKLIB's
convention: the converter does not populate `SSI` on `SignalObservation`.
Signal strength is carried in `CN0`, which the RINEX writer emits as the
`S<band><attr>` observation.
