# Position scatter panel

## Goal
Add a position scatter panel to the Monitor tab (see [ui-monitor-tab.md](ui-monitor-tab.md)) that shows how position varies over time as a 2D scatter plot of east/north deviations from a reference point. This is the primary tool for evaluating fix quality, multipath, antenna placement, and correction transitions.

## Design

### Contents
A Cartesian plot with east deviation on the X axis and north deviation on the Y axis. Each position fix is a dot. The plot shows the spatial distribution of fixes: a tight cluster means good precision, a spread cloud means noise or multipath, and systematic jumps indicate correction transitions.

### Axes
- Both axes are in meters, with the same scale (1:1 aspect ratio). The plot is always square so that the scatter shape is not distorted.
- Axis range auto-scales to fit all visible points with some padding.
- Grid lines at regular intervals with labeled ticks.
- Origin (0,0) is marked with a crosshair.

### Points
- Each point is a small dot.
- **Time fading**: older points fade in opacity. The most recent point is fully opaque; the oldest visible point is semi-transparent. This shows the temporal direction of drift.
- **Quality coloring** (optional): points colored by fix quality from `NavEpochMsg` (e.g. green for fixed, yellow for float, orange for code). This makes correction transitions visible as color bands in the scatter.

### Reference point
The reference point defines the origin (0,0). Two modes:
- **Auto**: the mean of all visible points. Re-centers gradually as new points arrive.
- **First fix**: the first received position. Does not move.

Default is auto (mean).

### No fix
When no position fix is received in an epoch, no dot is added. Existing dots continue to fade normally. The absence of new dots is self-evident -- the scatter simply stops growing.

### Panel size
Roughly square. Sits in the visual panels flex row alongside the map and sky view. Always visible (not collapsible).

### Controls
- **Clear**: reset the scatter (clear all points, reset reference).
- **Reference mode**: toggle between auto (mean) and first-fix reference point.

### Behavior
- One dot per navigation epoch (when a position fix is available).
- On disconnect, the scatter is preserved (showing the session's history).
- On reconnect, the scatter is cleared and a new session begins.

---

## Implementation

### Prerequisite
Position/velocity messages (from `plan/position-velocity-messages.md`). `NavEpochMsg` emission (from `plan/nav-epoch.md`).

### Position update strategy
Like the map panel, position messages are buffered during each navigation epoch. When `NavEpochMsg` arrives, the most recent buffered position is used to add a new dot. This produces one dot per epoch (typically 1 Hz) and associates each dot with the epoch's `NavEpochMsg` metadata (for quality coloring).

If no position message was received during an epoch, no dot is added.

### East/north deviation calculation
Convert each position to east/north offsets in meters from the reference point using a local tangent plane approximation:
- `east = (lon - ref_lon) * 111320 * cos(ref_lat * pi/180)`
- `north = (lat - ref_lat) * 110540`

This is simple arithmetic in JavaScript. No backend calls needed. The approximation is accurate for deviations of meters or even kilometers.

If only `PosECEFMsg` is available, convert to lat/lon using the existing `ECEFtoLLH` backend binding first.

### Rendering
Pure SVG with `viewBox` for responsive scaling. No external charting library needed -- it is dots on a grid.

### Point buffer
Store the last N positions (default ~500). Older points are discarded. If the position source provides multiple messages per epoch, only one dot per epoch is added (driven by `NavEpochMsg` arrival).

### State
- `points: Array<{east, north, quality?, age}>` -- buffered dots.
- `refPoint: {lat, lon}` -- current reference point (mean or first fix).
- `refMode: 'auto' | 'first'` -- reference mode toggle.

### Files changed
- `desktop/frontend/src/scatter-panel.tsx` (new component)
- `desktop/frontend/src/app.tsx` (add to Monitor tab, wire up epoch-synced position updates)

## Testing -- Playwright

- Connect to a receiver with position fix; verify dots appear on the scatter plot.
- Verify dots accumulate over time and older dots fade.
- Verify axes auto-scale as the spread changes.
- Click Clear; verify all dots are removed.
- Toggle reference mode; verify the origin shifts.
- Disconnect and reconnect; verify the scatter clears on reconnect.
