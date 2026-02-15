# UI panel: sky view

## Purpose
Visualize satellite geometry and usage at a glance in a compact polar plot.

## Context
The sky view is one of three square visual panels in a flex row (clock, map, sky view). At typical window widths it is roughly 250-320px. This small size means the plot must handle 30-50+ simultaneous satellites without becoming unreadable.

## Design

### Polar plot
Standard sky polar projection: zenith at center, horizon at edge. Azimuth maps to angle, elevation maps to distance from center. Elevation rings at 15/30/45/60 degrees. Twelve radial lines for compass bearings. Compass markers N/S/E/W.

### Satellite rendering
Satellites are rendered differently depending on the GNSS filter selection:

- **All constellations** -- each satellite is a filled circle (dot). No text labels. With 30-50+ satellites, text would overlap badly at this panel size.
- **Single constellation** -- each satellite is a two-digit SV number (e.g. "01", "12"), no dot. With at most ~36 satellites from one constellation, short numeric labels fit without excessive overlap.

In both cases, color indicates constellation and opacity indicates CN0 (signal strength).

### Constellation colors
Same scheme as existing `colorClassFor()` in `web/svg.tsx`:
- GPS: blue
- SBAS: blue (same as GPS)
- Galileo: green
- BeiDou: red
- GLONASS: magenta/fuchsia
- QZSS: amber
- NavIC: yellow

### Signal strength indication
Dot opacity varies by CN0 using the same thresholds as the existing `opacityClassFor()`.

### Used/unused distinction
When `UsedValidity` indicates the used flags are meaningful, unused satellites are shown with a hollow circle (stroke only, no fill) instead of a filled dot.

### Filtering controls
Two small dropdowns stacked vertically in the bottom-right corner of the panel, overlaid on the blank space outside the polar circle. The corner space is naturally unused by the circular plot, so the dropdowns don't obscure any satellite data. GNSS dropdown on top, band dropdown underneath. If stacking two dropdowns is too tight, experiment with other corner placements.

#### GNSS dropdown
Selects which constellation to display.

Options:
- **All** (default) -- show every satellite as a dot, colored by constellation. No SV numbers shown (too crowded).
- **GPS**, **Galileo**, **BeiDou**, **GLONASS**, **SBAS**, **QZSS**, **NavIC** -- show only satellites from that constellation. With fewer dots there is space to label each with its SV number (digits only, e.g. "01", "12" -- the constellation prefix letter is redundant since the dropdown already identifies the constellation).

A constellation appears as an option only when at least one satellite from it has been seen (in any `SatellitesMsg` since connection, not just the latest). SBAS is a separate option from GPS.

When a single constellation is selected, the band dropdown appears (if that constellation has signals on multiple bands).

#### Band dropdown
Only visible when a single constellation is selected AND the satellite data contains signals on more than one band for that constellation. Selects which signal band to display.

Options:
- **All** (default) -- show all satellites from the selected constellation. CN0 is the best (max) across all signals.
- Individual bands -- e.g. **L1**, **L5**, **E1**, **E5a**. Only satellites that have a signal on the selected band are shown. The dot opacity uses the CN0 of that specific signal. This lets the user see which satellites are transmitting on a particular frequency and how strong those signals are.

The options are frequency bands, using the same grouping as the `--band` CLI option and the `Band` type in `gpsprot/signal.go`:

- **L1** (1559-1610 MHz)
- **L2** (1215-1252 MHz)
- **L5** (1176.45 MHz)
- **E5b** (1202-1207 MHz)
- **E6** (1260-1300 MHz)

A band appears as an option only when the selected constellation has at least one signal on that band in the data. Each `SignalInfo.ID` maps to a band (e.g. "L1 C/A", "L1C", "E1", "B1I" are all L1; "E5a", "B2a" are L5; etc.). The mapping from signal ID to band is defined in the frontend using the same logic as `gpsprot/signal.go`.

When a band is selected, a satellite is shown only if it has a signal with a non-zero CN0 on that band. The CN0 used for opacity is the CN0 of that signal (not the max across all signals).

### Tooltips
Every satellite dot (in all views) has a tooltip on hover showing detail: full SVID (e.g. "G01"), azimuth, elevation, CN0 per signal, and used/unused status. This provides full detail without cluttering the plot.

### Legend
Below the polar plot, a small legend shows colored dots with constellation names, matching the colors used in the plot. Only show constellations that have at least one satellite in the current data. The legend helps the user interpret the "All" view where dots are unlabeled.

## Behavior
- Updates from semantic satellite events (`SatellitesMsg`), not packet parsing.
- Should stay useful at low update rates (1 Hz typical).
- Panel remains lightweight for continuous rendering.
- Dropdown selections are local UI state (not persisted).
- The panel appears only when satellite data has been received.

## Data dependencies
- `SatellitesMsg` from `gps:msg` events: `SVInfo[]` with `id` (SVID), `lookAngles` (azimuth, elevation), `signals[]` (each with `id` (SignalID), `cn0`, `used`), `used` flag, and `usedValidity`.

## Data source
Subscribes to the `gps:msg` event stream via the same satellite state as the signal strength panel.
