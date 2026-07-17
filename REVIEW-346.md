# Outstanding review comments: PR 346

## P1

### 1. A flushed DNU-keyed epoch is reused as nil

`gps/internal/septentrio/sbf.go:144`

`FlushNavEpoch` clears `curEpoch.msg` but deliberately retains `curEpoch` and
its timestamp. If `EndOfPVT` flushes a cold-start PVT epoch whose `(TOW, WNc)`
key is DNU, the next PVT-family block with the same documented DNU key does not
enter the initialization branch in `navEpoch`. `navEpoch` therefore returns
the nil `curEpoch.msg`, which `qualityPVT` and the covariance converters
immediately dereference. Two consecutive cold-start epochs with DNU header
timestamps can consequently panic the processor.

Reinitialize when `curEpoch.msg == nil`, independently of whether the retained
timestamp compares equal.

### 2. ReceiverTime's ReadDelay is a full epoch stale

`gps/internal/septentrio/sbf.go:199`

`p.curEpochStart` is assigned only in `navEpoch()`, which `Dispatch` calls only
for the PVT-family blocks. `ReceiverTime` goes straight to `emitTime`, and
`timeReceiverTime` sets `Ref: NavSolution`, so the `ReadDelay` branch fires
against whatever `curEpochStart` the previous epoch left behind.

`ntp.jsonl` shows `ReceiverTime` consistently *precedes* the PVT blocks of its
own epoch:

```
14:59:45.006 PVTCartesian  TOW 140403000
14:59:46.004 ReceiverTime  TOW 140404000   <- curEpochStart still 45.006
14:59:46.006 PVTCartesian  TOW 140404000
```

so `ReadDelay` comes out ~998 ms instead of ~0. `ReceiverTime` is the only SBF
block that populates `UTCTime`, and `timemsg.go:108` gates on exactly
`msg.UTCTime.IsSet()` before calling
`msgUTCTimer.MsgUTCTime(utc, tRead.Add(-msg.ReadDelay), ...)` at `:133` — so
every SBF UTC sample is paired with a system-clock instant one second early.
UBX sidesteps this by gating on `p.curNavEpochMsg != nil` (`ubx.go:274`), which
would not help here: the stale epoch is a live one. No test in the package
covers `ReadDelay`.

`ReceiverTime` should participate in epoch keying, so that `curEpochStart` is
the first block of the epoch rather than the previous epoch's. This follows
UBX: `NavTimeUTC` embeds `NavITOW`, so `ubx.go:52` routes NAV-TIMEUTC through
`handleNavEpoch` exactly like NAV-PVT, and whichever nav message arrives first
for an iTOW sets the epoch start.

Keying from `ReceiverTime` must keep allocating `curEpochMsg` eagerly, as
`handleNavEpoch` does. `ReceiverTime` is `Time`-group output on a fixed 1 s
interval while the PVT blocks are on a separately configurable rate, so an
epoch can contain `ReceiverTime` and no PVT block. Allocating only when a PVT
converter first needs it would suppress the `NavEpochMsg` for such an epoch --
and `NavEpochMsg` is the epoch boundary, not merely a quality summary:
`TimeTicker.NavEpoch` (`msg.go:1537`) is the only thing that clears the
one-`TimeMsg`-per-epoch latch, so suppressing it would forward one `TimeMsg`
and silently drop every later one. The epoch is real whenever `ReceiverTime`
arrives; reporting it with no quality fields set is correct.

## P2

### 3. NavEpochMsg merges NMEA's previous-epoch contribution

`gps/gpsprot/msg.go:1350`

If all first-epoch NMEA sentences trail the first `EndOfPVT`, `lastEpoch` is
empty and `EndOfProtocolEpoch` flushes SBF epoch N alone. The trailing NMEA
sentences then leave NMEA epoch N active. When SBF epoch N+1 starts, SBF is no
longer active, so `EpochStarted` adds it beside NMEA rather than flushing.
The next NMEA boundary consequently merges NMEA N with SBF N+1. The same
sequence repeats for every following epoch, so this is persistent rather than
the one lost startup merge allowed by plan section 4.2.

The offset is in the manager's bookkeeping and is mostly not observable. The
merged `NavEpochMsg` is emitted at the NMEA boundary rather than at
`EndOfPVT`, about 1 ms later, and carries NMEA's previous-second epoch. But
NMEA contributes only `FixLevel`, `SolutionDim`, `DOP` and `NumSVUsed`
(verified by replaying `duplicate.jsonl` with the SBF blocks stripped), the
SBF PVT set supplies all four, and `PriVendorLow` outranks `PriGenericHigh`,
so `MergeNavEpoch` discards every stale value. The emitted message matches
what SBF alone would produce. A stale value only reaches output through a
field NMEA sets and SBF leaves unset: `DOP` with the SBF `DOP` block disabled,
or `NumSVUsed` when `NrSV` is at its DNU value.

It is still wrong: the manager pairs epochs it does not intend to pair, plan
section 4.2 asserts a self-correction that does not happen, and any future
field that only NMEA supplies would silently arrive a second stale.

This is currently specific to the Septentrio SBF+NMEA path because SBF is the
only processor that calls `EndOfProtocolEpoch`. Processors with a
whole-receiver end marker use `EndOfEpoch`; processors without one remain
active until their own next-epoch `EpochStarted` call flushes the correctly
aligned set.

Design decision: replace the last-epoch participant set with
`lastEpochSingleProtocol`, scoped to processors that have called
`EndOfProtocolEpoch`. It identifies the sole such processor in the last
logical epoch and is nil when that epoch was empty, used multiple processors,
or involved only processors with whole-receiver or implicit boundaries.
`EndOfProtocolEpoch(f)` flushes only when the current active set is exactly
`{f}` and `lastEpochSingleProtocol == f`. An unknown cold-start state therefore
defers the first SBF-only epoch until the next TOW transition; that flush
establishes SBF as the sole protocol, after which `EndOfPVT` flushes promptly.
An `EndOfEpoch` from UBX or Quectel cannot arm the recovery state.

`EpochStarted(f)` also uses this state to recover when trailing NMEA appears
after SBF-only operation was established. If `f` is not active, another
processor is active, and `lastEpochSingleProtocol == f`, the active
contribution arrived after the previous protocol-local flush and is flushed
separately before `f` is registered. The manager then clears
`lastEpochSingleProtocol`, since both processors participated in the split
logical epoch. The transition epoch is consequently split into an SBF-only
and an NMEA-only `NavEpochMsg`, but the following epoch is aligned and normal
merging resumes. The late contribution must be emitted rather than discarded
because `NavEpochMsg` is also the boundary that resets downstream per-epoch
state.

The initial deferral is unavoidable without external configuration: at the
first `EndOfPVT`, an SBF-only receiver and one whose first NMEA sentences have
not arrived yet are indistinguishable. Consistently leading NMEA does not need
the protocol-local end handling because NMEA is already active when
`EndOfPVT` arrives.

Concurrent SBF and NMEA is not a useful configuration: with SBF PVT enabled,
NMEA supplies nothing SBF does not already supply better, and `setSBFOutput`
and `setNMEAOutput` both default to no output at all (every stream `none,
none, off`). The realistic way to reach it is enabling SBF and not disabling
NMEA. That is the scenario to capture.

The ordering the bug needs is confirmed by capture: SBF leads each second
(`ReceiverTime` at .005) and NMEA trails within it (`GNRMC` at .008, after
`ChannelStatus` at .007).

Being in `gps/gpsprot/msg.go`, this is also the only comment outside
`gps/internal/septentrio`, so it may be separable from this PR.

**Fixed.** `NavEpochManager` now defers an unestablished protocol-local end,
tracks sole-protocol history only for processors that call
`EndOfProtocolEpoch`, and splits a late trailing contribution once to restore
alignment. Regression tests cover cold-start trailing NMEA, NMEA appearing
after SBF-only operation, and UBX+NMEA remaining unchanged after `EndOfEpoch`.

### 4. MeasEpoch replaces rather than enriches the ChannelStatus SV set

`gps/internal/septentrio/sbfsat.go:48`

When a pending `MeasEpoch` is present, `addMeasEpoch` adds measurement-only SVs
and `pruneEmpty` removes every `ChannelStatus`-only SV. The emitted set thus
follows the possibly stale measurement block rather than the current
`ChannelStatus`: it can retain an SV lost since the measurement and omit a
newly tracked SV, an SV still acquiring, or one whose only measured signals
are deliberately unmapped (for example E5 AltBOC to RINEX `8Q`). This can
happen when the blocks run at different rates as well as at acquisition/loss
boundaries.

That contradicts plan section 9.1 ("`ChannelStatus` is the structural base ...
enriches") and makes the sky view depend on whether signal-strength output is
enabled. `sats-sig.jsonl` is exactly this both-blocks case (30 ChannelStatus,
30 MeasEpoch) and no test asserts SV counts on it. The combined message should
retain the mapped `ChannelStatus` SV set and use `MeasEpoch` only to add signals
to members of that set.

Relatedly, `emitChannelStatusSats` is the sole caller of `satellitesCombine` and
always passes a non-nil `chn`, so the `chn != nil` guard, the
`nativeID = "MeasEpoch"` branch, and `SatelliteUsedInvalid` are all unreachable.

### 5. Unmapped DiffCorrIn modes suppress native handling

`gps/internal/septentrio/sbf.go:129`

`corReportDiffCorrIn` returns nil for RTCMv2, CMR, RTCMV, and reserved modes
because there is no corresponding generic `gpsprot.Tag`. The `DiffCorrIn`
dispatch case nevertheless calls the no-op `emitCorReport(nil, ...)` and
returns true. `ProcessPacket` therefore never invokes `NativeMsg`, silently
dropping precisely the correction blocks that the conversion layer cannot
represent. The dispatch case should report unhandled when the converter
returns nil.

**Fixed.** Unmapped correction modes now return unhandled from `Dispatch`, so
`ProcessPacket` forwards the block to `NativeMsg`.

### 6. SPARTN NBytes is left unset while RTCM3 sets it

`gps/internal/septentrio/sbfcor.go:33`

Plan section 12 wants `NBytes` from the inner protocol's own framed length for
both modes. The RTCM3 branch populates it, but the SPARTN branch never does.
`spartnbin.FrameLen(hdr []byte) (int, bool)` (`spartnbin.go:123`) already
derives the exact self-delimiting SPARTN frame length without counting SBF
padding and should be used here.

**Fixed.** The SPARTN branch now uses `spartnbin.FrameLen` to populate
`NBytes`, excluding SBF padding.

### 7. channelLookAngles substitutes 0 for whichever component is DNU

`gps/internal/septentrio/sbfsat.go:184`

The ChannelStatus definition gives azimuth and elevation independent DNU
values, and guide section 4.1.7 requires a field at its DNU value to be
discarded. The function instead returns a `LookAngles` when either component
is available and leaves the other at its zero value. An SV with DNU elevation
is therefore reported at the horizon, while one with DNU azimuth is reported
due north. Both zero values are valid geometry, not missing-value markers.

`gpsprot.LookAngles` has no per-field optionality, so the whole value must be
left unset unless both components are available.

**Fixed.** `channelLookAngles` now requires both azimuth and elevation.

### 8. Leap-second converters consume DNU header timestamps

`gps/internal/septentrio/sbfutc.go:22`

Guide section 4.1.3 says either header timestamp field can be DNU for several
seconds after startup and explicitly says that this does not make the block's
payload unusable. Section 4.1.7 requires decoding software to discard each DNU
field. `leapGPSUtc`/`leapGALUtc`/`leapBDSUtc` nevertheless pass the timestamp
straight to `gpsHeaderTime` without the `headerTimeValid` guard used by the
other time converters. A DNU `WNc` becomes week -1, while a DNU `TOW` becomes
about 49.7 days after the nominal week start.

`gnssLeapSecond` uses this value as `now` to resolve the truncated `WN_LSF` and
enforce its candidate-date window. The UTC converters cannot safely perform
that resolution without a valid header timestamp, so they should return nil
when `headerTimeValid` is false.

**Fixed.** All three UTC converters now reject a header with either timestamp
field at its DNU value.

## P3

### 9. NrSV do-not-use is a bare wire literal

`gps/internal/septentrio/sbfnavepoch.go:57`

`c.NrSV != 255`. Plan section 3 makes this a rule: every DNU value referenced
here is an `sbfbin.` constant, and if one is not exported yet, export it.
`sbfbin/pvt.go:104` has `PVTAccuracyDNU`, `PVTReferenceIDDNU` and friends, but
no `PVTNrSVDNU`.

**Fixed.** `sbfbin.PVTNrSVDNU` is exported and used by the converter.

### 10. Stale doc comment on sbfSVID

`gps/internal/septentrio/sbfsignal.go:61`

The comment describes a `freqNr` parameter the function does not take, left over
from an earlier signature. The range arithmetic itself checks out against
`sbfbin/sat.go:41-67`, including the 69/70 fall-through and the G38/G39
extension.

**Fixed.** The stale parameter description was removed.

### 11. Inconsistent sub-block bounds guarding

`gps/internal/septentrio/sbfsat.go:73`

`addMeasEpoch` guards `i >= len(meas.Type2)` (`:96`) but `addChannelStatus`
indexes `chn.StateInfo[i]` unguarded. `twoLevelChunks`
(`sbfbin/common.go:207-228`) allocates the outer and inner slices to the same
length and panics if they ever differ, so neither guard is reachable from the
wire; only the hand-built test structs differ.

Drop the guard from `addMeasEpoch`, matching `addChannelStatus`. A sub-block
pair with mismatched lengths is a contract violation and should panic rather
than be silently skipped. The guard exists only because `testMeasBlock`
(`sbf_test.go:122`) sets `Type1` and leaves `Type2` nil, so that helper must
build the parallel `Type2` slice instead.

**Fixed.** The guard was removed and hand-built test blocks now satisfy the
parallel-slice contract.
