# Phase 8: Serial port improvements

## Goal
Enhance serial port support: automatic port discovery with a dropdown selector, cross-platform serial port compatibility (macOS and Windows), and Windows build support.

## Prerequisite
Phase 5b (tab-based layout with connection bar). All earlier phases should be stable on macOS/Linux first.

## Reference documents
- [ui-panel-connection.md](ui-panel-connection.md) - connection strip design (device input replaced with dropdown)
- `desktop/TODO.md` - describes serial port enumeration need and Windows serial port (`gps/lib/term`) gap

## Steps

### 1. Evaluate serial port approach
`gps/lib/term` uses Linux-specific terminal ioctls. It needs to work on macOS and Windows too. Options (TBD):
- Use a Go serial library (e.g. `go.bug.st/serial`) for all platforms or just non-Linux
- Write platform-specific implementations with build tags
- Use `go.bug.st/serial` only for enumeration, keep `gps/lib/term` for IO on Linux

Decision depends on how much of `gps/lib/term` is reusable across platforms vs how much needs replacing.

### 2. Serial port discovery

#### Backend
Add a package-level serial port enumeration function under `gps/` (e.g. in `gps/lib/term` or a new package) that returns available serial ports. Platform-specific filtering logic lives in the package, not in the Wails layer.

The Wails-bound method is a thin wrapper that calls the package function and returns the result.

#### Frontend
Replace the device path text input in the connection strip with a combo box:
- Dropdown lists discovered ports with descriptions
- Refresh button to re-scan
- Still allows manual text entry for unlisted paths
- Auto-refresh on app start

### 3. macOS serial port support
Review and fix `gps/lib/term` for macOS:
- Test with a real USB serial device on macOS
- Replace Linux-specific `syscall` calls with platform-appropriate equivalents, or use the chosen library

### 4. Windows serial port support
Make serial IO work on Windows:
- Either extend `gps/lib/term` with Windows build tags, or use a cross-platform library
- Use build tags to select the right implementation per platform

### 5. Wails Windows build
Verify the Wails build toolchain works on Windows:
- `wails build` should produce a `.exe`
- Ensure CGo dependencies (if any) work with MinGW or MSVC
- Test the embedded frontend works in the Windows WebView2 runtime

### 6. Path handling
Review all file path handling for Windows compatibility:
- Serial port paths are `COM3` etc., not `/dev/ttyACM0`
- Message file paths use backslashes
- Any hardcoded Unix paths in the frontend or backend

### 7. Installer/packaging (optional)
- Wails can produce a Windows installer (NSIS)
- Ensure the app icon and metadata are set in `wails.json`
- Code signing can be deferred

## Testing (Playwright)

### Port discovery (without hardware)
- Verify the port selector dropdown exists in the connection strip.
- Verify the dropdown opens and shows "No ports found" or similar when no devices are connected.
- Verify manual text entry still works (user can type a custom path).
- Verify the refresh button exists and is clickable.

### Port discovery (with hardware)
- Connect a USB GPS receiver.
- Click refresh; verify the port appears in the dropdown.
- Select the port from the dropdown; verify it populates the device field.
- Click Connect; verify connection succeeds.
- Disconnect the USB device; click refresh; verify the port disappears from the list.

### Windows
- Verify the app launches and shows the tab-based layout.
- Verify serial port enumeration lists COM ports.
- Connect to a receiver via COM port; verify connection succeeds.
- Verify packet monitor shows incoming packets.
- Verify configuration operations work (detect, apply).
- Verify message file loading works with Windows paths.

### Cross-platform regression
- Re-run earlier phase Playwright tests on macOS and Linux to verify nothing broke.

## Result
Users can select serial ports from a dropdown instead of typing device paths. The app builds and runs on macOS, Linux, and Windows.

## Files changed
- `gps/lib/term/` (platform-specific build tags or replacement)
- `desktop/go.mod` (possible new dependency)
- `desktop/app.go` (thin wrapper over serial port enumeration package)
- `desktop/frontend/src/connection-panel.tsx` (combo box replaces text input)
- `wails.json` (Windows metadata)
- `desktop/Makefile` or build scripts (Windows build target)
