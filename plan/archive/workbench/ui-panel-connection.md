# UI panel: connection

## Purpose
Provide connection/session control and immediate connection status.

## Design

### Contents
- device/path input
- local serial speed input
- connect/disconnect button
- connection status indicator
- last connection error summary (if any)

### Behavior
- always visible in top workspace strip.
- connect/disconnect action should not navigate away from current layout.
- connection errors appear in-place and in Logging panel.

### States
- disconnected
- connecting
- connected
- disconnecting
- error

### Data dependencies
- backend connection status events
- backend capability summary after successful probe/detect

### Notes
This panel owns connection state display.

## Implementation

### Component
Single `ConnectionPanel` component rendered as a non-resizable top strip in the outermost `PanelGroup`.

### Existing code
The current header in `app.tsx` (device input, speed select, connect button, status dot) is essentially this panel already. Refactor into its own component file.

### Wails bindings used
- `Connect(device, speed)` / `Disconnect()` (existing)
- `IsConnected()` (existing)
