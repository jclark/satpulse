# Monitor tab UI rewrite

## Goal

Restructure the Monitor tab to the layout described in [ui-monitor-tab.md](ui-monitor-tab.md):

- **Row 1** (no scroll, not collapsible): compact clock + summary, fixed height.
- **Row 2** (no scroll, not collapsible): map + sky view + GNSS legend, two squares side by side with the legend as a third column.
- Below: collapsible PVT messages, Satellites table, Survey single-line.

Branch: `desktop-monitor-ui`.

## What's already prototyped (current state of the branch)

- Sky view rewritten to use dots (filled = used, filled-with-hole = unused), color per GNSS, opacity from CN0.
- GNSS legend extracted to a separate component (`SkyViewLegend`) sized to match plot dot size via a shared `--sky-unit-px` CSS variable written from the sky view's ResizeObserver.
- Default window height bumped to 900 in `main.go`.
- Row 2 prototype in [app.tsx](../src/app.tsx) uses a placeholder X-marked square in place of the map (because `MapPanel` is fixed at 256x256 and cannot fluidly fill its container).
- Clock panel temporarily moved to its own row alongside `MapPanel` so we can experiment with row 2 in isolation.

These prototypes are throwaway scaffolding, not the final layout. The blocking issues that prevent the real layout are the next two stages.

## Stages

### Stage 1: Make `MapPanel` fluidly resize

`MapPanel` ([map-panel.tsx](../src/map-panel.tsx)) currently has `SIZE = 256` hardcoded. Tile math, viewport pixel positions, and the position-marker SVG all assume that size. The container is `width: 256px; height: 256px;` inline-styled, so it cannot grow to fill its parent.

Make it size to its container:

- Add a `useRef` on the outer div and a `ResizeObserver` to track its rendered size.
- Drive `SIZE` from the observed container width (always render a square, so width and height are equal).
- Recompute `HALF`, the dot position, and the tile grid offsets from the dynamic `SIZE`.
- Keep the same 256-pixel OSM tiles - just render more of them when the viewport is bigger. At `SIZE = 512`, draw a 3x3 grid of tiles instead of 2x2.
- Set the container to `width: 100%; height: 100%;` so it fills its parent.
- Test at 256, 400, 600, 800 px wide; verify the position marker stays correctly placed and tiles re-fetch sensibly when the dot approaches the edge.

Once this is done, the row 2 placeholder can be replaced with the real `MapPanel`, and the map will be a true square that scales with the row.

### Stage 2: Make `ClockPanel` fluidly resize

`ClockPanel` ([clock-panel.tsx](../src/clock-panel.tsx)) renders the DSEG7 web font as plain HTML text with hardcoded `fontSize: 72px` (HH:MM), `26px` (date), `36px` (seconds + tz). Container is fixed at `W = 320, H = 256`.

Make it scale to its container:

- Wrap the clock content in an element with `container-type: size`.
- Replace hardcoded `px` font sizes with container-relative units (e.g. `cqh`) so they scale with the container's height.
- Replace the fixed `W`/`H` container size with `width: 100%; height: 100%;`.
- The "ghost 8s" layer needs to scale identically (same `cqh` size) to stay aligned behind the real digits.
- Verify at small sizes (~80 px tall, the row-1 fixed height) that the digits stay legible and aligned.

### Stage 3: Build the row 1 summary block

New component `SummaryPanel` ([nav-summary.md](nav-summary.md) defines the content). Compact, unlabelled values where unambiguous; DOPs always labelled.

- Drop the time line from [nav-summary.md](nav-summary.md) - the clock owns date/time/leap seconds.
- Phase 1 of nav-summary: position (lat/lon/height/MSL), velocity (speed, course).
- Phase 2: fix quality badge, correction leaf bits, accuracy, DOPs, SV counts.
- Updates on `NavEpochMsg` (epoch-synced).

### Stage 4: Compose row 1 and row 2

Restructure the Monitor tab in [app.tsx](../src/app.tsx):

- Row 1: clock (left) + summary (right), fixed height, both fill that height.
- Row 2: map (square, flex-1) + sky view (square, flex-1) + legend (auto width).
- Both rows are non-collapsible. Together they should fit in the default 1024x900 window without scrolling, with a peek at the first collapsible section below.
- Remove the temporary placeholder square and the temporary clock-on-its-own-row layout.

### Stage 5: Replace the standalone Status panel

The current Status panel (collapsible, below row 2) is folded into the row-1 summary. Remove the `<CollapsibleSection title="Status">` block and the `StatusPanel` import once the summary block covers the same content.

### Stage 6: Satellites table

New collapsible section replacing the (planned but unimplemented) signal-strength panel. Flat table with constellation, SVID, az, el, signal, CN0, bar. Filter chips for constellation and band, plus a "used only" toggle.

[signal-strength.md](signal-strength.md) is superseded by this work; either delete it or mark it superseded with a pointer here.

### Stage 7: Plan file cleanup

- Update [nav-summary.md](nav-summary.md) to drop the time line.
- Mark or remove [signal-strength.md](signal-strength.md).
- Confirm [ui-monitor-tab.md](ui-monitor-tab.md) matches the implemented layout.

## Out of scope

- TCP connections, corrections-provide mode, map tile retry, packet-fix-rate - all listed in [issues.md](issues.md), unrelated to the monitor layout.
- Position scatter panel - keep as-is for now; it remains a collapsible section.
