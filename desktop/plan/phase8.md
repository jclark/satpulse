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

### 1. Backend: `serial.go`

Add `serial.go` in `desktop/`. This file wraps `go.bug.st/serial/enumerator` and gives the frontend exactly what it needs: two strings per port.

```go
// PortInfo is returned by ListPorts for the frontend dropdown.
type PortInfo struct {
    Device  string `json:"device"`  // device path passed to Connect (e.g. "/dev/cu.usbmodem312301", "COM11")
    Display string `json:"display"` // human-readable label for the dropdown
}

func (a *App) ListPorts() ([]PortInfo, error)
```

The library API (`go.bug.st/serial/enumerator`):

```go
// enumerator.GetDetailedPortsList() returns ([]*enumerator.PortDetails, error)
//
// type PortDetails struct {
//     Name         string // e.g. "/dev/ttyACM0" or "COM3"
//     IsUSB        bool
//     VID          string // USB vendor ID (hex string)
//     PID          string // USB product ID (hex string)
//     SerialNumber string
//     Product      string // OS-dependent; only populated on Windows (SetupDi friendly name)
// }
```

`serial.go` builds `Display` from the library fields:

Step 1: derive a u-blox tag from VID/PID (may be empty):
- VID `1546` with PID in `0x01A4`..`0x01AF`: `"u-blox gen N"` where N = PID - 0x1A0
- VID `1546` with other PID: `"u-blox"`
- Other VID: empty

Step 2: build `Display`:
- If `Product` is non-empty: `"Product - tag"` or just `Product` if no tag. On Windows, `Product` is the SetupDi friendly name (e.g. `"CDC-ACM (COM44)"`). Linux and macOS leave it empty (reads are commented out in the library).
- Otherwise: `"path (tag)"` or just `path` if no tag.

Examples:
- macOS u-blox: `/dev/cu.usbmodem312301 (u-blox gen 9)`
- Windows u-blox: `CDC-ACM (COM44) - u-blox gen 9`
- Unknown USB on Linux: `/dev/ttyUSB0`

Filtering (macOS only): skip ports where `IsUSB == false` (Bluetooth, debug-console -- macOS has no real non-USB serial ports). On Linux and Windows, list all ports.

On macOS, the library uses the IOKit `IOCalloutDevice` property, so it returns only `/dev/cu.*` paths (the correct outgoing/callout device), not the `/dev/tty.*` (incoming/dialin) variant.

Also add `desktop/cmd/listports/main.go` -- a small command-line tool that calls the same enumeration logic and prints one line per port: `device-name display-name`. It's in its own `package main` under `cmd/`, so it won't be pulled into the Wails build. Use `go run ./cmd/listports/` to test enumeration without the GUI.

### 2. Frontend combo box

Replace the device path text input with a combo box (text input with a dropdown):
- The input is always editable -- user can type any device path (e.g. `/dev/cu.usbmodem*` on macOS, `/dev/ttyACM*` on Linux)
- Selecting a port from the dropdown populates the text input with `Device`
- The dropdown shows `Display` for each entry
- Ports are fetched on app start and each time the dropdown is opened
- The frontend has no VID/PID logic -- all display formatting is in the backend

## Testing (Playwright)

### Without hardware
- Verify the port selector combo box exists in the connection strip.
- Verify the dropdown opens (may be empty if no devices connected).
- Verify manual text entry works (user can type a custom path and connect).

### With hardware
- Connect a USB GPS receiver.
- Open the dropdown; verify the port appears in the list.
- Select the port from the dropdown; verify it populates the device field.
- Click Connect; verify connection succeeds.
- Disconnect the USB device; re-open the dropdown; verify the port disappears from the list.

### Cross-platform regression
- Re-run earlier phase Playwright tests on macOS and Linux to verify nothing broke.

## Result
Users can select serial ports from a dropdown instead of typing device paths. Works on Linux, macOS, and (once phase 9 adds Windows serial I/O) Windows.

## Files changed
- `desktop/go.mod` -- add `go.bug.st/serial` dependency
- `desktop/serial.go` -- `PortInfo` type, `ListPorts` method, display formatting, filtering
- `desktop/frontend/src/connection-panel.tsx` -- combo box replaces text input
