# UI panel: signal view

## Purpose
Show per-satellite/per-signal strength and usage trends.

## Design

### Contents
- CN0 bar/graph visualization
- satellite labels grouped by constellation
- optional indicator of used-for-solution status

### Behavior
- updates from semantic satellite/signal events.
- support compact and expanded modes.

### Data dependencies
- semantic satellite signal payloads (satellite id, signals, cn0, usage)

### Controls
- sort mode (constellation vs strength)
- optional filter by constellation
- optional single/double-row density mode

## Implementation

### Component
`SignalViewPanel` component, rendered inside a `Panel` in the left column below Sky View.

### Rendering
Reuse/adapt SVG bar graph from `web/svg.tsx` (`SignalGraph` function). Port to a Preact component. Frontend controls thresholds, grouping, ordering, and density.

### Data source
Same semantic satellite events as Sky View panel.
