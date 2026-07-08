# UI panel: logging

## Purpose
Display structured backend logs and operation progress for observability and troubleshooting.

## Design

### Contents
- streaming log rows (structured)
- severity badges
- operation progress timeline/phase state
- filters and search

### Required structured fields
- level
- message
- component
- category
- attrs (key/value map)
- timestamp

### Behavior
- visible by default in workspace.
- supports semantic filtering (component/category/protocol/time).
- frontend can scope logs to an operation by timestamp range.

### Long-running operation support
For ~15 second config operations:
- phase labels should remain visible,
- logs should clearly indicate progress,
- completion state should remain pinned (`success`, `warning`, `failed`).

### Data dependencies
- structured backend log events
- config progress/result events

### Notes
No flattened plain-text-only logging contract.
Frontend must be able to filter semantically.

## Implementation

### Component
`LoggingPanel` component, rendered inside a `Panel` as a full-width bottom strip.

### Existing code
Log display currently lives inline in `receiver-panel.tsx`. Extract into its own component.

### Backend change required
The current `eventHandler` in `app.go` flattens slog attrs into a text string. It needs to emit structured JSON events with separate `level`, `message`, `component`, and `attrs` fields so the frontend can filter semantically.

### Virtualized list
Same consideration as Packet Monitor: use windowed rendering or a ring buffer for high-volume log streams.

### Data source
Subscribes to `gps:log` Wails events (existing, but payload format needs restructuring per above).
