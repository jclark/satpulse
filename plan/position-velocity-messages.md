# Position and velocity messages

## Motivation

The current `MsgHandler` interface provides `TimeMsg`, `LeapSecondMsg`, `SurveyMsg`, and `SatellitesMsg`, but has no standard messages for position or velocity. Position and velocity are currently only accessible through protocol-specific `NativeMsgHandler` callbacks. Adding four protocol-neutral message types enables applications to consume position/velocity data without knowing the underlying protocol.

This is a precursor to `SolutionMetaMsg` (see `plan/solution-metadata.md`). The position/velocity messages are emitted with minimal latency (immediately when the protocol message is parsed), while `SolutionMetaMsg` is synthesized at epoch boundaries from metadata that may arrive in different messages. The underlying protocol messages often serve both purposes: for example, UBX `NavPVT` provides position, velocity AND fix quality; NMEA `GGA` provides position AND quality indicator. By defining the position/velocity messages first, the `SolutionMetaMsg` implementation can reuse the same parsed data without re-parsing.

GNSS protocols consistently separate geodetic (LLH position / NED velocity) from earth-centered (ECEF position / ECEF velocity) coordinate frames. The four message types mirror this natural separation.

## Design: four message types

### Coordinate frame pairing

- **Geodetic**: position as latitude/longitude/height (LLH); velocity in the local tangent plane as north/east/down (NED) or equivalently as ground speed + course.
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

Placed in `gps/gpsprot/configtarget.go` alongside `Length` and `Angle`.

### Accuracy fields

Accuracy estimates (HAcc, VAcc, PAcc, SAcc, CAcc, TAcc) are NOT included in the position/velocity messages. They belong in `SolutionMetaMsg` alongside the DOP fields, because:

- Accuracy and DOPs are both "how good is this solution" metrics, just at different granularity. They share frame-dependence (HDOP/HAcc/VAcc are geodetic-frame; PDOP/PAcc are frame-independent). Putting DOPs in one place and accuracy in another is not justifiable.
- Accuracy changes much more slowly than coordinates themselves (it reflects solution geometry and correction state, which evolve over seconds to minutes). The slight extra latency of epoch-boundary emission via `SolutionMetaMsg` is acceptable.
- NMEA provides no metric accuracy estimates at all (only DOPs), so these fields would be unused for NMEA anyway.
- `TimeMsg.Accuracy` would also move to `SolutionMetaMsg` (as TAcc) to maintain consistency.

This means `SolutionMetaMsg` gains these additional `opt.Val` fields: HAcc, VAcc (geodetic position), PAcc (ECEF position), SAcc (speed), CAcc (course), TAcc (time).

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

#### `PosGeoMsg`

Geodetic position (latitude, longitude, height above WGS-84 ellipsoid).

`Height` and `HeightMSL` are optional because some sources (e.g. NMEA RMC) provide only lat/lon without height.

```go
type PosGeoMsg struct {
	LatLon    [2]Angle        `json:"latLon"`              // [lat, lon]; lat positive north, lon positive east
	Height    opt.Val[Length]  `json:"height,omitzero"`    // above WGS-84 ellipsoid
	HeightMSL opt.Val[Length]  `json:"heightMSL,omitzero"` // above mean sea level
	Tag         Tag    `json:"tag,omitzero"`
	NativeMsgID string `json:"nativeMsgID,omitempty"`
}
```

Sources: UBX `NavPosLLH` (all fields), UBX `NavPVT` (all fields), NMEA `GGA` (latLon, height, heightMSL), NMEA `RMC` (latLon only), Unicore `BESTNAV` (latLon, height).

#### `PosECEFMsg`

Earth-Centered, Earth-Fixed position.

```go
type PosECEFMsg struct {
	Pos  Point3D `json:"pos"` // ECEF X, Y, Z
	Tag         Tag    `json:"tag,omitzero"`
	NativeMsgID string `json:"nativeMsgID,omitempty"`
}
```

Sources: UBX `NavPosECEF`.

#### `VelGeoMsg`

Velocity in the local geodetic frame (NED components and/or ground speed + course).

Some fields are `opt.Val` because different protocols provide different subsets. NMEA gives only ground speed and course; UBX `NavVelNED` gives NED components, ground speed, 3D speed, and heading; UBX `NavPVT` gives NED components and ground speed. At least one velocity field will always be set.

NED components are grouped as `opt.Val[[3]Speed]` because they always come as a complete triple — no protocol provides only one or two components.

```go
type VelGeoMsg struct {
	VelNED      opt.Val[[3]Speed] `json:"velNED,omitzero"`      // north, east, down
	GroundSpeed opt.Val[Speed]    `json:"groundSpeed,omitzero"` // 2D ground speed
	Speed3D     opt.Val[Speed]    `json:"speed3D,omitzero"`     // 3D speed
	Course      opt.Val[Angle]    `json:"course,omitzero"`      // track over ground, true north
	Tag         Tag    `json:"tag,omitzero"`
	NativeMsgID string `json:"nativeMsgID,omitempty"`
}
```

Sources: UBX `NavVelNED` (all fields), UBX `NavPVT` (velNED, groundSpeed), NMEA `RMC` (groundSpeed, course), NMEA `VTG` (groundSpeed, course), Unicore `BESTNAV` (groundSpeed, course).

#### `VelECEFMsg`

Velocity in the ECEF frame.

```go
type VelECEFMsg struct {
	Vel  [3]Speed `json:"vel"` // ECEF VX, VY, VZ
	Tag         Tag    `json:"tag,omitzero"`
	NativeMsgID string `json:"nativeMsgID,omitempty"`
}
```

Sources: UBX `NavVelECEF`.

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

### Step 1: Add `Speed` type

In `gps/gpsprot/configtarget.go`, after the `Angle` type and before `Point3D`:

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

### Protocol extraction (steps 4–7)

Each sentence/message independently emits position/velocity messages when it has valid data — no dedup between sentences. This follows the `TimeMsg` precedent where multiple sources (RMC, ZDA) can emit time messages independently.

Each substep includes a unit test for the new extraction and a `make test` to confirm nothing is broken.

### Step 4: NMEA protocol extraction

Added to existing `nmea/nmea.go`; tests in existing `nmea/nmea_test.go`.

- **4a: RMC** — emit `PosGeoMsg` with latLon (height unset); emit `VelGeoMsg` with groundSpeed, course.
- **4b: GGA** — emit `PosGeoMsg` with latLon, height, heightMSL.
- **4c: VTG** — emit `VelGeoMsg` with groundSpeed, course.

### Step 5: UBX protocol extraction

New files: `ubx/ubxpv.go`, `ubx/ubxpv_test.go`. Dispatched from the existing `Dispatch` method.

- **5a: NAV-POSECEF** — emit `PosECEFMsg`.
- **5b: NAV-VELECEF** — emit `VelECEFMsg`.
- **5c: NAV-PVT** — emit `PosGeoMsg` and `VelGeoMsg` (when valid).
- **5d: NAV-POSLLH** — emit `PosGeoMsg` with all fields.
- **5e: NAV-VELNED** — emit `VelGeoMsg` with velNED, groundSpeed, speed3D, course.

### Step 6: Allystar protocol extraction

New files: `as/aspv.go`, `as/aspv_test.go`.

- **6a: NavPosECEF** — emit `PosECEFMsg`.
- **6b: NavPosLLH** — emit `PosGeoMsg` with all fields.
- **6c: NavVelECEF** — emit `VelECEFMsg`.
- **6d: NavVelNED** — emit `VelGeoMsg` with velNED, groundSpeed, speed3D, course.

### Step 7: CASIC protocol extraction

New files: `casic/caspv.go`, `casic/caspv_test.go`.

- **7a: NAV-SOL** — emit `PosECEFMsg` (when PosValid >= NavPos2D) and `VelECEFMsg` (when VelValid >= NavVel2D).
- **7b: NAV-PV** — define `NavPv` struct in `casbin/nav.go`; emit `PosGeoMsg` and `VelGeoMsg`.

### Step 8: Unicore protocol extraction

New files: `unc/nav.go`, `unc/nav_test.go`.

- **8a: BESTNAV** — emit `PosGeoMsg` (latLon, height) and `VelGeoMsg` (groundSpeed, course).
- **8b: BESTNAVXYZ** — emit `PosECEFMsg` and `VelECEFMsg`.

### Step 9 (future): SSE monitoring event

A `PosVelEvent` SSE event would combine all four position/velocity message types into a single event for the web UI / monitoring. It fires once per epoch, triggered by `SolutionMetaMsg` (which marks the epoch boundary). The handler would cache the most recent position/velocity messages and snapshot them when `SolutionMetaMsg` arrives. This naturally includes the solution metadata itself, giving consumers a complete per-epoch view: coordinates + quality.

## Open questions

1. **Naming**: `VelGeo` vs `VelNED` -- "Geo" is used here for consistency with the position message naming and the "geodetic vs earth-centered" framing. NED velocity is technically in the local tangent plane derived from the geodetic position, not "geodetic" per se. Alternative: `VelLocal`.

2. **`Speed` placement**: Proposed in `configtarget.go` next to `Length`/`Angle` for consistency, even though the file name suggests configuration. An alternative would be a new `units.go` file, but that would orphan `Length`/`Angle` in `configtarget.go`.
