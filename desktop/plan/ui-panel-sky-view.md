# UI panel: sky view

## Purpose
Visualize satellite geometry and usage at a glance.

## Design

### Contents
- sky polar view
- compass markers
- per-satellite labels
- optional usage/quality overlays

### Behavior
- updates from semantic satellite events, not packet parsing.
- should stay useful at low update rates.
- panel remains lightweight for continuous rendering.

### Data dependencies
- semantic satellite events (satellite id, look angles, usage state, signal summary)

### Controls
- optional filters by constellation
- optional toggle: show unused satellites

## Implementation

### Component
`SkyViewPanel` component, rendered inside a `Panel` in the left column.

### Rendering
Reuse/adapt SVG polar plot from `web/svg.tsx` (`SkyView` function). Port to a Preact component that accepts satellite data as props. Frontend decides color/opacity thresholds. No protocol-specific decoding in panel code.

### Data source
Subscribes to the semantic `*Msg` stream (`gps:msg` events) and extracts satellite position messages.
