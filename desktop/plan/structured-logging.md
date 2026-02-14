# Structured logging

## Goal
Replace flat text log events with structured JSON so the frontend can filter by level, component, and attributes. Build a proper logging panel with severity badges and filtering.

## Prerequisite
layout-shell.

## Reference documents
- [ui-panel-logging.md](ui-panel-logging.md) - logging panel design (structured fields, filtering, progress display)
- [backend.md](backend.md) - logging stream payload shape

## Steps

### 1. Backend: rework eventHandler
Change `eventHandler` in `app.go` to emit structured events instead of flattening attrs into a string.

Current `LogEvent`:
```go
type LogEvent struct {
    Level   string `json:"level"`
    Message string `json:"message"`
    Time    string `json:"time"`
}
```

New `LogEvent`:
```go
type LogEvent struct {
    Level     string         `json:"level"`
    Message   string         `json:"message"`
    Time      string         `json:"time"`
    Component string         `json:"component,omitempty"`
    Attrs     map[string]any `json:"attrs,omitempty"`
}
```

Extract `component` from the slog logger name or a well-known attr. Pass remaining attrs as the map rather than stringifying them.

### 2. Frontend: update LogEntry type
Update `LogEntry` in the frontend to match the new structured payload.

### 3. Frontend: build LoggingPanel
Replace the simple log list from layout-shell with a proper panel:
- Severity badges (color-coded by level: DEBUG/INFO/WARN/ERROR)
- Filter by level (dropdown or toggle buttons)
- Filter by component (if present)
- Text search across message and attrs
- Auto-scroll to bottom with manual scroll override
- Timestamp column

### 4. Operation progress display
When a config operation is running (known from app state), highlight log entries from that time window. Show a pinned status line at the top of the logging panel indicating operation state (running/success/failed).

## Result
Log panel shows structured, filterable log entries. Users can quickly find errors, filter to a specific component, or see only warnings and above. Config operations have clear progress visibility.

## Testing (Playwright)

### Structured log display
- Verify log entries show severity badges (colored by level).
- Verify each log entry has a timestamp column.
- Verify log entries with attrs display the attrs (not flattened into the message).

### Filtering
- Use the level filter to show only WARN and above; verify DEBUG/INFO entries are hidden.
- Use the component filter (if entries with components exist); verify filtering works.
- Type in the text search box; verify only matching entries are shown.

### Auto-scroll
- Verify the log panel auto-scrolls to show new entries.
- Scroll up manually; verify auto-scroll pauses.
- Scroll back to bottom; verify auto-scroll resumes.

### Operation progress
- Trigger a config operation (e.g. click Apply with no hardware - expect an error).
- Verify the pinned status line shows operation state (running then error).

## Files changed
- `desktop/app.go` (eventHandler restructured)
- `desktop/frontend/src/app.tsx` (LogEntry type updated)
- `desktop/frontend/src/logging-panel.tsx` (major rewrite)
