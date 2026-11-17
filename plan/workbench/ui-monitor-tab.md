# UI design: Monitor tab

## Purpose
Define the overall layout and content of the Monitor tab. This is the primary operational view -- it answers "what is my receiver doing right now?" across positioning, satellite tracking, and signal quality.

## Context
The app is a general-purpose GPS tool, not timing-specific. The Monitor tab prioritises position and velocity visibility alongside satellite geometry and signal health.

The tab is organised into two non-collapsible "always visible" rows at the top -- sized to fit without scrolling -- followed by full-width collapsible sections below.

## Layout overview

```
+----------------------+------------------------------------+
|                      |                                    |
|   Clock              |   Summary                          |  Row 1 (no scroll,
|   (compact, ~4 lines)|   (compact unlabeled values)       |  not collapsible)
|                      |                                    |
+----------------------+--------------------+---------------+
|                                           |               |
|                                           |   Sky view    |
|   Map                                     |   (dots)      |  Row 2 (no scroll,
|   (tiles)                                 |   + GNSS      |  not collapsible)
|                                           |   legend      |
|                                           |               |
+-------------------------------------------+---------------+
| > PVT messages                                            |  Collapsible
+-----------------------------------------------------------+
| > Satellites                                              |  Collapsible
+-----------------------------------------------------------+
| Survey: 48.12N 11.57E +/-1.2m 5432 obs Valid              |  Conditional
+-----------------------------------------------------------+
```

The first two rows together occupy a fixed-height region intended to be visible without scrolling. Everything below scrolls.

## Row 1: clock and summary

Clock and summary sit side by side, sharing a common height (~4 lines of text).

### Clock
Same content as the existing clock component (UTC time, TAI, GPS time, leap seconds, source/accuracy), rendered more compactly to fit ~4 lines.

### Summary
Compact navigation summary -- same content as the existing nav-summary plan ([nav-summary.md](nav-summary.md)) **minus the time line** (the clock owns date/time and leap seconds).

Values are rendered without field labels where the meaning is unambiguous from context. Labels are kept where ambiguity demands them -- DOPs always carry their label (`HDOP`, `VDOP`, `PDOP`, etc.). Other potentially ambiguous tokens (e.g. `MSL` after height, `RTK`/`SBAS` correction badges) keep their inline label.

Content (drawn from [nav-summary.md](nav-summary.md), reorganised for compactness):

- Position: lat, lon, ellipsoidal height, MSL height
- Velocity: ground speed, course over ground
- Fix state: quality badge, dimensionality, correction leaf-bit badges, aux source badges
- Accuracy: horizontal, vertical, 3D, speed (omit when not available)
- DOPs (labelled): HDOP, VDOP, PDOP, GDOP, TDOP (only those available)
- Satellite counts: used/tracked

Lines or values with no data are omitted -- never rendered as blanks or dashes.

The summary is not collapsible. When no data is available, it is not rendered (it does not occupy space).

## Row 2: map and sky view

Below row 1, the map and sky view sit side by side. Both are roughly square visual elements.

### Map
OpenStreetMap tile map showing current position. Takes the larger share of horizontal space. See [map.md](map.md).

### Sky view
Polar satellite plot, smaller than the map. To save space, satellites are rendered as dots rather than labelled markers:

- **Color** -- one color per GNSS constellation (GPS, GAL, BDS, GLO, QZSS, SBAS).
- **Fill** -- hollow dot for satellites not contributing to the fix; solid dot for satellites used in the fix.

A **GNSS legend** (color-to-constellation mapping) sits beside the sky view (between the map and the sky view, or to the right of the sky view -- to be settled during implementation).

See [ui-panel-sky-view.md](ui-panel-sky-view.md) for sky view details; this document supersedes any prior sizing/labelling decisions there.

## Row 2 responsive behavior

At narrow widths the map and sky view stack vertically. The "no scroll" goal applies only at typical desktop widths -- on narrow windows the user may need to scroll.

Row 1 also wraps at narrow widths (clock above summary).

## Collapsible sections

Below row 2 is a stack of full-width collapsible sections. In order:

### PVT messages
Per-source-message tables for position, velocity, time. Already implemented (see [pvt-msgs-panel.md](pvt-msgs-panel.md)). Most-detailed/technical view of PVT data; placed first among the collapsibles since it is the next level of detail after the summary.

### Satellites
A flat table showing one row per signal, providing the in-depth view of satellite tracking and signal quality. This replaces the previously planned signal-strength panel ([signal-strength.md](signal-strength.md)).

Columns:
- GNSS (constellation)
- SVID
- Azimuth
- Elevation
- Signal (signal/band identifier)
- CN0 (signal strength, dB-Hz)
- Bar (visual indicator of CN0; CSS box scaled to CN0)

Additional columns may be added as `SatellitesMsg` exposes more information (e.g. health status, used flag, multipath indicator).

**Filtering** rather than tree expand/collapse:

- Constellation filter -- multi-select chips (GPS, GAL, BDS, GLO, QZSS, SBAS).
- Band filter -- multi-select (L1/L2/L5 or whatever bands the receiver reports).
- Used-only toggle -- hide signals not contributing to the fix.

Default sort is by constellation, then SVID, then signal. The bar column makes strength comparisons visually obvious without sorting by CN0.

Tree-style expand/collapse (by constellation, then by SV) was considered but deferred -- a flat filterable table is simpler to build and use. We can revisit if filtering proves insufficient in practice.

### Survey
Compact single-line survey-in status, only rendered when survey data is present. Not collapsible (a single line does not benefit from collapsing).

## Elements not in this tab

- **Standalone status block** -- removed. Its content is folded into the summary in row 1.
- **Standalone signal-strength panel** -- removed. Replaced by the Satellites table. The plan file [signal-strength.md](signal-strength.md) is superseded by this document.
- **Time panel** -- not present. Time information is shown in the clock (row 1) and in the PVT messages panel (per-source detail).
- **Live messages** (packet statistics) -- relocated to the Packets tab. See [archive/ui-packet-tab.md](archive/ui-packet-tab.md).

## Data sources

| Element | Backend event/message |
|---------|----------------------|
| Clock | `TimeMsg`, `NavEpochMsg` |
| Summary | `NavEpochMsg`, `PosGeoMsg`/`PosECEFMsg`, `VelGeoMsg` |
| Map | Position messages (`PosGeoMsg` or `PosECEFMsg` via `ECEFtoLLH`) |
| Sky view | `SatellitesMsg` (positions, look angles, used flag) |
| PVT messages | `PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`, `TimeMsg` |
| Satellites | `SatellitesMsg` (per-signal CN0, band, used flag, plus any extra fields) |
| Survey | `SurveyMsg` |

## Dependencies on backend work

- **Summary** requires `NavEpochMsg` metadata fields (from `plan/solution-metadata.md`) and position/velocity messages (from `plan/position-velocity-messages.md`).
- **Map** requires position messages (from `plan/position-velocity-messages.md`).
- **Sky view** and **Satellites** table require only the existing `SatellitesMsg`. Additional satellite metadata (health, etc.) can be added incrementally as backend support exposes it.
- **PVT messages** panel already exists.
- **Survey** panel already exists.
- **Clock** already exists; needs a compact rendering mode.
