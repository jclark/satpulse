# Phase 6: Sky View and Signal View

## Goal
Add graphical satellite visualization panels: polar sky plot and CN0 signal strength bar graph. Port SVG rendering from `web/svg.tsx`.

## Prerequisite
Phase 3 (semantic data stream providing satellite data).

## Reference documents
- [ui-panel-sky-view.md](ui-panel-sky-view.md) - sky view panel design (polar plot, controls, rendering)
- [ui-panel-signal-view.md](ui-panel-signal-view.md) - signal view panel design (CN0 bars, controls, rendering)
- [ui-workspace-panels.md](ui-workspace-panels.md) - panel layout for left column
- [backend.md](backend.md) - semantic `*Msg` stream (satellite data)

## Steps

### 1. Port SVG rendering code
Copy and adapt `web/svg.tsx` into the desktop frontend. This contains:
- `SkyView(svs)` - polar plot with satellite positions
- `SignalGraph(svs, maxCount, isDoubleRow)` - CN0 bar graph
- `simplifySignals(svs)` - signal data simplification
- `SVInfo` type definition

Adapt to work as proper Preact components with props rather than plain function calls. The web version uses `className`; the desktop version should use `class` (or keep `className` with preact/compat).

### 2. Sky View panel
New `sky-view-panel.tsx`:
- Renders polar plot from satellite position data
- Compass markers (N/S/E/W)
- Per-satellite labels colored by constellation
- Usage/quality overlays (used-in-solution vs tracked)
- Optional filter by constellation
- Optional toggle to show/hide unused satellites

Data source: `SatellitesMsg` from the `gps:msg` event stream (phase 3).

### 3. Signal View panel
New `signal-view-panel.tsx`:
- CN0 bar graph grouped by constellation
- Color-coded by constellation
- Optional used-for-solution indicator
- Sort mode (by constellation or by signal strength)
- Optional filter by constellation
- Compact/expanded density mode

Data source: same `SatellitesMsg` as Sky View.

### 4. Update panel layout
Add Sky View and Signal View to the left column of the panel grid:
```
+-----------------------------------------------+
| Connection strip                               |
+-------------------+---------------------------+
| Sky View          | Config / other panels     |
+-                  +                           |
| Signal View       |                           |
+-------------------+---------------------------+
| Logging strip                                  |
+-----------------------------------------------+
```

Adjust default panel sizes. Both panels should handle being resized gracefully - SVG viewBox scaling for Sky View, responsive bar widths for Signal View.

### 5. Responsive SVG sizing
Both panels use SVG. Ensure:
- SVG uses `viewBox` and scales to fill available panel space
- No fixed pixel dimensions - panels resize smoothly
- Signal graph adapts bar count/width to available width

## Result
The desktop app has full satellite visualization matching the web dashboard's capabilities. Users can monitor satellite geometry and signal quality while configuring the receiver.

## Testing (Playwright)

### Without hardware (UI structure)
- Verify Sky View panel exists and shows the polar plot grid (circles, compass markers) even with no satellite data.
- Verify Signal View panel exists and shows empty state or axis labels.
- Verify both panels resize correctly when dragging panel dividers.
- Verify SVG scales to fill panel (no fixed-size overflow or clipping).

### With hardware (live data)
- Connect to a receiver.
- Verify Sky View populates with satellite dots within a few seconds.
- Verify satellite labels appear and are colored by constellation.
- Verify Signal View shows CN0 bars.
- Resize the panels; verify SVG rescales smoothly.
- Use constellation filter (if implemented); verify only selected constellation's satellites are shown.

### Responsive sizing
- Resize the browser window to a small size; verify panels adapt without overflow.
- Resize to a large size; verify SVG fills available space.

## Files changed
- `desktop/frontend/src/svg.tsx` (new, ported from web/svg.tsx)
- `desktop/frontend/src/sky-view-panel.tsx` (new)
- `desktop/frontend/src/signal-view-panel.tsx` (new)
- `desktop/frontend/src/app.tsx` (layout updated, panels added)
