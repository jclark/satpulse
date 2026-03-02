# UI design: Monitor tab

## Purpose
Define the overall layout and content of the Monitor tab. This is the primary operational view -- it answers "what is my receiver doing right now?" across positioning, satellite tracking, and signal quality.

## Context
The app is a general-purpose GPS tool, not timing-specific. The Monitor tab prioritises position and velocity visibility alongside satellite geometry and signal health. Timing information is shown within the PVT messages panel rather than as a separate element.

## Elements

The Monitor tab contains these elements, listed top to bottom:

1. **Navigation summary** -- compact block showing position, velocity, time, fix quality, corrections, accuracy, DOPs, satellite counts. Always visible when data is available. Not collapsible. See [nav-summary.md](nav-summary.md).

2. **Map** -- OpenStreetMap tile map showing current position. See [map.md](map.md).

3. **Position scatter** -- east/north deviation plot from a reference point. See [position-scatter.md](position-scatter.md).

4. **Sky view** -- polar satellite plot. See [ui-panel-sky-view.md](ui-panel-sky-view.md).

5. **Signal strength** -- per-constellation signal graphs with per-signal bars colored by frequency band. Full width. See [signal-strength.md](signal-strength.md) (existing design).

6. **Survey** -- compact single-line survey-in status. Only visible when survey data is present.

7. **PVT messages** -- per-source-message tables for position, velocity, time. Full width. The most detailed/technical element; placed last. Already implemented (see [pvt-msgs-panel.md](pvt-msgs-panel.md)).

## Layout

### Navigation summary
A non-collapsible block at the top of the tab. Always visible when data is available. Shows position, velocity, time, fix quality, corrections, accuracy, DOPs, and satellite counts in a compact dense layout.

### Visual panels row
Below the status block, the map, position scatter, and sky view sit in a flex row. These are all roughly square visual elements with natural aspect ratios. They share horizontal space side by side at wide window widths and wrap/stack at narrow widths.

The visual panels are always visible when data is available -- not collapsible. Collapsing one element in a flex row creates awkward layout gaps.

### Full-width sections below
Signal strength and PVT messages are full-width collapsible sections below the visual panels row. Signal graphs benefit from horizontal space (many bars). PVT messages use wide tables. Survey is a single non-collapsible line.

### Overall structure

```
+-----------------------------------------------------+
| 48.1235N 11.5679E  523m  489m MSL  14:23:07 UTC  LS37|  Navigation
| 1.23m/s  247deg  Fixed 3D  Base Stn RTCM  12/24 SVs |  summary
| +/-0.014m hor  +/-0.021m vert  HDOP 0.8  PDOP 1.2   |
+-----------------+-----------------+------------------+
|                 |                 |                   |
|   Map           | Position        |  Sky View         |  Visual panels
|   (tiles)       | Scatter         |  (polar plot)     |  (flex row)
|                 | (E/N dots)      |                   |
+-----------------+-----------------+------------------+
| GPS  ||||||| |||                                     |
| GAL  |||||| ||                                       |  Signal graphs
| BDS  ||||||||||                                      |  (per constellation)
+-----------------------------------------------------+
| Survey: 48.12N 11.57E  +/-1.2m  5432 obs  Valid     |  Survey (conditional)
+-----------------------------------------------------+
| PVT Messages                                         |
|  Position: NAV-PVT  48.1234  11.5678  ...            |  Full-width
|  Velocity: NAV-PVT  1.23 m/s  ...                   |  tables
|  Time:     NAV-PVT  14:23:07  ...                    |  (most technical)
+-----------------------------------------------------+
```

The whole tab scrolls vertically. The visual panels row uses `flex-wrap` so elements wrap gracefully.

### Responsive behavior
- **Wide**: map, scatter, sky view sit side by side in one row.
- **Medium**: two elements in the first row, one wraps to a second row.
- **Narrow**: all three visual panels stack vertically.

Signal graphs, survey, and PVT messages are always full width.

## Elements not in this tab

- **Time panel** -- removed. Time information is shown in the PVT messages panel (per-source time table with UTC, TAI, leap seconds, accuracy, GNSS source). The solution status block shows epoch-level timing info if needed.

- **Live messages** (packet statistics) -- relocated to the Packets tab, which has been redesigned around a message-type table. See [ui-packet-tab.md](archive/ui-packet-tab.md).

## Data sources

| Element | Backend event/message |
|---------|----------------------|
| Navigation summary | `NavEpochMsg`, `PosGeoMsg`/`PosECEFMsg`, `VelGeoMsg`, `TimeMsg` |
| Map | Position messages (`PosGeoMsg` or `PosECEFMsg` via `ECEFtoLLH`) |
| Position scatter | Position messages (`PosGeoMsg` or `PosECEFMsg` via `ECEFtoLLH`) |
| Sky view | `SatellitesMsg` (satellite positions, look angles) |
| Signal strength | `SatellitesMsg` (per-signal CN0, band, used flags) |
| Survey | `SurveyMsg` |
| PVT messages | `PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`, `TimeMsg` |

## Dependencies on backend work

- **Solution status** requires `NavEpochMsg` metadata fields (from `plan/solution-metadata.md`).
- **Map** and **position scatter** require position/velocity messages (from `plan/position-velocity-messages.md`).
- **Sky view** and **signal strength** require only the existing `SatellitesMsg`.
- **PVT messages** panel already exists and works with current backend.
- **Survey** panel already exists.
