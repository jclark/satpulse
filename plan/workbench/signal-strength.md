# Signal graph

## Goal
Add per-constellation signal strength graphs to the Monitor tab, showing CN0 for each individual signal (L1, L5, E1, E5a, etc.) as vertical bars. This is the primary multi-band visibility feature -- showing per-signal detail that older single-band tools don't provide.

## Prerequisite
semantic-stream (satellite data). layout-rework.

## Motivation
Dual/multi-band receivers are becoming affordable and widespread. The key advantage of multi-band is better accuracy and faster convergence, but users need visibility into per-signal health to understand what their receiver is actually doing. Existing tools typically show only one bar per satellite (max CN0 across signals). SatPulse shows each signal separately, making it easy to see at a glance whether L5/E5a signals are healthy, which satellites are providing multi-band coverage, and how signal quality varies across bands.

## Concept

### Per-constellation sections
Each constellation (GPS, Galileo, BeiDou, GLONASS, etc.) gets its own collapsible section. Within each section, a vertical bar chart shows CN0 for every signal of every satellite in that constellation.

This keeps each graph manageable -- a constellation typically has 8-12 visible satellites with 2-3 signals each, so 16-36 bars per graph. The full set of constellations would be unreadable in a single flat graph (60-90 bars), but per-constellation sections are comfortable.

Sections only appear for constellations present in the satellite data. If no satellites are being tracked for a constellation, its section doesn't render.

### Vertical bars
Bars are vertical: satellites along the x-axis, CN0 (0-55 dB) on the y-axis. Each satellite has a small cluster of bars, one per signal. The graph is wide and short, fitting naturally as a row in the Monitor tab.

Satellite IDs appear below each cluster on the x-axis. Signal bars within a cluster are differentiated by color (see band coloring below).

### Band-consistent coloring
Signal bars are colored by frequency band, not by constellation. The same band color is used across all constellation graphs:
- L1 band (1575 MHz): GPS L1 C/A, GPS L1C, Galileo E1, BeiDou B1C, QZSS L1, NavIC L1, SBAS L1
- L2 band (1227 MHz): GPS L2P, GPS L2C, GLONASS G2, QZSS L2C
- L5 band (1176 MHz): GPS L5, Galileo E5a, BeiDou B2a, QZSS L5, NavIC L5, SBAS L5
- E5b band (1207 MHz): Galileo E5b, BeiDou B2I, GLONASS G3
- E6 band (1278 MHz): Galileo E6, BeiDou B3I, QZSS L6

This lets you glance across constellation graphs and immediately identify band health by color. "All the green bars are strong" means L5 is healthy everywhere.

### Per-constellation legend
Each constellation section includes a legend showing its own signal names mapped to band colors. GPS might show "L1 C/A" and "L5" while Galileo shows "E1" and "E5a" -- different names, same band colors.

The legend cannot be shared across constellations because signal names are constellation-specific.

### Band mapping
The `SignalInfo` received by the frontend has a string `ID` field (e.g. "L1 C/A", "E5a", "B2a") but no band information. The frontend needs to map signal IDs to bands for coloring. Options:
- The backend adds a `band` string field to `SignalInfo` in the JSON (cleanest -- the band logic already exists in `gpsprot` via `sigIndex` values).
- A backend method returns the signal-to-band mapping table.
- A hardcoded map in the frontend (fragile, duplicates backend knowledge).

The preferred approach is to add a `band` field to the JSON representation of `SignalInfo`, since the backend already knows the band for every signal.

### Used-in-solution indicator
Signals used in the navigation solution should be visually distinguished from tracked-but-unused signals. This could be full opacity vs reduced opacity, or a marker/outline on used bars. The `SignalInfo.Used` and `SVInfo.Used` fields provide this data, and `SatellitesMsg.UsedValidity` indicates whether the used flags are meaningful.

### Data source
`SatellitesMsg` from the `gps:msg` event stream. The backend already emits this via `msgHandler.Satellites`. The frontend adds a `case 'satellites'` to the existing event handler in `app.tsx`.

## Steps

### 1. Backend: add band field to SignalInfo JSON
Extend the JSON representation of `SignalInfo` to include a `band` field (e.g. "L1", "L2", "L5", "E5b", "E6"). The band is derived from the signal's `sigIndex` which is already part of the signal architecture in `gpsprot`. This avoids duplicating band-mapping logic in the frontend.

### 2. Frontend: satellite state
Add `case 'satellites'` to the `gps:msg` event handler in `app.tsx`. Store the latest `SatellitesMsg` in state.

### 3. Signal graph component
Create `signal-graph-panel.tsx`:
- Receives satellite data as props.
- Groups satellites by constellation (first character of SVID: G=GPS, E=Galileo, C=BeiDou, R=GLONASS, J=QZSS, I=NavIC, S=SBAS).
- For each constellation with visible satellites, renders a collapsible section containing:
  - A legend showing signal names for that constellation, colored by band.
  - An SVG vertical bar chart: satellites on x-axis sorted by SVID number, CN0 on y-axis (0-55 dB scale), one bar per signal per satellite.
  - Satellite ID labels below each cluster.
  - Used-in-solution visual distinction.

### 4. SVG rendering
The SVG uses `viewBox` and scales to fill available width. No fixed pixel dimensions.

Y-axis: CN0 scale 0-55 with grid lines at 10 dB intervals.
X-axis: satellite clusters, each containing 1-3 signal bars. Small gap between bars within a cluster, larger gap between clusters.

Bar colors come from a band color map (5 bands = 5 colors). The color map is defined once and used by all constellation graphs.

### 5. Add to Monitor tab
Add the signal graph sections to the Monitor tab in `app.tsx`, below Time and Survey. Sections only appear when satellite data has been received. Each constellation is a separate `CollapsibleSection`.

## Result
The Monitor tab shows per-constellation signal strength graphs with individual bars for each signal, colored by frequency band. Users can immediately see multi-band coverage and per-signal health across all tracked constellations.

## Files changed
- `gps/gpsprot/msg.go` (add `Band` field to `SignalInfo` JSON, or add a method to derive it)
- `desktop/frontend/src/signal-graph-panel.tsx` (new component)
- `desktop/frontend/src/app.tsx` (satellite state, signal graph sections in Monitor tab)

## Testing -- Playwright

### Without hardware (UI structure)
- Verify the Monitor tab shows Time and Survey sections but no signal graphs when no satellite data is available.

### With hardware (live data)
- Connect to a receiver.
- Verify constellation sections appear as satellite data arrives.
- Verify each constellation section shows a bar chart with satellite labels.
- Verify multiple bars appear per satellite for multi-signal receivers.
- Verify bar colors are consistent across constellations for the same band.
- Verify each constellation has its own legend with correct signal names.
- Collapse a constellation section; verify it collapses cleanly.
- Verify used-in-solution indicators appear when `UsedValidity` is valid.

### Responsive sizing
- Resize the window; verify SVG scales smoothly within its section.
- Verify bar charts remain readable at different widths.
