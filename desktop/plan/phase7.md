# Phase 7: Message file support

## Goal
Allow the desktop app to load TOML message files and send user-defined messages to the receiver, with protocol-aware response formatting. This requires refactoring `internal/gpscmd` code into public packages.

## Prerequisite
Phase 1 (panel layout). Phase 2 (structured logging) recommended for visibility.

## Reference documents
- [ui-panel-packet-monitor.md](ui-panel-packet-monitor.md) - packet monitor design (enhanced with formatted output)
- [backend.md](backend.md) - packet stream payload shape
- `desktop/TODO.md` - describes the msgfile.go and response.go refactoring needs

## Steps

### 1. Refactor msgfile.go into public package
Move `internal/gpscmd/msgfile.go` and its types (`MsgFile`, `LineMsg`, `BinaryMsg`, `NMEAMsg`, `UBXMsg`, `CASBINMsg`, `ASBINMsg`) to a package under `gps/` (e.g. `gps/app/msgfile`).

This makes the TOML message file loader available to both `satpulsetool` and the desktop app without depending on `internal/`.

Update `internal/gpscmd/gpscmd.go` to import from the new package location.

### 2. Refactor response.go into public package
Move `internal/gpscmd/response.go` (`responsePrinter` and protocol-aware packet formatting for UBX, CASBIN, ASBIN, NMEA with hex dump fallback) to a package under `gps/` (e.g. `gps/app/pktfmt`).

The desktop packet monitor currently shows raw `pkt.Data` strings. With this formatting, it can show human-readable protocol-aware output.

Update `internal/gpscmd/gpscmd.go` to import from the new package location.

### 3. Backend: thin wrappers over message file packages
Add Wails-bound methods that are thin wrappers over the new `gps/app/msgfile` package APIs:
- Load a message file (delegates to `msgfile.Load`)
- Send messages from a loaded file with tag selection (delegates to the package-level send function)

The backend does not implement workflow logic — file parsing, tag selection, message conversion, and sequential send are all handled by the `gps/app/msgfile` package. The Wails layer just bridges the transport boundary and provides the connection.

### 4. Backend: enhanced packet formatting
Use the refactored response formatter in the packet event worker so `gps:packet` events include human-readable formatted output instead of raw data strings.

Update `PacketEvent` to include a `formatted` field with the protocol-aware rendering.

### 5. Frontend: message file UI
Add a message file section to the UI (either in the config panel or as a separate panel):

- File picker (open dialog) to select a TOML message file
- Display available tags and their descriptions after loading
- Tag selection checkboxes
- Send button
- Progress display via logging panel
- Response display in packet monitor

### 6. Frontend: improved packet monitor formatting
Update the packet monitor panel to display the `formatted` field from packet events when available, falling back to raw hex/ascii display.

## Testing (Playwright)

### Without hardware
- Verify file picker opens and accepts a .toml file.
- Load a sample message file; verify tags and descriptions are displayed.
- Verify tag selection checkboxes are interactive.
- Verify Send button is disabled when not connected.

### With hardware
- Connect to a receiver.
- Load a message file and select tags.
- Click Send; verify log entries show progress for each message sent.
- Verify packet monitor shows formatted responses.
- Verify errors are reported if a message fails.

### Packet formatting
- Verify packet monitor shows protocol-aware formatting (e.g. UBX class/id names) instead of raw hex.
- Verify NMEA packets show readable text.
- Verify unknown packets fall back to hex dump.

## Result
Users can load and send TOML message files from the desktop app, matching the `satpulsetool gps -m` workflow. Packet monitor shows human-readable formatted output.

## Files changed
- `gps/app/msgfile/` (new package, moved from internal/gpscmd/msgfile.go)
- `gps/app/pktfmt/` (new package, moved from internal/gpscmd/response.go)
- `internal/gpscmd/gpscmd.go` (updated imports)
- `desktop/app.go` (thin wrappers over msgfile/pktfmt packages, enhanced packet formatting)
- `desktop/frontend/src/msgfile-panel.tsx` or config panel extension (new)
- `desktop/frontend/src/monitor-panel.tsx` (formatted packet display)
