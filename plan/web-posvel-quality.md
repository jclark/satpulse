# Web front end: position/velocity and solution quality SSE events

Prerequisite: [position-velocity-messages.md](position-velocity-messages.md) (adds `PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg` and `MsgHandler` methods). Implemented.

Prerequisite: [solution-quality.md](solution-quality.md) (adds quality fields to `NavEpochMsg`: `FixLevel`, `FixDim`, `CorrKind`, `AuxSrc`, `DOP`, `Accuracy`, `DiffAge`, `RTCMRefBaseID`, `NumSVUsed`, `NumSVTracked`, `SignalsUsed`). Implemented.

## Motivation

The web dashboard currently shows time, PHC clock state, survey-in status, receiver info, and satellite signals. It has no position, velocity, or solution quality information. With the position/velocity messages and `NavEpochMsg` quality fields now implemented in the GPS protocol layer, the web frontend can display a complete picture of the navigation solution: where the receiver is, how fast it's moving, how good the fix is, and what corrections are in use.

## Design

Two new SSE event types are added:

1. **`posvel`** — position and velocity, emitted once per navigation epoch
2. **`quality`** — solution quality metadata, emitted once per navigation epoch

Both are triggered by the `NavEpoch` callback, which marks the epoch boundary. The `SSEObserver` accumulates position/velocity messages as they arrive during the epoch, then snapshots them into the `posvel` event when `NavEpoch` fires. The `quality` event is derived directly from the `NavEpochMsg` fields.

### Why two events instead of one

Position/velocity data and solution quality data serve different UI purposes. The position card shows coordinates, height, and motion; the quality card shows fix type, corrections, accuracy, and DOPs. Separating them allows the frontend to render independent cards that appear/disappear based on data availability. It also keeps each event focused and small.

### Why emit at epoch boundary

Position and velocity messages arrive independently during an epoch (e.g. NAV-POSLLH, NAV-VELNED, or combined NAV-PVT). Some protocols send multiple messages per epoch (ECEF + geodetic). Emitting at the epoch boundary (when `NavEpoch` arrives) ensures:

- All position/velocity messages for the epoch have been received
- Priority-based merging has selected the best source
- The quality metadata is complete (DOPs, correction info, SV counts)
- A single coherent snapshot per epoch is sent to the frontend

## SSE data structures

### Design principles for SSE types

The SSE types are not wire copies of the `gpsprot` message types. They are simplified for JSON transport and web display:

- **SI units as float64**: `Length` (micrometers) becomes meters, `Speed` (micrometers/sec) becomes m/s, `Angle` (nanodegrees) becomes degrees. This avoids the frontend needing to know about internal unit representations.
- **`opt.Val[T]` becomes JSON omitzero**: Fields use `opt.Val[float64]` (or `opt.Val[uint16]`, etc.) so they are omitted from JSON when unset, rather than appearing as null or zero. The frontend checks for field presence.
- **Keywords for fix and corrections**: The fix description is split into two fields. `Fix` combines `FixLevel`, `FixDim`, and `AuxSrc` — these are self-explanatory keywords ("carrierFixed", "3D", "INS"). `Corrections` holds the `CorrKind` leaf items — keywords like "baseStation", "used", "SBAS" that only make sense in a corrections context. This avoids the frontend needing to understand the enum semantics or the `CorrKind` partial order.

### `PosVelSSE`

```go
// PosVelSSE is the SSE event data for position and velocity.
// Emitted once per navigation epoch. Fields are omitted when
// no data is available for that category.
type PosVelSSE struct {
	// Geodetic position
	LatLon    opt.Val[[2]float64] `json:"latLon,omitzero"`    // [lat, lon] degrees
	Height    opt.Val[float64]    `json:"height,omitzero"`    // meters above WGS-84 ellipsoid
	HeightMSL opt.Val[float64]    `json:"heightMSL,omitzero"` // meters above mean sea level

	// ECEF position (only if no geodetic position, or if ECEF-only protocol)
	ECEF opt.Val[[3]float64] `json:"ecef,omitzero"` // [X, Y, Z] meters

	// Velocity
	GroundSpeed opt.Val[float64]    `json:"groundSpeed,omitzero"` // m/s
	Speed3D     opt.Val[float64]    `json:"speed3D,omitzero"`     // m/s
	Course      opt.Val[float64]    `json:"course,omitzero"`      // degrees, true north
	VelNED      opt.Val[[3]float64] `json:"velNED,omitzero"`      // [north, east, down] m/s
}
```

Notes:
- All optional fields use `opt.Val` with `omitzero`, so unset fields are omitted from JSON.
- Geodetic position is preferred over ECEF. If both are available, only geodetic is sent (ECEF is redundant and harder to display). ECEF is included only for protocols that provide it exclusively.
- All units are SI floats: meters, m/s, degrees.

### `QualitySSE`

```go
// QualitySSE is the SSE event data for solution quality metadata.
// Emitted once per navigation epoch.
type QualitySSE struct {
	// Fix describes the current fix using keywords from FixLevel,
	// FixDim, and AuxSrc (in that order). Examples:
	//   ["carrierFixed", "3D"]
	//   ["code", "3D"]
	//   ["none"]
	//   ["carrierFloat", "3D", "INS"]
	Fix []string `json:"fix"`
	// Corrections describes the corrections applied. CorrKind marshals
	// as a JSON array of leaf keywords (minimal representation, omitting
	// implied bits). Examples: ["fullDualFreq"], ["SBAS"], ["used"].
	// Zero value omitted.
	Corrections gpsprot.CorrKind `json:"corrections,omitzero"`

	// Accuracy estimates (meters, m/s, degrees)
	AccHor         opt.Val[float64] `json:"accHor,omitzero"`         // horizontal position accuracy, meters
	AccVert        opt.Val[float64] `json:"accVert,omitzero"`        // vertical position accuracy, meters
	AccPos         opt.Val[float64] `json:"accPos,omitzero"`         // 3D position accuracy, meters
	AccSpeed       opt.Val[float64] `json:"accSpeed,omitzero"`       // 3D speed accuracy, m/s
	AccGroundSpeed opt.Val[float64] `json:"accGroundSpeed,omitzero"` // 2D ground speed accuracy, m/s
	AccCourse      opt.Val[float64] `json:"accCourse,omitzero"`      // course accuracy, degrees

	// Dilution of precision
	GDOP opt.Val[float64] `json:"gdop,omitzero"`
	PDOP opt.Val[float64] `json:"pdop,omitzero"`
	HDOP opt.Val[float64] `json:"hdop,omitzero"`
	VDOP opt.Val[float64] `json:"vdop,omitzero"`
	TDOP opt.Val[float64] `json:"tdop,omitzero"`

	// Satellite counts
	NumSVUsed    opt.Val[uint16] `json:"numSVUsed,omitzero"`
	NumSVTracked opt.Val[uint16] `json:"numSVTracked,omitzero"`

	// Signals used in the solution.
	// JSON: {"GPS": ["L1", "L5"], "GAL": ["E1", "E5b"]}
	// Uses SignalSet's MarshalJSON (to be added, using GNSSSignalMap/ParseSignalMap).
	SignalsUsed gpsprot.SignalSet `json:"signalsUsed,omitzero"`

	// Correction metadata
	DiffAge       opt.Val[float64] `json:"diffAge,omitzero"`       // seconds
	RTCMRefBaseID opt.Val[uint16]  `json:"rtcmRefBaseID,omitzero"`
}
```

### Fix and corrections keyword assembly

The `Fix` field combines FixLevel, FixDim, and AuxSrc (in that order). These are self-explanatory keywords that describe the fix technique, dimensionality, and additional sources:

- **FixLevel**: "none", "notMeasured", "code", "codeCorrected", "carrierFloat", "carrierFixed"
- **FixDim**: "2D", "3D", "timeOnly", "velocityOnly"
- **AuxSrc**: "DR", "INS"

If FixLevel is zero (not provided), `Fix` is an empty list. Otherwise, FixLevel is always present; FixDim and AuxSrc contribute keywords only when non-zero.

The `Corrections` field holds the CorrKind leaf items (minimal representation omitting implied bits). These keywords only make sense in a corrections context ("used", "baseStation", "SBAS", etc.):

- **CorrKind** (leaf items): "used", "baseStation", "wideArea", "RTCM", "partialDualFreq", "fullDualFreq", "SBAS", "CLAS", "SPARTN", "PPP", "PPP-RTK", "PPPConverging", "PPPConverged"

Example mappings:

| Receiver state | Fix | Corrections |
|---|---|---|
| Standalone 3D fix | `["code", "3D"]` | `[]` |
| RTK fixed, narrow-lane | `["carrierFixed", "3D"]` | `["fullDualFreq"]` |
| RTK float | `["carrierFloat", "3D"]` | `["baseStation"]` |
| SBAS corrected | `["codeCorrected", "3D"]` | `["SBAS"]` |
| PPP converging with INS | `["carrierFloat", "3D", "INS"]` | `["PPPConverging"]` |
| No fix | `["none"]` | `[]` |
| Dead reckoning only | `["none", "DR"]` | `[]` |
| Not provided | `[]` | `[]` |

## Go implementation

### Accumulator in `SSEObserver`

`NavEpochAccum` (in `gpsprot`) already implements `MsgHandler` for `PosGeo`, `PosECEF`, `VelGeo`, `VelECEF`, and `Time`, merging incoming messages into a `MsgBundle` by priority. Its `NavEpoch` method clears the bundle. The `SSEObserver` embeds `NavEpochAccum` to get the accumulation for free, and overrides `NavEpoch` to emit SSE events before clearing.

Currently `SSEObserver` embeds `obs.DefaultObserver` (which embeds `gpsprot.DefaultHandler`). The change replaces `DefaultHandler` with `NavEpochAccum` (which itself embeds `DefaultHandler`). The `SSEObserver` already overrides `Time` and `Satellites` and `Survey`; those continue to work. The `PosGeo`/`PosECEF`/`VelGeo`/`VelECEF` methods are inherited from `NavEpochAccum` and accumulate automatically.

```go
type SSEObserver struct {
	gpsprot.NavEpochAccum // accumulates PosGeo, PosECEF, VelGeo, VelECEF
	sseCh     chan<- sse.Event
	lg        *slog.Logger
	lastTime  ptime.Time
	ls        ptime.LeapSecond
	initEvent sse.Event
}
```

Note: `SSEObserver` no longer embeds `obs.DefaultObserver`. It gets `DefaultHandler` through `NavEpochAccum`, and the `phcsync.Sampler`, `ReopenLog`, and `Release` methods that `obs.DefaultObserver` provided are implemented directly (as they already are for `Sample` and `Release`; `ReopenLog` needs a no-op stub).

### `NavEpoch` handler

The `NavEpoch` override emits both events, then calls the embedded `NavEpochAccum.NavEpoch` to clear the bundle:

```go
func (o *SSEObserver) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	pv := buildPosVelSSE(&o.Bundle)
	if pv != nil {
		o.sendSSE("posvel", pv)
	}
	o.sendSSE("quality", buildQualitySSE(msg))
	o.NavEpochAccum.NavEpoch(msg, tRead) // clears Bundle
}
```

### `buildPosVelSSE`

Converts the accumulated `MsgBundle` into `PosVelSSE`:

```go
func buildPosVelSSE(b *gpsprot.MsgBundle) *PosVelSSE {
	if b.PosGeo == nil && b.PosECEF == nil && b.VelGeo == nil && b.VelECEF == nil {
		return nil
	}
	var pv PosVelSSE
	if b.PosGeo != nil {
		pv.LatLon.Set([2]float64{
			b.PosGeo.LatLon[0].Degrees(),
			b.PosGeo.LatLon[1].Degrees(),
		})
		setOptMeters(&pv.Height, &b.PosGeo.Height)
		setOptMeters(&pv.HeightMSL, &b.PosGeo.HeightMSL)
	} else if b.PosECEF != nil {
		pv.ECEF.Set([3]float64{
			b.PosECEF.Pos[0].Meters(),
			b.PosECEF.Pos[1].Meters(),
			b.PosECEF.Pos[2].Meters(),
		})
	}
	if b.VelGeo != nil {
		setOptMPS(&pv.GroundSpeed, &b.VelGeo.GroundSpeed)
		setOptMPS(&pv.Speed3D, &b.VelGeo.Speed3D)
		setOptDeg(&pv.Course, &b.VelGeo.Course)
		if b.VelGeo.VelNED.IsSet() {
			ned := b.VelGeo.VelNED.Get()
			pv.VelNED.Set([3]float64{
				ned[0].MetersPerSecond(),
				ned[1].MetersPerSecond(),
				ned[2].MetersPerSecond(),
			})
		}
	}
	return &pv
}
```

### `buildQualitySSE`

Converts `NavEpochMsg` into `QualitySSE`:

```go
func buildQualitySSE(msg *gpsprot.NavEpochMsg) *QualitySSE {
	q := &QualitySSE{
		Fix:           buildFixKeywords(msg),
		Corrections:   msg.Correction,
		NumSVUsed:     msg.NumSVUsed,
		NumSVTracked:  msg.NumSVTracked,
		RTCMRefBaseID: msg.RTCMRefBaseID,
	}
	// Accuracy: Length -> meters, Speed -> m/s, Angle -> degrees
	setOptMeters(&q.AccHor, &msg.Acc.Hor)
	setOptMeters(&q.AccVert, &msg.Acc.Vert)
	setOptMeters(&q.AccPos, &msg.Acc.Pos)
	setOptMPS(&q.AccSpeed, &msg.Acc.Speed)
	setOptMPS(&q.AccGroundSpeed, &msg.Acc.GroundSpeed)
	setOptDeg(&q.AccCourse, &msg.Acc.Course)
	// DOPs: pass through directly
	q.GDOP = msg.DOP.Geom
	q.PDOP = msg.DOP.Pos
	q.HDOP = msg.DOP.Hor
	q.VDOP = msg.DOP.Vert
	q.TDOP = msg.DOP.Time
	// DiffAge: Duration -> seconds
	if msg.DiffAge.IsSet() {
		q.DiffAge.Set(msg.DiffAge.Get().Seconds())
	}
	q.SignalsUsed = msg.SignalsUsed
	return q
}
```

### `buildFixKeywords`

```go
func buildFixKeywords(msg *gpsprot.NavEpochMsg) []string {
	if msg.FixLevel == 0 {
		return nil
	}
	var kw []string
	kw = append(kw, msg.FixLevel.String())
	if msg.FixDim != 0 {
		kw = append(kw, msg.FixDim.String())
	}
	kw = append(kw, msg.AuxSrc.Items()...)
	return kw
}
```

The `Corrections` field is assigned directly from `msg.Correction`; `CorrKind.MarshalJSON()` handles the leaf-only serialization.

Note: `AuxSrc.items()` is currently unexported. It needs to be exported as `Items()`. The existing internal callers (`String()`, `MarshalJSON()`) are updated to call `Items()`.

### Unit conversion helpers

Small helpers to convert `opt.Val[Length]`, `opt.Val[Speed]`, `opt.Val[Angle]` to `opt.Val[float64]`:

```go
func setOptMeters(dst *opt.Val[float64], src *opt.Val[gpsprot.Length]) {
	if src.IsSet() {
		dst.Set(src.Get().Meters())
	}
}

func setOptMPS(dst *opt.Val[float64], src *opt.Val[gpsprot.Speed]) {
	if src.IsSet() {
		dst.Set(src.Get().MetersPerSecond())
	}
}

func setOptDeg(dst *opt.Val[float64], src *opt.Val[gpsprot.Angle]) {
	if src.IsSet() {
		dst.Set(src.Get().Degrees())
	}
}
```

## Web frontend

### Event registration

Add `"posvel"` and `"quality"` to the `EVENT_TYPES` array in `dashboard.tsx`:

```typescript
const EVENT_TYPES = ["satellites", "time", "phc", "survey", "receiver", "init", "posvel", "quality"] as const;
```

### Position card

Both new cards use the existing `PropertyCard` / `EventFormat` / `addFields` infrastructure. The `posvel` event is rendered using a `posvelFormat` object:

```typescript
const posvelFormat: EventFormat = {
    latLon: formatLL,                                        // reuse from survey card: clickable Maps link
    height: ["Altitude", formatAlt],                         // reuse from survey card
    heightMSL: ["Altitude (MSL)", formatAlt],
    ecef: formatECEF3,                                       // fallback when no geodetic position
    groundSpeed: ["Ground speed", (arg: number) => `${arg.toFixed(2)} m/s`],
    course: ["Course", (arg: number) => `${arg.toFixed(1)}\u00b0`],
    speed3D: ["3D speed", (arg: number) => `${arg.toFixed(2)} m/s`],
};
```

This reuses `formatLL` (clickable Google Maps link) and `formatAlt` (`X.XX m`) from the survey card for consistency. In due course the coordinates will be replaced by a map view (cf. desktop-gui branch).

### Solution quality card

The `quality` event is rendered using a `qualityFormat` object:

```typescript
const qualityFormat: EventFormat = {
    fix: ["Fix", (kw: string[]) => kw.join(" ")],
    corrections: ["Corrections", formatCorrections],         // keywords with icons
    accHor: ["Accuracy (H)", (arg: number) => `${arg.toFixed(3)} m`],
    accVert: ["Accuracy (V)", (arg: number) => `${arg.toFixed(3)} m`],
    hdop: ["HDOP"],
    vdop: ["VDOP"],
    pdop: ["PDOP"],
    numSVUsed: formatSVCounts,                               // "12 used / 24 tracked"
    signalsUsed: ["Signals", formatSignalsUsed],             // map -> "GPS: L1, L5; GAL: E1"
    diffAge: ["Diff age", (arg: number) => `${arg.toFixed(1)} s`],
    rtcmRefBaseID: ["Base station"],
};
```

The `formatCorrections` function maps correction keywords to display strings, using Unicode icons where appropriate (e.g. "used" as a checkmark). The `formatSVCounts` complex formatter reads both `numSVUsed` and `numSVTracked` from the event object.

Fields are only shown when present (JSON omitzero means absent fields won't appear in the event data).

### Card ordering

The dashboard renders cards in this order:

1. Sky view (if look angles available)
2. Signal levels
3. **Position** (new) — `PropertyCard` with `posvelFormat`
4. **Solution quality** (new) — `PropertyCard` with `qualityFormat`
5. Current GPS time
6. PTP Hardware Clock
7. Receiver
8. Survey-in status

### No init event changes needed

These events arrive every second (at each navigation epoch), so new clients will see data almost immediately. No caching in the init event is needed.

## Changes needed to `gpsprot`

Export `AuxSrc.items()`:

- Rename `func (a AuxSrc) items() []string` to `func (a AuxSrc) Items() []string`
- Update internal callers (`String()`, `MarshalJSON()`) to use `Items()`

(`CorrKind.items()` stays unexported — `CorrKind` is used directly as a field and its `MarshalJSON()` handles serialization.)

Add JSON marshalling to `SignalSet` in `signal.go`:

- `MarshalJSON()`: marshal via `GNSSSignalMap()` (produces `{"GPS":["L1","L5"],"GAL":["E1","E5b"]}`)
- `UnmarshalJSON()`: unmarshal via `ParseSignalMap()`

## Phasing

### Phase 1: Go backend

1. Export `AuxSrc.Items()` in `gps/gpsprot/msg.go`. Add `SignalSet.MarshalJSON()`/`UnmarshalJSON()` in `signal.go`.
2. Add `PosVelSSE` and `QualitySSE` structs to `time/internal/sseobs/sse.go`.
3. Replace `obs.DefaultObserver` embedding in `SSEObserver` with `gpsprot.NavEpochAccum` embedding. Add `ReopenLog` no-op stub. `PosGeo`/`PosECEF`/`VelGeo`/`VelECEF` methods are inherited from `NavEpochAccum`.
4. Add `NavEpoch` override to `SSEObserver` that builds and sends both events, then calls `NavEpochAccum.NavEpoch` to clear the bundle.
5. Add `buildPosVelSSE`, `buildQualitySSE`, `buildFixKeywords`, and unit conversion helpers.
6. Add tests for `buildFixKeywords`, `buildQualitySSE`, `buildPosVelSSE`.

### Phase 2: Web frontend

1. Add `"posvel"` and `"quality"` to `EVENT_TYPES`.
2. Add `PositionCard` component with coordinates, height, speed, course, velocity.
3. Add `QualityCard` component with fix badges, accuracy, DOPs, SV counts.
4. Add format functions for the new event types.
5. Wire cards into the dashboard layout.
