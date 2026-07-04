# Rework Septentrio SatellitesMsg construction (#NNN)

Replace the SatellitesMsg construction shipped in `plan/septentrio-msg.md`
section 9 with a simpler, more correct two-block combine. The original
design (three-way `ChannelStatus` + `MeasEpoch` + `SatVisibility` combine,
with a coarse family <-> signal-number join table) has correctness and
model-fit problems that only became clear once real multi-frequency
captures and the RINEX signal mapping were available. This plan supersedes
section 9; it does not touch the rest of `septentrio-msg.md`.

Issue heading above is a placeholder -- file (or point this at) a real
issue before this plan lands.

## 1. Background: why redo

The shipped combine (see `gps/internal/septentrio/sbfsat.go`) makes
`ChannelStatus` the structural base at *family* granularity, overlays
`MeasEpoch` for CN0 and a precise `SignalID` via a hand-maintained
`(GNSS, family) <-> MeasEpoch signal number` join table (`chanFamilies`),
and overlays `SatVisibility` for look angles and extra SVs. Four things
are wrong with it:

- **`SatVisibility` contributes nothing observable.** Its finer 0.01-deg
  look angles cannot be represented -- `gpsprot.LookAngles` is whole-degree
  (`Azimuth int16`, `Elevation int8`), and `visibilityLookAngles` already
  rounds them off. Its only other contribution is orbit-visible-but-
  untracked SVs, which is exactly the set UBX deliberately prunes
  (`ubxsats.go` `satellitesPrune` drops every SV with no signals). So it
  adds a third block for zero net effect, and the current code is actually
  inconsistent with UBX by emitting those empty-signal SVs.

- **Signals are built in the wrong namespace.** The base is family-level
  (`ChannelStatus` bit-slots), and `MeasEpoch`'s fine per-signal identity
  is force-fit onto it through the join table. But `MeasEpoch` already
  carries the fine signal identity and is the *only* source of CN0. The
  natural model is: signals come from `MeasEpoch` in the fine space, full
  stop.

- **Type2 measurements are dropped.** `overlayMeasEpoch` walks only
  `MeasEpoch.Type1`. For a multi-frequency satellite the non-reference
  signals (L2/L5/E5b/...) live in the nested `Type2` slaves, so their CN0
  and identity are lost -- flagged in review on PR #346.

- **The family/band join granularity is wrong for "used".** See section 5.

CN0 matters: `MeasEpoch` is the *only* SBF block carrying CN0 (no
lightweight NAV-SIG analogue exists), and the web dashboard's Signal
Levels card is driven entirely by CN0 (`web/svg.tsx` `simplifySignals`
filters `cn0 > 0`). So `MeasEpoch` is not an optional overlay; it is a
first-class source, and a `MeasEpoch`-only message (CN0 bar chart, no
used/angles) is legitimate.

## 2. What each block provides

| Block | Per epoch | Gives | Missing |
|---|---|---|---|
| `ChannelStatus` (4013) | 1 | SV set (tracked), per-family tracking/used/health, whole-degree look angles | no CN0, no fine signal identity |
| `MeasEpoch` (4027) | 1 | fine per-signal identity + CN0 (Type1 masters + Type2 slaves) | no used, no look angles |

They are near-perfect complements. The redo keeps them as the only two
sources and joins on SVID.

## 3. Combine shape

Two-way, SVID-keyed. No `SatVisibility`, no family join table.

- **Signals + CN0**: from `MeasEpoch` exclusively, in the fine `SignalID`
  space. Walk `Type1` and each `Type1`'s `Type2` slaves. One `SignalInfo`
  per measured signal: `{ID, CN0, Used}`. SVID from `Type1.SVID` (Type2
  inherits it); Type2 CN0 is absolute (same encoding as Type1), its signal
  number from its own `Type`/`ObsInfo`.
- **Look angles**: from `ChannelStatus` per SVID (whole-degree). This is
  the only `ChannelStatus` field with no signal axis, so it joins by SVID
  alone.
- **Used**: from `ChannelStatus.PVTStatus`, joined to `MeasEpoch` signals
  on RINEX codes -- section 5.
- **Prune** SVs with no signals before emit, as UBX does.

`SVInfo.Used` is true if the satellite has any used signal.
`UsedValidity` is `SatelliteUsedSignal` when `ChannelStatus` contributed,
else `SatelliteUsedInvalid` (a `MeasEpoch`-only message has no used
concept). `NativeMsgID` is `"ChannelStatus"` when it contributed, else
`"MeasEpoch"` (mirrors the existing convention; one string can't say
"both").

## 4. Signal identity via the RINEX pivot

Stop maintaining a bespoke SBF-signal-number -> `SignalID` table
(`sbfSignalTable.id` / `measEpochSignalID`). Instead pivot through RINEX,
which `sbfbin/rinex.go` already provides:

- `sbfbin.RINEXSig(n, commonFlags) (sys, code)` -- SBF signal number to
  RINEX system letter + 2-char obscode, already resolving the Galileo
  E6-B/E6-C flip from `CommonFlags`.
- New in gpsprot: `SignalIDFromRINEX(sys, code) SignalID`, backed by a
  `(sys, code) -> SignalID` table. This is the first step toward making
  gpsprot signal identity RINEX-centric (as SVIDs already are).

Then a `MeasEpoch` signal's `SignalID` is
`gpsprot.SignalIDFromRINEX(sbfbin.RINEXSig(n, flags))`, and the
`sbfSignalTable.id` column and the E6 special-case in `measEpochSignalID`
are deleted. Note the RINEX mapping is *more* complete than the current
`SignalID` set (e.g. E5 AltBOC is `8Q` in RINEX but has no `SignalID`
yet -- section 7).

## 5. Used-join on RINEX codes (not band)

`PVTStatus` is one 16-bit word per satellite: eight 2-bit fields, one per
per-constellation signal slot (index 0..7). Values: `0` not used, `1`
waiting for ephemeris, `2` used, `3` rejected. Only `2` counts as used.
Max eight signals per constellation (QZSS uses all eight).

The used flag must be attached to each fine `MeasEpoch` signal. The
tempting coarse key -- **band** -- is wrong: `gpsprot.BandL1` merges
1561.098 MHz (BDS B1I) and 1575.42 MHz (BDS B1C) into one band, so marking
"L1 used" wrongly marks B1C used when only B1I is. Verified real: capture
SV147 measures B1C (CN0 47.2 dB-Hz) but `ChannelStatus` uses B1I, not B1C.

**Join on RINEX codes.** Both sides speak RINEX:

- Each *used* `ChannelStatus` slot maps to the RINEX code(s) of its family
  -- a new per-constellation `(GNSS, slot) -> RINEX code(s)` table in
  `sbfbin/rinex.go`, the `ChannelStatus` analogue of `RINEXSig`. For each
  SVID, the union of the used slots' codes is that satellite's set of used
  RINEX codes.
- A `MeasEpoch` signal's RINEX code comes from `RINEXSig`.
- `SignalInfo.Used = true` iff the signal's RINEX code is in the SVID's
  used-code set.

This is exact at the signal level and dissolves every gap:

| Case | RINEX | Result |
|---|---|---|
| BDS B1I vs B1C | `2I` vs `1P` | distinct -- the band-join bug is gone |
| QZSS L1CB | `1E` | joins directly; no fold to L1CA needed |
| Galileo E5 AltBOC | `8Q` | joins directly; no expand to {E5a,E5b} |
| GPS/GLO P(Y) | `1W`/`2W` | join directly, and stay never-used via PVTStatus |

## 6. Flush: two slices

Epoch keys are `sbfbin.TimeStamp` (`{TOW, WNc}`, directly comparable), not
separate TOW/WNc fields. A combined `sec1` capture (all nine blocks, 30
epochs) shows a fixed per-epoch order:

```
MeasEpoch -> MeasExtra -> EndOfMeas -> PVTGeodetic -> DOP -> EndOfPVT
          -> ChannelStatus -> SatVisibility
```

So within an epoch `MeasEpoch` arrives first and `ChannelStatus` closes
it; `EndOfMeas` and `EndOfPVT` are *not* usable triggers, both precede
`ChannelStatus`. The flush is built in two slices.

### 6.1 Slice 1: emit on ChannelStatus, reuse the last MeasEpoch

- On `MeasEpoch`: store it (overwrite) and *retain* it. It is never
  cleared on emit -- it stays available as "the last MeasEpoch".
- On `ChannelStatus`: emit `combine(chan, lastMeas)` immediately, then
  drop the `ChannelStatus`. The retained `MeasEpoch` is reused by the next
  `ChannelStatus` if no fresher one has arrived.
- No `SatVisibility`: its Dispatch case and stored field are removed.

This emits one message per `ChannelStatus` (its 1 Hz OnChange rate). When
`MeasEpoch` and `ChannelStatus` share a TOW (aligned `sec1`), the message
carries fresh CN0. The simplification is that intermediate `MeasEpoch`
epochs -- when `MeasEpoch` outpaces `ChannelStatus` -- are overwritten and
lost; slice 2 recovers them.

### 6.2 Slice 2: MeasEpoch-only fallback for the faster stream

Track the `TimeStamp` of the last emitted `SatellitesMsg`. On `MeasEpoch`
arrival whose `TimeStamp` differs from the retained one, if the retained
`MeasEpoch` was never emitted (its `TimeStamp` != the last-emitted key),
emit it as a `MeasEpoch`-only message before overwriting. `ChannelStatus`
emits also set the last-emitted key, so a `MeasEpoch` already closed by a
`ChannelStatus` never double-emits.

The result: when `ChannelStatus` keeps pace, every `MeasEpoch` is closed by
a `ChannelStatus` and no standalone message is emitted; when `MeasEpoch`
outpaces `ChannelStatus`, the un-closed epochs flush as `MeasEpoch`-only
messages (CN0 bars, no used/angles -- `NativeMsgID` `"MeasEpoch"`,
`UsedValidity` `SatelliteUsedInvalid`, per section 3).

## 7. Gaps and open decisions

- **Galileo E5 AltBOC** (`MeasEpoch` #22, RINEX `8Q`): no `gpsprot.SignalID`
  and no coarse `Signal`. The RINEX used-join handles it, but the emitted
  `SignalInfo.ID` is empty until a `SignalID` is added. Decide: add one, or
  emit these signals with no ID.
- **QZSS L1CB** (`1E`): keep as a distinct fine `SignalID`
  (`SigIDQZSSL1CB` exists); it has no coarse `Signal`, which is fine --
  coarse membership is not used in the observation path.
- **Satellite health**: `ChannelStatus.HealthStatus` is coarse
  (per-family), not in the current `gpsprot` model. Out of scope here;
  see `plan/satellite-health.md`.
- **Whole-degree look angles only**: unchanged model limitation; dropping
  `SatVisibility` loses no representable precision.

## 8. Web dashboard change (separate, `web/`)

`simplifySignals` drops SVs with `cn0 == 0`. For UBX that means "not
tracked"; for a Septentrio `ChannelStatus`-only stream (no `MeasEpoch`) it
means "level unavailable", so the filter blanks satellites we *are*
tracking. The dashboard must stop using `cn0 == 0` as the drop criterion
so `ChannelStatus`-only streams still render. Tracked separately from this
Go plan.

## 9. Code changes

Delete:
- `overlaySatVisibility`, the `vis`/`SatVisibility` arm of
  `satellitesCombine`, the `SatVisibility` Dispatch case and `satVis`
  field, and the old `satBoundary`/`flushSats` TOW-rollover machinery.
- `chanFamilies` (the family <-> signal-number join table) and the join
  map built in `addChannelStatus`.
- `sbfSignalTable.id` and `measEpochSignalID` (E6 handling moves into
  `RINEXSig`).

Add:
- `gpsprot.SignalIDFromRINEX(sys, code)`.
- `sbfbin` `ChannelStatus (GNSS, slot) -> RINEX code(s)` table + accessor,
  next to `RINEXSig`.
- Type2 iteration in the `MeasEpoch` signal walk.
- RINEX-keyed used computation; empty-signal prune.
- Slice 1 flush: `ChannelStatus`-triggered emit reusing the retained
  `MeasEpoch`; epoch keys as `sbfbin.TimeStamp`.
- Slice 2 flush: last-emitted `TimeStamp` tracking; `MeasEpoch`-only
  emission when `MeasEpoch` outpaces `ChannelStatus`.

Change:
- `satellitesCombine` to the two-way SVID-keyed form (`chan`, `meas`).

## 10. Testing

Seed unit tests from the combined `sec1` capture (all nine block types,
30 epochs; copy into `gps/testdata/packets/septentrio/mosaic-G5/`), which
the current corpus lacks -- its `daemon`/`meas` captures never contain
both `ChannelStatus` and `MeasEpoch`. Cover: Type2 CN0 present on
multi-frequency SVs; B1I-used / B1C-unused distinguished (SV147);
empty-signal SVs pruned; slice-1 flush on `ChannelStatus` arrival reusing
the retained `MeasEpoch`; slice-2 `MeasEpoch`-only emission (CN0-only
message) when `MeasEpoch` outpaces `ChannelStatus`.
