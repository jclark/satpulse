# Map panel

## Goal
Add a map panel to the Monitor tab (see [ui-monitor-tab.md](ui-monitor-tab.md)) that shows the receiver's current position on a map so the user can confirm they are in the right place.

## Design

### Contents
- A map centered on the current position using OpenStreetMap raster tiles.
- A position marker (dot or crosshair) at the center.

### Zoom level
Fixed at zoom 18 or 19 (showing roughly 75-150m of surroundings). The goal is to show enough context (buildings, roads) for the user to recognise where the antenna is.

### No panning or zooming
The map is a static display. The position is always at the center. No panning, zooming, or dragging.

### Open in Google Maps
Clicking the map (or a dedicated button) opens the user's default browser at the current position in Google Maps.

### Offline behavior
When tiles cannot be loaded (no internet), the map area shows a placeholder with the position coordinates as text.

### No fix
When no position fix is received in an epoch, the map displays a "No fix for N seconds" indicator. The last known position and tiles remain visible underneath.

### Panel size
Roughly square. Sits in the visual panels flex row alongside the position scatter and sky view. Always visible (not collapsible).

---

## Implementation

### Prerequisite
Position/velocity messages (from `plan/position-velocity-messages.md`). `NavEpochMsg` emission (from `plan/nav-epoch.md`).

### Key design decision: backend-synthesized epoch navigation event

Rather than having each frontend panel independently buffer position messages, the Go backend `msgHandler` synthesizes a single **`gps:epochNav`** event when `NavEpoch` fires. It carries all position, velocity, and time representations for that epoch. Native values are used when available; missing representations are computed where possible. Frontend panels just consume this one event.

**Position preference**: prefer PosGeoMsg (native lat/lon) over PosECEFMsg. Compute the missing representation (ECEF from LLH or LLH from ECEF).

**Velocity**: include native fields only for now. Cross-frame conversion (NED<->ECEF) will be added once `gps/lib/geopos` provides the needed rotation functions (see [appendix](#appendix-nedecef-velocity-conversion-for-gpslibgeopos)).

**Time**: synthesize both TAI and UTC using `ComputeTAITime` and leap second state.

### Files changed
- `desktop/app.go` (NavEpoch handler, message buffering, epochNav emission)
- `desktop/frontend/src/map-panel.tsx` (new component)
- `desktop/frontend/src/app.tsx` (handle `gps:epochNav`, render MapPanel)

---

## Backend: epochNav synthesis in app.go

### MsgBundle

Buffer the latest message of each type during an epoch:

```go
// MsgBundle holds the most recent message of each type within a navigation epoch.
// Cleared after each NavEpochMsg.
type MsgBundle struct {
    PosGeo  *gpsprot.PosGeoMsg
    PosECEF *gpsprot.PosECEFMsg
    VelGeo  *gpsprot.VelGeoMsg
    VelECEF *gpsprot.VelECEFMsg
    Time    *gpsprot.TimeMsg
}
```

Add a `cur MsgBundle` field to `msgHandler`. Each existing handler (PosGeo, PosECEF, VelGeo, VelECEF, Time) sets the corresponding field in addition to emitting `gps:msg`.

### EpochNav event types

```go
type EpochNav struct {
    Pos  opt.Val[EpochPos]  `json:"pos,omitzero"`
    Vel  opt.Val[EpochVel]  `json:"vel,omitzero"`
    Time opt.Val[EpochTime] `json:"time,omitzero"`
}

type EpochPos struct {
    Lat       float64              `json:"lat"`                  // degrees
    Lon       float64              `json:"lon"`                  // degrees
    Height    opt.Val[float64]     `json:"height,omitzero"`      // meters above WGS84
    HeightMSL opt.Val[float64]     `json:"heightMSL,omitzero"`   // meters above MSL
    ECEF      opt.Val[[3]float64]  `json:"ecef,omitzero"`        // meters
}

type EpochVel struct {
    GroundSpeed opt.Val[float64]    `json:"groundSpeed,omitzero"` // m/s
    Speed3D     opt.Val[float64]    `json:"speed3D,omitzero"`     // m/s
    Course      opt.Val[float64]    `json:"course,omitzero"`      // degrees, true north
    VelNED      opt.Val[[3]float64] `json:"velNED,omitzero"`      // m/s [N,E,D]
    VelECEF     opt.Val[[3]float64] `json:"velECEF,omitzero"`     // m/s [X,Y,Z]
}

type EpochTime struct {
    TAI       opt.Val[int64]  `json:"tai,omitzero"`        // TAI seconds
    UTC       string          `json:"utc,omitempty"`        // ISO 8601
    Accuracy  opt.Val[int64]  `json:"accuracy,omitzero"`   // nanoseconds
    UTCOffset opt.Val[uint8]  `json:"utcOffset,omitzero"`  // leap seconds
    GNSS      string          `json:"gnss,omitempty"`
}
```

### NavEpoch handler

1. Emit `gps:msg` with kind `"navEpoch"` (existing pattern).
2. Build `EpochNav` from `h.cur`:
   - **Pos**: If `cur.PosGeo` exists (preferred): lat/lon from nanodegrees, height/heightMSL from micrometers if present, ECEF from `cur.PosECEF` native or computed via `LLHtoECEF` (requires height). If only `cur.PosECEF`: ECEF native, lat/lon/height via `ECEFtoLLH`. HeightMSL omitted (can't derive from ECEF).
   - **Vel**: Copy native fields from `cur.VelGeo` (groundSpeed, speed3D, course, velNED) and `cur.VelECEF` (velECEF). Convert units (um/s to m/s, nanodeg to deg). No NED<->ECEF cross-frame conversion yet -- will be added when `geopos.WGS84.NEDtoECEF` / `ECEFtoNED` are available (see [appendix](#appendix-nedecef-velocity-conversion-for-gpslibgeopos)).
   - **Time**: Use `ComputeTAITime(h.ls)` on `cur.Time`. Derive UTC from TAI minus leap seconds or use native UTC. Copy accuracy, utcOffset, gnss.
3. Emit `runtime.EventsEmit(h.ctx, "gps:epochNav", epochNav)` if any field is set.
4. Reset `h.cur = MsgBundle{}`.

Existing conversion functions to reuse:
- `geopos.WGS84.ECEFtoLLH()` (already used by `App.ECEFtoLLH` in `app.go`)
- `geopos.WGS84.LLHtoECEF()` (already used by `App.LLHtoECEF` in `app.go`)

---

## Frontend: handle epochNav in app.tsx

Add state:
```typescript
const [mapPos, setMapPos] = useState<{lat: number; lon: number} | null>(null);
const [noFixSecs, setNoFixSecs] = useState(0);
```

Add event listener for `gps:epochNav`:
```typescript
EventsOn('gps:epochNav', (nav: any) => {
    if (nav.pos) {
        setMapPos({lat: nav.pos.lat, lon: nav.pos.lon});
        setNoFixSecs(0);
    } else {
        setNoFixSecs(prev => prev + 1);
    }
});
```

On disconnect: clear `mapPos`, reset `noFixSecs`.

---

## Frontend: map-panel.tsx component

### Props
```typescript
interface MapPanelProps {
    pos: {lat: number; lon: number} | null;
    noFixSecs: number;
}
```

### Tile math (pure functions, no library)
- `latLonToTile(lat, lon, zoom)` returns `{tileX, tileY, pixelX, pixelY}` where pixelX/Y is the offset within the 256px tile.
- Zoom 18. Standard OSM slippy map formulas.

### 2x2 tile grid
Compute which 4 tiles to show so the position marker is at the center of the 512x512 display area:
- If pixel offset in right half: use `[tileX, tileX+1]`, else `[tileX-1, tileX]`.
- Same vertically. CSS offset positions grid so marker pixel is at (256, 256).

### Rendering
- 512x512 container, `overflow: hidden`, `position: relative`.
- 4 `<img>` tags from `https://tile.openstreetmap.org/{z}/{x}/{y}.png`, positioned absolutely.
- Crosshair/dot marker at center.
- Tiles re-fetched only when tile coordinates change (useMemo).
- No panning/zooming/dragging.
- OSM attribution "(c) OpenStreetMap contributors" at bottom-right.

### States
- `pos === null`: placeholder "Waiting for position".
- `noFixSecs > 0`: last tiles shown with overlay "No fix for N s".
- Normal: tiles + marker.

### Click to open Google Maps
`BrowserOpenURL` (from `wailsjs/runtime/runtime`) with `https://www.google.com/maps/@{lat},{lon},18z`.

### Wire into Monitor tab

Add `<MapPanel>` in a flex row in the Monitor tab, before collapsible sections:
```tsx
<div class="flex flex-wrap gap-4 p-4">
    <MapPanel pos={mapPos} noFixSecs={noFixSecs} />
</div>
```

---

## Testing -- Playwright

- Connect to a receiver with position fix; verify the map shows tiles centered on the position.
- Verify the position marker is visible at the center.
- Click the map; verify Google Maps opens in the browser with the correct coordinates.
- Disconnect; verify the last position remains displayed.
- Verify "no fix" indicator appears when the receiver loses fix.

---

## Appendix: NED<->ECEF velocity conversion for gps/lib/geopos

This section describes functions to add to `gps/lib/geopos/geopos.go` in a separate task. Once available, the epochNav velocity synthesis can populate both VelNED and VelECEF regardless of which the receiver provides natively.

### Background

Converting velocity between NED (North, East, Down) and ECEF frames requires a rotation matrix that depends on the geodetic position (latitude, longitude). The rotation matrix R transforms NED to ECEF:

```
        [ -sin(lat)*cos(lon)   -sin(lon)   -cos(lat)*cos(lon) ]
    R = [ -sin(lat)*sin(lon)    cos(lon)   -cos(lat)*sin(lon) ]
        [  cos(lat)              0          -sin(lat)          ]
```

`V_ecef = R * V_ned` and `V_ned = R^T * V_ecef` (R is orthogonal, so inverse = transpose).

### Types

```go
// VelNED represents velocity in the North, East, Down frame (m/s).
type VelNED [3]float64

// VelECEF represents velocity in the ECEF frame (m/s).
type VelECEF [3]float64
```

### Functions to add

Both are methods on `wgs84` to match the existing pattern (`WGS84.NEDtoECEF(...)`, `WGS84.ECEFtoNED(...)`).

#### NEDtoECEF

```go
// NEDtoECEF converts a velocity vector from NED to ECEF frame.
// lat and lon are the geodetic position in degrees.
func (wgs84) NEDtoECEF(vel VelNED, lat, lon float64) VelECEF
```

Implementation: convert lat/lon to radians, build rotation matrix R, multiply `R * vel`.

#### ECEFtoNED

```go
// ECEFtoNED converts a velocity vector from ECEF to NED frame.
// lat and lon are the geodetic position in degrees.
func (wgs84) ECEFtoNED(vel VelECEF, lat, lon float64) VelNED
```

Implementation: convert lat/lon to radians, build R^T (transpose of the same matrix), multiply `R^T * vel`.

### Implementation notes

- Both functions share the same rotation matrix computation. Factor out a helper that returns the sin/cos values, or compute inline (it's just 4 trig calls: sinLat, cosLat, sinLon, cosLon).
- No error return needed -- any lat/lon is valid for rotation.
- Add tests using known reference points (e.g. velocity at the equator/prime meridian where the matrix simplifies to identity-like forms, and at the poles).
- File: add to existing `gps/lib/geopos/geopos.go`, tests in `geopos_test.go`.
