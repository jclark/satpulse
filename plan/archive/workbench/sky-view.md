# Sky view

## Goal
Add a polar sky plot to the Monitor tab showing satellite positions by azimuth and elevation, closely matching the existing `SkyView()` in `web/svg.tsx`.

## Layout

The monitor tab's visual panels change from a single flex row to a 2-column layout:

```
┌──────────┬─────────────────────┐
│  Clock   │                     │
├──────────┤      Sky View       │
│   Map    │                     │
└──────────┴─────────────────────┘
```

Left column (`flex: 1`): clock and map stacked vertically. Right column (`flex: 2`): sky view, filling the full height of both left panels. The sky view SVG maintains a square aspect ratio within its container.

## Design

### Polar plot
Standard sky polar projection: zenith at center, horizon at edge. Elevation rings at 15/30/45/60 degrees. Twelve radial lines for compass bearings. Compass markers N/S/E/W.

### Satellite rendering
Every satellite is labeled with its SVID: constellation letter + two-digit SV number (e.g. "G01", "E24", "C05", "R12"). At this panel size there is enough room for 30-50+ labels. No dots, no dropdowns, no mode switching. The letter prefix identifies the constellation, so no legend is needed.

### Colors
Same scheme as `colorClassFor()` in `web/svg.tsx`: G/S=blue, E=green, C=red, R=fuchsia, J=amber, I=yellow.

### Signal strength
Label opacity varies by CN0 using the same thresholds as `opacityClassFor()`. CN0 is the best (max) across all signals per satellite, via `simplifySignals()`.

### Used/unused
When any satellite reports `used: true`, unused satellites get surrounding dashes (e.g. `-G01-`) with the leading dash invisible for alignment. Matches `web/svg.tsx`.

### Tooltips
Each label gets a `<title>` tooltip: full SVID, azimuth, elevation, CN0, used/unused.

## Prerequisite
Satellite state wiring in `app.tsx` (handling `SatellitesMsg` from the `gps:msg` event stream).

## What already exists
- `web/svg.tsx`: working `SkyView` SVG function with polar projection, grid, compass markers, constellation colors, opacity by CN0, `simplifySignals` helper, and text labels at each satellite position.
- `desktop/app.go`: emits `SatellitesMsg` via `gps:msg` events with kind `"satellites"`.
- `desktop/frontend/src/app.tsx`: handles `gps:msg` events but does not yet handle `"satellites"` kind.

## Steps

### 1. Wire satellite state in app.tsx

Add `case 'satellites'` to the `gps:msg` event handler. Store the latest `SatellitesMsg` in state.

The `SatellitesMsg` shape (from Go, JSON-serialized):
```typescript
interface SatellitesMsg {
    tag?: string;
    nativeMsgID?: string;
    info: SVInfo[];
    usedValidity?: number; // 0=invalid, 1=SV-level, 2=signal-level
}
interface SVInfo {
    id: string;           // e.g. "G01", "E12", "S38"
    lookAngles?: { azimuth: number; elevation: number };
    signals: SignalInfo[];
    used?: boolean;
}
interface SignalInfo {
    id?: string;  // e.g. "L1 C/A", "E5a", "B2a"
    cn0: number;
    used?: boolean;
}
```

### 2. Create sky-view-panel.tsx

New component `SkyViewPanel`. Single prop: `msg: SatellitesMsg`.

Port from `web/svg.tsx`:
- Same `viewBox`, `toXY` projection, elevation rings, radial lines, compass markers.
- Port `colorClassFor`, `opacityClassFor`, and `simplifySignals`.
- Satellite rendering: text labels with SVID, colored by constellation, opacity by CN0, dashes for unused.
- `<title>` tooltips on each label.

No dropdowns, no legend, no dots-vs-text switching.

### 3. Update monitor tab layout

Change the visual panels area from a single flex row to the 2-column layout described above:

```tsx
<div class="flex gap-4 p-4">
    <div class="flex flex-col gap-4">
        <ClockPanel ... />
        <MapPanel ... />
    </div>
    {satsMsg && <SkyViewPanel msg={satsMsg} />}
</div>
```

The sky view only appears when satellite data has been received.

### 4. Clear state on disconnect

In the `gps:state` handler, when state becomes `'disconnected'`, clear the satellite message state.

## Files changed
- `desktop/frontend/src/sky-view-panel.tsx` (new)
- `desktop/frontend/src/app.tsx` (satellite state, layout change, clear on disconnect)

## Testing -- Playwright

### Without hardware
- Verify the sky view does not appear when no satellite data is available.
- Verify the clock and map panels still display correctly in the left column.

### With hardware
- Connect to a receiver.
- Verify the sky view appears with satellite labels within a few seconds.
- Verify labels are colored by constellation.
- Verify compass markers (N/S/E/W) are visible.
- Verify elevation rings are drawn.
- Verify label opacity varies (strong vs weak signals).
- Verify unused satellites show dashes (if receiver provides used flags).
- Hover over a satellite; verify tooltip shows SVID, azimuth, elevation, CN0.

### Responsive sizing
- Resize the window; verify the SVG scales smoothly while maintaining square aspect ratio.
- Verify the 2-column layout works at various window widths.
