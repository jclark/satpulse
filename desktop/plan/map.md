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

### Position update strategy
The map does not update on every individual `PosGeoMsg` or `PosECEFMsg`. Instead, position messages are buffered during each navigation epoch. When `NavEpochMsg` arrives (marking the epoch boundary), the most recent buffered position is displayed. This produces one update per epoch (typically 1 Hz) synchronized with the solution status block.

If no position message was received during an epoch, the map keeps the last known position and increments a "no fix" counter displayed on the map.

### Tile fetching
Use a 2x2 grid of OpenStreetMap tiles. Compute the tile coordinates and pixel offset from lat/lon so the position marker is at the center of the displayed area. Tiles are fetched directly as `<img>` elements from `https://tile.openstreetmap.org/{z}/{x}/{y}.png`. No mapping library is needed.

Tiles are only re-fetched when the position moves enough to require different tile coordinates. For a stationary receiver this means tiles are fetched once.

### ECEF conversion
If only `PosECEFMsg` is available (no `PosGeoMsg`), convert to lat/lon using the existing `ECEFtoLLH` backend binding before computing tile coordinates.

### State
- `lastPos: {lat, lon} | null` -- last displayed position.
- `noFixSeconds: number` -- seconds since last valid position (reset to 0 on each position update).
- Tile image URLs and pixel offset (recomputed only when tile coordinates change).

### Files changed
- `desktop/frontend/src/map-panel.tsx` (new component)
- `desktop/frontend/src/app.tsx` (add to Monitor tab, wire up epoch-synced position updates)

### Notes
- OpenStreetMap attribution ("(c) OpenStreetMap contributors") must be displayed.
- The 2x2 tile grid with CSS offset is sufficient to keep the position centered regardless of where it falls within a tile.

## Testing -- Playwright

- Connect to a receiver with position fix; verify the map shows tiles centered on the position.
- Verify the position marker is visible at the center.
- Click the map; verify Google Maps opens in the browser with the correct coordinates.
- Disconnect; verify the last position remains displayed.
- Verify "no fix" indicator appears when the receiver loses fix.
