# Position track panel

## Goal
Add a position track panel to the Monitor tab (see [ui-monitor-tab.md](ui-monitor-tab.md)) that shows where the antenna has been as a trail of dots. Works for both static antennas (tight cluster reveals precision and multipath) and moving antennas with RTK (trail shows how precisely position follows the actual motion).

## Design

### Contents
A square panel showing a trail of position dots. No axes -- just dots on a plain background. A **scale bar** (like on a map) shows distance: a horizontal line segment in the corner labeled with its length (e.g. "1 m", "10 m", "100 m"). The scale bar updates as the view zooms.

### View
- 1:1 aspect ratio (always square) so the track shape is not distorted.
- No axis labels, no grid lines, no crosshair.
- **Initial scale**: the panel starts at 5 cm x 5 cm physical coverage, tight enough to see individual RTK-precision fixes.
- **Stable view**: the view does not recenter or rescale on every update. It only adjusts when a new dot falls outside the visible area or gets too close to the edge (similar to the map panel's margin-based recentering). This keeps the display steady while the antenna is stationary or moving within the current view, and smoothly adapts when the track moves or grows beyond it.
- When rescaling is needed, zoom out to fit all points with padding. The view never zooms back in automatically -- only Clear resets the zoom and returns to the initial 5 cm scale.

### Styling
Use semantic tokens throughout, following the existing pattern in `style.css` and other panels. No hardcoded colors. Reference CSS custom properties (`var(--accent)`, `var(--surface-2)`, etc.) for SVG fill/stroke and Tailwind semantic utilities (`bg-surface-1`, `text-text-secondary`, etc.) for layout. Dark mode is handled automatically via CSS variable swap -- no `dark:` prefixes.

### Points
- Each point is a small dot using `var(--track-dot)` fill (black in light mode, white in dark mode). Add this token to `style.css`.
- **Adaptive opacity**: per-dot opacity scales inversely with the number of points. Few points: each dot is clearly visible. Many points: each dot is faint so overlapping dots build up darker, naturally revealing density. This prevents static clusters from becoming a solid black blob while keeping individual dots visible when there are only a handful.

### Scale bar
A short horizontal line in the bottom-left corner with a label showing the real-world distance it represents. Line uses `var(--border-strong)`, label uses `var(--text-secondary)`. As the view zooms in or out, the scale bar length and label update (snapping to round values: 0.1 m, 0.5 m, 1 m, 2 m, 5 m, 10 m, 50 m, 100 m, etc.).

### No fix
When no position fix is received in an epoch, no dot is added. The trail simply stops growing.

### Panel size
Roughly square. Sits in the visual panels flex row alongside the map and sky view. Always visible (not collapsible).

### Controls
- **Clear**: remove all dots, reset the view to initial 5 cm scale, and reset the tangent plane (next position becomes the new origin).

### Behavior
- One dot per navigation epoch (when a position fix is available).
- On disconnect, the track is cleared (consistent with other panels which reset on the `gps:state` `disconnected` event).

---

## Implementation

### Prerequisite
Position/velocity messages (from `plan/position-velocity-messages.md`). `NavEpochMsg` emission (from `plan/nav-epoch.md`).

### Position update strategy
Subscribe to the `gps:epochPVT` event, which already emits one `PVMsgBundle` per navigation epoch (from `PVMsgAccum` in `app.go`). Extract lat/lon from `nav.posGeo.latLon[0]` (lat) and `nav.posGeo.latLon[1]` (lon). If `posGeo` is not set, no dot is added for that epoch.

### East/north calculation
Convert each position to east/north offsets in meters from the first point using a local tangent plane approximation:
- `east = (lon - first_lon) * 111320 * cos(first_lat * pi/180)`
- `north = (lat - first_lat) * 110540`

The first point is always at (0, 0). All subsequent points are offsets from it. This is simple arithmetic in JavaScript -- no backend calls needed. The approximation is accurate for deviations of meters or even kilometers.

### Rendering
Pure SVG with `viewBox` for responsive scaling. Each position is a small `<circle>` element. The scale bar is an SVG `<line>` with a `<text>` label, positioned in the bottom-left corner.

### Point buffer
Points stored as interleaved east/north in a `Float64Array` for minimal overhead. Cap at 1,000,000 points (~16 MB); oldest points discarded when full.

### State
- `points: Array<{east, north}>` -- buffered dots in meters from first point.
- `firstPoint: {lat, lon} | null` -- the first received position (defines the coordinate origin).

### Files changed
- `desktop/frontend/src/track-panel.tsx` (new component)
- `desktop/frontend/src/style.css` (add `--track-dot` token to `@theme`, `:root`, and dark mode sections)
- `desktop/frontend/src/app.tsx` (add to Monitor tab visual panels row, pass position from `gps:epochPVT`)

## Testing -- Playwright

- Connect to a receiver with position fix; verify dots appear on the track panel.
- Verify dots accumulate over time forming a trail.
- Verify the scale bar is visible and updates as the view auto-scales.
- Click Clear; verify all dots are removed.
- Disconnect; verify the track clears.
