# SBF RINEX observation conversion (#342)

A follow-up add-on to **`plan/septentrio-core.md`** (#340). It **depends
only on
`plan/sbfbin.md`** (the `MeasEpoch`/`MeasExtra` decode) and
`gps/lib/rinex`, so it is schedulable any time after `sbfbin` lands.

## Problem and scope

satpulse produces RINEX observation files from raw receiver
measurements via a per-vendor converter package under `gps/lib`: u-blox
has `gps/lib/rnxubx`, converting `ubxbin.RxmRawx` to
`gps/lib/rinex.SignalObservation`. Septentrio needs the analogous
package, `gps/lib/rnxsbf`, converting `sbfbin.MeasEpoch` (optionally
enriched by `sbfbin.MeasExtra`) to the same `rinex.SignalObservation`
record type.

Scope is RINEX **observation** files only -- pseudorange, carrier
phase, Doppler, and carrier-to-noise density per satellite signal per
epoch. It does not cover RINEX navigation files (broadcast ephemerides
are a separate, unrelated conversion, not addressed by this plan) and
it does not go through `gpsprot` at all: this is a direct
binary-block-to-RINEX-record path, independent of the
`gps/internal/septentrio` packet-to-`gpsprot.Msg` conversion
(`plan/septentrio-msg.md`). The two conversions read the same wire bytes
for different purposes and should not be confused with each other, the
same way `rnxubx` and `gps/internal/ubx` are independent consumers of
UBX messages.

The target receiver is the mosaic-G5. Its raw-measurement path is
`MeasEpoch` (4027) plus the optional `MeasExtra` (4000) companion
block; these are confirmed byte-identical to the mosaic-X5 for every
field this plan uses (see "Model differences" below). Septentrio also
has a compact `Meas3` block family (`Meas3Ranges`, `Meas3CN0HiRes`,
`Meas3Doppler`, `Meas3PP`, `Meas3MP`) on the X5 line, but it is absent
from the G5 firmware entirely (zero occurrences in the G5 reference
guide, and not a documentation lag: Meas3 predates the G5 product
line). There is no fallback or alternative source to consider --
`MeasEpoch` is the only raw-observable block this receiver emits, and
this plan treats it as such.

## Dependencies

`gps/lib/rnxsbf` depends only on `gps/lib/sbfbin` (phase 1: the
`MeasEpoch`/`MeasExtra` decode structs) and `gps/lib/rinex` (the output
record types). It does not depend on `gpsprot`, `gps/internal/septentrio`,
or any other phase of the Septentrio work, matching the layering rule
that `gps/lib/*` packages only import stdlib and sibling `lib/*`
packages.

## Design

### Wire constants and mappings come from `sbfbin`

Per the wire-format-ownership rule (`plan/sbfbin.md`), `rnxsbf` inlines
no magic wire values. The scale factors (`0.001` m, `0.0001` Hz, ...),
the Do-Not-Use sentinels (`0x80000000`, `-128`, `255`, `65535`/`65534`,
...), the extended-signal/GLONASS-`FreqNr` `ObsInfo` decode, and the
SVID-to-`rinex.SatelliteID` and signal-number-to-RINEX-code mappings
are all exported from `sbfbin` (the RINEX mappings mirroring
`ubxbin.RINEXSatNum`/`RINEXSig`; see `plan/sbfbin.md`). The CN0 formula
is shared with the gpsprot `SatellitesMsg` path, so it lives as a
`sbfbin` accessor rather than being duplicated in both consumers. The
tables and formulas spelled out below are for the reader; in code they
are `sbfbin` calls.

### Converter shape

Modeled directly on `rnxubx.Converter`:

```go
// Package rnxsbf converts Septentrio SBF raw observation messages to RINEX records.
package rnxsbf

type Converter struct {
    sink  rinex.Sink
    state map[signalKey]signalState
}

func New(sink rinex.Sink) *Converter { ... }

// ConvertBlock converts one SBF block, pairing each MeasEpoch with the
// MeasExtra block of the same epoch and ignoring blocks of other types.
func (c *Converter) ConvertBlock(b *sbfbin.Block) (bool, error)

// Flush converts a MeasEpoch held by ConvertBlock whose MeasExtra can no
// longer arrive, such as at the end of the input stream.
func (c *Converter) Flush() error

// ConvertMeasEpoch converts one SBF MeasEpoch block, and the MeasExtra
// block for the same epoch if available, to RINEX observations.
func (c *Converter) ConvertMeasEpoch(ts sbfbin.TimeStamp, m *sbfbin.MeasEpoch, extra *sbfbin.MeasExtra) error
```

`ConvertBlock` is the stream entry point, fed one block at a time in
wire order the same way `rnxrtcm.ConvertMsg` is fed individual RTCM
messages; it holds each `MeasEpoch` until the next measurement block
(or `Flush`) decides whether a `MeasExtra` pairs with it.
`ConvertMeasEpoch` is the specific entry for a pre-correlated pair,
parallel to `rnxrtcm.ConvertMSM7`. The block-header `TOW`/`WNc` lives
on `sbfbin.Block`, not on the `MeasEpoch` params struct, so
`ConvertMeasEpoch` takes the timestamp explicitly.

`extra` is optional (pass `nil` when `MeasExtra` output is not
enabled or has not arrived for this epoch); it contributes refining the
CN0 resolution from 0.25 dB-Hz to 0.03125 dB-Hz (see "CN0" below) and
the `CumLossCont` loss-of-continuity counter (see "Arc and
loss-of-lock" below). Unlike u-blox's RAWX, which is one flat
per-signal array, `MeasEpoch`'s per-satellite `MeasEpochChannelType1`
sub-block plus its nested `MeasEpochChannelType2` sub-blocks together
enumerate all signals for one satellite; `Converter` walks both levels
and emits one `rinex.SignalObservation` per signal, exactly as
`rnxubx` emits one per `RxmRawxMeas` entry. `MeasExtra`'s own
sub-blocks (`MeasExtraChannelSub`) key back to `MeasEpoch` entries by
`(RxChannel, Type)` -- see "Correlating MeasExtra" below -- so the
converter builds a lookup from `extra` before walking `m`.

Epoch time is `rinex.TimeFromGPSWeekMillis(int64(m.WNc), m.TOW)`.
Per the SBF specification, block-header `TOW`/`WNc` always uses the
GPS week convention regardless of which constellation a given
sub-block's satellite belongs to (Galileo week is `WNc-1024`, BeiDou
week is `WNc-1356`, but the wire field itself is never converted to
those scales) -- so no per-GNSS branching is needed here, unlike SBF
blocks that carry an explicit `TimeSystem` field for something else
(clock bias). If `TOW` or `WNc` is the Do-Not-Use sentinel
(`0xFFFFFFFF` / `0xFFFF`), the whole block is unusable and
`ConvertMeasEpoch` returns without emitting any observations (this can
happen briefly at receiver startup before time is set).

### Satellite identification: SVID to RINEX satellite ID

`MeasEpochChannelType1.SVID` folds constellation and satellite number
into one byte (unlike UBX's separate `gnssId`+`svId`), per the SBF
"Satellite ID" convention (section 4.1.9 in the reference guide). A
single lookup function maps it directly to a `rinex.SatelliteID`,
analogous to `ubxbin.RINEXSys`+`RINEXSatNum` combined into one step
since there is only one input field:

| SVID range | System | RINEX satellite number |
|---|---|---|
| 0 | (Do-Not-Use) | -- |
| 1-37 | GPS (`G`) | `= SVID` |
| 38-61 | GLONASS (`R`) | `= SVID-37` |
| 62 | GLONASS, unknown slot | not representable in RINEX; skip |
| 63-68 | GLONASS (`R`) | `= SVID-38` |
| 71-106 | Galileo (`E`) | `= SVID-70` |
| 107-119 | L-band (MSS) | not a GNSS satellite; skip |
| 120-140 | SBAS (`S`) | `= SVID-100` |
| 141-180 | BeiDou (`C`) | `= SVID-140` |
| 181-190 | QZSS (`J`) | `= SVID-180` |
| 191-197 | NavIC/IRNSS (`I`) | `= SVID-190` |
| 198-215 | SBAS (`S`) | `= SVID-157` |
| 216-222 | NavIC/IRNSS (`I`) | `= SVID-208` |
| 223-245 | BeiDou (`C`) | `= SVID-182` |
| 246-249 | (reserved gap) | not representable; skip |
| 250-251 | GPS (`G`) | `= SVID-212` |
| >= 252 | (undefined) | not representable; skip |

A `MeasEpochChannelType1` sub-block whose `SVID` falls in a skip range,
or matches no range at all (future SBF revisions may define new ranges
per the spec's forward-compatibility rule), is dropped entirely: its
Type2 children are not walked either, since they describe signals for
the same unrepresentable satellite. `rinex.SatelliteID` is formatted as
`fmt.Sprintf("%s%02d", sys, num)`, matching `rnxubx`'s construction.

### Signal identification: signal number to RINEX signal code

SBF's per-signal "signal number" (`Type` bits 0-4, `SigIdxLo`, extended
via `ObsInfo` bits 3-7 plus 32 when `SigIdxLo == 31`) already names a
specific RINEX observation code in the reference guide's own signal
table -- Septentrio's documentation publishes the (constellation,
signal number) -> RINEX-code mapping directly, so `rnxsbf` needs only
one flat table keyed by the raw signal number (0-39), not a
two-key GNSS+signal lookup the way `ubxbin.RINEXSig` needs for u-blox:

| # | RINEX code | # | RINEX code | # | RINEX code | # | RINEX code |
|---|---|---|---|---|---|---|---|
| 0 | 1C | 10 | 2P | 20 | 5Q | 30 | 6I |
| 1 | 1W | 11 | 2C | 21 | 7Q | 31 | (extension escape, not a signal) |
| 2 | 2W | 12 | 3Q | 22 | 8Q | 32 | 1L |
| 3 | 2L | 13 | 1P | 23 | (not a GNSS signal; skip) | 33 | 1Z |
| 4 | 5Q | 14 | 5P | 24 | 1C | 34 | 7D |
| 5 | 1L | 15 | 5A | 25 | 5I | 35-36 | (reserved; skip) |
| 6 | 1C | 16 | (reserved; skip) | 26 | 5Q | 37 | 1P |
| 7 | 2L | 17 | 1C | 27 | (blank in the guide; skip) | 38 | 1E |
| 8 | 1C | 18 | (reserved; skip) | 28 | 2I | 39 | 5P |
| 9 | 1P | 19 | 6C or 6B (see below) | 29 | 7I | | |

This table is confirmed byte-for-byte identical between the mosaic-X5
and mosaic-G5 reference guides for every signal number, including the
reserved slots and signal 27's blank RINEX-code cell (a gap in
Septentrio's own documentation, not an extraction error on satpulse's
part -- signal 27, QZSS L6, has no published RINEX code in either
guide; skip it rather than guessing). One entry needs a decode-time
choice: signal 19 (Galileo E6) is `6C` by default, or `6B` if
`MeasEpoch.CommonFlags` bit 6 ("E6B used") is set, meaning at least one
Galileo satellite in this epoch has its E6-C component encrypted. That
bit is block-wide, so it applies uniformly to every signal-19 entry in
one `MeasEpoch` block.

Because SBF's signal number already encodes the constellation
implicitly (a `MeasEpoch` sub-block's `Type` field only makes sense
combined with the sub-block's own `SVID`, and the guide's table is
written per-constellation), the RINEX-code lookup here does not need
the satellite's system as a second key the way UBX's `rinexSigMap` is
keyed by `(GNSSID, sigId)` -- one 40-entry array indexed by signal
number is sufficient and self-contained.

GLONASS carrier frequency (needed by `rinex.SignalValues.Frq`, the
FDMA channel number) is not a separate field on `MeasEpochChannelType1`/
`Type2`: for signal numbers 8-11 (the four GLONASS signals),
`ObsInfo` bits 3-7 carry the GLONASS `FreqNr` (1-14) instead of the
signal-number extension, and `Frq = FreqNr - 8` (channel range -7..+6,
matching the FDMA channel convention `rnxubx` uses for u-blox's
`freqId - 7`). For every other signal number, `ObsInfo` bits 3-7 either
extend the signal number (`SigIdxLo == 31`) or are reserved and
ignored.

### Extended signal number and GLONASS FreqNr decode

Both uses of `ObsInfo` bits 3-7 dispatch on the *same* sub-block's own
`Type` bits 0-4, independently for each Type1 master and each of its
Type2 children (a Type2 slave signal can carry a different `SigIdxLo`,
and therefore a different `ObsInfo` interpretation, than its Type1
parent):

```go
func resolveSignal(sigIdxLo byte, obsInfo byte) (num byte, freqNr byte) {
    switch {
    case sigIdxLo == 31:
        return 32 + (obsInfo >> 3), 0
    case sigIdxLo >= 8 && sigIdxLo <= 11:
        return sigIdxLo, obsInfo >> 3
    default:
        return sigIdxLo, 0
    }
}
```

`freqNr` is only meaningful (and only consulted) when the resolved
signal number is one of the GLONASS signals 8-11.

### Pseudorange, carrier phase, Doppler: scaling and Do-Not-Use

Type1 (master) fields, per signal:

- **Pseudorange**: `PR = (CodeMSB*4294967296 + CodeLSB) * 0.001` meters,
  where `CodeMSB` is `Misc` bits 0-3 (unsigned) and `CodeLSB` is the
  `CodeLSB` field. Invalid (do not populate `SignalValues.PR`) iff
  `CodeMSB == 0 AND CodeLSB == 0` -- a compound predicate, not a single
  sentinel value.
- **Doppler**: `Do = Doppler * 0.0001` Hz (positive = approaching).
  Invalid iff `Doppler == -2147483648` (`0x80000000`), a plain
  single-field sentinel.
- **Carrier phase**: `CP = PR/lambda + (CarrierMSB*65536 + CarrierLSB) * 0.001`
  cycles, where `lambda = 299792458 / f`, `f` is the carrier frequency
  of the sub-block's own resolved signal (needed only for this
  formula, not stored in the output record), `CarrierMSB` is `int8`,
  `CarrierLSB` is `uint16`. Invalid iff `CarrierMSB == -128 AND
  CarrierLSB == 0`. Carrier phase depends on a valid pseudorange (the
  formula divides by `PR`), so if `PR` itself is invalid, `CP` cannot
  be computed either and must also be left unset even if the raw
  carrier fields look individually valid.
- **CN0**: see "CN0" below.
- **LockTime**: `uint16`, 1 s units, clipped at 65534, DNU sentinel
  65535 (a different value from the clip ceiling -- do not conflate a
  clipped-but-valid 65534 s lock with "unavailable"). Used only to
  drive `Arc`/`HC` (loss-of-lock detection), not stored directly in
  `rinex.SignalValues`, matching `rnxubx`'s treatment of UBX
  `LockTime`.

Type2 (slave) fields are deltas relative to the Type1 master of the
same satellite, reconstructed to absolute values before emitting a
`rinex.SignalObservation` (RINEX has no concept of "delta from another
signal" -- each signal is its own independent record):

- **Pseudorange**: `PR = PR_type1 + (CodeOffsetMSB*65536 + CodeOffsetLSB) * 0.001`,
  where `CodeOffsetMSB` is bits 0-2 of `OffsetsMSB` (3-bit two's
  complement, sign-extend before use) and `CodeOffsetLSB` is `uint16`.
  Invalid iff `CodeOffsetMSB == -4 AND CodeOffsetLSB == 0`.
- **Doppler**: `Do = Do_type1*alpha + (DopplerOffsetMSB*65536 + DopplerOffsetLSB) * 0.0001`,
  where `DopplerOffsetMSB` is bits 3-7 of `OffsetsMSB` (5-bit two's
  complement) and `alpha` is the ratio of this Type2 signal's carrier
  frequency to the Type1 master's. Invalid iff `DopplerOffsetMSB ==
  -16 AND DopplerOffsetLSB == 0`.
- **Carrier phase**: `CP = PR_type2/lambda_type2 + (CarrierMSB*65536 + CarrierLSB) * 0.001`,
  where `lambda_type2` uses the Type2 sub-block's own signal, not the
  Type1 master's. Invalid iff `CarrierMSB == -128 AND CarrierLSB == 0`,
  same rule as Type1. As with Type1, this needs a valid `PR_type2`
  first (which itself needs a valid Type1 `PR`); if either predecessor
  is invalid, leave `CP` unset.
- **LockTime**: `uint8`, 1 s units, clipped at 254, DNU sentinel 255 --
  note the different width and different clip/DNU values from Type1's
  `uint16` field; do not share constants between the two.
- **CN0**: same formula as Type1, evaluated against the Type2
  sub-block's own resolved signal number.

`OffsetsMSB` packs two independently-signed bitfields in one byte:
bits 0-2 are `CodeOffsetMSB` (3-bit two's complement, range -4..3),
bits 3-7 are `DopplerOffsetMSB` (5-bit two's complement, range
-16..15). Both must be sign-extended from their native bit width, not
just masked, before use in either the reconstruction formulas or the
DNU-predicate comparisons.

### CN0

Both Type1 and Type2 share one formula, keyed on the sub-block's own
resolved signal number:

```
CN0 [dB-Hz] = raw*0.25          if signal number is 1 or 2 (GPS L1P, L2P)
CN0 [dB-Hz] = raw*0.25 + 10     otherwise
```

`raw == 255` is the Do-Not-Use sentinel for both Type1 and Type2's
`CN0` byte; leave `SignalValues.CN0` unset in that case rather than
applying the formula to 255.

### Correlating MeasExtra

When `extra` is non-nil, it refines CN0 resolution from 0.25 dB-Hz to
0.03125 dB-Hz using `MeasExtraChannelSub.Misc` bits 0-2
(`CN0HighRes`, only present at `MeasExtra` block revision 3 or above --
gate on `SBLength >= 16`, since earlier revisions don't have the
`Misc` byte at all):

```
CN0_refined [dB-Hz] = CN0_MeasEpoch [dB-Hz] + CN0HighRes * 0.03125
```

`MeasExtra` carries no `SVID`; each of its `MeasExtraChannelSub` entries
correlates to a `MeasEpoch` sub-block (Type1 or Type2) sharing the same
`RxChannel` and the same resolved signal number (via its own `Type`/
`Misc` fields, resolved the same way as `MeasEpoch`'s) within the same
epoch. Build this correlation as a map keyed by `(RxChannel, signal
number)` before walking `MeasEpoch`, since the two blocks are not
guaranteed to list their sub-blocks in the same order. If `extra` is
nil, or has no revision-3-or-above entry for a given signal, or is
simply absent from this epoch's output configuration, fall back to
`MeasEpoch`'s own CN0 with no refinement -- this is a pure enhancement,
never a requirement.

### Arc and loss-of-lock (LLI)

`rinex.SignalValues.Arc`/`HC`/`BT` express RINEX's loss-of-lock
indicator: `Arc` increments on every detected loss of continuous lock,
`HC` flags an unresolved half-cycle ambiguity, `BT` flags BOC tracking
(not applicable to SBF's signal set; always false here). Track
per-`(satellite, signal)` state across calls the same way `rnxubx`
does with its `signalKey`/`signalState` map, driven by:

- **LockTime reset**: a `LockTime` of 0 signals a fresh lock (the SBF
  guide states `LockTime` "resets to 0 at the initial lock after a
  signal (re)acquisition or any loss-of-lock"), so an incoming
  `LockTime == 0` (when the previous state had already seen this
  signal) marks a pending arc increment.
  Also mark pending if the new `LockTime` is smaller than the last
  observed value for this key (a decrease implies the counter reset
  and re-grew since the last epoch, which can happen if intervening
  epochs were missed or the counter briefly wrapped).
- **CumLossCont**: when a `MeasExtra` entry correlates with this
  sub-block (same `(RxChannel, signal number)` key as the CN0
  refinement), any change in its `CumLossCont` counter also marks a
  pending arc increment. The receiver increments this modulo-256
  counter at each initial lock after signal (re)acquisition or detected
  cycle slip, so it catches slips the `LockTime` comparison cannot see
  (e.g. a slip followed by an outage long enough for the lock time to
  re-clip at its ceiling). The lock-time rule remains the only signal
  when `MeasExtra` is absent. Note `LockTime` moves between encodings
  when the receiver re-selects a satellite's master signal (Type1 clips
  at 65534, Type2 at 254), so the decrease comparison clamps both sides
  to the smaller of the two ceilings involved.
- **Half-cycle ambiguity**: `ObsInfo` bit 2 for this sub-block, mapped
  straight to `HC` when a carrier phase is present.
- **Arc increments only when a carrier-phase value is actually
  present** this epoch (matching `rnxubx`'s `phase && st.pending`
  gate) -- a pending flag from a lock-time reset waits for the next
  epoch that actually reports a phase before bumping `Arc`, so a
  gap where only pseudorange is reported does not itself count as a
  cycle slip.

This state map is per-`Converter` instance, matching `rnxubx`'s design;
a `Converter` is not safe for concurrent use and is expected to process
one receiver's epochs in time order.

### Emitting observations

For each satellite (`MeasEpochChannelType1` sub-block) with a valid
`SVID` mapping:

1. Resolve the Type1 signal number and (if GLONASS) `FreqNr`; if the
   signal number lookup misses the RINEX-code table (a reserved or
   not-a-GNSS-signal slot), skip this sub-block's observation but
   still walk its Type2 children (they can name different, valid
   signals).
2. Build a `rinex.SignalObservation` with `T`, `Sat`, `Sig`, and
   whichever of `PR`/`CP`/`Do`/`CN0`/`Frq` are valid per the rules
   above, plus `Arc`/`HC` from the per-key state.
3. If none of `PR`/`CP`/`Do`/`CN0` ended up set, drop the record
   entirely (mirrors `rnxubx`'s "nothing to report" check) rather than
   emitting an empty observation.
4. Repeat steps 1-3 for each nested Type2 sub-block, reconstructing
   absolute values relative to the Type1 master as described above.

`ConvertMeasEpoch` calls `c.sink.Observation(obs)` for each emitted
record and returns the first error encountered, matching
`rnxubx.ConvertRAWX`'s control flow exactly.

## Model differences (mosaic-G5 vs mosaic-X5)

Everything this converter reads is confirmed identical between the two
reference guides:

- `MeasEpoch` (4027) and `MeasExtra` (4000): field layout, bit
  positions, scales, and Do-Not-Use sentinels are byte-for-byte
  identical, including the `CommonFlags`/`Type`/`ObsInfo` sub-block
  shapes.
- The signal-number-to-RINEX-code table (section 4.1.10 of the
  reference guide) is byte-for-byte identical, including the reserved
  slots and signal 27's blank cell; only a cosmetic difference exists
  in how the two guides name the GLONASS FDMA offset variable
  (`FN` vs `FreqNr-8`), not the formula itself.
- The satellite-numbering table (section 4.1.9) is identical in
  content; the X5 guide's markdown rendering is simply messier to
  extract from (garbled OCR around the two SBAS ranges and a couple of
  missing rows), so the table above is sourced from the cleaner G5
  rendering, but describes the same underlying SBF specification on
  both models.
- The one genuine model difference in this area is that the X5 line
  additionally has the `Meas3*` compact block family, which the G5
  does not implement at all. Since this plan only targets the G5 and
  only uses `MeasEpoch`/`MeasExtra`, that difference has no effect on
  `rnxsbf`'s design; it is recorded here only so a future reader
  extending this package to the X5 knows `Meas3` is out of scope for
  now, by omission rather than by oversight.

No other model-dependent behavior applies to this package: `rnxsbf`
never touches the ASCII configuration interface, PPS, or any of the
blocks the two firmware lines diverge on (PVT variants, network
status, L-band beams, etc.).

## Testing

Follow `rnxubx`'s test shape: same-package tests (`package rnxsbf`),
a small `testSink` capturing emitted `rinex.SignalObservation` values,
and hand-built `sbfbin.MeasEpoch`/`MeasExtra` literals (not full wire
bytes -- these are Go struct literals, since `sbfbin` already owns wire
decode/round-trip testing) exercising:

- SVID-to-satellite mapping across each range boundary in the table
  (including the DNU `0`, unknown-GLONASS-slot `62`, and L-band
  `107-119` skip cases).
- Signal-number-to-RINEX-code mapping, including the `SigIdxLo == 31`
  extension path and the GLONASS `FreqNr` overlay path.
- Pseudorange/carrier-phase/Doppler compound Do-Not-Use predicates,
  both Type1 and Type2, confirming a clipped-but-valid `LockTime`
  (65534/254) is treated differently from the DNU sentinel
  (65535/255).
- Type2 delta reconstruction against a known Type1 master, including
  sign-extension of the packed `OffsetsMSB` byte.
- CN0 formula's signal-1/2 special case, and the `MeasExtra`
  high-resolution refinement (present and absent, and gated on
  `MeasExtra` revision via `SBLength`).
- `Arc`/`HC` transitions across a `LockTime` reset and across an
  epoch where only a pseudorange (no phase) is reported.
- The Galileo E6 `CommonFlags` bit 6 dispatch between `6C` and `6B`.

Once example SBF captures with real `MeasEpoch`/`MeasExtra` traffic are
available (see `CLAUDE.local.md`), cross-check emitted observations
against the receiver's own SBF-to-RINEX conversion (if the Septentrio
tools used to build the reference captures include one) as an
end-to-end sanity check, though this is not required to land the
package -- the guide's formulas above are the authoritative spec.

## Open decisions

- **SVID range 246-249**: the reference guide's satellite-numbering
  table has no entry between BeiDou's 223-245 and GPS's 250-251; treat
  246-249 as an undefined gap (skip, per the general "ignore ranges
  this document doesn't define" decoding rule) rather than guessing an
  extension. Revisit if a future guide revision fills it in.
- **Type2 GLONASS `FreqNr` overlay**: the reference guide states the
  `ObsInfo`-bits-3-7 GLONASS `FreqNr` rule explicitly only for Type1;
  Type2's own field description documents only the `SigIdxLo == 31`
  extension case and is silent on the GLONASS case. This plan applies
  the same rule to Type2 for consistency (GLONASS FDMA slave signals
  are rare but not impossible), but this is an inference, not a
  directly documented guarantee -- confirm against a real capture with
  a GLONASS Type2 slave signal once hardware is available.
- **Whether to expose `MeasEpoch.CommonFlags` bit 7 ("Scrambling")** as
  a diagnostic: when set, every measurement in the block is silently
  degraded with no per-field Do-Not-Use marker to signal it. This
  converter has no natural place to surface a block-wide warning
  (`rinex.Sink` has no side channel for it); decide whether that
  belongs in `rnxsbf` at all, or purely in the `gps/internal/septentrio`
  diagnostic path (phase 3), before implementing.
