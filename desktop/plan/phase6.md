# Phase 6: Sky View and Signal View

## Goal
Add graphical satellite visualization panels: polar sky plot and CN0 signal strength bar graph. Port SVG rendering from `web/svg.tsx`.

## Prerequisite
Phase 3 (semantic data stream providing satellite data).

## Reference documents
- [ui-panel-sky-view.md](ui-panel-sky-view.md) - sky view panel design (polar plot, controls, rendering)
- [ui-panel-signal-view.md](ui-panel-signal-view.md) - signal view panel design (CN0 bars, controls, rendering)
- [ui-workspace-panels.md](ui-workspace-panels.md) - panel layout

## What already exists

The backend already emits `gpsprot.SatellitesMsg` directly as a `"gps:msg"` event with `kind: "satellites"`. The `msgHandler.Satellites` method in `app.go` passes `*msg` through without DTO conversion. The `SatellitesMsg` contains `SVs []SVInfo` with look angles, signal info, and usage flags.

The `gps:msg` event subscription in `app.tsx` already dispatches by `kind`. Add a `case 'satellites'` to store the latest satellite data.

The panel layout uses a 3-column structure: left (receiver/config), center (time/survey), right (monitor), with a logging strip below. Sky View and Signal View panels add to this layout.

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

Data source: `SatellitesMsg` from the `gps:msg` event stream (already emitted by backend).

### 3. Signal View panel
New `signal-view-panel.tsx`:
- CN0 bar graph grouped by constellation
- Color-coded by constellation
- Optional used-for-solution indicator
- Sort mode (by constellation or by signal strength)
- Optional filter by constellation
- Compact/expanded density mode

Data source: same `SatellitesMsg` as Sky View.

### 4. Frontend state for satellites
Add `case 'satellites'` to the existing `gps:msg` event handler in `app.tsx`. Store the latest `SatellitesMsg` in state and pass to Sky View and Signal View panels.

Add `skyView` and `signalView` entries to `PanelVisibility` and `panelLabels` in `connection-panel.tsx`.

### 5. Update panel layout
Add Sky View and Signal View as a new left-most column, shifting the existing columns right:
```
+----------------------------------------------------------+
| Connection strip                                          |
+-------------+-----------+-----------+--------------------+
| Sky View    | Receiver  | Time      | Packet monitor     |
|             | Config    | Survey    |                    |
| Signal View |           |           |                    |
+-------------+-----------+-----------+--------------------+
| Logging strip                                             |
+----------------------------------------------------------+
```

Adjust default panel sizes. Both panels should handle being resized gracefully - SVG viewBox scaling for Sky View, responsive bar widths for Signal View.

### 6. Responsive SVG sizing
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
- `desktop/frontend/src/app.tsx` (satellites state, layout updated, panels added)
- `desktop/frontend/src/connection-panel.tsx` (skyView/signalView panel toggles)
