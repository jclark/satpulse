# Phase 8: Serial port enumeration

## Goal
Add serial port discovery so users can select a port from a dropdown instead of typing device paths manually. Works on Linux, macOS, and Windows.

## Current state
Serial port enumeration does not exist on any platform. Users must type the full device path (e.g. `/dev/ttyACM0`) into a text input.

## Prerequisite
Phase 5b (tab-based layout with connection bar).

## Reference documents
- [ui-panel-connection.md](ui-panel-connection.md) -- connection strip design (device input replaced with dropdown)
- `desktop/TODO.md` -- describes serial port enumeration need

## Approach

Use `go.bug.st/serial/enumerator` for port enumeration on all platforms. It wraps platform-specific APIs (SetupDi on Windows, IOKit on macOS, sysfs on Linux) and returns port name, VID/PID, and description. The dependency is added only to `desktop/go.mod`, so the main `gps/` module stays dependency-free.

We use the library only for enumeration, not for I/O.

## Steps

### 1. Backend enumeration

Add a new package `gps/lib/serialport` (or add to `gps/lib/term`) with a function like:

```go
type PortInfo struct {
    Path        string // e.g. "/dev/ttyACM0" or "COM3"
    Description string // e.g. "u-blox GNSS receiver"
    VID, PID    string // USB vendor/product ID if available
}

func ListPorts() ([]PortInfo, error)
```

Implementation calls `go.bug.st/serial/enumerator.GetDetailedPortsList()` and maps the result to our type, applying platform-specific filtering (e.g. skip `/dev/ttyS*` ports with no USB info on Linux, skip Bluetooth ports).

The Wails-bound method in `app.go` is a thin wrapper that calls this function.

### 2. Frontend combo box

Replace the device path text input in the connection strip with a combo box:
- Dropdown lists discovered ports with descriptions
- Refresh button to re-scan
- Still allows manual text entry for unlisted paths
- Auto-refresh on app start and when the dropdown is opened

## Testing (Playwright)

### Without hardware
- Verify the port selector dropdown exists in the connection strip.
- Verify the dropdown opens and shows "No ports found" or similar when no devices are connected.
- Verify manual text entry still works (user can type a custom path).
- Verify the refresh button exists and is clickable.

### With hardware
- Connect a USB GPS receiver.
- Click refresh; verify the port appears in the dropdown.
- Select the port from the dropdown; verify it populates the device field.
- Click Connect; verify connection succeeds.
- Disconnect the USB device; click refresh; verify the port disappears from the list.

### Cross-platform regression
- Re-run earlier phase Playwright tests on macOS and Linux to verify nothing broke.

## Result
Users can select serial ports from a dropdown instead of typing device paths. Works on Linux, macOS, and (once phase 9 adds Windows serial I/O) Windows.

## Files changed
- `gps/lib/serialport/` or `gps/lib/term/enum.go` -- port enumeration using `go.bug.st/serial/enumerator`
- `desktop/go.mod` -- add `go.bug.st/serial` dependency
- `desktop/app.go` -- thin wrapper for port enumeration
- `desktop/frontend/src/connection-panel.tsx` -- combo box replaces text input
