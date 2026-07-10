# UI panel: packet monitor

## Purpose
Provide low-level packet diagnostics for protocol/IO troubleshooting.

## Design

### Contents
- packet timeline rows
- protocol tag, message id, text/hex payload
- clear/pause controls

### Behavior
- optional panel; hidden by default.
- supports high-throughput stream without freezing UI.
- independent from semantic message panels.

### Data dependencies
- packet stream events (`gps:packet` events)

### Controls
- pause/resume live append
- clear view
- protocol filter
- text search

### Notes
This panel is diagnostic; it should not be required for normal operation.

## Implementation

### Component
`PacketMonitorPanel` component. When shown, rendered inside a `Panel` that can be placed in the right column or bottom strip depending on layout.

### Existing code
The current `monitor-panel.tsx` is essentially this panel. Refactor into the new component structure.

### Virtualized list
For high-throughput packet streams, use a virtualized/windowed list to avoid DOM bloat. Consider a lightweight virtual scroll implementation or cap visible rows with a ring buffer (current approach caps at 200 entries).

### Data source
Subscribes to `gps:packet` Wails events (existing).
