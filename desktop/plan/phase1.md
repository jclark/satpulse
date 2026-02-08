# Phase 1: Panel layout shell

## Goal
Replace the tab-based UI with a resizable panel grid using `react-resizable-panels`. Same functionality as today, but all panels visible simultaneously and resizable.

## Prerequisite
None. This is the first phase.

## Reference documents
- [ui-workspace-panels.md](ui-workspace-panels.md) - workspace layout model and `react-resizable-panels` usage
- [ui-panel-connection.md](ui-panel-connection.md) - connection strip design and implementation
- [ui-panel-logging.md](ui-panel-logging.md) - logging panel (extracted in this phase, improved in phase 2)
- [ui-panel-configuration.md](ui-panel-configuration.md) - config panel design
- [ui-panel-packet-monitor.md](ui-panel-packet-monitor.md) - packet monitor design
- [backend.md](backend.md) - backend event architecture

## Steps

### 1. Install react-resizable-panels
- `npm install react-resizable-panels` in `desktop/frontend/`

### 2. Create panel layout skeleton
Replace the tab bar and single-content-area structure in `app.tsx` with nested `PanelGroup`/`Panel`/`PanelResizeHandle` components.

Target layout:
```
+-----------------------------------------------+
| Connection strip (full width, fixed height)    |
+-------------------+---------------------------+
|                   |                           |
| Left column       | Right column              |
| (config panel)    | (packet monitor)          |
|                   |                           |
+-------------------+---------------------------+
| Logging strip (full width)                     |
+-----------------------------------------------+
```

The left/right split and bottom strip are all resizable. Connection strip is fixed height (not a resizable panel).

Note: Sky View and Signal View panels don't exist yet. The left column starts with just the config panel. Layout will be adjusted in later phases as panels are added.

### 3. Extract ConnectionPanel component
Move the header bar (device input, speed select, connect/disconnect button, status indicator) from `app.tsx` into `connection-panel.tsx`. Render it outside the `PanelGroup` as a fixed top strip.

### 4. Extract LoggingPanel component
Move the log display currently inline in `receiver-panel.tsx` into `logging-panel.tsx`. For now it just renders the existing flat log entries in a scrollable list. Structured logging comes in phase 2.

### 5. Refactor existing panels
- `config-panel.tsx` - keep as-is, rendered in a `Panel`
- `monitor-panel.tsx` - keep as-is, rendered in a `Panel`
- `receiver-panel.tsx` - the detect/refresh functionality stays for now; it will be rewritten as auto-probe in phase 4

### 6. Panel show/hide
Add a simple `Panels` menu (or toolbar toggles) to show/hide individual panels. When hidden, the panel is removed from the `PanelGroup` and remaining panels fill the space. Panel state is preserved when hidden.

### 7. Style resize handles
Style `PanelResizeHandle` with Tailwind - thin divider bar, visible on hover, drag cursor.

## Result
All existing functionality works. Panels are visible simultaneously. User can drag dividers to resize. Packet monitor and config panel are no longer mutually exclusive tabs.

## Testing (Playwright)

Test against the Vite dev server (localhost:5173) or Wails dev server (localhost:34115).

### Layout structure
- Verify the connection strip is visible at the top with device input, speed select, and connect button.
- Verify at least two panel areas are visible simultaneously (no tab switching required).
- Verify resize handles exist between panels.

### Panel resizing
- Drag a resize handle and verify panel dimensions change.
- Verify panels have minimum sizes (don't collapse to zero).

### Panel show/hide
- Toggle a panel off via the Panels menu and verify it disappears.
- Toggle it back on and verify it reappears.
- Verify remaining panels fill the space when one is hidden.

### Existing functionality preserved
- Device input and speed select are present and editable.
- Connect button is clickable (test without real hardware: verify it attempts connection and shows error state).
- Config panel fields are visible and interactive.
- Packet monitor area is visible.
- Log entries area is visible.

## Files changed
- `desktop/frontend/package.json` (new dependency)
- `desktop/frontend/src/app.tsx` (major rewrite)
- `desktop/frontend/src/connection-panel.tsx` (new, extracted from app.tsx header)
- `desktop/frontend/src/logging-panel.tsx` (new, extracted from receiver-panel.tsx)
- `desktop/frontend/src/config-panel.tsx` (minor: remove tab awareness)
- `desktop/frontend/src/monitor-panel.tsx` (minor: remove tab awareness)
- `desktop/frontend/src/receiver-panel.tsx` (minor: log display extracted)
