# Phase 6b: Sky view

## Goal
Add a polar sky plot to the Monitor tab showing satellite positions by azimuth and elevation. Port the SVG rendering from `web/svg.tsx`.

## Prerequisite
Phase 6a (satellite state wiring in `app.tsx`). Phase 5b (tab-based layout).

## What already exists
The web dashboard has a working `SkyView` function in `web/svg.tsx` that renders a polar plot with:
- Elevation rings at 15/30/45/60 degrees
- 12 radial lines for compass bearings
- Compass markers (N/S/E/W)
- Satellite labels colored by constellation, opacity by CN0
- Used/unused distinction (unused satellites marked with "-")

The desktop version ports this as a Preact component. The SVG rendering logic is largely reusable. The main change is accepting props rather than being a plain function call.

## Concept

### Polar plot
Standard sky polar projection: zenith at center, horizon at edge. Azimuth maps to angle, elevation maps to distance from center. Satellites appear as colored text labels at their current position.

### Constellation coloring
Same color scheme as `web/svg.tsx`:
- GPS/SBAS: blue
- Galileo: green
- BeiDou: red
- GLONASS: magenta/fuchsia
- QZSS: amber
- NavIC: yellow

### Signal strength indication
Opacity varies by CN0 (5 levels from weak to strong), matching the web version's `opacityClassFor()` logic.

### Used/unused distinction
When `UsedValidity` indicates the used flags are meaningful, unused satellites are visually marked (trailing "-" with invisible leading "-" for centering, matching the web version).

### Data source
Same `SatellitesMsg` state from phase 6a. The sky view uses `simplifySignals()` to collapse multiple signals to a single CN0 per satellite (since the sky plot shows position, not per-signal detail).

## Steps

### 1. Port SVG rendering
Create `sky-view-panel.tsx` porting the `SkyView` function from `web/svg.tsx` as a Preact component:
- Accept satellite data as props.
- Reuse the polar projection math (`toXY`), grid circles, radial lines, compass markers.
- Constellation color and opacity functions.
- SVG uses `viewBox` and scales to fill available width while maintaining square aspect ratio.

### 2. Port helper functions
Port `simplifySignals()`, `colorClassFor()`, and `opacityClassFor()` from `web/svg.tsx`. These can live in `sky-view-panel.tsx` or a shared utility if the signal graph also needs them.

### 3. Add to Monitor tab
Add the sky view as a collapsible section in the Monitor tab, below Time/Survey and above the signal graph sections. Only appears when satellite data has been received.

The sky view is inherently square. For now it is simply stacked vertically in the Monitor tab like everything else -- the layout rework in phase 6c will arrange it more efficiently.

## Result
The Monitor tab includes a polar sky plot showing satellite positions, colored by constellation with signal strength opacity. Combined with the per-signal bar graphs from phase 6a, users have full visibility into both satellite geometry and signal quality.

## Files changed
- `desktop/frontend/src/sky-view-panel.tsx` (new component, ported from web/svg.tsx)
- `desktop/frontend/src/app.tsx` (add sky view section to Monitor tab)

## Testing -- Playwright

### Without hardware
- Verify the sky view does not appear when no satellite data is available.

### With hardware
- Connect to a receiver.
- Verify the sky view appears and populates with satellite labels within a few seconds.
- Verify satellite labels are colored by constellation.
- Verify compass markers (N/S/E/W) are visible.
- Verify elevation rings are drawn.
- Verify satellite opacity varies by signal strength.
- Verify used/unused distinction appears when applicable.

### Responsive sizing
- Resize the window; verify the SVG scales smoothly while maintaining square aspect ratio.
- Verify no overflow or clipping at small window sizes.
