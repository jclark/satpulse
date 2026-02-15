# Sky view

## Goal
Add a polar sky plot to the Monitor tab showing satellite positions by azimuth and elevation. See [ui-panel-sky-view.md](ui-panel-sky-view.md) for the full UI design.

## Prerequisite
Satellite state wiring in `app.tsx` (handling `SatellitesMsg` from the `gps:msg` event stream).

## What already exists
- `web/svg.tsx`: working `SkyView` SVG function with polar projection, grid, compass markers, constellation colors (`colorClassFor`), opacity by CN0 (`opacityClassFor`), and `simplifySignals` helper. This renders text labels at each satellite position.
- `desktop/app.go`: emits `SatellitesMsg` via `gps:msg` events with kind `"satellites"`.
- `desktop/frontend/src/app.tsx`: handles `gps:msg` events but does not yet handle `"satellites"` kind.
- `gps/gpsprot/signal.go`: defines `SignalID` constants and band groupings (`Band` type, `sigIndex*` constants) that define the signal-to-band mapping.

## Steps

### 1. Wire satellite state in app.tsx

Add `case 'satellites'` to the `gps:msg` event handler. Store the latest `SatellitesMsg` in state. Also maintain a `Set<string>` of constellation prefixes ever seen (accumulated across all messages since connection, cleared on disconnect).

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

### 2. Signal-to-band mapping

Create a small lookup in `sky-view-panel.tsx` that maps `SignalInfo.ID` strings to band names. The bands match `gpsprot/signal.go`:

| Band | Signal IDs |
|------|-----------|
| L1 | L1 C/A, L1C, L1C-D, L1C-P, L1P, L1P(Y), L1M, L1, E1, E1-B, E1-C, E1-A, B1I, B1I-D1, B1I-D2, B1C, B1C-P, B1C-D, B1A, L1S, L1 C/B |
| L2 | L2P, L2C-M, L2C-L, L2, L2P, B2I, B2I-D1, B2I-D2, B2Q |
| L5 | L5-I, L5-Q, L5, E5a, E5a-I, E5a-Q, B2a, B2a-P, B2a-D |
| E5b | E5b, E5b-I, E5b-Q, E5, L3-I, L3-Q, B2b, B2b-I, B2a+b |
| E6 | E6, E6-A, E6-B, E6-C, B3I, B3I-D1, B3I-D2, B3Q, B3A, L6, L6E |

This is a `Record<string, string>` mapping signal ID to band name. Unknown signal IDs are ignored (satellite still shown in "All" band view, just not in specific band views).

### 3. Create sky-view-panel.tsx

New component `SkyViewPanel`. Props:
- `satellites: SVInfo[]` -- current satellite data
- `usedValidity: number` -- whether used flags are meaningful
- `seenConstellations: Set<string>` -- constellation prefixes ever seen

Internal state:
- `gnssFilter: string` -- "All" or a constellation prefix ("G", "E", "C", "R", "S", "J", "I")
- `bandFilter: string` -- "All" or a band name ("L1", "L2", "L5", "E5b", "E6")

#### Layout
- Fixed square aspect ratio container (same sizing pattern as `ClockPanel` and `MapPanel`).
- Dropdowns stacked vertically in the bottom-right corner (GNSS on top, band underneath), overlaid on the blank space outside the polar circle.
- SVG polar plot fills the panel.
- Legend below the SVG (small colored dots with constellation short names).

#### SVG polar plot
Port from `web/svg.tsx`:
- Same `viewBox`, `toXY` projection, elevation rings, radial lines, compass markers.
- Port `colorClassFor` and `opacityClassFor` (reuse the same Tailwind classes for fill colors and opacity).

#### Satellite rendering
- **All constellations**: render each satellite as a small `<circle>` with fill color from `colorClassFor` and opacity from `opacityClassFor`. No text labels.
- **Single constellation**: render each satellite as a `<text>` element with the numeric part of the SVID (e.g. "01" from "G01"). Color from `colorClassFor`, opacity from `opacityClassFor`.
- When `usedValidity > 0`: used satellites are filled dots (or bold text in single-constellation view), unused satellites are hollow circles (stroke only, no fill) or lighter-weight text. When `usedValidity` is 0 (no usage data), all satellites are rendered as filled dots -- no hollow circles at all.

#### CN0 selection
- **Band = "All"**: CN0 is the max across all signals with non-zero CN0 (same as `simplifySignals` logic from `web/svg.tsx`).
- **Band = specific**: CN0 is the CN0 of the signal on that band. If the satellite has no signal on the selected band (or CN0 is 0), the satellite is not shown.

#### Tooltips
Each satellite element gets a `<title>` child with: full SVID, azimuth, elevation, per-signal CN0 list, and used/unused status. SVG `<title>` gives native browser tooltips without extra libraries.

#### GNSS dropdown logic
- Options: "All" plus one entry per constellation that has been seen (`seenConstellations` set).
- Display names: All, GPS, Galileo, BeiDou, GLONASS, SBAS, QZSS, NavIC.
- Map from prefix to display name: G=GPS, E=Galileo, C=BeiDou, R=GLONASS, S=SBAS, J=QZSS, I=NavIC.
- When switching from a single constellation back to "All", reset band filter to "All".

#### Band dropdown logic
- Only visible when `gnssFilter` is not "All".
- Populated from the signals actually present in the current satellite data for the selected constellation.
- Only shown when there are 2+ distinct bands present.
- When switching GNSS, reset band filter to "All".

#### Legend
- Row of small colored dots + short constellation names, below the SVG.
- Only constellations present in the current data (not `seenConstellations`).
- Hidden when a single constellation is selected (redundant).

### 4. Add to Monitor tab

In `app.tsx`, add `SkyViewPanel` to the visual panels flex row alongside `ClockPanel` and `MapPanel`:

```tsx
<div class="flex flex-wrap gap-4 p-4">
    <ClockPanel msg={timeMsg} leapSecond={leapSecond} />
    <MapPanel pos={mapPos} course={mapCourse} noFixSecs={noFixSecs} />
    {satsMsg && <SkyViewPanel satellites={satsMsg.info} usedValidity={satsMsg.usedValidity ?? 0} seenConstellations={seenConstellations} />}
</div>
```

The sky view only appears when satellite data has been received (conditional on `satsMsg`).

### 5. Clear state on disconnect

In the `gps:state` handler, when state becomes `'disconnected'`, clear the satellite message state and the `seenConstellations` set.

## Files changed
- `desktop/frontend/src/sky-view-panel.tsx` (new)
- `desktop/frontend/src/app.tsx` (satellite state, seenConstellations, SkyViewPanel in monitor tab, clear on disconnect)

## Testing -- Playwright

### Without hardware
- Verify the sky view does not appear when no satellite data is available.

### With hardware
- Connect to a receiver.
- Verify the sky view appears with satellite dots within a few seconds.
- Verify dots are colored by constellation.
- Verify compass markers (N/S/E/W) are visible.
- Verify elevation rings are drawn.
- Verify dot opacity varies (some dots darker than others).
- Select a single constellation from the GNSS dropdown; verify dots change to numeric labels.
- If multiple bands present, verify band dropdown appears and filtering works.
- Switch back to "All"; verify dots return and band dropdown disappears.
- Hover over a satellite; verify tooltip shows SVID, azimuth, elevation, CN0.

### Responsive sizing
- Resize the window; verify the SVG scales smoothly while maintaining square aspect ratio.
- Verify dropdowns remain usable at small panel sizes.
