# Position scatter panel

Replaces the position track panel ([position-track.md](position-track.md)) with a scatter plot focused on static antenna use. Shows precision and multipath behavior as a cluster of dots centered on the mean position, with concentric distance rings and statistics.

## Goal

Visualise the scatter of a static antenna's position fixes. The dot cluster reveals precision, multipath, and fix quality at a glance. Statistics (CEP, RMS) give quantitative precision numbers. The panel reuses the existing collapsible section in the Monitor tab.

## Design

### Layout

```
+-----------------------------------------------------------+
| Position Scatter                                      [v] |
+-----------------------------------------------------------+
| +----------+  50% CEP   0.012 m  Mean ECEF               |
| |          |  95% CEP   0.031 m  X  4000000.123 m        |
| |  scatter |  RMS Horiz 0.015 m  Y  1000000.456 m        |
| |   plot   |  RMS Vert  0.018 m  Z  4500000.789 m        |
| |          |  RMS 3D    0.022 m                           |
| |          |  Points    1234                              |
| +----------+                                    [Clear]   |
+-----------------------------------------------------------+
```

Left: square scatter plot. Right: two columns -- precision statistics on the left, mean ECEF position on the right. Clear button at the bottom right.

### Scatter plot

A square SVG showing position dots centered on the mean.

- **Crosshair**: thin lines through the center (E=0, N=0), using `var(--border-subtle)`.
- **Distance rings**: thin concentric circles at round distances from the center, using `var(--border-subtle)`. Ring distances snap to round values (same sequence as the old scale bar: 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10 m, ...). Choose a step so that 2-4 rings fit in the visible area. Label one ring with its distance (e.g. "1 cm", "5 m") in `var(--text-secondary)`, small font.
- **No axes, no grid, no axis labels.** Just the crosshair, rings, and dots.

### View

- 1:1 aspect ratio (always square).
- **Initial scale**: 5 cm x 5 cm physical coverage.
- **Stable view**: same margin-based recentering as the track panel. The view recenters/rescales only when a dot falls outside the visible area minus margin. Never zooms back in -- only Clear resets zoom.
- The crosshair and rings are always centered on (0, 0) in the plot coordinate system (which corresponds to the current mean ECEF position projected to ENU).

### Points

- Same styling as the track panel: `var(--track-dot)` fill, adaptive opacity (`max(0.05, min(1, 5/sqrt(n)))`), latest dot highlighted with `var(--accent)`.
- One dot per navigation epoch when a position fix is available.

### Statistics

Displayed to the right of the plot in two side-by-side columns:

**Precision statistics** (left column):

| Label    | Value | Description |
|----------|-------|-------------|
| 50% CEP  | m     | Radius containing 50% of horizontal points (circular error probable) |
| 95% CEP  | m     | Radius containing 95% of horizontal points |
| RMS Horiz | m     | RMS of horizontal distance from mean |
| RMS Vert  | m     | RMS of vertical (Up) distance from mean |
| RMS 3D    | m     | RMS of 3D distance from mean |
| Points   | count | Number of accumulated points |

**Mean ECEF position** (right column):

| Label | Value | Description |
|-------|-------|-------------|
| X     | m     | Mean ECEF X coordinate |
| Y     | m     | Mean ECEF Y coordinate |
| Z     | m     | Mean ECEF Z coordinate |

Format precision values with appropriate precision: use mm when < 1 m (e.g. "12.3 mm"), meters when >= 1 m (e.g. "1.23 m"). Format ECEF coordinates with 3 decimal places (mm resolution). Use `text-xs font-mono` for values, `text-xs text-text-secondary` for labels.

Statistics update on every new point.

### Controls

- **Clear**: remove all dots, reset view to initial 5 cm scale, reset ECEF buffer and mean. Positioned at the bottom right of the stats column.

### Behavior

- One dot per navigation epoch (when a position fix is available).
- On disconnect, the scatter is cleared (via `key={trackGen}` remount, same as current track panel).

---

## Implementation

### Prerequisite

Position/velocity messages (from `plan/position-velocity-messages.md`). `NavEpochMsg` emission (from `plan/nav-epoch.md`). The existing track panel implementation.

### Data flow change

Currently `app.tsx` passes `{lat, lon}` to TrackPanel. The scatter panel takes ECEF instead. In `app.tsx`, extract `nav.posECEF.pos` from the `gps:epochPVT` event and pass it to the scatter panel. When `posECEF` is not set (no height available), no dot is added for that epoch.

New prop type:

```tsx
interface ScatterPanelProps {
    ecef: [number, number, number] | null;
}
```

In `app.tsx`, extract from the epoch event:

```tsx
if (nav.posECEF) {
    setScatterECEF(nav.posECEF.pos);
}
```

### ECEF buffer

Store raw ECEF positions in a `Float64Array` with interleaved X, Y, Z (3 values per point). Cap at 1,000,000 points (~24 MB). When full, discard oldest points (shift by 3).

On each new point:
1. Append ECEF X, Y, Z to the buffer.
2. Update the running ECEF mean incrementally: `mean = mean + (new - mean) / n`. (If the buffer wraps, recompute the mean from all remaining points.)
3. Compute ENU offset of every point from the mean (see below).
4. Compute statistics from the ENU offsets.

### ECEF to ENU conversion

Convert ECEF offsets from the mean to East-North-Up.

**Step 1: Geodetic lat/lon of the mean.** Call the `ECEFtoLLH` Go binding (already exposed in `app.go`) with the mean ECEF. One async call per epoch.

**Step 2: Rotation matrix.** Compute sin/cos of the returned lat/lon.

**Step 3: For each point, compute ENU offset:**

```
dx = ecef[i] - meanX
dy = ecef[i+1] - meanY
dz = ecef[i+2] - meanZ

E = -sinLon * dx + cosLon * dy
N = -sinLat * cosLon * dx - sinLat * sinLon * dy + cosLat * dz
U =  cosLat * cosLon * dx + cosLat * sinLon * dy + sinLat * dz
```

This is the standard ECEF-to-ENU rotation (same trig as `ECEFtoNED` in `geopos.go`, axes reordered).

### Statistics computation

On each new point, after recomputing ENU offsets from the (updated) mean:

1. **Horizontal distances**: `h[i] = sqrt(E[i]^2 + N[i]^2)` for each point.
2. **3D distances**: `d3[i] = sqrt(E[i]^2 + N[i]^2 + U[i]^2)` for each point.
3. **50% CEP**: sort horizontal distances, take median.
4. **95% CEP**: sort horizontal distances, take 95th percentile.
5. **RMS Horiz**: `sqrt(mean(h[i]^2))`.
6. **RMS Vert**: `sqrt(mean(U[i]^2))`.
7. **RMS 3D**: `sqrt(mean(d3[i]^2))`.

Sorting can use a temporary array. For 1M points this is ~10ms, acceptable at 1 Hz update rate.

Performance note: recomputing all ENU offsets from the mean on every epoch is necessary because the mean shifts. With the interleaved `Float64Array`, iterating 1M points is fast (~5ms). If profiling shows issues, we can recompute only every N epochs, but this is unlikely to be needed at typical GPS rates (1-20 Hz).

### Distance rings

Pick a ring step from the scale sequence such that 2-4 rings fit in `span/2`. Draw circles at `step, 2*step, 3*step, ...` from center until they exceed `span/2`. Label the outermost visible ring.

### Rendering

Pure SVG with `viewBox` as in the current track panel.

1. Crosshair: two `<line>` elements spanning the full viewBox.
2. Distance rings: `<circle>` elements centered at viewBox center.
3. Position dots: `<circle>` elements as before.
4. Ring label: `<text>` positioned near the outermost ring.

### Files changed

- `desktop/frontend/src/scatter-panel.tsx` -- rewrite to scatter panel (rename component to `ScatterPanel`)
- `desktop/frontend/src/style.css` -- no changes needed (existing `--track-dot` token works)
- `desktop/frontend/src/app.tsx` -- change data passed to panel: add ECEF position state, pass to ScatterPanel; rename section title to "Position Scatter"

## Testing -- Playwright

- Connect to a receiver with position fix; verify dots appear on the scatter panel.
- Verify dots cluster around the center with crosshair and rings visible.
- Verify statistics appear and update (50% CEP, 95% CEP, RMS Horiz, RMS 3D, Points).
- Verify ring labels show sensible distances.
- Click Clear; verify all dots and stats are removed.
- Disconnect; verify the scatter clears.
