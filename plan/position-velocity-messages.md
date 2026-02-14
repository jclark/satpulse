# Position and velocity messages

## Motivation

The current `MsgHandler` interface provides `TimeMsg`, `LeapSecondMsg`, `SurveyMsg`, and `SatellitesMsg`, but has no standard messages for position or velocity. Position and velocity are currently only accessible through protocol-specific `NativeMsgHandler` callbacks. Adding four protocol-neutral message types enables applications to consume position/velocity data without knowing the underlying protocol.

This is a precursor to `SolutionMetaMsg` (see `plan/solution-metadata.md`). The position/velocity messages are emitted with minimal latency (immediately when the protocol message is parsed), while `SolutionMetaMsg` is synthesized at epoch boundaries from metadata that may arrive in different messages. The underlying protocol messages often serve both purposes: for example, UBX `NavPVT` provides position, velocity AND fix quality; NMEA `GGA` provides position AND quality indicator. By defining the position/velocity messages first, the `SolutionMetaMsg` implementation can reuse the same parsed data without re-parsing.

GNSS protocols consistently separate geodetic (LLH position / NED velocity) from earth-centered (ECEF position / ECEF velocity) coordinate frames. The four message types mirror this natural separation.

## Design: four message types

### Coordinate frame pairing

- **Geodetic**: position as latitude/longitude/height (LLH); velocity in the local tangent plane as north/east/down (NED) or equivalently as ground speed + course over ground.
- **ECEF**: position and velocity in the Earth-Centered, Earth-Fixed frame.

No protocol mixes these frames in a single message. Each message type is only emitted when the receiver reports a valid result; there is no "invalid" state within these messages.

### New unit type: `Speed`

A `Speed` type is needed for velocity fields, following the pattern of `Length` (micrometers) and `Angle` (nanodegrees). The base unit is micrometers per second, so `Speed` is dimensionally consistent with `Length / time.Second`.

```go
// Speed represents a speed in micrometers per second.
type Speed int64

const (
	MicrometerPerSecond Speed = 1
	MillimeterPerSecond Speed = 1000 * MicrometerPerSecond
	CentimeterPerSecond Speed = 10 * MillimeterPerSecond
	MeterPerSecond      Speed = 100 * CentimeterPerSecond
)
```

With conversion methods `MetersPerSecond() float64`, `String()`, and a constructor `MetersPerSecondFromFloat(float64) Speed`, following the pattern of `Length.Meters()` / `Meters(float64)` and `Angle.Degrees()` / `DegreesFromFloat(float64)`.

Placed in `gps/gpsprot/types.go` alongside `Length` and `Angle`.

### Accuracy fields

Accuracy estimates are included in the position/velocity messages as `opt.Val` fields, because every protocol that provides accuracy bundles it with the value itself. NMEA provides no metric accuracy (only DOPs in GSA), so the fields simply stay unset.

- `PosGeoMsg`: `HAcc`, `VAcc` (horizontal, vertical position accuracy)
- `PosECEFMsg`: `PAcc` (3D position accuracy)
- `VelGeoMsg`: `SAcc` (speed accuracy), `CAcc` (course accuracy)
- `VelECEFMsg`: `SAcc` (speed accuracy)

DOPs remain in `SolutionMetaMsg` since they come from separate messages (e.g. NMEA GSA, UBX NavDOP).

### `SolutionEngine` (future enhancement)

Receivers like Unicore/NovAtel can output multiple simultaneous solutions per epoch from different processing engines (BESTNAV, PPPNAV, SPPNAV, RTKPOS). These produce genuinely different coordinates in the same epoch. `FixQuality` + `CorrKind` describe *what the solution achieved*, but not *which engine produced it* — when BESTNAV happens to select the PPP engine, it's indistinguishable from PPPNAV by quality alone.

`SolutionEngine` identifies the processing engine. It appears on all four position/velocity message types and on `SolutionMetaMsg`, linking each solution's coordinates to its metadata.

```go
// SolutionEngine identifies which processing engine produced a navigation
// solution. Receivers may run multiple engines concurrently (e.g. SPP, RTK,
// PPP) and select a "best" result. The zero value means the receiver's
// preferred/only engine (the common case for single-engine receivers like
// u-blox, and for NovAtel/Unicore BEST* messages).
type SolutionEngine uint8

const (
	SolnEngineBest SolutionEngine = iota // receiver's preferred/auto-selected
	SolnEngineSPP                        // single point positioning
	SolnEngineRTK                        // RTK (base-station referenced)
	SolnEnginePPP                        // PPP (wide-area corrections)
)
```

Protocol mapping:
- **UBX**: always `SolnEngineBest` (single engine)
- **NMEA**: always `SolnEngineBest` (single solution)
- **Unicore/NovAtel**: `BEST*` → `SolnEngineBest`, `SPP*` → `SolnEngineSPP`, `RTK*` → `SolnEngineRTK`, `PPP*` → `SolnEnginePPP`

Implementation is deferred until Unicore/NovAtel multi-solution support is needed. For now, all messages implicitly come from the receiver's single/best engine.

### Message types

All four message types include `Tag` and `NativeMsgID` fields for protocol identification, following the existing `TimeMsg` / `SatellitesMsg` pattern. They contain pure coordinates with no quality/accuracy metadata. When `SolutionEngine` is implemented, an `Engine` field will be added.

Each protocol message independently emits position/velocity messages when it has valid data — no dedup between messages. This follows the `TimeMsg` precedent where multiple sources can emit time messages independently.

#### `PosGeoMsg`

Geodetic position (latitude, longitude, height above WGS-84 ellipsoid).

`Height` and `HeightMSL` are optional because some sources (e.g. NMEA RMC) provide only lat/lon without height.

```go
type PosGeoMsg struct {
	LatLon    [2]Angle        `json:"latLon"`              // [lat, lon]; lat positive north, lon positive east
	Height    opt.Val[Length]  `json:"height,omitzero"`    // above WGS-84 ellipsoid
	HeightMSL opt.Val[Length]  `json:"heightMSL,omitzero"` // above mean sea level
	HAcc      opt.Val[Length]  `json:"hAcc,omitzero"`      // horizontal position accuracy
	VAcc      opt.Val[Length]  `json:"vAcc,omitzero"`      // vertical position accuracy
	Tag         Tag    `json:"tag"`
	NativeMsgID string `json:"nativeMsgID"`
}
```

#### `PosECEFMsg`

Earth-Centered, Earth-Fixed position.

```go
type PosECEFMsg struct {
	Pos  Point3D        `json:"pos"`            // ECEF X, Y, Z
	PAcc opt.Val[Length] `json:"pAcc,omitzero"` // 3D position accuracy
	Tag         Tag    `json:"tag"`
	NativeMsgID string `json:"nativeMsgID"`
}
```

#### `VelGeoMsg`

Velocity in the local geodetic frame (NED components and/or ground speed + course over ground).

Some fields are `opt.Val` because different protocol messages provide different subsets. Some provide only ground speed and course; others provide NED components, ground speed, 3D speed, and course. At least one velocity field will always be set.

NED components are grouped as `opt.Val[[3]Speed]` because they always come as a complete triple — no source provides only one or two components.

```go
type VelGeoMsg struct {
	VelNED      opt.Val[[3]Speed] `json:"velNED,omitzero"`      // north, east, down
	GroundSpeed opt.Val[Speed]    `json:"groundSpeed,omitzero"` // 2D ground speed
	Speed3D     opt.Val[Speed]    `json:"speed3D,omitzero"`     // 3D speed
	Course      opt.Val[Angle]    `json:"course,omitzero"`      // course over ground, true north
	SAcc        opt.Val[Speed]    `json:"sAcc,omitzero"`        // speed accuracy
	CAcc        opt.Val[Angle]    `json:"cAcc,omitzero"`        // course accuracy
	Tag         Tag    `json:"tag"`
	NativeMsgID string `json:"nativeMsgID"`
}
```

#### `VelECEFMsg`

Velocity in the ECEF frame.

```go
type VelECEFMsg struct {
	Vel  [3]Speed       `json:"vel"`            // ECEF VX, VY, VZ
	SAcc opt.Val[Speed]  `json:"sAcc,omitzero"` // speed accuracy
	Tag         Tag    `json:"tag"`
	NativeMsgID string `json:"nativeMsgID"`
}
```

### MsgHandler interface changes

Add four methods to `MsgHandler`:

```go
type MsgHandler interface {
	Time(msg *TimeMsg, tRead time.Time)
	LeapSecond(msg *LeapSecondMsg, tRead time.Time)
	Survey(msg *SurveyMsg, tRead time.Time)
	Satellites(msg *SatellitesMsg, tRead time.Time)
	PosGeo(msg *PosGeoMsg, tRead time.Time)
	PosECEF(msg *PosECEFMsg, tRead time.Time)
	VelGeo(msg *VelGeoMsg, tRead time.Time)
	VelECEF(msg *VelECEFMsg, tRead time.Time)
}
```

`DefaultHandler` gets four empty method implementations. `MultiHandler` gets four fan-out methods.

### Implementations that embed `DefaultHandler`

The following types embed `DefaultHandler` and will automatically satisfy the expanded interface without changes:

- `gps/app/gpscfg/gpscfg.go` (the main app handler)
- `gps/internal/nmea/nmea_test.go` (`timeHandler`)
- `gps/internal/nmea/nmeasats_test.go` (`testSatellitesBufferMsgHandler`, `testPacketHandler`)

The following test handler does NOT embed `DefaultHandler` and needs updating:

- `gps/internal/ubx/ubx_test.go` (`testMsgHandler`) -- add four empty methods

## Implementation steps

### Step 0: Factor out shared types into `types.go`

Create `gps/gpsprot/types.go` and move types shared between `configtarget.go` and `msg.go` into it. From `configtarget.go`: `Length`, `Point3D`, and `Angle` (used by the new message types). From `msg.go`: `GNSS` (used by `configtarget.go`).

### Step 1: Add `Speed` type

In `gps/gpsprot/types.go`, after `Angle` and before `Point3D`:

- `Speed` type with constants and methods following the `Length`/`Angle` pattern
- No `ParseSpeed` needed initially (not used in configuration)

### Step 2: Add message types and handler methods

In `gps/gpsprot/msg.go`:

- Add four message struct types
- Extend `MsgHandler` interface with four new methods
- Add four empty `DefaultHandler` methods
- Add four `MultiHandler` fan-out methods
- Import `opt` package

### Step 3: Fix test handlers

In `gps/internal/ubx/ubx_test.go`:

- Add four empty methods to `testMsgHandler`

### Step 4: UBX protocol extraction

New files: `gps/internal/ubx/ubxpv.go`, `gps/internal/ubx/ubxpv_test.go`. Each substep includes a unit test for the new extraction and a `make test` to confirm nothing is broken.

#### Extraction function pattern

Extraction functions take a `ubxbin` struct and return a `gpsprot` message pointer. They set `NativeMsgID` but not `Tag` (Dispatch sets `Tag`). They return `nil` when the message has no valid data. This matches the existing pattern in `ubxtime.go` (e.g. `timeNavTimeGPS(*ubxbin.NavTimeGPS) *gpsprot.TimeMsg`).

For messages that produce multiple outputs (4c: NAV-PVT), use separate functions per output type, each returning a single message pointer.

#### Unit conversions

Units verified against u-blox F10 SPG 6.00 interface description.

**Position** (POSECEF, POSLLH, PVT):

| ubxbin field | ubxbin unit | gpsprot type | conversion |
|---|---|---|---|
| `ECEF [3]int32` | cm | `Point3D` (`[3]Length`) | `Length(v) * Centimeter` |
| `PAcc uint32` | cm | `opt.Val[Length]` | `opt.Make(Length(v) * Centimeter)` |
| `Lat/Lon int32` | 1e-7 deg | `Angle` | `Angle(v) * 100 * Nanodegrees` |
| `Height/HMSL int32` | mm | `Length` | `Length(v) * Millimeter` |
| `HAcc/VAcc uint32` | mm | `opt.Val[Length]` | `opt.Make(Length(v) * Millimeter)` |

**Velocity -- VELECEF, VELNED** (cm/s):

| ubxbin field | ubxbin unit | gpsprot type | conversion |
|---|---|---|---|
| `ECEFV [3]int32` | cm/s | `[3]Speed` | `Speed(v) * CentimeterPerSecond` |
| `VelNED [3]int32` | cm/s | `opt.Val[[3]Speed]` | `Speed(v) * CentimeterPerSecond` |
| `Speed uint32` | cm/s | `opt.Val[Speed]` | `Speed(v) * CentimeterPerSecond` | → `Speed3D` |
| `GSpeed uint32` | cm/s | `opt.Val[Speed]` | `Speed(v) * CentimeterPerSecond` |
| `SAcc uint32` | cm/s | `opt.Val[Speed]` | `opt.Make(Speed(v) * CentimeterPerSecond)` |
| `Heading int32` | 1e-5 deg | `opt.Val[Angle]` | `Angle(v) * 10000 * Nanodegrees` | → `Course` |
| `CAcc uint32` | 1e-5 deg | `opt.Val[Angle]` | `opt.Make(Angle(v) * 10000 * Nanodegrees)` | → `CAcc` |

**Velocity -- PVT** (mm/s, different from VELNED/VELECEF):

| ubxbin field | ubxbin unit | gpsprot type | conversion |
|---|---|---|---|
| `VelN/VelE/VelD int32` | mm/s | `opt.Val[[3]Speed]` | `Speed(v) * MillimeterPerSecond` |
| `GSpeed int32` | mm/s | `opt.Val[Speed]` | `Speed(v) * MillimeterPerSecond` |
| `SAcc uint32` | mm/s | `opt.Val[Speed]` | `opt.Make(Speed(v) * MillimeterPerSecond)` |
| `HeadMot int32` | 1e-5 deg | `opt.Val[Angle]` | `Angle(v) * 10000 * Nanodegrees` | → `Course` |
| `HeadAcc uint32` | 1e-5 deg | `opt.Val[Angle]` | `opt.Make(Angle(v) * 10000 * Nanodegrees)` | → `CAcc` |

#### Dispatch pattern

Position/velocity cases use the early-return pattern (like `NavTimeLS`), calling the handler inline and returning `true`. This avoids modifying the existing `time`/`sv`/`sats` dispatch logic at the bottom of `Dispatch`.

```go
case *ubxbin.NavPosECEF:
	msg := posECEF(mt)
	if msg != nil && h != nil {
		msg.Tag = Tag
		h.PosECEF(msg, tRead)
	}
	return msg != nil
```

#### Testing pattern

Tests in `ubxpv_test.go` test extraction functions directly: construct a `ubxbin` struct with known values, call the extraction function, verify the `gpsprot` message fields. This tests unit conversion logic without needing serialization or a mock handler.

```go
func TestPosECEF(t *testing.T) {
	m := &ubxbin.NavPosECEF{
		ECEF: [3]int32{-267173351, -402753274, 391919498}, // cm
		PAcc: 1543, // cm
	}
	got := posECEF(m)
	if got == nil {
		t.Fatal("expected non-nil PosECEFMsg")
	}
	// Check ECEF coordinates (cm -> Length in micrometers)
	want := gpsprot.Point3D{
		gpsprot.Length(-267173351) * gpsprot.Centimeter,
		gpsprot.Length(-402753274) * gpsprot.Centimeter,
		gpsprot.Length(391919498) * gpsprot.Centimeter,
	}
	if got.Pos != want {
		t.Errorf("Pos = %v, want %v", got.Pos, want)
	}
	// Check PAcc
	wantPAcc := gpsprot.Length(1543) * gpsprot.Centimeter
	if v, ok := got.PAcc.Get(); !ok || v != wantPAcc {
		t.Errorf("PAcc = %v, want %v", got.PAcc, wantPAcc)
	}
	// Check NativeMsgID
	if got.NativeMsgID != "UBX-NAV-POSECEF" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "UBX-NAV-POSECEF")
	}
}
```

The same pattern applies to all five substeps: construct input, call function, verify output fields.

#### Substeps

- **4a: NAV-POSECEF** — `posECEF(*ubxbin.NavPosECEF) *gpsprot.PosECEFMsg`. Fields: `ECEF` (cm) → `Pos`, `PAcc` (cm) → `PAcc`. Always returns non-nil (no validity flags).
- **4b: NAV-VELECEF** — `velECEF(*ubxbin.NavVelECEF) *gpsprot.VelECEFMsg`. Fields: `ECEFV` (cm/s) → `Vel`, `SAcc` (cm/s) → `SAcc`. Always returns non-nil.
- **4c: NAV-PVT** — two functions: `posGeoNavPVT(*ubxbin.NavPVT) *gpsprot.PosGeoMsg` and `velGeoNavPVT(*ubxbin.NavPVT) *gpsprot.VelGeoMsg`. Returns nil when fix is invalid (`FixType < NavPVT2DFix` or `Flags & NavPVTGNSSFixOK == 0`). Also returns nil for position when `Flags3 & NavPVTInvalidLlh != 0`. The existing `timeNavPVT` case in Dispatch must be extended to also call these two functions and dispatch PosGeo/VelGeo alongside the TimeMsg. Position fields: `Lat/Lon` (1e-7 deg), `Height/HMSL` (mm), `HAcc/VAcc` (mm). Velocity fields (all mm/s, not cm/s): `VelN/VelE/VelD` (mm/s) → `VelNED`, `GSpeed` (mm/s) → `GroundSpeed`, `HeadMot` (1e-5 deg) → `Course`, `SAcc` (mm/s) → `SAcc`, `HeadAcc` (1e-5 deg) → `CAcc`.
- **4d: NAV-POSLLH** — `posLLH(*ubxbin.NavPosLLH) *gpsprot.PosGeoMsg`. Fields: `Lat/Lon` (1e-7 deg), `Height/HMSL` (mm), `HAcc/VAcc` (mm). Always returns non-nil.
- **4e: NAV-VELNED** — `velNED(*ubxbin.NavVelNED) *gpsprot.VelGeoMsg`. Fields: `VelNED` (cm/s) → `VelNED`, `Speed` (cm/s) → `Speed3D`, `GSpeed` (cm/s) → `GroundSpeed`, `Heading` (1e-5 deg) → `Course`, `SAcc` (cm/s) → `SAcc`, `CAcc` (1e-5 deg) → `CAcc`. Always returns non-nil.

#### Cross-message position consistency test

After 4a, 4c, and 4d are implemented, add a test in `ubxpv_test.go` that verifies the three position message types (NAV-POSECEF, NAV-POSLLH, NAV-PVT) produce consistent ECEF positions when fed real captured data from a stationary antenna.

**Capturing testdata** (three separate captures, since `--pvt-out` can only select one position message type at a time per the logic in `ubxcfgmsg.go`):

```bash
# NAV-POSECEF
satpulsetool gps -d /dev/ttyACM0 -s 38400 --pvt-out pos,ecef,off --capture 5 --packet-log /tmp/posecef.jsonl

# NAV-POSLLH
satpulsetool gps -d /dev/ttyACM0 -s 38400 --pvt-out pos,off --capture 5 --packet-log /tmp/posllh.jsonl

# NAV-PVT
satpulsetool gps -d /dev/ttyACM0 -s 38400 --pvt-out pos,vel,off --capture 5 --packet-log /tmp/pvt.jsonl
```

**Extracting testdata with jq** (extract hex binary for the relevant message types):

```bash
jq -r 'select(.msg == "NAV-POSECEF" and .out == false) | .bin' /tmp/posecef.jsonl > testdata/posecef.hex
jq -r 'select(.msg == "NAV-POSLLH" and .out == false) | .bin' /tmp/posllh.jsonl > testdata/posllh.hex
jq -r 'select(.msg == "NAV-PVT" and .out == false) | .bin' /tmp/pvt.jsonl > testdata/pvt.hex
```

Each file has one hex-encoded UBX packet per line. Commit these in `gps/internal/ubx/testdata/`.

**Test structure:**

1. Read hex lines from each testdata file, decode to binary, parse with `ubxbin.ParseMsg`.
2. Run the extraction functions: `posECEF()` on each NAV-POSECEF, `posLLH()` on each NAV-POSLLH, `posGeoNavPVT()` on each NAV-PVT.
3. Collect all ECEF positions: NAV-POSECEF gives ECEF directly; for NAV-POSLLH and NAV-PVT, convert LLH to ECEF using `geopos.WGS84.LLHtoECEF()`.
4. Compute the centroid of all ECEF positions.
5. Verify every position is within `posConsistencyTolerance` (const, initially 1m) of the centroid. This checks that all three message types agree on where the antenna is.

### Step 5: NMEA protocol extraction

Added to existing `nmea/nmea.go`; tests in existing `nmea/nmea_test.go`. Each substep includes a unit test for the new extraction and a `make test` to confirm nothing is broken.

- **5a: RMC** — emit `PosGeoMsg` with latLon (height unset); emit `VelGeoMsg` with groundSpeed, course.
- **5b: GGA** — emit `PosGeoMsg` with latLon, height, heightMSL.
- **5c: VTG** — emit `VelGeoMsg` with groundSpeed, course.

### Step 6: Allystar protocol extraction

New files: `as/aspv.go`, `as/aspv_test.go`. Each substep includes a unit test for the new extraction and a `make test` to confirm nothing is broken.

- **6a: NavPosECEF** — emit `PosECEFMsg`.
- **6b: NavPosLLH** — emit `PosGeoMsg` with all fields.
- **6c: NavVelECEF** — emit `VelECEFMsg`.
- **6d: NavVelNED** — emit `VelGeoMsg` with velNED, groundSpeed, speed3D, course.

### Step 7: CASIC protocol extraction

New files: `casic/caspv.go`, `casic/caspv_test.go`. Each substep includes a unit test for the new extraction and a `make test` to confirm nothing is broken.

- **7a: NAV-SOL** — emit `PosECEFMsg` (when PosValid >= NavPos2D) and `VelECEFMsg` (when VelValid >= NavVel2D).
- **7b: NAV-PV** — define `NavPv` struct in `casbin/nav.go`; emit `PosGeoMsg` and `VelGeoMsg`.

### Step 8: Unicore protocol extraction

New files: `unc/nav.go`, `unc/nav_test.go`. Each substep includes a unit test for the new extraction and a `make test` to confirm nothing is broken.

- **8a: BESTNAV** — emit `PosGeoMsg` (latLon, height) and `VelGeoMsg` (groundSpeed, course).
- **8b: BESTNAVXYZ** — emit `PosECEFMsg` and `VelECEFMsg`.

### Step 9 (future): SSE monitoring event

A `PosVelEvent` SSE event would combine all four position/velocity message types into a single event for the web UI / monitoring. It fires once per epoch, triggered by `SolutionMetaMsg` (which marks the epoch boundary). The handler would cache the most recent position/velocity messages and snapshot them when `SolutionMetaMsg` arrives. This naturally includes the solution metadata itself, giving consumers a complete per-epoch view: coordinates + quality.

