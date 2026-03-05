# CASIC V5 position, velocity, and solution quality

CASIC V5 (class 0x01 NAV) firmware runs on ZKW single-frequency receivers. This plan covers adding position, velocity, nav epoch, and solution quality support for V5 binary messages.

Prerequisite: position/velocity message types (`PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`) and `MsgHandler` methods must already exist in `gpsprot` (see [position-velocity-messages.md](position-velocity-messages.md)).

## Part 1: casbin message parsing

### 1a: NAV-PV struct (0x01 0x03, 80 bytes)

Add `NavPv` to `casbin/nav.go`. Move `NavPvID` constant from `other.go` to `nav.go` (alongside the other NAV IDs like `NavSolID`). Remove the `idNameMap[NavPvID] = "PV"` line from `other.go` `init()`. Register in `nav.go` `init()` with `regMsg[NavPv]("PV")` (this registers both the factory and the name).

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

Add `NavDop` to `casbin/nav.go`. Move `NavDopID` constant from `other.go` to `nav.go`. Remove the `idNameMap[NavDopID] = "DOP"` line from `other.go` `init()`. Register in `nav.go` `init()` with `regMsg[NavDop]("DOP")`.

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

Follow the Allystar `asproc.go` pattern (simpler than UBX):

Add `curNavEpochMsg *gpsprot.NavEpochMsg` field to `PacketProcessor` in `casproc.go`.

In `handleNavEpoch`, after `p.curNavEpoch = e`:
```go
p.curNavEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
```

Update `FlushNavEpoch`:
```go
func (p *PacketProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	p.satAccum.epochChange(p.mh, tRead)
	msg := p.curNavEpochMsg
	p.curNavEpochMsg = nil
	if msg != nil {
		msg.Tag = Tag
	}
	return msg, gpsprot.PriVendorLow, p.mh
}
```

Note: `curNavEpochMsg` is guaranteed non-nil when extraction functions are called, because `NavSol`/`NavPv`/`NavDop` all embed `NavRunTime` (implementing `casbin.NavMsg`), so `handleNavEpoch` runs first and creates the epoch msg.

### 2b: NAV-SOL position/velocity extraction

New file: `gps/internal/casic/caspv.go` (and `caspv_test.go`).

Two extraction functions:

- `posECEFNavSol(ne *gpsprot.NavEpochMsg, m *casbin.NavSol) *gpsprot.PosECEFMsg` -- returns nil when `PosValid < NavPos2D`. Position: `ECEFX/Y/Z` (float64, m) -> `Point3D` (Length in micrometers). Accuracy: `sqrt(PAcc)` (m) -> `ne.Acc.Pos`. Sets `NativeMsgID = "NAV-SOL"`.
- `velECEFNavSol(ne *gpsprot.NavEpochMsg, m *casbin.NavSol) *gpsprot.VelECEFMsg` -- returns nil when `VelValid < NavVel2D`. Velocity: `ECEFVX/Y/VZ` (float32, m/s) -> `[3]Speed` (micrometers/s). Accuracy: `sqrt(SAcc)` (m/s) -> `ne.Acc.Speed`. Sets `NativeMsgID = "NAV-SOL"`.

Quality fields on `ne` (populated alongside position extraction):
- `FixLevel`, `FixDim`, `AuxSrc`: from `PosValid` (see mapping table below)
- `NumSVUsed`: from `NumSV`
- `DOP.Pos`: from `PDOP`

Unit conversions use existing `gpsprot` float constructors (unlike UBX/Allystar which use integer-scaled helpers):
- Position: `gpsprot.Meters(v)` for float64 metres
- Velocity: `gpsprot.MetersPerSecondFromFloat(float64(v))` for float32 m/s
- Accuracy: `gpsprot.Meters(math.Sqrt(float64(variance)))` for position, `gpsprot.MetersPerSecondFromFloat(math.Sqrt(float64(variance)))` for speed

Both NAV-SOL and NAV-PV use `Set` for accuracy fields (last writer wins within the epoch).

Quality is extracted via a shared function `qualityFromPosValid(ne, posValid, numSV, pdop)` used by both NAV-SOL and NAV-PV (same mapping table, see below).

#### NAV-SOL dispatch integration

The existing `case *casbin.NavSol:` in `dispatch()` currently only extracts time. Extend it to also call `posECEFNavSol` and `velECEFNavSol`. Restructure to follow the Allystar dispatch pattern (declare local vars, dispatch all non-nil results):

```go
case *casbin.NavSol:
	tm := timeNavSol(mt)
	posE := posECEFNavSol(p.curNavEpochMsg, mt)
	velE := velECEFNavSol(p.curNavEpochMsg, mt)
	if h != nil {
		if tm != nil {
			tm.Tag = Tag
			h.Time(tm, tRead)
		}
		if posE != nil {
			posE.Tag = Tag
			posE.Priority = gpsprot.PriVendorLow
			h.PosECEF(posE, tRead)
		}
		if velE != nil {
			velE.Tag = Tag
			velE.Priority = gpsprot.PriVendorLow
			h.VelECEF(velE, tRead)
		}
	}
	return true
```

Note: `timeNavSol` always returns non-nil (empty TimeMsg for invalid fix), so NAV-SOL always returns true. This preserves existing behaviour.

### 2c: NAV-PV position/velocity extraction

Added to `caspv.go` and `caspv_test.go`.

Two extraction functions:

- `posGeoNavPv(ne *gpsprot.NavEpochMsg, m *casbin.NavPv) *gpsprot.PosGeoMsg` -- returns nil when `PosValid < NavPos2D`. Position: `Lat/Lon` (float64, deg) -> `[2]Angle` (nanodegrees). Height: `Height` (float32, m) -> `Length` (set in `opt.Val`). HeightMSL: `Height - SepGeoid` (omitted when `SepGeoid` is 0, since CASIC V5 does not provide geoid separation). Accuracy: `sqrt(HAcc)` (m) -> `ne.Acc.Hor`; `sqrt(VAcc)` (m) -> `ne.Acc.Vert`. Sets `NativeMsgID = "NAV-PV"`.
- `velGeoNavPv(ne *gpsprot.NavEpochMsg, m *casbin.NavPv) *gpsprot.VelGeoMsg` -- returns nil when `VelValid < NavVel2D`. Velocity: `VelN`, `VelE`, `-VelU` (m/s, note negation of Up to Down) -> `VelNED`. `Speed2D` (m/s) -> `GroundSpeed`. `Speed3D` (m/s) -> `Speed3D`. `Heading` (deg) -> `Course`. Accuracy: `sqrt(SAcc)` (m/s) -> `ne.Acc.Speed`; `sqrt(CAcc)` (deg) -> `ne.Acc.Course`. Sets `NativeMsgID = "NAV-PV"`.

Quality fields on `ne` (populated alongside position extraction):
- `FixLevel`, `FixDim`, `AuxSrc`: from `PosValid` (same mapping as NAV-SOL)
- `NumSVUsed`: from `NumSV`
- `DOP.Pos`: from `PDOP`

Unit conversions use the same `gpsprot` float constructors as NAV-SOL, plus:
- Lat/Lon: `gpsprot.DegreesFromFloat(v)` for float64 degrees
- Height: `gpsprot.Meters(float64(v))` for float32 metres
- Course: `gpsprot.DegreesFromFloat(float64(v))` for float32 degrees
- Accuracy (course): `gpsprot.DegreesFromFloat(math.Sqrt(float64(variance)))` for heading accuracy

All messages use `PriVendorLow` for dispatch priority.

#### NAV-PV dispatch integration

Add `case *casbin.NavPv:` to `dispatch()`. Follow the Allystar pattern:

```go
case *casbin.NavPv:
	posG := posGeoNavPv(p.curNavEpochMsg, mt)
	velG := velGeoNavPv(p.curNavEpochMsg, mt)
	if posG == nil && velG == nil {
		return false
	}
	if h != nil {
		if posG != nil {
			posG.Tag = Tag
			posG.Priority = gpsprot.PriVendorLow
			h.PosGeo(posG, tRead)
		}
		if velG != nil {
			velG.Tag = Tag
			velG.Priority = gpsprot.PriVendorLow
			h.VelGeo(velG, tRead)
		}
	}
	return true
```

NAV-PV uses `PriVendorLow`, same as NAV-SOL.

### 2d: NAV-DOP extraction

Added to `caspv.go`.

```go
func dopNavDop(ne *gpsprot.NavEpochMsg, m *casbin.NavDop) {
	ne.DOP.Pos = opt.Make(float64(m.PDOP))
	ne.DOP.Hor = opt.Make(float64(m.HDOP))
	ne.DOP.Vert = opt.Make(float64(m.VDOP))
	ne.DOP.Time = opt.Make(float64(m.TDOP))
}
```

NDOP and EDOP are not represented in `gpsprot.DOP`. Values are direct float32 (no 0.01 scaling like UBX).

#### NAV-DOP dispatch integration

Add to `dispatch()`, same pattern as UBX:
```go
case *casbin.NavDop:
	dopNavDop(p.curNavEpochMsg, mt)
	return true
```

No handler callback needed -- DOP is part of NavEpochMsg, emitted at epoch flush.

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

NAV-PV and NAV-DOP are enabled via `CFG-MSG` tags (`casbin-nav-pv`, `casbin-nav-dop`) in the message file. NAV-SOL is already enabled for time and carries position/velocity at no additional cost.

### Implementation order

0. Capture real NAV-PV and NAV-DOP packets from hardware to use as test data for struct parsing.
1. 1a (NavPv struct) + 1b (NavDop struct) -- casbin changes, `go test -v ./gps/lib/casbin/` (use captured packets for test data)
2. 2a (curNavEpochMsg) -- infrastructure, `make test`
3. 2b (NAV-SOL extraction + dispatch) -- `go test -v ./gps/internal/casic/`
4. 2c (NAV-PV extraction + dispatch) -- `go test -v ./gps/internal/casic/`
5. 2d (NAV-DOP extraction) -- `go test -v ./gps/internal/casic/`
6. Hardware test with CASIC V5 receiver at /dev/ttyUSB1 (9600 baud)
