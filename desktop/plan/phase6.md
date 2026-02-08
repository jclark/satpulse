# Phase 6: Sky view and signal view

## Goal
Add graphical satellite visualization to the Monitor tab: polar sky plot and CN0 signal strength bar graph. Port SVG rendering from `web/svg.tsx`. Rework the Monitor tab layout from a single column of collapsible sections to a responsive grid that uses horizontal space effectively.

## Prerequisite
Phase 3 (semantic data stream providing satellite data). Phase 5b (tab-based layout).

## Reference documents
- [ui-panel-sky-view.md](ui-panel-sky-view.md) - sky view design (polar plot, controls, rendering)
- [ui-panel-signal-view.md](ui-panel-signal-view.md) - signal view design (CN0 bars, controls, rendering)

## What already exists

The backend already emits `gpsprot.SatellitesMsg` directly as a `"gps:msg"` event with `kind: "satellites"`. The `msgHandler.Satellites` method in `app.go` passes `*msg` through without DTO conversion. The `SatellitesMsg` contains `SVs []SVInfo` with look angles, signal info, and usage flags.

The `gps:msg` event subscription in `app.tsx` already dispatches by `kind`. Add a `case 'satellites'` to store the latest satellite data.

The Monitor tab (from phase 5b) currently uses vertically stacked collapsible sections for Time and Survey. That works for two small key-value sections, but adding Sky View (a large square polar plot) and Signal View (a wide bar graph) would waste horizontal space in a single column. This phase reworks the Monitor tab to use a responsive grid.

## Monitor tab layout rework

Replace the single-column collapsible section layout with a responsive CSS grid, similar to the web dashboard's `grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3` approach. Each section becomes a card in the grid.

```
Wide window (lg, 3 columns):
┌──────────────────┬──────────────┬──────────────┐
│                  │              │              │
│  Sky View        │  Time        │  Signal View │
│  (2 cols,        │              │  (spans 2    │
│   2 rows)        ├──────────────┤   rows if    │
│                  │              │   many sats) │
│                  │  Survey      │              │
│                  │              │              │
└──────────────────┴──────────────┴──────────────┘

Medium window (md, 2 columns):
┌──────────────────┬──────────────┐
│                  │              │
│  Sky View        │  Time        │
│  (spans 2 rows)  ├──────────────┤
│                  │  Survey      │
├──────────────────┴──────────────┤
│  Signal View (full width)       │
└─────────────────────────────────┘

Narrow window (1 column):
┌─────────────────────────────────┐
│  Time                           │
├─────────────────────────────────┤
│  Survey                         │
├─────────────────────────────────┤
│  Sky View                       │
├─────────────────────────────────┤
│  Signal View                    │
└─────────────────────────────────┘
```

Key differences from the web dashboard:
- The web dashboard uses cards with borders and shadows. The desktop Monitor tab should use the same `CollapsibleSection` component (clickable header bar with collapse arrow) so sections can still be collapsed, but arranged in a grid instead of a single column.
- Sky View spans 2 columns and 2 rows at `lg` width (same as the web dashboard's `SkyViewCard`).
- Signal View spans 2 rows when there are many satellites (same high-water-mark logic as the web dashboard's `SignalGraphCard`).
- Time and Survey are small cards that fit in a single grid cell.

The grid scrolls vertically if content overflows (same `overflow-y-auto` as now).

## Steps

### 1. Port SVG rendering code
Copy and adapt `web/svg.tsx` into the desktop frontend. This contains:
- `SkyView(svs)` - polar plot with satellite positions
- `SignalGraph(svs, maxCount, isDoubleRow)` - CN0 bar graph
- `simplifySignals(svs)` - signal data simplification
- `SVInfo` type definition

Adapt to work as proper Preact components with props rather than plain function calls. The web version uses `className`; the desktop version should use `class` (or keep `className` with preact/compat).

### 2. Sky View component
New `sky-view-panel.tsx`:
- Renders polar plot from satellite position data
- Compass markers (N/S/E/W)
- Per-satellite labels colored by constellation
- Usage/quality overlays (used-in-solution vs tracked)
- Optional filter by constellation
- Optional toggle to show/hide unused satellites

Data source: `SatellitesMsg` from the `gps:msg` event stream (already emitted by backend).

### 3. Signal View component
New `signal-view-panel.tsx`:
- CN0 bar graph grouped by constellation
- Color-coded by constellation
- Optional used-for-solution indicator
- Sort mode (by constellation or by signal strength)
- Optional filter by constellation
- Compact/expanded density mode

Data source: same `SatellitesMsg` as Sky View.

### 4. Frontend state for satellites
Add `case 'satellites'` to the existing `gps:msg` event handler in `app.tsx`. Store the latest `SatellitesMsg` in state and pass to Sky View and Signal View components.

### 5. Rework Monitor tab to responsive grid
Replace the single-column collapsible section layout in `app.tsx` with a responsive grid:
- Container: `grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 p-4 overflow-y-auto`
- Each section wrapped in a `CollapsibleSection` inside a grid cell
- Sky View: `md:col-span-2 lg:col-span-2 md:row-span-2 lg:row-span-2` (matches web dashboard)
- Signal View: dynamic `md:row-span-2 lg:row-span-2` when satellite count >= 15 (high-water-mark, matches web dashboard)
- Time and Survey: single grid cell each
- Section order in markup: Sky View, Time, Signal View, Survey (grid auto-placement fills cells naturally)
- At narrow widths (1 column), all sections stack vertically in source order

Sky View and Signal View sections only appear when satellite data has been received (same conditional rendering as the web dashboard).

### 6. Responsive SVG sizing
Both components use SVG. Ensure:
- SVG uses `viewBox` and scales to fill available grid cell width
- No fixed pixel dimensions - cells resize smoothly with the window
- Signal graph adapts bar count/width to available width

## Result
The Monitor tab has a responsive grid layout with satellite visualization matching the web dashboard's capabilities. Small sections (Time, Survey) use space efficiently alongside large graphical sections (Sky View, Signal View).

## Testing (Playwright)

### Without hardware (UI structure)
- Verify the Monitor tab shows Time and Survey sections in a grid layout.
- Verify Sky View and Signal View do not appear without satellite data.
- Verify all sections collapse and expand correctly.

### With hardware (live data)
- Connect to a receiver.
- Verify Sky View appears and populates with satellite dots within a few seconds.
- Verify satellite labels appear and are colored by constellation.
- Verify Signal View shows CN0 bars.
- Verify the grid layout arranges sections appropriately for the window width.
- Resize the window; verify layout reflows between 1/2/3 column modes.
- Use constellation filter (if implemented); verify only selected constellation's satellites are shown.

### Responsive sizing
- Resize the browser window to a small size; verify sections stack vertically.
- Resize to a large size; verify grid fills space with Sky View spanning multiple cells.
- Verify SVG scales smoothly within its grid cell (no overflow or clipping).

## Files changed
- `desktop/frontend/src/svg.tsx` (new, ported from web/svg.tsx)
- `desktop/frontend/src/sky-view-panel.tsx` (new)
- `desktop/frontend/src/signal-view-panel.tsx` (new)
- `desktop/frontend/src/app.tsx` (satellites state, Monitor tab reworked to responsive grid)
