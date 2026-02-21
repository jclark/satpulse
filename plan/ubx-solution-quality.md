# UBX solution quality implementation

## Context

Phase 1 (type definitions) is complete: `NavEpochMsg` already has all quality fields (`FixLevel`, `FixDim`, `Correction`, `AuxSrc`, `DOP`, `DiffAge`, `NumSVUsed`, `NumSVTracked`, `SignalsUsed`), `MergeNavEpoch` merges them, and `PVTMsgQuality` flag exists. No protocol currently populates these fields. This plan implements Phase 2b (UBX) from `plan/solution-quality.md`.

## Phasing

### Phase A: NAV-PVT quality extraction

Add a `qualityNavPVT(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPVT)` function in `ubxpv.go` (follows existing pattern of `posGeoNavPVT`/`velGeoNavPVT`). Call it from the `case *ubxbin.NavPVT:` in `Dispatch`.

Populates on `ne`:
- **FixDim + AuxSrc** from `m.FixType` (0=none, 1=DR, 2=2D, 3=3D, 4=GNSS+DR, 5=timeOnly). `HeadVehValid` flag adds `AuxSrcINS`.
- **FixLevel + Correction** from `m.Flags`: if `!gnssFixOK` -> `FixLevelNone`; if DR-only -> `FixLevelNone`; else from `carrSoln` (fixed/float/none) and `diffSoln`.
- **NumSVUsed** from `m.NumSV`
- **DOP.Pos** from `m.PDOP` (scale 0.01)
- **DiffAge** from `m.Flags3 & NavPVTLastCorrectionAgeMask` -> bucket lower bound as `time.Duration`

`qualityNavPVT` is always called (even when `pvtFixValid` returns false and pos/vel are nil), because quality metadata should report "no fix" states too.

Unit tests in `ubxpv_test.go`: table-driven tests covering each FixType x Flags combination.

**Files**: `gps/internal/ubx/ubxpv.go`, `gps/internal/ubx/ubx.go` (1 line), `gps/internal/ubx/ubxpv_test.go`

### Phase B: NAV-DOP dispatch

Add `case *ubxbin.NavDOP:` in `Dispatch` that populates `DOP.Geom/Pos/Hor/Vert/Time` on `curNavEpochMsg` from the NavDOP fields (all scale 0.01). Inline in Dispatch (no separate function needed -- it's just 5 field assignments). Return `true` since the message was handled.

Add `NavDOPID` to `msgIDKey` map in `ubxcfgmsg.go`.

Unit test: verify DOP fields are populated correctly.

**Files**: `gps/internal/ubx/ubx.go`, `gps/internal/ubx/ubxcfgmsg.go`, `gps/internal/ubx/ubxpv_test.go`

### Phase C: PVTMsgQuality message enablement

In `ubxcfgmsg.go` `pvt()` method, handle `PVTMsgQuality`: enable NAV-DOP and ensure NAV-PVT is enabled (when protocol supports it). NAV-PVT is the primary quality source; NAV-DOP provides the full DOP set. Quality metadata from NAV-PVT is extracted in Phase A even when PVTMsgQuality is not explicitly set (it piggybacks on NAV-PVT which is already enabled for pos/vel/time).

Add test case to `TestMsgChangesPVT` for `PVTMsgQuality`.

**Files**: `gps/internal/ubx/ubxcfgmsg.go`, `gps/internal/ubx/ubxcfgmsg_test.go`

### Phase D: NAV-SIG/NAV-SAT correction source extraction

Accumulate `CorrKind` on `curNavEpochMsg` from per-signal `CorrSource` (NAV-SIG) or per-SV correction flags (NAV-SAT). This enriches the base `CorrUsed` set by NAV-PVT with specific style bits (`CorrBaseStation`, `CorrSBAS`, `CorrRTCM`, `CorrSPARTN`, `CorrCLAS`, `CorrWideArea`).

In `satellitesNavSig`: iterate *used* signals, map `CorrSource` enum to `CorrKind` bits, return the accumulated bitmask alongside the `SatellitesMsg` (or via a separate function).

In `satellitesNavSat` (fallback): iterate *used* SVs, check `NavSatSbasCorrUsed`, `NavSatRtcmCorrUsed`, `NavSatSlasCorrUsed`, `NavSatSpartnCorrUsed`, `NavSatClasCorrUsed` flags.

In `Dispatch` NAV-SIG/NAV-SAT cases: OR the accumulated `CorrKind` into `curNavEpochMsg.Correction`.

Conflict rule: if both base-station and wide-area correction sources appear, leave style bits unset but keep `CorrRTCM` if RTCM is known.

**Files**: `gps/internal/ubx/ubxsats.go`, `gps/internal/ubx/ubx.go`, `gps/internal/ubx/ubxsats_test.go`

### Phase E: Hardware test

Use the `hardware-test-gps-msgs` skill against the ZED-F9P on `/dev/ttyACM0` (38400 baud) with `--pvt-out pos,vel,time,epoch,qual --sats-out sat,sig` to verify that `NavEpochMsg` fields are populated with plausible values from a live receiver.

## Verification

After each phase:
- `make test` (all tests pass)
- After Phase E: hardware test shows populated quality fields in `NavEpochMsg`
