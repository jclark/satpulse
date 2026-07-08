# SBF block to gpsprot Msg mapping (#340)

Core sub-plan of #340 (`plan/septentrio-core.md`); no separate issue.

This is a sub-plan of **`plan/septentrio-core.md`** (a checkbox in that
issue, not a separate issue): the conversion layer that turns decoded
SBF blocks into device-independent `gpsprot` Msgs in
`gps/internal/septentrio`. **It depends on `plan/sbfbin.md`** -- the SBF
wire-format layer (the `PacketFormat` scanner, block structs,
`ParseMsg`, CRC). Target hardware is the **mosaic-G5**; mosaic-X5
differences are called out where they change this layer's design
(inline below).

## 1. Scope

Build the real `gps/internal/septentrio.PacketProcessor`: the type that
implements `gpsprot.PacketProcessor` + `gpsprot.EpochFlusher`, parses
each packet via `sbfbin.ParseMsg`, converts recognized blocks into
`gpsprot.Msg` values, accumulates per-epoch state, and falls back to
`NativeMsgHandler.NativeMsg` for every block this phase does not map.
In scope:

- Epoch keying and flush mechanics (`NavEpochManager` integration).
- Time family: `TimeMsg` (from `ReceiverTime`, `PVTGeodetic`/
  `PVTCartesian`, `xPPSOffset`) and `LeapSecondMsg` (from `GPSUtc`/
  `GALUtc`, and by the same pattern `BDSUtc`).
- Position/velocity family: `PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`,
  `VelECEFMsg` (from `PVTGeodetic`/`PVTCartesian`).
- `NavEpochMsg`: solution-quality accumulation from up to six
  cooperating PVT-family blocks (`PVTGeodetic`/`PVTCartesian`, `DOP`,
  `PosCovCartesian`/`PosCovGeodetic`, `VelCovCartesian`/
  `VelCovGeodetic`). The per-satellite blocks do not feed it
  (section 4).
- `SatellitesMsg`: the three-way combine of `ChannelStatus`,
  `MeasEpoch`, and `SatVisibility`, and the SVID/`SignalID` mapping
  tables both draw on.
- `SurveyMsg` (from `PVTGeodetic`/`PVTCartesian.Mode` bits) and
  `CorReportMsg` (from `DiffCorrIn`/`BaseStation`).
- Registration in `gps/gpsreg/reg.go`.

Out of scope: RINEX raw-observation conversion (`gps/lib/rnxsbf`,
`plan/sbf-rinex.md`), ASCII config/response handling
(`plan/septentrio-msgfile.md`), and high-level `ConfigProtocol`
(`plan/septentrio-config.md`). The
`ExtEvent` family is not decoded at all (see `plan/sbfbin.md`'s
excluded-blocks list), so nothing external-event-related reaches this
layer.

## 2. Pipeline and package shape

```
raw bytes --scan(PacketFormat)--> gpsprot.Packet
         --ProcessPacket--> sbfbin.ParseMsg --Dispatch--> gpsprot Msg(s)
```

`gps/internal/septentrio` follows the shape of `gps/internal/ubx` and
`gps/internal/nov`:

- `const Tag gpsprot.Tag = "SBF"`, re-exported from `gps/gpsreg/reg.go`
  as `TagSBF = septentrio.Tag`.
- `NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor`,
  embedding `gpsprot.DefaultPacketProcessor`, holding the `MsgHandler`,
  the shared `*NavEpochManager`, and per-epoch accumulator state.
- `ProcessPacket(data string, tRead time.Time) (string, error)` calls
  `sbfbin.ParseMsg`, then `Dispatch`.
- `Dispatch(m sbfbin.Msg, tRead time.Time) bool`, a `switch` on
  concrete block type, one `case` per mapped block, falling through to
  `NativeMsgHandler.NativeMsg` for anything unmapped (mirrors
  `ubx.go`'s `Dispatch`). `Tag`/`Priority` are stamped centrally in
  `Dispatch` after each per-block conversion function returns, not
  inside the conversion function -- keeps per-block helpers
  protocol-detail-only.
- Per-block, protocol-detail conversion functions (named like
  `posGeoPVTGeodetic`, `velECEFPVTCartesian`, `timeReceiverTime`,
  `satellitesChannelStatus`), returning `nil` on DNU/no-fix so
  `Dispatch` skips that message for the epoch.
- `FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority,
  gpsprot.MsgHandler)` implementing `gpsprot.EpochFlusher`, always
  returning `gpsprot.PriVendorLow` (SBF is a vendor binary protocol;
  no block set analyzed here justifies `PriVendorHigh` for any field).

## 3. General wire conventions carried into every converter

- **No magic wire constants in this package -- `sbfbin` owns them
  all.** Every raw wire value a converter tests against (enum codes
  like `Mode`/`TimeSystem`/`WACorrInfo`, DNU sentinels, bitfield
  masks/shifts, signal numbers, SVID ranges, block numbers) is an
  exported named constant (or method) in `gps/lib/sbfbin`
  (`plan/sbfbin.md` makes this a rule on that side too). This package
  references those constants only; it never inlines a literal like
  `-2e10`, `0xFFFF`, or a `Mode` code. The literal values quoted
  throughout this document are for the reader's benefit -- in code they
  appear as `sbfbin.` constants. If a needed value isn't exported yet,
  export it from `sbfbin` rather than hard-coding it here.
- **DNU sentinels are per-field, not per-type**: `f4`/`f8` position,
  velocity, and covariance fields use `-2e10` (exactly representable
  in both `float32` and `float64`, safe for `==` comparison); `u4 TOW`
  uses `0xFFFFFFFF`; `u2 WNc` uses `0xFFFF`; most other `u1`/`u2`
  fields use `255`/`65535` (sometimes clipped one below, e.g.
  `HAccuracy`/`VAccuracy` clip at `65534` = 655.34 m); `i1` fields
  (UTC components, `DeltaLS`) use `-128`; a few whole-degree fields
  use `511`/`-128` (`ChannelStatus` azimuth/elevation). Do not infer a
  sentinel from wire type alone -- gate each field on its own
  documented DNU value.
- **`PVTGeodetic`/`PVTCartesian.Mode & 0xF == 0`** ("No GNSS PVT
  available") is itself meaningful data, not a reason to skip
  `NavEpochMsg` updates, but it does mean every other field on that
  block (position, velocity, accuracy, `RxClkBias`, ...) is at its DNU
  sentinel. Gate `PosGeoMsg`/`PosECEFMsg`/`VelGeoMsg`/`VelECEFMsg`
  construction on both `Mode&0xF==0` and the field-level DNU check
  (belt and braces): they are expected to co-occur, but the guide
  documents one exception (`Latency` is not reliably gated by `Error`
  the way other post-`Error` fields are), so do not rely on `Mode`
  alone for fields this document does check explicitly.
- **`PVTCartesian` and `PVTGeodetic` are the same navigation solution
  in two frames**, sharing `Mode`/`Error`/`RxClkBias`/`RxClkDrift`/
  `TimeSystem`/`Datum`/`NrSV`/`WACorrInfo`/`ReferenceID`/
  `MeanCorrAge`/`SignalInfo`/`AlertFlag`/`NrBases`/`PPPInfo`/
  `Latency`/`HAccuracy`/`VAccuracy`/`Misc`/`COG` byte-for-byte. If a
  receiver is configured to output both in the same epoch, extract the
  shared quality fields (`Mode`/`Error`/`WACorrInfo`/etc., feeding
  `NavEpochMsg`) exactly once per epoch, and build `PosECEFMsg`/
  `VelECEFMsg` only from `PVTCartesian` and `PosGeoMsg`/`VelGeoMsg`
  only from `PVTGeodetic` -- no cross-block fallback or priority
  arbitration is needed at this layer (the generic
  `gpsprot.PVMsgBundle.FillDerived` already covers the case where only
  one frame is configured, via `geopos.WGS84` LLH<->ECEF/NED
  conversions, as a safety net that should not be the primary path).
- **gpsprot has no field for a non-WGS84 `Datum`.** SBF's `Datum`
  field (u1, DNU `255`; `0`=WGS84/ITRS, `19`="same as DGNSS/RTK base",
  `30`=ETRS89, `31`-`33`=NAD83 variants, `34`/`35`=GDA94/GDA2020,
  `36`=JGD2011, `250`/`251`=user-defined) is read but has nowhere to
  go on `PosGeoMsg`/`PosECEFMsg`. Populate position unconditionally
  from `Latitude`/`Longitude`/`Height` (or `X`/`Y`/`Z`) regardless of
  `Datum`, matching the implicit WGS84 assumption every other
  currently-supported protocol already makes. This is a pre-existing
  `gpsprot` gap, not specific to Septentrio; a fix belongs in
  `gpsprot`, not this package.

## 4. Epoch model

### 4.1 Epoch key and what an epoch carries

Every SBF block header carries `(TOW, WNc)` on the GPS convention
(section 5.1), so a navigation epoch is identified by that key. The
`PacketProcessor` holds one accumulating `*gpsprot.NavEpochMsg`
(`p.curEpochMsg`) plus the current epoch key; the first block whose
`(TOW, WNc)` differs from the current key begins a new epoch.

`NavEpochMsg` is built **only from the PVT-family blocks**
(`PVTGeodetic`/`PVTCartesian`, `DOP`, and the covariance blocks
`PosCov*`/`VelCov*` -- section 8). Nothing from the per-satellite
blocks (`ChannelStatus`, `SatVisibility`, `MeasEpoch`) feeds it; those
feed the independent `SatellitesMsg` stream (section 9), which is not
part of the epoch (section 4.3).

### 4.2 Flush triggers and coexistence with NMEA

Two distinct signals bound a Septentrio epoch, and they mean different
things:

- **A TOW increase is a whole-receiver epoch advance.** SBF
  synchronous (receiver-timestamped) blocks carry monotonically
  non-decreasing timestamps (guide 4.1.3), and the receiver emits an
  epoch's output as a unit before starting the next. So the first
  block whose `(TOW, WNc)` exceeds the current key marks the true end
  of the previous epoch for the *whole receiver*, including any NMEA
  the same receiver emits. On that transition the processor calls
  `mgr.EpochStarted(p, tRead)` -- the existing hard, all-active flush
  (`gpsprot.NavEpochManager.EpochStarted`), unchanged. It drives the
  merged `NavEpochMsg` emission and is correct whether SBF runs alone
  or alongside NMEA.

- **`EndOfPVT` (5921) marks the end of the PVT *family* for the same
  TOW**, not the end of the epoch (guide: "marks the end of
  transmission of all PVT related blocks belonging to the same
  epoch"). It is a mid-epoch marker: the receiver is still on the
  current TOW and will keep emitting that epoch's `EndOfMeas`,
  `EndOfAtt`, `ChannelStatus`, `SatVisibility`, and NMEA sentences. So
  `EndOfPVT` must **not** trigger the all-active flush -- doing so
  would finalize and reset a concurrently-active NMEA processor's
  still-in-progress epoch before its trailing sentences for the same
  instant arrive. The mosaic-G5 guide configures NMEA and SBF as
  independent output streams (`setNMEAOutput`/`setSBFOutput`) and
  documents no ordering guarantee between them, so this coexistence is
  a real runtime case, not a theoretical one: all processors share one
  `NavEpochManager` (created in `gpsreg.CreatePacketProcessors`) and
  are live at once, routed by packet format.

`EndOfPVT` therefore calls a **new** manager method,
`EndOfProtocolEpoch(f, tRead)`, meaning "processor `f`'s own epoch has
ended, but `f` cannot assert the epoch is over for the whole
receiver." The flush-now-or-defer decision keys on **whether another
protocol took part in the last epoch**, not on whether one happens to
be active at this instant. NMEA output does not flicker on and off, so
last epoch's participation is a reliable predictor of this epoch's --
whereas "is NMEA active right now" is not: its sentences for the
current epoch may not have arrived when `EndOfPVT` fires (no NMEA/SBF
ordering guarantee), so a naive is-anyone-else-active check would flush
an NMEA-bearing epoch prematurely.

- If `f` is alone now **and** no other processor took part in the last
  epoch, flush now -- the SBF-only case. This is **required, not
  optional**: a no-NMEA receiver must emit its `NavEpochMsg` at
  `EndOfPVT`, not one epoch late.
- Otherwise (another processor is active now, or one was present last
  epoch and is expected again), do nothing: `f`'s completed accumulator
  stays in the active set, swept up and merged when the next
  whole-receiver boundary arrives (the next `EpochStarted`/`EndOfEpoch`
  -- a TOW increase, or the concurrently-active NMEA processor's own
  boundary). SBF's next-TOW `EpochStarted` flushes and resets its
  accumulator before accumulating the new epoch, so the staged value is
  never clobbered.

The manager records the set of processors that participated in each
flushed epoch (`m.lastEpoch`, the keys of `m.active` captured by
`flush` before it clears them) so `EndOfProtocolEpoch` can consult it:

```go
// EndOfProtocolEpoch is called by a processor whose own epoch has ended
// but which cannot assert the epoch is over for the whole receiver
// (e.g. Septentrio's EndOfPVT). It flushes now only when f is the sole
// active processor AND no other processor took part in the last epoch;
// otherwise it defers to the next whole-receiver boundary, so a
// concurrently-active protocol (e.g. NMEA whose sentences trail
// EndOfPVT) is not flushed mid-epoch. NMEA presence is stable across
// epochs, so last epoch's participation predicts this epoch's.
func (m *NavEpochManager) EndOfProtocolEpoch(f EpochFlusher, tRead time.Time) {
	for g := range m.active {
		if g != f {
			return // another protocol mid-epoch
		}
	}
	for g := range m.lastEpoch {
		if g != f {
			return // NMEA present last epoch; expect it again, wait for the merge
		}
	}
	m.flush(tRead)
}
```

Cold start: for the very first epoch `m.lastEpoch` is empty, so an
SBF-only receiver flushes promptly; if NMEA is in fact present but its
first-epoch sentences trail `EndOfPVT`, only that one startup epoch
fails to merge -- steady state is correct once NMEA has appeared in a
single epoch.

This is a required `gpsprot.msg.go` change accompanying SBF
integration (it may land as its own small commit ahead of this
package). It adds the `EndOfProtocolEpoch` method and the `lastEpoch`
participant-set tracking in `flush`, and leaves `EpochStarted`/
`EndOfEpoch` semantics untouched, so UBX/Quectel/NMEA-only and SBF-only
receivers behave exactly as today; only the SBF+NMEA coexistence path
changes.

### 4.3 Resolved: SatellitesMsg is not part of the epoch

This was the central open cross-protocol question; it is now decided.
`SatellitesMsg` is a separate stream: it dispatches on its own
boundary detection (section 9), is not consulted when building
`NavEpochMsg`, and is not guaranteed to arrive before the epoch's
`NavEpochMsg`. The observation that `ChannelStatus`/`SatVisibility`
land after `EndOfPVT` in the sample captures is therefore not a
problem to design around -- those blocks simply feed the independent
`SatellitesMsg` path. This matches the existing NMEA path, where
satellite information is likewise not part of the epoch. The
consequences for `NavEpochMsg`'s satellite-derived fields
(`NumSVTracked`/`NumSVInView` have no PVT-family source and are left
unset; `GNSSUsed`/`BandsUsed` come from the PVT `SignalInfo` bitmask)
are in section 8.4.

## 5. Time handling

### 5.1 TOW/WNc scale: always GPS convention

**Every SBF block's `TOW`/`WNc` header pair is on the GPS time
convention, regardless of which constellation the block otherwise
describes** (guide sec 4.1.3: `TOW` is "whole milliseconds from the
beginning of the current GPS week", `WNc` is the continuous GPS week
number, `WNc`=0/`TOW`=0 is 1980-01-06). `PVTGeodetic`/`PVTCartesian`'s
`TimeSystem` field says only what `RxClkBias` is expressed relative
to; it does not change the timescale of the header timestamp itself.

So for the PVT-family blocks, the correct construction is:

```go
now := ptime.GPS(int16(m.WNc), time.Duration(m.TOW)*time.Millisecond)
taiTime := now.Add(-time.Duration(m.RxClkBias * float64(time.Millisecond)))
```

for **every** valid `TimeSystem` (`0` GPS, `1` Galileo, `4` BeiDou,
`5` QZSS) -- do not additionally shift the week number by 1024
(Galileo) or 1356 (BeiDou) before calling `ptime.GPS`. Shifting the
week and then calling `ptime.Galileo`/`ptime.BeiDou` looks plausible
(it is numerically a no-op for Galileo, since
`ptime.TAIMinusGalileo == ptime.TAIMinusGPS`) but for BeiDou it adds a
spurious 14 seconds (`ptime.TAIMinusBeiDou` is 33 vs `TAIMinusGPS`'s
19; the header is GPS-labelled, so applying BeiDou's larger TAI
offset on top double-counts it). A whole-second error is catastrophic
for a timing daemon, so re-confirm this against guide 4.1.3's exact
wording when writing this path, but the design is: **one constructor,
`ptime.GPS(WNc, TOW)`, for every `TimeSystem`; `TimeSystem` only picks
the `GNSS` label and confirms `RxClkBias`'s frame.**

`TimeSystem` -> `GNSS`: `0`->GPS, `1`->GAL, `4`->BDS, `5`->QZSS. Skip
`TimeMsg`/`GNSS` construction entirely (do not emit a message with a
guessed `GNSS`) for `TimeSystem` `2` (unassigned), `3` (GLONASS -- no
documented GNSS-time mapping for this scale), `100` (FugroAtomiChron),
or `255` (DNU).

This "always GPS convention" rule is for the **PVT-family blocks**,
which carry `TimeSystem` to disambiguate. `ReceiverTime` and
`xPPSOffset` carry no `TimeSystem` field at all, and their `TOW`/`WNc`
is the *receiver's own steered clock*, which follows whatever
`setTimingSystem`/`sts` the receiver is configured for. The default
(and satpulse's expected configuration) is GPS, in which case
`ptime.GPS(WNc, TOW)` is correct for these two blocks as well; if a
deployment ever configures `sts` to BeiDou or GLONASS, `ReceiverTime`/
`xPPSOffset`'s header would be offset from GPS by a whole number of
seconds with no field-level way to detect or correct it from either
block alone. This is a real, narrow edge case (default-config
deployments are unaffected) -- flagged in section 10, not resolved
here.

### 5.2 Three independent TimeMsg sources per epoch

Three SBF blocks can each produce a candidate `TimeMsg` in the same
epoch, and they are complementary, not redundant:

| Block | `TimeRef` | Gives | Missing |
|---|---|---|---|
| **ReceiverTime** (5914) | `NavSolution` | Native `UTCTime` + `UTCOffset` (from `DeltaLS`) and a whole-millisecond `TAITime`, available as soon as `TOWSET`/`WNSET` (seconds after power-up) | No sub-ms precision (free-running clock can sit up to +-0.5 ms off true GNSS time between integer-ms jumps) |
| **PVTGeodetic**/**PVTCartesian** (4007/4006) | `NavSolution` | `RxClkBias`-corrected `TAITime` (sub-ms, often sub-us when steered) + `GNSS` | No UTC/leap-second fields; only available after a full position/time fix (`FINETIME`), later than `ReceiverTime` |
| **xPPSOffset** (5911) | `PostPulse` | The *only* source of `PulseOffset` | No `UTCTime`/`UTCOffset`; only present on epochs where a PPS pulse actually occurred (PPS rate can be slower than the nav-epoch rate) |

Each block's `TimeMsg` is dispatched independently as its packet is
decoded -- no ordering constraint, no competition, no buffering.
`xPPSOffset` is `Ref = PostPulse` (guide: "output right after each xPPS
pulse") and is the only carrier of `PulseOffset`. `ReceiverTime` and
the PVT family are `NavSolution` (the time-of-day). These serve
different consumers and do not contend: the dispatcher hands every
`TimeMsg` both to `time/internal/timemsg`'s buffer -- which keeps the
pre-/post-pulse corrections by `PulseOffset` -- and, separately, to
`gpsprot.TimeTicker`, which latches one time-of-day per epoch.
`TimeTicker` plays no part in `PulseOffset` delivery, so there is no
"which TimeMsg wins" question and nothing special to do here beyond
setting each block's `Ref` and fields correctly (section 5.3).

### 5.3 Field-by-field construction

**`ReceiverTime` (5914)**: `TOW`/`WNc` (u4/u2, DNU
`0xFFFFFFFF`/`0xFFFF`) -> `TAITime = ptime.GPS(int16(WNc), TOW ms)`.
`UTCYear`/`Month`/`Day`/`Hour`/`Min`/`Sec` (each i1, DNU `-128`,
shared sentinel across all six) -> `UTCTime =
ptime.UTC(2000+UTCYear, UTCMonth, UTCDay, UTCHour, UTCMin, UTCSec, 0)`
when not DNU (whole-second resolution, `nanos=0`). `DeltaLS` (i1, DNU
`-128`, "positive if GPS time is ahead of UTC") -> `UTCOffset =
uint8(int(DeltaLS) + ptime.TAIMinusGPS)` (`TAIMinusGPS == 19`; real
captures: `DeltaLS==18` -> `UTCOffset==37`, the correct current
TAI-UTC offset). `GNSS` left zero (block is not tied to one
constellation). `Ref = NavSolution`. `NativeMsgID = "ReceiverTime"`.

**`PVTGeodetic`/`PVTCartesian` (4007/4006)**: per section 5.1,
`TAITime = ptime.GPS(int16(WNc), TOW ms).Add(-RxClkBias ms)`, gated on
`TOW != 0xFFFFFFFF && WNc != 0xFFFF && RxClkBias != -2e10 &&
TimeSystem in {0,1,4,5}`. `GNSS` from `TimeSystem` (section 5.1).
`UTCTime`/`UTCOffset` left unset (back-filled later by
`TimeTicker.fill` from the stored leap-second table). `Ref =
NavSolution`. `NativeMsgID` = `"PVTCartesian"` or `"PVTGeodetic"`,
whichever produced the winning message.

**`xPPSOffset` (5911)**: `TOW`/`WNc` -> `TAITime = ptime.GPS(int16(WNc),
TOW ms)` (marks the pulse-edge instant, before applying the
correction). `Offset` (f4, already nanoseconds, no scale factor) ->
`PulseOffset = opt.Make(-float64(Offset))` -- **sign flip required**:
the guide defines `Offset` as negative when the pulse is early, but
`TimeMsg.PulseOffset`'s contract is `trueTime = pulseTime +
PulseOffset`, so an early pulse needs a positive `PulseOffset`. This
sign flip has not been verified against real hardware (no
oscilloscope/NTP-SHM ground truth yet); it is the single
highest-priority item to re-check once hardware and a PPS reference
are available. `Timescale` (a distinct enum from `TimeSystem`: `1`
GPS, `2` UTC, `3` Receiver, `4` GLO, `5` GAL, `6` BDS, `100` Fugro) ->
`GNSS`: `1`->GPS, `4`->GLO, `5`->GAL, `6`->BDS; leave zero for `2`/`3`/
`100`. Note `xPPSOffset` can attribute a `TimeMsg` to GLONASS (via
`Timescale==4`) even though the PVT family cannot (`TimeSystem==3` has
no documented week-epoch mapping) -- `xPPSOffset`'s `TAITime` never
depends on `Timescale`, only the header convention (always GPS,
section 5.1). `Ref = PostPulse`. `NativeMsgID = "xPPSOffset"`.

`Accuracy time.Duration` has **no SBF source anywhere** (no field
analogous to u-blox's `NAV-TIMEGPS.tAcc`); leave it zero for every
Septentrio-derived `TimeMsg`. `ReadDelay` is set by the
`PacketProcessor` framework, not per block:
`tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curEpochStart))`,
matching every other protocol's `ReadDelay` assignment.

### 5.4 Sync-state gating (`SyncLevel`, `FINETIME`)

`ReceiverTime.SyncLevel` (mirrored in `ReceiverStatus.RxState` bits
4-6) is a 3-bit `WNSET`/`TOWSET`/`FINETIME` readiness state;
`xPPSOffset.SyncAge` is seconds since the pulse was last
resynchronized. Neither has a `TimeMsg` field to populate --
`gpsprot.TimeMsg` has no "trust/sync state" concept today (a
pre-existing, cross-protocol gap, not specific to Septentrio). Prefer
the per-field DNU checks in section 5.3 (unambiguous) over gating on
`SyncLevel`; treat `SyncLevel`/`SyncAge` as log-only diagnostics for
now.

Cold-start sequence (guide sec 2.3, authoritative): at power-up,
`TOW`/`WNc` are DNU everywhere. Once any tracked SV's time-of-week
decodes, `TOWSET` (and shortly after, `WNSET`) set and `TAITime`
becomes constructible from `ReceiverTime`'s own header. `FINETIME`
(and a populated, non-DNU `RxClkBias`) only follows a full
position/time fix -- so the PVT-family `TAITime` source becomes
available strictly later than `ReceiverTime`'s. UTC-parameter
availability (`GPSUtc`/`GALUtc`/`BDSUtc` decode, feeding both
`ReceiverTime.UTCTime` and `LeapSecondMsg`) is a logically independent,
typically much slower condition (up to ~12.5 minutes for one SV's GPS
page).

## 6. LeapSecondMsg

Source: `GPSUtc` (5894) or `GALUtc` (4031) -- either alone is
sufficient and both are equally authoritative (same real-world leap
second, `LeapSecondMsg.UpdateLeapSecond`'s greatest-`OffChangeTime`
merge naturally converges). `BDSUtc` (4121) follows the identical
pattern with `dnBase=0` (BeiDou's `DN` is 0-based, Sunday=0, unlike
GPS/Galileo's 1-based `DN`); not detailed further here.

```go
g := ptime.GNSSLeapSecond{WNLSF: b.WN_LSF, DN: b.DN, DeltaLS: b.DEL_t_LS, DeltaLSF: b.DEL_t_LSF}
now := ptime.GPS(int16(b.WNc), time.Duration(b.TOW)*time.Millisecond) // GPS convention even inside GALUtc, section 5.1
ls, err := ptime.GPSLeapSecond(g, now) // GALUtc: ptime.GalileoLeapSecond(g, now)
if err != nil {
	return nil // suppress: no valid/unambiguous leap-second date found this instance
}
msg := &gpsprot.LeapSecondMsg{LeapSecond: ls, GNSS: gpsprot.GPS} // or gpsprot.GAL
```

`GPSUtc`/`GALUtc` are emitted once per contributing SV whose relevant
page/word the receiver decodes, so a tracked constellation can produce
many `LeapSecondMsg` dispatches describing the same event; no dedup is
needed beyond `UpdateLeapSecond`'s existing merge. Both blocks
re-broadcast the leap-second-event fields (`DEL_t_LS`/`WN_LSF`/`DN`/
`DEL_t_LSF`) without their own documented DNU sentinel -- the only
failure mode is `ptime.GPSLeapSecond`/`GalileoLeapSecond` returning an
error (zero or multiple candidate dates), which must suppress dispatch
for that instance, not emit a partial message.

**`ReceiverTime.DeltaLS` must never be used to construct or update a
`LeapSecondMsg`.** It carries only the current offset with no
boundary date, and inventing one would either be a harmless no-op or
would durably corrupt `UpdateLeapSecond`'s chronological-priority merge
once a genuine `GPSUtc`/`GALUtc` value arrives (a "yesterday" placeholder
would permanently outrank the real 2016-12-31-equivalent boundary once
it arrives). `DeltaLS`'s correct home is `TimeMsg.UTCOffset` directly
(section 5.3) -- a separate mechanism that, in practice, closes the
startup gap before the first `LeapSecondMsg` exists, since
`ReceiverTime` is available well before a full UTC page decode.

## 7. Position and velocity family

All four Msgs come from `PVTGeodetic` (4007, geodetic pair) or
`PVTCartesian` (4006, ECEF pair) alone -- no cross-block
reconciliation is needed beyond the shared gating in section 3.
`EndOfPVT` plays no data role for any of them (epoch-flush concerns
only, section 4).

### 7.1 PosGeoMsg (from PVTGeodetic)

| Field | Source | Computation |
|---|---|---|
| `LatLon [2]Angle` | `Latitude`/`Longitude` (f8, rad, DNU `-2e10`) | `DegreesFromFloat(rad * 180/pi)`, no sign flip (SBF is already lat positive-N, lon positive-E) |
| `Height opt.Val[Length]` | `Height` (f8, m, DNU `-2e10`) | `Meters(Height)`, independently DNU-gated |
| `HeightMSL opt.Val[Length]` | `Height`, `Undulation` (f4, m, DNU `-2e10`) | `Meters(Height - Undulation)`; gate on **both** fields' own DNU independently -- `Undulation` can be DNU (geoid model not yet evaluated, or disabled) while `Height` is valid |
| `Priority`/`Tag`/`NativeMsgID` | -- | `PriVendorLow`; `Tag` (dispatch); `"PVTGeodetic"` |

Suppress the whole message when `Mode&0xF==0` or `Latitude`/
`Longitude`/`Height` == `-2e10` (check all three defensively even
though they share one cause).

### 7.2 PosECEFMsg (from PVTCartesian)

`Pos = Point3D{Meters(X), Meters(Y), Meters(Z)}` (f8, m, DNU `-2e10`
each), same axis convention as `gpsprot`, no transform. Suppress on
`Mode&0xF==0` or any of `X`/`Y`/`Z == -2e10`. `NativeMsgID =
"PVTCartesian"`.

### 7.3 VelGeoMsg (from PVTGeodetic)

| Field | Source | Computation |
|---|---|---|
| `VelNED opt.Val[[3]Speed]` | `Vn`/`Ve`/`Vu` (f4, m/s, DNU `-2e10`) | `{MetersPerSecondFromFloat(Vn), ...(Ve), ...(-Vu)}` -- **Down = -Up**, the single most important sign detail here; `Vn`/`Ve` need no flip |
| `GroundSpeed opt.Val[Speed]` | `Vn`, `Ve` | `MetersPerSecondFromFloat(math.Hypot(Vn, Ve))` -- SBF has no native combined ground-speed field, and `gpsprot.PVMsgBundle.FillDerived` does not synthesize one either (it only derives `Speed3D`). This is a one-line derivation this package must add (see section 10 for whether it belongs here or in `FillDerived` generically). |
| `Speed3D` | -- | Leave unset; `FillDerived` already computes `sqrt(Vn^2+Ve^2+Vd^2)` from `VelNED` generically -- do not duplicate |
| `Course opt.Val[Angle]` | `COG` (f4, deg, DNU `-2e10`, overloaded: also DNU when speed < 0.1 m/s, indistinguishable from "not computed") | `DegreesFromFloat(COG)`, no rad->deg step (already degrees) |

Suppress the whole message on `Mode&0xF==0` or `Vn`/`Ve`/`Vu ==
-2e10`. `Course`'s DNU is independent -- a valid velocity fix can
still have unset `Course` near zero speed; do not gate `VelNED`/
`GroundSpeed` on it. `PVTCartesian` also carries its own `COG`
(numerically identical); only `PVTGeodetic`'s is wired to
`VelGeoMsg.Course`. `NativeMsgID = "PVTGeodetic"`.

### 7.4 VelECEFMsg (from PVTCartesian)

`Vel = [3]Speed{MetersPerSecondFromFloat(Vx), ...(Vy), ...(Vz)}` (f4,
m/s, DNU `-2e10` each, same ECEF axes as `PosECEFMsg.Pos`, no sign
flip). Suppress on `Mode&0xF==0` or any of `Vx`/`Vy`/`Vz == -2e10`.
`NativeMsgID = "PVTCartesian"`.

### 7.5 What is deliberately not on these Msgs

`HAccuracy`/`VAccuracy` (u2, 0.01 m, DNU `65535`, clip `65534`) --
2DRMS horizontal / 2-sigma vertical -- populate `NavEpochMsg.Acc.Hor`/
`Acc.Vert` (section 8), not any position/velocity Msg field, since
`gpsprot` deliberately factors solution-quality out of the position
messages. `Error` (meaningful only when `Mode&0xF==0`) has no home on
any Msg in this codebase; log via `NativeMsgHandler` only if wanted.

## 8. NavEpochMsg

`NavEpochMsg` is an accumulator held per-epoch by the
`PacketProcessor` (`p.curEpochMsg`, section 4), updated as a side
effect by whichever block-specific conversion function fires each
epoch, and dispatched once at flush via `FlushNavEpoch`. No single SBF
block supplies it end to end -- expected, and matching every other
protocol's multi-message quality picture, just spread across more
blocks (up to six) with cleaner per-facet ownership than UBX's
single `NAV-PVT`.

### 8.1 FixLevel / SolutionDim (from PVTGeodetic/PVTCartesian.Mode)

```go
switch mode & 0xF {
case 0:
	ne.FixLevel = gpsprot.FixLevelNone
case 1, 2, 6: // standalone, SBAS-DGNSS, SBAS-aided
	ne.FixLevel = gpsprot.FixLevelCode
case 3:
	ne.FixLevel = gpsprot.FixLevelNotMeasured // fixed/manual location
case 4, 7: // RTK fixed, moving-base RTK fixed
	ne.FixLevel = gpsprot.FixLevelCarrierFixed
case 5, 8: // RTK float, moving-base RTK float
	ne.FixLevel = gpsprot.FixLevelCarrierFloat
case 10: // PPP
	ne.FixLevel = gpsprot.FixLevelCarrierFloat // conservative; see note
default: // 9, 11, 12: reserved
	ne.FixLevel = gpsprot.FixLevelNone
}
if ne.FixLevel >= gpsprot.FixLevelCode {
	ne.SolutionDim = gpsprot.SolutionDim3D
	if mode&0x80 != 0 { // bit 7: 2D mode
		ne.SolutionDim = gpsprot.SolutionDim2D
	}
}
```

Mode `10` (PPP) is the one place this loses real fidelity: SBF cannot
distinguish converging vs converged PPP the way e.g. NovAtel's
`PosType` can (`PPPInfo` bits 13-15 describe seed provenance, not
convergence), so default to the conservative `FixLevelCarrierFloat`
rather than guessing `FixLevelCarrierFixed`. Moving-base RTK (7/8)
collapses onto the same `FixLevel` as ordinary RTK (4/5) --
`gpsprot` has no "moving baseline" concept, a deliberate, not
accidental, loss. `SolutionDimTimeOnly` has **no SBF source** at all
(no `Mode` value means "time-only fix"); a receiver solving for clock
bias alone without position is indistinguishable from `Mode==0`.

### 8.2 Correction (from WACorrInfo / Mode)

```go
switch waCorrInfo >> 5 & 0x3 {
case 1, 2: // physical base, virtual base (VRS)
	ne.Correction |= gpsprot.CorrOSR.Expand()
case 3: // SSR (RTK-SSR / PPP-RTK)
	ne.Correction |= gpsprot.CorrSSR.Expand()
}
switch mode & 0xF {
case 6:
	ne.Correction |= gpsprot.CorrSBAS.Expand()
case 10:
	ne.Correction |= gpsprot.CorrPPP.Expand()
}
```

`WACorrInfo == 0` is a legitimate "no differential info used" state
(all bits clear), not a sentinel meaning "field unavailable" -- decode
it as no `Correction` bits set. `CorrRTCM` and the PPP-provenance bits
(`CorrPPPHAS`/`CorrPPPMDC`/`CorrPPPB2b`) have **no direct per-epoch SBF
signal**. Leave the PPP-provenance leaf bits unset; `CorrPPP` alone is
the most specific claim the wire data supports.

### 8.3 AuxSrc, DOP, Acc

`AuxSrc`: **no SBF source at all** for this receiver family -- neither
guide mentions dead-reckoning/INS anywhere; leave zero.

`DOP.Pos`/`Hor`/`Vert`/`Time` from the `DOP` block (4001)
`PDOP`/`HDOP`/`VDOP`/`TDOP` (u2, x0.01, **`0` means "not available"**,
not the usual 65535/-2e10 style):

```go
func dop01(v uint16) opt.Val[float64] {
	if v == 0 {
		return opt.Val[float64]{}
	}
	return opt.Make(float64(v) * 0.01)
}
```

`DOP.Geom`/`North`/`East` have **no SBF source** (SBF's `DOP` block
reports 4 of UBX `NAV-DOP`'s 7 figures); leave unset, do not
synthesize GDOP from PDOP/TDOP alone.

`Acc.Hor`/`Acc.Vert`: primary source `PVTGeodetic`/`PVTCartesian`'s
`HAccuracy`/`VAccuracy` (u2, 0.01 m, DNU `65535`); secondary fallback
`PosCovGeodetic` (5906) `Cov_latlat+Cov_lonlon` / `Cov_hgthgt` (f4,
m^2, DNU `-2e10`) via RSS-of-variances, only when the primary is DNU
(use `.Fill`, not `.Set`, so the direct reading always wins when both
are present). Note the **confidence-level mismatch**: `HAccuracy` is
documented 2DRMS (95%), `VAccuracy` 2-sigma, while the
`PosCovGeodetic` fallback is 1-sigma RSS of variances -- not the same
statistical basis. This is a pre-existing `gpsprot.Accuracy` ambiguity
(no documented required confidence level), not something to silently
normalize away; prefer the primary source to minimize how often the
mismatch is exposed.

`Acc.Pos` (3D): only source is `PosCovCartesian` (5905)
`Cov_xx+yy+zz`, RSS of variances, gated on none of the three being
`-2e10` and on 2D mode being clear (in 2D mode `PosCovCartesian`
voids all its `Cov_*` fields together, unlike `PosCovGeodetic`'s
partial voiding). `Acc.Speed`: only source `VelCovCartesian` (5907)
`Cov_VxVx+VyVy+VzVz`, same RSS pattern, same 2D all-or-nothing rule.
`Acc.GroundSpeed`: only source `VelCovGeodetic` (5908)
`Cov_VnVn+VeVe`. `Acc.Course`: **no SBF source found in any block
analyzed**.

`Acc.Pos`/`Acc.Speed`/`Acc.GroundSpeed` are each gated behind a
separately-enabled Cov block (`PosCovCartesian`/`VelCovCartesian`/
`VelCovGeodetic`) -- a minimal configuration that only enables
`PVTGeodetic` gets `Acc.Hor`/`Acc.Vert` for free but never these
three; this is a configuration choice, not a decode gap.

### 8.4 DiffAge, RTCMRefBaseID, NumSV*, GNSSUsed/BandsUsed

`DiffAge`: `PVTGeodetic`/`PVTCartesian.MeanCorrAge` (u2, 0.01 s, DNU
`65535`) -> `Duration(MeanCorrAge) * 10ms`.

`RTCMRefBaseID`: primary `BaseStation` (5949) `BaseStationID` (u2, no
documented DNU) whenever a `BaseStation` block has been observed for
the current base (retain the last-seen value across epochs --
`BaseStation` is event-driven, not epoch-keyed); conditional fallback
`PVTGeodetic`/`PVTCartesian.ReferenceID` (u2, DNU `65535`, `65534`=
"multiple bases") only when `WACorrInfo` indicates a physical/virtual
base (bits 5-6 = 1 or 2, not 3=SSR) and `ReferenceID` is neither
sentinel -- note `ReferenceID` means an SBAS PRN instead when
`Mode&0xF==6`, must not be read as an RTCM base ID in that case.

`NumSVUsed`: primary `PVTGeodetic`/`PVTCartesian.NrSV` (u1, DNU
`255`); fallback `DOP.NrSV` (u1, "0 = not available") only when the
primary is DNU.

`NumSVTracked` and `NumSVInView`: **left unset.** Their only sources
are the per-satellite blocks (`ChannelStatus`'s tracked-channel count,
`SatVisibility.N`), which are not part of the epoch (section 4.3) and
so do not feed `NavEpochMsg`. The tracked/in-view picture still
reaches observers via the independent `SatellitesMsg` (section 9); it
is simply not duplicated onto the epoch summary -- a deliberate
consequence of the section 4.3 decision.

`GNSSUsed`/`BandsUsed`: derived from `PVTGeodetic`/
`PVTCartesian.SignalInfo` (u4 bitmask, bit *i* = signal number *i*
used) mapped through the section 9 signal table. `SignalInfo` is a
PVT-family field, so this needs no per-satellite block (section 4.3).
It is lossy -- the bitmask only covers signal numbers 0-31, while the
guide documents numbers up to 39 (QZSS L1C/L1S/L1CB/L5S, BeiDou B2b,
NavIC L1 are unrepresentable) -- but the fuller set is carried by the
independent `SatellitesMsg`'s own `GNSSUsed()`/`BandsUsed()`
(section 9).

`Tag`: set centrally in `FlushNavEpoch`, not per block.

## 9. SatellitesMsg

No single SBF block is sufficient (unlike the position/velocity Msgs'
single-block case); this is a **three-way combine** across
`ChannelStatus` (4013), `MeasEpoch` (4027), and `SatVisibility` (4012),
generalizing UBX's two-way `NAV-SAT`+`NAV-SIG` `satellitesCombine`.

### 9.1 What each block contributes

| Block | Granularity | Gives | Missing |
|---|---|---|---|
| `ChannelStatus` | Per-channel, per-signal-**family** | Only source of `Used` (`PVTStatus`); whole-degree look angles | No `CN0` at all |
| `MeasEpoch` | Per-channel, per-signal-**number** (finer) | Only source of `CN0`; finest per-signal identity | No `Used`, no look angles |
| `SatVisibility` | Per-satellite, orbit-data only (no channel needed) | Broadest SV set; 0.01-deg-precision look angles | No signal/CN0/`Used` data at all |

Combine order: **`ChannelStatus` is the structural base** (one
`SVInfo` per `ChannelSatInfo`, one `SignalInfo` per family slot where
`TrackingStatus==3` (Tracking), `Used` from `PVTStatus==2`,
`UsedValidity = SatelliteUsedSignal`). **Overlay `MeasEpoch`** onto
matching `(GNSS, family)` keys (section 9.3's join table) for `CN0`
and, where available, a more precise `SignalID`; never touch `Used`
from this source. **Overlay `SatVisibility`** for higher-precision
look angles (prefer its 0.01-deg values over `ChannelStatus`'s
whole-degree ones when both are present and valid), and append
orbit-visible-but-unallocated SVs (present in `SatVisibility`, absent
from the tracking set) with empty `Signals`, `Used: false`.

If `ChannelStatus` did not contribute this epoch at all,
`UsedValidity` downgrades to `SatelliteUsedInvalid` for the whole
message (neither `MeasEpoch` nor `SatVisibility` carries any `Used`
concept). `NativeMsgID` is `"ChannelStatus"` when it contributed,
else `"MeasEpoch"`, else `"SatVisibility"` -- a single string cannot
represent "combination of 3 blocks"; this mirrors `ubxsats.go`'s
equivalent choice.

**Flush timing**: `SatellitesMsg` is a separate stream, not part of
the epoch (section 4.3), so its combine flushes on its own boundary --
a TOW change among the contributing blocks
(`ChannelStatus`/`MeasEpoch`/`SatVisibility`) -- and dispatches
directly via `h.Satellites(...)`, independent of the `NavEpochMsg`
flush. It does **not** go through `FlushNavEpoch` (unlike `ubx.go`,
which folds the combine into the epoch flush), and it is not tied to
`EndOfPVT`. Since the per-satellite blocks land after `EndOfPVT`, the
combine typically flushes when the next epoch's first
satellite-block TOW arrives.

### 9.2 SVID mapping

All three blocks share one SVID numbering table (guide sec 4.1.9),
implemented as one shared helper, `sbfSVID(svid uint16, freqNr byte)
(gpsprot.SVID, bool)`:

| SVID range | GNSS | `Num` |
|---|---|---|
| 1-37 | GPS | `= SVID` |
| 38-61 | GLONASS | `= SVID-37` |
| 62 | GLONASS | `GLOUnknown` (0) |
| 63-68 | GLONASS | `= SVID-38` |
| 71-106 | Galileo | `= SVID-70` |
| 107-119 | (L-band beam, not a GNSS SV) | skip |
| 120-140 | SBAS | `= SVID-100` |
| 141-180 | BeiDou | `= SVID-140` |
| 181-190 | QZSS | `= SVID-180` |
| 191-197 | NavIC | `= SVID-190` |
| 198-215 | SBAS | `= SVID-157` |
| 216-222 | NavIC | `= SVID-208` |
| 223-245 | BeiDou | `= SVID-182` |
| 250-251 (MeasEpoch only) | GPS | `= SVID-212` (G38/G39) |
| 0 | escape: use `SVIDFull` instead (`ChannelStatus`; `SatVisibility` only at guide-documented revision 1, G5 doc only, unconfirmed on real hardware) |

`FreqNr` (GLONASS FDMA channel number, offset +8) never feeds
`SVID.Num` directly; it only disambiguates `62` (unknown slot) from
`38-61`/`63-68` (known slot). GPS SVID 33-37 and `MeasEpoch`'s 250-251
exceed `gpsprot.GNSS.IsValidSVNum(GPS)`'s `<=32` ceiling -- emit the
`SVInfo` anyway (the wire format documents these as valid PRNs;
silently dropping SVs the receiver reports would be real data loss),
letting `IsValidSVNum` flag it downstream if a caller cares (open item,
section 10). Multiple simultaneously-tracked GLONASS satellites with
unknown slots collide onto the same `{GLO, GLOUnknown}` key and merge
together -- an existing `gpsprot.SVID` model limitation, not
introduced here.

### 9.3 Signal mapping

SBF has **two** signal namespaces that must not be conflated:

- **Observed axis** (guide sec 4.1.10, "Signal Type"): `MeasEpoch`'s
  per-signal-number identity. For most combo signals (Galileo E1,
  E5a, E5b; BeiDou B1C, B2a; GPS L2C, L5, L1C; GLONASS L3), the
  receiver only ever measures **one** physical component (documented
  explicitly per-signal in guide sec 2.2.1) and reports it under one
  signal number -- there is no separate SBF number for the
  unmeasured component. Galileo E6 (signal 19) is the one dynamic
  exception: the component (E6-B vs E6-C) depends on
  `MeasEpoch.CommonFlags` bit 6 ("E6B used").
- **Enablement/tracking axis**: `ChannelStatus`'s per-constellation
  `HealthStatus`/`TrackingStatus`/`PVTStatus` bit-slot labels, spelled
  identically to the `setSignalTracking`/`getReceiverCapabilities`
  command-line names (`L1CA`, `P1(Y)`, `L2C`, ... for GPS; `E1BC`,
  `E6BC`, `E5a`, `E5b`, `E5ab` for Galileo; etc.) -- this is a
  **coarser, family-level** namespace than the observed axis, not the
  same one at a different granularity.

A decoder building `SatellitesMsg` from `ChannelStatus` works in the
coarse (family) namespace; from `MeasEpoch`, the fine
(signal-number) namespace. Do not conflate them when combining (section
9.1) -- use the join table below to match a `ChannelStatus` bit-slot
to its `MeasEpoch` signal number for the same `(GNSS, family)`.

**Observed-axis master table** (signal number, guide `SignalType`
label, constellation, component actually tracked per sec 2.2.1, RINEX
obscode from the guide verbatim, best-fit `gpsprot.SignalID`):

| # | Label | GNSS | Component | RINEX | `gpsprot.SignalID` |
|---|---|---|---|---|---|
| 0 | L1CA | GPS | -- | 1C | `SigIDGPSL1CA` |
| 1 | L1P | GPS | -- | 1W | `SigIDGPSL1PY` |
| 2 | L2P | GPS | -- | 2W | `SigIDGPSL2P` |
| 3 | L2C | GPS | L2C-L (pilot) | 2L | `SigIDGPSL2CL` |
| 4 | L5 | GPS | L5-Q (pilot) | 5Q | `SigIDGPSL5Q` |
| 5 | L1C | GPS | L1C-P (pilot) | 1L | `SigIDGPSL1CP` |
| 6 | L1CA | QZSS | -- | 1C | `SigIDQZSSL1CA` |
| 7 | L2C | QZSS | L2C-L (pilot) | 2L | `SigIDQZSSL2CL` |
| 8 | L1CA | GLONASS | -- | 1C | `SigIDGLOL1` |
| 9 | L1P | GLONASS | -- | 1P | `SigIDGLOL1P` |
| 10 | L2P | GLONASS | -- | 2P | `SigIDGLOL2P` |
| 11 | L2CA | GLONASS | -- | 2C | `SigIDGLOL2` |
| 12 | L3 | GLONASS | L3-Q (pilot) | 3Q | `SigIDGLOL3Q` |
| 13 | B1C | BeiDou | B1C pilot | 1P | `SigIDBDSB1CP` (`SigIDBDSB1C` for `ChannelStatus`'s family slot) |
| 14 | B2a | BeiDou | B2a pilot | 5P | `SigIDBDSB2aP` (`SigIDBDSB2a` for the family slot) |
| 15 | L5 | NavIC | -- | 5A | `SigIDNAVICL5` |
| 16 | reserved | -- | -- | -- | -- |
| 17 | E1 | Galileo | E1-C (pilot) | 1C | `SigIDGALE1C` (`SigIDGALE1` for the family slot) |
| 18 | reserved | -- | -- | -- | -- |
| 19 | E6 | Galileo | E6-C default, E6-B if `CommonFlags` bit 6 set | 6C/6B | `SigIDGALE6C`/`SigIDGALE6B` (dynamic; `SigIDGALE6` for the family slot) |
| 20 | E5a | Galileo | E5a-Q (pilot) | 5Q | `SigIDGALE5aQ` (`SigIDGALE5a` for the family slot) |
| 21 | E5b | Galileo | E5b-Q (pilot) | 7Q | `SigIDGALE5bQ` (`SigIDGALE5b` for the family slot) |
| 22 | E5AltBOC | Galileo | AltBOC joint E5a+E5b, Q component | 8Q | **none -- gap, section 10** |
| 23 | LBand | MSS beam | -- (not a GNSS signal) | -- | n/a |
| 24 | L1CA | SBAS | -- | 1C | `SigIDGPSL1CA` (reused) |
| 25 | L5 | SBAS | data (I) | 5I | `SigIDGPSL5I` (reused) |
| 26 | L5 | QZSS | L5-Q (pilot) | 5Q | `SigIDQZSSL5Q` |
| 27 | L6 | QZSS | undocumented | *(blank in guide)* | `SigIDQZSSL6` (assumed L6D; guide gap, section 10) |
| 28 | B1I | BeiDou | -- | 2I | `SigIDBDSB1I` |
| 29 | B2I | BeiDou | -- | 7I | `SigIDBDSB2I` |
| 30 | B3I | BeiDou | -- | 6I | `SigIDBDSB3I` |
| 31 | (escape) | -- | -- | -- | not a signal: see `ObsInfo` bits 3-7, add 32 |
| 32 | L1C | QZSS | L1C-P (pilot) | 1L | `SigIDQZSSL1CP` |
| 33 | L1S | QZSS | -- | 1Z | `SigIDQZSSL1S` |
| 34 | B2b | BeiDou | B2b-I (data) | 7D | `SigIDBDSB2bI` |
| 35-36 | reserved | -- | -- | -- | -- |
| 37 | L1 | NavIC | -- | 1P | `SigIDNAVICL1` |
| 38 | L1CB | QZSS | -- | 1E | `SigIDQZSSL1CB` |
| 39 | L5S | QZSS | -- | 5P | `SigIDQZSSL5S` |

Confirmed byte-identical between the mosaic-X5 and mosaic-G5 guides for
this table, so it needs no model branch. The RINEX-obscode column is
the same mapping `sbfbin` exports for `rnxsbf` (`plan/sbf-rinex.md`) --
one signal-number table serves both the gpsprot `SignalID` mapping here
and the RINEX code mapping there.

**`ChannelStatus` <-> `MeasEpoch` family join table** (used to
overlay `MeasEpoch`'s `CN0`/precise `SignalID` onto the `ChannelStatus`
base, section 9.1):

| GNSS | Family (`ChannelStatus` slot) | Bit-slot | `MeasEpoch` # |
|---|---|---|---|
| GPS | L1CA / P1(Y) / P2(Y) / L2C / L5 / L1C | 0-5 | 0 / 1 / 2 / 3 / 4 / 5 |
| GLONASS | L1CA / L1P / L2P / L2CA / L3 | 0-4 | 8 / 9 / 10 / 11 / 12 |
| Galileo | E1BC / E6BC / E5a / E5b / E5ab | 1/3/4/5/6 | 17 / 19 / 20 / 21 / 22 |
| SBAS | L1 / L5 | 0/1 | 24 / 25 |
| BeiDou | B1I / B2I / B3I / B1C / B2a / B2b | 0-5 | 28 / 29 / 30 / 13 / 14 / 34 |
| QZSS | L1CA / L2C / L5 / L6 / L1C / L1S / L1CB / L5S | 0-7 | 6 / 7 / 26 / 27 / 32 / 33 / 38 / 39 |
| NavIC | L5 / L1 | 0/1 | 15 / 37 |

Bit-slot numbers key into `ChannelStatus.ChannelStateInfo`'s 2-bit
`TrackingStatus`/`PVTStatus`/`HealthStatus` fields
(`0`=not tracked/used, `1`=waiting-for-ephemeris, `2`=used/tracking,
`3`=rejected, exact meaning depends on which of the three 2-bit
fields); consult `sbfbin`'s `ChannelStatus` decode for the exact bit
offsets per constellation once that layer exists.

`CN0`: only `MeasEpoch.CN0` (u1, DNU `255`) -> `cn0 := raw*0.25; if
sigNum != 1 && sigNum != 2 { cn0 += 10 }` (the +10 correction applies
to every signal number except GPS L1P/L2P), rounded to nearest int.
Leave `CN0` at its Go zero value when only `ChannelStatus` contributed
that slot -- there is no way to distinguish "known zero" from
"unreported" on the current `SignalInfo.CN0` design, a pre-existing,
not Septentrio-specific, ambiguity.

## 10. Open decisions

These are recorded, not resolved, per the plan/CLAUDE.md convention of
keeping speculation and unresolved forks explicit:

- **GPS SVID 33-37 / `MeasEpoch` 250-251** vs. `gpsprot.GNSS.
  IsValidSVNum(GPS)`'s `<=32` ceiling: this document recommends
  emitting the `SVInfo` anyway (section 9.2); confirm before coding.
- **Galileo E5AltBOC** (signal 22): needs a new `gpsprot.SignalID`
  (e.g. `SigIDGALE5AltBOC`) or an explicit decision to drop AltBOC
  measurements until one exists -- reusing `SigIDGALE5` ("combo of
  E5a and E5b") would misrepresent the physical measurement.
- **QZSS L6 (signal 27) and SBAS L5 (signal 25)**: the guide itself
  leaves the RINEX-obscode cell blank for signal 27 on both models;
  `gpsprot.SignalID` has no SBAS-specific constants, so signal 25
  reuses `SigIDGPSL5I` by the existing SBAS/GPS convention -- both
  worth a second look once a real capture or vendor clarification is
  available.
- **`VelGeoMsg.GroundSpeed = hypot(Vn, Ve)`** (section 7.3): a
  protocol-local one-line derivation, since no existing protocol has
  needed it; whether it stays local or becomes a generic
  `gpsprot.PVMsgBundle.FillDerived` step is the open question.
- **`RTCMRefBaseID` cross-block reconciliation** (`DiffCorrIn`/
  `BaseStation`, section 8.4): last-known-`BaseStationID` carry-over
  across epochs is best-effort and has zero real-capture validation to
  date (both blocks were absent from every available sample capture);
  re-verify once an RTK/NTRIP-fed capture exists.
- **`Acc.Hor`/`Acc.Vert` statistical-basis mismatch** (2DRMS/2-sigma
  primary vs. 1-sigma-RSS fallback, section 8.3) and **`SurveyMsg`'s
  equivalent mismatch** (`HAccuracy` 95%-class vs. every existing
  `SurveyMsg.Accuracy` producer's 1-sigma convention, section 11): both
  are pre-existing `gpsprot.Accuracy`/`SurveyMsg` ambiguities (no
  documented required confidence level) that Septentrio's dual-source
  situation makes concrete; not fixed here.
- **Non-default `setTimingSystem`** (section 5.1): if a deployment
  ever configures the receiver's timing system to BeiDou or GLONASS,
  `ReceiverTime`/`xPPSOffset`'s header timestamp would be offset from
  GPS by a whole number of seconds with no field-level way to detect
  it. Out of scope for the default (GPS timing system) configuration
  this plan assumes; flag if `setTimingSystem` configuration is ever
  exposed.
- **`CorrRTCM`/`CorrPPPHAS`/`CorrPPPMDC`/`CorrPPPB2b` provenance**
  (section 8.2): no per-epoch SBF signal distinguishes these beyond
  `Mode==10` (bare PPP) and `WACorrInfo`'s OSR/SSR bits; leave unset
  for v1.
- **Missing combo `gpsprot.SignalID` constants** for GPS L2C/L5/L1C,
  GLONASS L3, SBAS L5, QZSS L2C/L5/L1C families (section 9.3): affects
  only the `ChannelStatus`-only fallback path when `MeasEpoch` is not
  enabled; the plain/family-level constants already in `gpsprot` are
  adequate for `ChannelStatus`'s own bit-slot granularity.

## 11. SurveyMsg

**No dedicated survey-status block exists in either guide** (zero
hits for "self-survey"/"survey-in" in Appendix A/B; `BaseStation` is a
*remote* base's coordinates, not this receiver's). The only usable
signal is the `Mode` bitfield shared by `PVTGeodetic`/`PVTCartesian`:

| Field | Source | Computation |
|---|---|---|
| `Position Point3D` | `PVTCartesian.X`/`Y`/`Z` (ECEF, matching every other protocol's `SurveyMsg.Position` frame) | Same as `PosECEFMsg.Pos`; **not a running mean** -- unlike `NAV-SVIN.MeanX/Y/Z`, this is just the current epoch's instantaneous fix, so it will not visibly settle during convergence the way other protocols' survey messages do |
| `Accuracy Length` | Primary: `PosCovCartesian.Cov_xx+yy+zz` (1-sigma RSS, the statistically clean choice); fallback: `PVTCartesian.HAccuracy` alone (95%-class, a real confidence-level mismatch vs. every existing `SurveyMsg.Accuracy` producer -- flag, don't silently treat as equivalent) | -- |
| `Valid bool` | `Mode&0xF == 3` ("Fixed location") | -- |
| `InProgress bool` | `Mode` bit 6 ("still determining fixed position") | Unverified against any real capture (none available shows bit 6 set); re-check once hardware can capture a converging self-survey |
| `ObsCount` | **no SBF source** | leave unset; do not approximate |
| `ObsTime` | **no SBF source** | leave unset; do not estimate it |

`ObsCount`/`ObsTime` have no SBF source and are left unset -- they are
being made optional (`opt.Val`) in `plan/gpsprot-json.md`, so Septentrio
simply does not provide them rather than fabricating a value.

`Valid` cannot distinguish an auto-survey-derived fixed position from
a manually-entered one (`setStaticPosGeodetic`/`Cartesian`) -- both
report `Mode&0xF==3` identically; only satpulse's own request state
(`gpsprot.TimeModeSurvey` vs `TimeModeFixed`) can tell them apart.
Similarly, since ordinary rover-mode operation never sets bit 6 or
reaches type 3, a Septentrio `SurveyMsg` decoder needs its own gating
(only construct/dispatch when local config state shows survey mode is
active) to avoid emitting a meaningless "no survey" message on every
ordinary epoch -- unlike `ubx.go`, which only ever builds a `SurveyMsg`
when a `NAV-SVIN`/`TIM-SVIN` message physically arrives.

## 12. CorReportMsg

Only the `Source = CorReportSourceReceiver` half is in scope here
(`CorReportSourcePull` comes from raw network bytes upstream of any
receiver protocol and needs no SBF-specific code). Source:
`DiffCorrIn` (5919), a near-exact structural analogue of UBX's
`RXM-COR`, one instance per inbound correction message.

| Field | Source | Computation |
|---|---|---|
| `Source` | block presence | `CorReportSourceReceiver` (constant) |
| `Tag` | `Mode` (u1: 0 RTCM2, 1 CMR, 2 RTCM3, 3 RTCMV, 4 SPARTN, 5 reserved) | `2`->`rtcm.Tag`, `4`->`spartn.Tag`, else return `nil` (no `gpsprot.Tag` exists yet for RTCM2/CMR/RTCMV -- a pre-existing codebase gap shared with `ubxcor.go`, not new here) |
| `MsgID`/`NativeMsg`/`NBytes` | the mode-specific trailing content (`raw[16:Length]`, untrimmed -- the inner protocol is self-delimiting) | `rtcmbin.ExtractMsgID`/`ParseMsg` for Mode 2, `spartnbin.MsgID`/`Parse` for Mode 4; `NBytes` from the inner protocol's own framed length, never from SBF's `Length-16` (which includes 0-3 padding bytes) |
| `ChecksumOK` | block existence | `true` unconditionally -- the receiver only emits `DiffCorrIn` for messages that already passed their own frame-integrity check |
| `RTCMRefBaseID` | `BaseStation.BaseStationID`, only when `BaseStation.Source==8` (RTCM3 1005/1006) | best-effort cross-block fill (section 10) |
| `Used`, `FinalFragment` | **no SBF field for either** | always absent for SBF-sourced reports |

`DiffCorrIn` is purely event-driven with no periodic/keepalive
emission; "no `CorReport` dispatch yet" is the correct read for "no
correction feed connected", not a decode error.

## 13. Model differences relevant to this layer (G5 vs X5)

- **No dedicated Galileo HAS/PPP SBF block on either model.** `PPP
  GalileoHAS-SIS` is a `getReceiverCapabilities` capability value, not
  an Appendix B block. HAS corrections are decoded internally once
  `GALE6BC` tracking/usage and `setPVTMode,,+PPP` are configured, and
  surface only as `Mode==10` ("PPP") in `PVTGeodetic`/`PVTCartesian` --
  section 8.1/8.2 already treat `Mode==10` as the sole PPP signal; do
  not expect a separate HAS block to decode further on either model.
- **G5 has no `Meas3*` family** (compact/delta-coded measurements) --
  irrelevant to this phase, since `MeasEpoch` (not `Meas3`) is the
  source used throughout.
- **`ChannelStatus`, `MeasEpoch`, and `xPPSOffset` are confirmed
  byte-identical between G5 and X5** (field layout and the sec 4.1.10
  signal table) -- the mappings in sections 5, 8, and 9 need no model
  branch.
- **`PVTGeodetic`/`PVTCartesian.AlertFlag` bits 5-6** are named
  `SIG_AUTH_ALERT`/`NAV_MSG_AUTH_ALERT` on G5 (Rev3) and left reserved
  on X5, but the underlying detection already exists identically on
  both via `RFStatus.Flags` bits 0-1 -- decoding the raw bits without
  the G5 names is safe on both models; a model/revision check is only
  needed to surface the named semantics. Out of scope for this
  phase's Msg set (no `gpsprot` field carries alert flags today).
- **`ReceiverStatus` bit 11** is `OUTOFGEOFENCE` on X5 and renamed/
  widened to `OUTOFFENCE` (also covering motion-fencing) on G5 --
  irrelevant to this phase (`ReceiverStatus` is not mapped to any
  `gpsprot` Msg here; it only mirrors `SyncLevel` for the
  `WNSET`/`TOWSET`/`FINETIME` gate, section 5.4).
- **G5's second PPS (`PPS2`/`sps2`) is not covered by `xPPSOffset`.**
  The guide states `xPPSOffset` "always refers to the first PPS
  output" on both models; out of scope for this phase, which only
  decodes the first (and, on X5, only) PPS via `xPPSOffset`.
- **G5's `NavCart`/`NavGeod`** (4272/4275, "everything in one block":
  `PVTCartesian`+`AttEuler`+`DOP`+`ReceiverTime` combined) are a G5-only
  convenience not used by this design -- the per-block sources listed
  throughout (`PVTGeodetic`/`PVTCartesian`, `DOP`, `ReceiverTime`
  separately) work on both models and are simpler to reason about
  block-by-block.

## 14. Registration

In `gps/gpsreg/reg.go` (note `VendorSeptentrio` already exists at
`reg.go:32`):

- Add `TagSBF = septentrio.Tag` re-export.
- Add the SBF `PacketFormat` (phase 1) to `allVendorPacketFormats` and
  the per-vendor map.
- Add `septentrio.NewPacketProcessor(mgr)` to `CreatePacketProcessors`,
  sharing the one `*gpsprot.NavEpochManager` instance passed to every
  vendor's processor.
- `CreateConfigProtocols` stays empty for Septentrio (phase 5, not
  this phase).

## 15. Phasing within this plan

1. `PacketProcessor` skeleton: `ProcessPacket`/`Dispatch` wired to
   `sbfbin.ParseMsg` and `NativeMsgHandler` fallback for every
   unmapped block; epoch key-change machinery (section 4) with no Msg
   converters yet (verifies dispatch/epoch bookkeeping against a real
   capture before adding conversion logic).
2. Time family: `ReceiverTime`/`xPPSOffset`/PVT-family `TimeMsg`
   construction (section 5) and `LeapSecondMsg` (section 6) -- the
   two most safety-critical paths (whole-second and PPS-sign
   correctness), landed and tested first.
3. Position/velocity family (section 7) and `NavEpochMsg`'s
   quality-field accumulation (section 8), sharing the `Mode`/`Error`
   extraction.
4. `SatellitesMsg` three-way combine (section 9), as an independent
   stream (section 4.3), dispatched on its own boundary rather than
   through the epoch flush.
5. `SurveyMsg` (section 11) and `CorReportMsg` (section 12) -- lower
   priority, event-driven or config-mode-dependent, no timing-daemon
   correctness risk.
6. Registration (section 14).

## 16. Testing

- **Unit tests**, same-package (`package septentrio`), table-driven
  per converter function: hand-built `sbfbin` struct literals covering
  the DNU/edge cases enumerated throughout this document (cold-start
  `Mode==0`, 2D mode, fixed/survey `Mode` values, each block's DNU
  sentinels, the BeiDou/Galileo week-conversion case, the PPS sign
  flip). Follow the `go-unit-test` skill for style; place tests
  alongside the file they cover (`ubxtime.go` <-> `ubxtime_test.go`
  convention).
- **Real-capture replay**: once phase 1 (`sbfbin`/`PacketFormat`)
  exists, use the `packet-testdata` skill's corpus approach and the
  `satpulsed-test-instance`/`drive-satpulsed-from-log` skills to
  replay the example SBF captures (see `CLAUDE.local.md`) through a
  real `satpulsed` and inspect the resulting `gpsprot.Msg` stream
  end-to-end, cross-checking against the guide-documented sample
  values (e.g. the `all_blocks_0000.sbf`/`large_0000.sbf`/
  `log_0000.sbf` example captures).
- **Deferred to real hardware**: the `xPPSOffset.Offset` sign flip
  (section 5.3) and `SurveyMsg.InProgress` (section 11) have no
  real-capture evidence available today (no capture shows a converging
  self-survey; the PPS sign has no oscilloscope/NTP-SHM ground truth)
  -- verify both against real hardware before considering this phase
  fully validated, per `CLAUDE.local.md`'s hardware-testing notes.
