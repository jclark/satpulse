# CASIC V6 NAV2 position, velocity, and solution quality

CASIC V6 (class 0x11 NAV2) firmware runs on ZKW F8 dual-band receivers (e.g. ATGM332D-6N74). This plan covers adding full NAV2 message support: binary parsing in `casbin`, extraction and dispatch in `internal/casic`, position/velocity messages, nav epoch, and solution quality.

Prerequisite: position/velocity message types (`PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`) and `MsgHandler` methods must already exist in `gpsprot` (see [position-velocity-messages.md](position-velocity-messages.md)).

Prerequisite: `curNavEpochMsg` on the CASIC `PacketProcessor` must already exist (see [casic-pos-vel-qual.md](casic-pos-vel-qual.md) step 2a).

## Key differences from V5

| Aspect | V5 NAV (0x01) | V6 NAV2 (0x11) |
|--------|---------------|-----------------|
| Epoch key | `RunTime` (U4, ms since boot) | `tow` (I4, GPS TOW in ms) |
| Accuracy units | Variances (m^2, (m/s)^2, deg^2) -- need sqrt | Standard deviations (m, m/s, deg) -- direct |
| Constellations | GPS, BDS, GLONASS | Adds Galileo, QZSS, SBAS, IRNSS |
| Fix quality | 0-8 scale | Extended 0-15 (adds DGPS=8, RTK float=9, RTK fixed=10, timing fixed=15) |
| Geodetic msg | NAV-PV (80 bytes) | NAV2-PVH (88 bytes, adds per-constellation counts + separate accuracy) |
| ECEF msg | NAV-SOL (72 bytes) | NAV2-SOL (72 bytes, restructured) |

## Part 1: casbin message parsing

NAV2 messages use GPS TOW in integer milliseconds for epoch keying instead of RunTime. A new `Nav2TOW` embedded type provides the `NavMsg` interface.

### 1a: Nav2TOW epoch key type

Add to `casbin/nav.go`:

```go
// Nav2TOW is embedded in NAV2 messages to provide epoch tracking via GPS TOW.
type Nav2TOW struct {
	TOW int32 // GPS time of week in ms
}

func (m *Nav2TOW) NavEpoch() uint32 { return uint32(m.TOW) }
```

This implements the existing `NavMsg` interface. The `PacketProcessor.handleNavEpoch` will track NAV2 epochs via TOW changes, just as it tracks V5 epochs via RunTime changes.

### 1b: Nav2FixFlags type

The V6 fix flags extend V5 `posValid` with values 8-15. Add a new type to `casbin/nav.go`:

```go
type Nav2FixFlags uint8

const (
	Nav2FixInvalid       Nav2FixFlags = 0
	Nav2FixExternal      Nav2FixFlags = 1
	Nav2FixRoughEstimate Nav2FixFlags = 2
	Nav2FixHold          Nav2FixFlags = 3
	Nav2FixDeadReckoning Nav2FixFlags = 4
	Nav2FixQuickMode     Nav2FixFlags = 5
	Nav2Fix2D            Nav2FixFlags = 6
	Nav2Fix3D            Nav2FixFlags = 7
	Nav2FixDGPS          Nav2FixFlags = 8
	Nav2FixRTKFloat      Nav2FixFlags = 9
	Nav2FixRTKFixed      Nav2FixFlags = 10
	Nav2FixTimingFixed   Nav2FixFlags = 15
)
```

A `Nav2VelFlags` type with the same underlying values (same PVT valid flag scale) is also needed for velocity validity.

### 1c: Nav2GnssMask type

```go
type Nav2GnssMask uint8

const (
	Nav2GnssGPS  Nav2GnssMask = 1 << iota
	Nav2GnssBDS
	Nav2GnssGLN
	Nav2GnssGAL
	Nav2GnssQZSS
	Nav2GnssSBAS
	Nav2GnssIRNSS
)
```

### 1d: Nav2Sol struct (0x11 0x02, 72 bytes)

Add to `casbin/nav.go`. Move `Nav2SolID` from `other.go` to `nav.go`. Register in `init()`.

```go
// Nav2Sol is NAV2-SOL (0x11 0x02) - ECEF position and velocity (72 bytes)
type Nav2Sol struct {
	Nav2TOW
	Wn         uint16
	_          uint16       // reserved
	FixFlags   Nav2FixFlags
	VelFlags   Nav2VelFlags
	_          uint8        // reserved
	GnssMask   Nav2GnssMask
	NumFixTot  uint8
	NumFixGPS  uint8
	NumFixBDS  uint8
	NumFixGLN  uint8
	NumFixGAL  uint8
	NumFixQZSS uint8
	NumFixSBAS uint8
	NumFixIRN  uint8
	_          uint32       // reserved
	X          float64      // m, ECEF X
	Y          float64      // m, ECEF Y
	Z          float64      // m, ECEF Z
	PAcc       float32      // m, 3D position accuracy (std dev, NOT variance)
	VX         float32      // m/s, ECEF X velocity
	VY         float32      // m/s, ECEF Y velocity
	VZ         float32      // m/s, ECEF Z velocity
	SAcc       float32      // m/s, 3D speed accuracy (std dev, NOT variance)
	PDOP       float32
}

func (m *Nav2Sol) ID() MsgID { return Nav2SolID }
```

### 1e: Nav2Pvh struct (0x11 0x03, 88 bytes)

Add to `casbin/nav.go`. Move `Nav2PvhID` from `other.go` to `nav.go`. Register in `init()`.

```go
// Nav2Pvh is NAV2-PVH (0x11 0x03) - geodetic position and velocity (88 bytes)
type Nav2Pvh struct {
	Nav2TOW
	Wn         uint16
	_          uint16       // reserved
	FixFlags   Nav2FixFlags
	VelFlags   Nav2VelFlags
	_          uint8        // reserved
	GnssMask   Nav2GnssMask
	NumFixTot  uint8
	NumFixGPS  uint8
	NumFixBDS  uint8
	NumFixGLN  uint8
	NumFixGAL  uint8
	NumFixQZSS uint8
	NumFixSBAS uint8
	NumFixIRN  uint8
	_          uint32       // reserved
	Lon        float64      // deg
	Lat        float64      // deg
	Height     float32      // m, ellipsoidal
	SepGeoid   float32      // m, geoid separation
	VelE       float32      // m/s, East velocity
	VelN       float32      // m/s, North velocity
	VelU       float32      // m/s, Up velocity (negate for NED down)
	Speed3D    float32      // m/s
	Speed2D    float32      // m/s, ground speed
	Heading    float32      // deg
	HAcc       float32      // m, horizontal position accuracy (std dev)
	VAcc       float32      // m, vertical position accuracy (std dev)
	SAcc       float32      // m/s, 3D speed accuracy (std dev)
	CAcc       float32      // deg, heading accuracy (std dev)
}

func (m *Nav2Pvh) ID() MsgID { return Nav2PvhID }
```

### 1f: Nav2Dop struct (0x11 0x01, 24 bytes)

Add to `casbin/nav.go`. Move `Nav2DopID` from `other.go` to `nav.go`. Register in `init()`.

NAV2-DOP has no `tow` field. It does not implement `NavMsg`.

```go
// Nav2Dop is NAV2-DOP (0x11 0x01) - dilution of precision (24 bytes)
type Nav2Dop struct {
	PDOP float32
	HDOP float32
	VDOP float32
	NDOP float32
	EDOP float32
	TDOP float32
}

func (m *Nav2Dop) ID() MsgID { return Nav2DopID }
```

### 1g: Nav2TimeUTC struct (0x11 0x05, 20 bytes)

Add to `casbin/nav.go`. Move `Nav2TimeUTCID` from `other.go` to `nav.go`. Register in `init()`.

NAV2-TIMEUTC does not have a standard epoch key field (no `tow` or `RunTime`). It does not implement `NavMsg`.

```go
// Nav2TimeUTC is NAV2-TIMEUTC (0x11 0x05) - UTC time information (20 bytes)
type Nav2TimeUTC struct {
	TAcc    float32       // ns, time accuracy estimate
	Subms   int32         // ms, fractional ms (scale 2^-30)
	Subcs   int8          // ms, centisecond error (-5 to 5 ms)
	Cs      uint8         // centiseconds (0-99)
	Year    uint16
	Month   uint8
	Day     uint8
	Hour    uint8
	Min     uint8
	Sec     uint8
	TFlags  Nav2TimeFlags
	TimeSrc Nav2TimeSrc
	LeapSec int8
}

type Nav2TimeFlags uint8

const (
	Nav2TimeTOWValid  Nav2TimeFlags = 1 << iota
	Nav2TimeWNValid
	Nav2TimeLeapValid
	Nav2TimeReliable
)

type Nav2TimeSrc uint8

const (
	Nav2TimeSrcGPS Nav2TimeSrc = iota
	Nav2TimeSrcBDS
	Nav2TimeSrcGLN
	Nav2TimeSrcGAL
	Nav2TimeSrcIRN
)

func (m *Nav2TimeUTC) ID() MsgID { return Nav2TimeUTCID }
```

### 1h: Nav2Sat struct (0x11 0x04, variable)

Add to `casbin/nav.go`. Move `Nav2SatID` from `other.go` to `nav.go`. Register in `init()`.

```go
// Nav2SatFixed is the fixed part of NAV2-SAT (12 bytes)
type Nav2SatFixed struct {
	TOW       uint32 // GPS TOW in ms
	NumViewTot uint8
	NumFixTot  uint8
	_         uint8  // reserved
	_         uint8  // reserved
	_         uint32 // reserved
}

// Nav2SVInfo is a per-satellite entry in NAV2-SAT (12 bytes each)
type Nav2SVInfo struct {
	Chn     uint8
	SVID    uint8
	GNSSID  uint8  // 0=GPS, 1=BDS, 2=GLN, 3=GAL, 4=QZSS, ...
	Flags   uint8  // B0=used, B7-B4: orbit source
	Quality uint8  // signal quality bitmask
	CNO     uint8  // dBHz
	SigID   uint8
	Elev    uint8  // deg
	Azim    uint16 // deg
	PRRes   int16  // dm, pseudorange residual
}

// Nav2Sat is NAV2-SAT (0x11 0x04) - satellite information
type Nav2Sat struct {
	Nav2SatFixed
	SVs []Nav2SVInfo
}

func (m *Nav2Sat) ID() MsgID { return Nav2SatID }

// InitVaryingPart, FixedPart, VaryingPart implement VaryingMsg
```

### 1i: Nav2Sig struct (0x11 0x06, variable)

Add to `casbin/nav.go`. Move `Nav2SigID` from `other.go` to `nav.go`. Register in `init()`.

```go
// Nav2SigFixed is the fixed part of NAV2-SIG (8 bytes)
type Nav2SigFixed struct {
	TOW       uint32 // GPS TOW in ms
	_         uint8  // reserved
	NumTrkTot uint8
	NumFixTot uint8
	_         uint8  // reserved
}

// Nav2SigInfo is a per-signal entry in NAV2-SIG (16 bytes each)
type Nav2SigInfo struct {
	GNSSID   uint8  // system ID
	SVID     uint8  // satellite ID
	SigID    uint8  // signal ID
	FreqID   uint8  // GLONASS frequency ID
	PRRes    int16  // dm, pseudorange residual
	CNO      uint8  // dBHz
	TrkInd   uint8  // signal quality
	CorFlags uint8  // correction flag (B[6:4]=iono model, B[2:0]=correction source)
	SolFlags uint8  // solution flag (B0=PR used, B1=CP used, B2=Doppler, B3=smoothing, B[7:4]=sat status)
	Chn      uint8
	Elev     uint8  // deg
	Azim     uint16 // deg
	IonoDelay int16 // dm, ionosphere delay correction
}

// Nav2Sig is NAV2-SIG (0x11 0x06) - per-signal tracking information
type Nav2Sig struct {
	Nav2SigFixed
	Sigs []Nav2SigInfo
}

func (m *Nav2Sig) ID() MsgID { return Nav2SigID }

// InitVaryingPart, FixedPart, VaryingPart implement VaryingMsg
```

The `CorFlags` correction source field (bits 2:0) is used for correction disambiguation in Part 2.

## Part 2: internal/casic extraction and dispatch

### Epoch keying for NAV2 messages

NAV2-SOL and NAV2-PVH embed `Nav2TOW` which implements `NavMsg`. Their `NavEpoch()` returns `uint32(tow)`. The existing `handleNavEpoch` in `casproc.go` already calls `NavEpoch()` on any `casbin.NavMsg`, so NAV2-SOL and NAV2-PVH will trigger epoch changes via TOW changes.

NAV2-DOP has no epoch key -- it is associated with the current epoch on arrival (same approach as NMEA GSA).

NAV2-TIMEUTC has no standard epoch key. It is dispatched independently without epoch association (same as V5 NavTimeUTC which dispatches TimeMsg directly).

NAV2-SAT and NAV2-SIG have a `TOW` field (U4, ms) in their fixed header. These should implement `NavMsg` by providing a `NavEpoch()` method.

### 2a: NAV2-TIMEUTC time extraction

New extraction function in `castime.go` (or a new `castime2.go`):

```go
func timeNav2TimeUTC(m *casbin.Nav2TimeUTC) *gpsprot.TimeMsg
```

Returns a `TimeMsg` with `NativeMsgID = "NAV2-TIMEUTC"`. Time construction from the V6 fields:
- Full fractional second: `(cs*10 + subcs + subms*2^-30)` milliseconds
- UTC time: year/month/day/hour/min/sec + fractional ms
- Only valid when `TFlags` has `Nav2TimeTOWValid` and `Nav2TimeReliable`
- Accuracy: `TAcc` is in nanoseconds (direct, not variance). Convert to `time.Duration`.
- GNSS: map `Nav2TimeSrc` to `gpsprot.GNSS` (GPS/BDS/GLN/GAL/IRN -> GPS/BDS/GLO/GAL/IRNSS)

#### Dispatch integration

Add `case *casbin.Nav2TimeUTC:` to `dispatch()`. Same pattern as existing `NavTimeUTC` case.

### 2b: NAV2-SOL position/velocity extraction

New file: `gps/internal/casic/caspv2.go` (and `caspv2_test.go`), or extend `caspv.go`.

Two extraction functions:

- `posECEFNav2Sol(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sol) *gpsprot.PosECEFMsg` -- returns nil when `FixFlags < Nav2Fix2D`. Position: `X/Y/Z` (float64, m) -> `Point3D` (Length in micrometres). Accuracy: `PAcc` (m, direct -- no sqrt) -> `ne.Acc.Pos`. Sets `NativeMsgID = "NAV2-SOL"`.
- `velECEFNav2Sol(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sol) *gpsprot.VelECEFMsg` -- returns nil when `VelFlags < Nav2Fix2D` (using the same threshold). Velocity: `VX/VY/VZ` (float32, m/s) -> `[3]Speed`. Accuracy: `SAcc` (m/s, direct) -> `ne.Acc.Speed`. Sets `NativeMsgID = "NAV2-SOL"`.

Quality fields on `ne` (using `PriVendorLow`):
- `FixLevel`, `FixDim`, `AuxSrc`, `Correction`: from `FixFlags` (see mapping table below)
- `NumSVUsed`: from `NumFixTot`
- `DOP.Pos`: from `PDOP`

Unit conversions (same as V5, but no sqrt for accuracy):
- Position: `Length(v * 1e6)` (metres to micrometres)
- Velocity: `Speed(v * 1e6)` (m/s to micrometres/s)
- Accuracy position: `Length(PAcc * 1e6)` (direct, not variance)
- Accuracy speed: `Speed(SAcc * 1e6)` (direct, not variance)

#### Dispatch integration

Add `case *casbin.Nav2Sol:` to `dispatch()`. Call both extraction functions and dispatch to handler.

### 2c: NAV2-PVH position/velocity extraction

Added to `caspv2.go` and `caspv2_test.go`.

Two extraction functions:

- `posGeoNav2Pvh(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Pvh) *gpsprot.PosGeoMsg` -- returns nil when `FixFlags < Nav2Fix2D`. Position: `Lat/Lon` (float64, deg) -> `[2]Angle` (nanodegrees). Height: `Height` (m) -> `Length`. HeightMSL: `Height - SepGeoid`. Accuracy: `HAcc` (m, direct) -> `ne.Acc.Hor`; `VAcc` (m, direct) -> `ne.Acc.Vert`. Sets `NativeMsgID = "NAV2-PVH"`.
- `velGeoNav2Pvh(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Pvh) *gpsprot.VelGeoMsg` -- returns nil when `VelFlags < Nav2Fix2D`. Velocity: `VelN`, `VelE`, `-VelU` (note negation for NED down) -> `VelNED`. `Speed2D` -> `GroundSpeed`. `Speed3D` -> `Speed3D`. `Heading` -> `Course`. Accuracy: `SAcc` (m/s, direct) -> `ne.Acc.Speed`; `CAcc` (deg, direct) -> `ne.Acc.Course`. Sets `NativeMsgID = "NAV2-PVH"`.

Quality fields on `ne` (using `PriVendorHigh` -- more detailed than NAV2-SOL):
- Same `FixFlags` mapping as NAV2-SOL
- `NumSVUsed`: from `NumFixTot`
- `DOP.Pos`: from `PDOP` (NAV2-PVH does not have a PDOP field -- use only from NAV2-SOL or NAV2-DOP)

Unit conversions (same as V5 caspv.go, but no sqrt for accuracy):
- Lat/Lon: `Angle(v * 1e9)` (degrees to nanodegrees)
- Height: `Length(v * 1e6)` (metres to micrometres)
- Velocity: `Speed(v * 1e6)` (m/s to micrometres/s)
- Course: `Angle(v * 1e9)` (degrees to nanodegrees)
- Accuracy: direct conversion, no sqrt

#### Dispatch integration

Add `case *casbin.Nav2Pvh:` to `dispatch()`. Call both extraction functions and dispatch to handler.

### 2d: NAV2-DOP extraction

Function: `dopNav2Dop(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Dop)` -- populates `ne.DOP` fields (PDOP, HDOP, VDOP, TDOP). Values are direct float32. NDOP/EDOP are not represented in `gpsprot.DOP`.

#### Dispatch integration

Add `case *casbin.Nav2Dop:` to `dispatch()`. NAV2-DOP does not implement `NavMsg` (no epoch key), so it is not tracked by `handleNavEpoch`. It writes to `p.curNavEpochMsg` directly. Return `true`.

### 2e: NAV2-SAT satellite extraction

Map NAV2-SAT to `SatellitesMsg` following the same pattern as the V5 `NavGPSInfo`/`NavBDSInfo`/`NavGLNInfo` satellite accumulator. The key difference: NAV2-SAT provides all constellations in a single message (via `GNSSID` per SV), whereas V5 has separate per-constellation messages.

Since NAV2-SAT already contains all constellations, no accumulation across messages is needed. A single extraction function converts `Nav2Sat` directly to `*gpsprot.SatellitesMsg`.

The `GNSSID` values (0=GPS, 1=BDS, 2=GLN, 3=GAL, 4=QZSS) map to `gpsprot.GNSS` values.

#### Dispatch integration

Add `case *casbin.Nav2Sat:` to `dispatch()`. Emit `SatellitesMsg` directly to handler.

### 2f: NAV2-SIG correction disambiguation

NAV2-SIG provides per-signal `CorFlags` (bits 2:0 = correction source) that can enrich the `Correction` bitmask on `curNavEpochMsg`. This is analogous to UBX NAV-SIG's `CorrSource`.

| CorFlags bits 2:0 | Description | CorrKind |
|--------------------|-------------|----------|
| 0 | NULL (no corrections) | (none) |
| 1 | SBAS | CorrSBAS |
| 2 | BDS (B2b PPP) | CorrWideArea |
| 3 | RTCM2 | CorrRTCM |
| 4 | OSR (observation-space) | CorrBaseStation \| CorrRTCM |
| 5 | SSR (state-space) | CorrWideArea \| CorrRTCM |

As NAV2-SIG signals are processed, accumulate `CorrKind` on `curNavEpochMsg` from per-signal correction sources of signals marked as used (`SolFlags` bit 0 = PR used). This enriches the base correction set from `FixFlags`.

#### Dispatch integration

Add `case *casbin.Nav2Sig:` to `dispatch()`. Process correction flags for quality enrichment, and optionally emit satellite tracking info (NumSVTracked from `NumTrkTot`).

### fixFlags -> NavEpochMsg mapping (NAV2-SOL and NAV2-PVH)

| fixFlags | Description | AuxSrc | FixLevel | FixDim | Correction |
|----------|-------------|--------|----------|--------|------------|
| 0 | Invalid | 0 | FixLevelNone | 0 | 0 |
| 1 | External input | 0 | FixLevelNotMeasured | 0 | 0 |
| 2 | Rough estimate | 0 | FixLevelNone | 0 | 0 |
| 3 | Maintaining / hold | 0 | FixLevelNone | 0 | 0 |
| 4 | Dead reckoning | AuxSrcDR | FixLevelNone | 0 | 0 |
| 5 | Quick mode | 0 | FixLevelCode | FixDim3D | 0 |
| 6 | 2D positioning | 0 | FixLevelCode | FixDim2D | 0 |
| 7 | 3D positioning | 0 | FixLevelCode | FixDim3D | 0 |
| 8 | DGPS | 0 | FixLevelCodeCorrected | FixDim3D | CorrUsed |
| 9 | RTK float | 0 | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 10 | RTK fixed | 0 | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |
| 15 | Timing fixed position | 0 | FixLevelNotMeasured | FixDimTimeOnly | 0 |

Notes:
- Values 0-7 share the same semantics as V5 `posValid`
- Values 8-10 and 15 are new in V6
- DGPS (8) sets `CorrUsed` only; the `fixFlags` alone does not distinguish base-station vs wide-area. NAV2-SIG `CorFlags` can disambiguate (see 2f above)
- RTK float (9) and fixed (10) assert `CorrBaseStation | CorrUsed`
- Fixed timing position (15) is a user-set or averaged position where only the clock is solved

### GNSSID mapping for Nav2TimeSrc

| Nav2TimeSrc | gpsprot.GNSS |
|-------------|--------------|
| 0 (GPS) | GPS |
| 1 (BDS) | BDS |
| 2 (GLN) | GLO |
| 3 (GAL) | GAL |
| 4 (IRN) | IRNSS |

The existing `casbin.GNSSID` only covers GPS/BDS/GLN. V6 adds GAL and IRN. The `gnssIDToGNSS` helper in `castime.go` needs extending, or a separate V6 mapping function.

### Testing pattern

Tests in `caspv2_test.go` construct casbin structs with known values, call extraction functions, and verify:
- gpsprot message fields (coordinates, velocities, course)
- Accuracy accumulation on `NavEpochMsg` (direct values, no sqrt)
- Quality field mapping from `fixFlags` (extended 0-15 scale)
- Nil return for invalid fix flags
- VelU negation for NED convention
- Correction disambiguation from NAV2-SIG `CorFlags`

### Fields not available from V6 CASIC binary

- **SignalsUsed**: derive from NAV2-SIG per-signal `SigID` and `SolFlags` (bit 0 = PR used), analogous to UBX NAV-SIG.
- **NumSVTracked**: from NAV2-SAT `NumViewTot` or NAV2-SIG `NumTrkTot`.
- **DiffAge, RTCMRefBaseID**: not available from V6 CASIC binary. Only available via NMEA GGA when enabled alongside binary.

### Message enablement

Message file tags in `configs/gpsmsg/atgm332d-v6.toml` (already exists for NAV2-SOL and NAV2-TIMEUTC):
- `casbin-nav2-sol`: enable NAV2-SOL (already defined)
- `casbin-nav2-timeutc`: enable NAV2-TIMEUTC (already defined)

New tags to add:
- `casbin-nav2-pvh`: enable NAV2-PVH at 1Hz (class 0x11, id 0x03, rate 1)
- `casbin-nav2-dop`: enable NAV2-DOP at 1Hz (class 0x11, id 0x01, rate 1)
- `casbin-nav2-sat`: enable NAV2-SAT at 1Hz (class 0x11, id 0x04, rate 1)
- `casbin-nav2-sig`: enable NAV2-SIG at 1Hz (class 0x11, id 0x06, rate 1)

### Implementation order

1. 1a (Nav2TOW) + 1b (Nav2FixFlags) + 1c (Nav2GnssMask) -- types, `make test`
2. 1d (Nav2Sol) + 1e (Nav2Pvh) + 1f (Nav2Dop) -- structs, `make test`
3. 1g (Nav2TimeUTC) -- struct, `make test`
4. 1h (Nav2Sat) + 1i (Nav2Sig) -- variable-length structs, `make test`
5. 2a (NAV2-TIMEUTC time extraction + dispatch) -- `make test`
6. 2b (NAV2-SOL extraction + dispatch) -- `make test`
7. 2c (NAV2-PVH extraction + dispatch) -- `make test`
8. 2d (NAV2-DOP extraction) -- `make test`
9. 2e (NAV2-SAT satellite extraction) -- `make test`
10. 2f (NAV2-SIG correction disambiguation) -- `make test`
11. Add message file tags to `atgm332d-v6.toml` -- `make test`
