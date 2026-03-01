# CASIC V5 position, velocity, and solution quality

CASIC V5 (class 0x01 NAV) firmware runs on ZKW single-frequency receivers. This plan covers adding position, velocity, nav epoch, and solution quality support for V5 binary messages.

Prerequisite: position/velocity message types (`PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`) and `MsgHandler` methods must already exist in `gpsprot` (see [position-velocity-messages.md](position-velocity-messages.md)).

## Part 1: casbin message parsing

### 1a: NAV-PV struct (0x01 0x03, 80 bytes)

Add `NavPv` to `casbin/nav.go`. Move `NavPvID` from `other.go` to `nav.go`. Register in `init()`.

The existing `NavPosValid`/`NavVelValid` types and constants (used by `NavSol`) are reused.

```go
// NavPv is NAV-PV (0x01 0x03) - geodetic position and velocity (80 bytes)
type NavPv struct {
	NavRunTime
	PosValid  NavPosValid
	VelValid  NavVelValid
	System    NavSystem
	NumSV     uint8
	NumSVGPS  uint8
	NumSVBDS  uint8
	NumSVGLN  uint8
	_         uint8   // reserved
	PDOP      float32
	Lon       float64 // deg
	Lat       float64 // deg
	Height    float32 // m, ellipsoidal
	SepGeoid  float32 // m, geoid separation (ellipsoidal minus MSL)
	HAcc      float32 // m^2, variance of horizontal position accuracy
	VAcc      float32 // m^2, variance of vertical position accuracy
	VelN      float32 // m/s
	VelE      float32 // m/s
	VelU      float32 // m/s (UP, not down -- negate for NED)
	Speed3D   float32 // m/s
	Speed2D   float32 // m/s, ground speed
	Heading   float32 // deg
	SAcc      float32 // (m/s)^2, variance of ground speed accuracy
	CAcc      float32 // deg^2, variance of heading accuracy
}
```

### 1b: NAV-DOP struct (0x01 0x01, 28 bytes)

Add `NavDop` to `casbin/nav.go`. Move `NavDopID` from `other.go` to `nav.go`. Register in `init()`.

```go
// NavDop is NAV-DOP (0x01 0x01) - dilution of precision (28 bytes)
type NavDop struct {
	NavRunTime
	PDOP float32
	HDOP float32
	VDOP float32
	NDOP float32
	EDOP float32
	TDOP float32
}
```

## Part 2: internal/casic extraction and dispatch

### 2a: Add `curNavEpochMsg` to `PacketProcessor`

Add a `curNavEpochMsg *gpsprot.NavEpochMsg` field to `PacketProcessor` in `casproc.go`. Allocate a fresh `NavEpochMsg` at the start of each epoch in `handleNavEpoch` (same pattern as UBX/Allystar). Update `FlushNavEpoch` to return `curNavEpochMsg` with `Tag=CASIC` and `PriVendorLow` (instead of returning nil).

### 2b: NAV-SOL position/velocity extraction

New file: `gps/internal/casic/caspv.go` (and `caspv_test.go`).

Two extraction functions:

- `posECEFNavSol(ne *gpsprot.NavEpochMsg, m *casbin.NavSol) *gpsprot.PosECEFMsg` -- returns nil when `PosValid < NavPos2D`. Position: `ECEFX/Y/Z` (float64, m) -> `Point3D` (Length in micrometers). Accuracy: `sqrt(PAcc)` (m) -> `ne.Acc.Pos`. Sets `NativeMsgID = "NAV-SOL"`.
- `velECEFNavSol(ne *gpsprot.NavEpochMsg, m *casbin.NavSol) *gpsprot.VelECEFMsg` -- returns nil when `VelValid < NavVel2D`. Velocity: `ECEFVX/Y/VZ` (float32, m/s) -> `[3]Speed` (micrometers/s). Accuracy: `sqrt(SAcc)` (m/s) -> `ne.Acc.Speed`. Sets `NativeMsgID = "NAV-SOL"`.

Quality fields on `ne` (populated alongside position extraction):
- `FixLevel`, `FixDim`, `AuxSrc`: from `PosValid` (see mapping table below)
- `NumSVUsed`: from `NumSV`
- `DOP.Pos`: from `PDOP`

Unit conversions:
- Position: `Length(v * 1e6)` (metres to micrometres via float64)
- Velocity: `Speed(v * 1e6)` (m/s to micrometres/s via float32)
- Accuracy: `sqrt(variance)` then convert to Length/Speed as above

#### NAV-SOL dispatch integration

The existing `case *casbin.NavSol:` in `dispatch()` currently only extracts time. Extend it to also call `posECEFNavSol` and `velECEFNavSol`, dispatching `PosECEF`/`VelECEF` to the handler alongside the existing `Time` dispatch. The time extraction continues to use `PriVendorLow`.

### 2c: NAV-PV position/velocity extraction

Added to `caspv.go` and `caspv_test.go`.

Two extraction functions:

- `posGeoNavPv(ne *gpsprot.NavEpochMsg, m *casbin.NavPv) *gpsprot.PosGeoMsg` -- returns nil when `PosValid < NavPos2D`. Position: `Lat/Lon` (float64, deg) -> `[2]Angle` (nanodegrees). Height: `Height` (float32, m) -> `Length` (set in `opt.Val`). HeightMSL: `Height - SepGeoid`. Accuracy: `sqrt(HAcc)` (m) -> `ne.Acc.Hor`; `sqrt(VAcc)` (m) -> `ne.Acc.Vert`. Sets `NativeMsgID = "NAV-PV"`.
- `velGeoNavPv(ne *gpsprot.NavEpochMsg, m *casbin.NavPv) *gpsprot.VelGeoMsg` -- returns nil when `VelValid < NavVel2D`. Velocity: `VelN`, `VelE`, `-VelU` (m/s, note negation of Up to Down) -> `VelNED`. `Speed2D` (m/s) -> `GroundSpeed`. `Speed3D` (m/s) -> `Speed3D`. `Heading` (deg) -> `Course`. Accuracy: `sqrt(SAcc)` (m/s) -> `ne.Acc.Speed`; `sqrt(CAcc)` (deg) -> `ne.Acc.Course`. Sets `NativeMsgID = "NAV-PV"`.

Quality fields on `ne` (populated alongside position extraction, using `PriVendorHigh` since NAV-PV is more detailed than NAV-SOL):
- `FixLevel`, `FixDim`, `AuxSrc`: from `PosValid` (same mapping as NAV-SOL)
- `NumSVUsed`: from `NumSV`
- `DOP.Pos`: from `PDOP`

Unit conversions:
- Lat/Lon: `Angle(v * 1e9)` (degrees to nanodegrees via float64)
- Height: `Length(v * 1e6)` (metres to micrometres via float32)
- Velocity: `Speed(v * 1e6)` (m/s to micrometres/s via float32)
- Course: `Angle(v * 1e9)` (degrees to nanodegrees via float32)
- Accuracy (position): `sqrt(variance) * 1e6` for Length
- Accuracy (speed): `sqrt(variance) * 1e6` for Speed
- Accuracy (course): `sqrt(variance) * 1e9` for Angle

#### NAV-PV dispatch integration

Add `case *casbin.NavPv:` to `dispatch()`. Call `posGeoNavPv` and `velGeoNavPv`, dispatch to handler with `Tag = Tag`.

### 2d: NAV-DOP extraction

Added to `caspv.go` (or a separate `casdop.go` if cleaner).

Function: `dopNavDop(ne *gpsprot.NavEpochMsg, m *casbin.NavDop)` -- populates `ne.DOP` fields (PDOP, HDOP, VDOP, TDOP). NDOP and EDOP are not represented in `gpsprot.DOP`. Values are direct float32 (no scaling needed, unlike UBX which uses 0.01 scale).

#### NAV-DOP dispatch integration

Add `case *casbin.NavDop:` to `dispatch()`. Call `dopNavDop` with `p.curNavEpochMsg`. No handler callback needed (DOP is part of NavEpochMsg, emitted at epoch flush). Return `true`.

### posValid -> NavEpochMsg mapping (shared by NAV-SOL and NAV-PV)

| posValid | Description | AuxSrc | FixLevel | FixDim |
|----------|-------------|--------|----------|--------|
| 0 | Invalid | 0 | FixLevelNone | 0 |
| 1 | External input | 0 | FixLevelNotMeasured | 0 |
| 2 | Rough estimate | 0 | FixLevelNone | 0 |
| 3 | Maintaining last | 0 | FixLevelNone | 0 |
| 4 | Dead reckoning | AuxSrcDR | FixLevelNone | 0 |
| 5 | Quick mode | 0 | FixLevelCode | FixDim3D |
| 6 | 2D positioning | 0 | FixLevelCode | FixDim2D |
| 7 | 3D positioning | 0 | FixLevelCode | FixDim3D |
| 8 | GNSS+DR | AuxSrcDR | FixLevelCode | FixDim3D |

V5 CASIC binary cannot distinguish correction levels (no DGPS/RTK indicators). `Correction` is always left unset. NMEA GGA/RMC (when enabled alongside binary) can provide correction information via the NMEA synthesis path.

### Testing pattern

Tests in `caspv_test.go` construct casbin structs with known values, call extraction functions, and verify:
- gpsprot message fields (coordinates, velocities, course)
- Accuracy accumulation on `NavEpochMsg`
- Quality field mapping (FixLevel, FixDim, AuxSrc)
- Nil return for invalid positions/velocities
- VelU negation for NED convention

### Fields not available from V5 CASIC binary

- **DiffAge, RTCMRefBaseID, SignalsUsed**: not available. Only available via NMEA GGA when enabled alongside binary.
- **NumSVTracked**: not directly available. NAV-GPSINFO/BDSINFO/GLNINFO provide `NumViewSV` per constellation, but these are already used for `SatellitesMsg` and would require summing across constellations.

### Message enablement

CASIC uses programmatic configuration via `CFG-MSG` (0x06 0x01) to set per-message output rates. Message enablement is via message files (`configs/gpsmsg/`). Tags:
- `casbin-nav-pv`: enable NAV-PV at 1Hz
- `casbin-nav-dop`: enable NAV-DOP at 1Hz
- NAV-SOL is already enabled for time and carries position/velocity at no additional cost

### Implementation order

1. 1a (NavPv struct) + 1b (NavDop struct) -- casbin changes, `make test`
2. 2a (curNavEpochMsg) -- infrastructure, `make test`
3. 2b (NAV-SOL extraction + dispatch) -- `make test`
4. 2c (NAV-PV extraction + dispatch) -- `make test`
5. 2d (NAV-DOP extraction) -- `make test`
