# Phase 9: Windows support

## Goal
Make the desktop app build and run on Windows with full serial I/O including error detection.

## Current state
`gps/lib/term` works on Linux and macOS/BSD:
- Linux: full support including serial error counters via `TIOCGICOUNT` ioctl
- macOS/BSD: full I/O support via BSD termios; `GetErrorCounts` returns zero (no kernel API for error stats); `DevKind` detects USB device types by path pattern
- Windows: **not supported** -- no `term_windows.go` exists

File path handling in the desktop app already uses `filepath.Join` and `os.Open` -- no issues found. The frontend device input is a plain text field with no Unix assumptions.

## Prerequisite
Phase 8 (serial port enumeration). The enumeration dropdown will already work on Windows via `go.bug.st/serial/enumerator`; this phase adds the serial I/O backend.

## Approach decisions

**Windows serial I/O:** roll our own `term_windows.go` using Win32 APIs (`windows.CreateFile`, `GetCommState`/`SetCommState`, `ReadFile`/`WriteFile`). This gives us control over file locking and serial error detection, matching the level of control we have on Linux. A third-party serial library would hide the error information we need.

**Serial error API:** simplify from counts to flags. The existing `ErrorCounts` struct with `int32` fields is over-specified -- all consumers just check whether errors occurred (`> 0`), not how many. Replace with boolean flags:

```go
type ReadErrors struct {
    Framing bool
    Parity  bool
    Overrun bool
}
```

This maps directly to Windows `ClearCommError` flags (`CE_FRAME`, `CE_RXPARITY`, `CE_OVERRUN`). On Linux, convert non-zero `TIOCGICOUNT` deltas to `true`. On macOS, return zero (no error info available). The `gpscfg.SerialError` interface changes from `FramingErrs() int` to `HasFramingErr() bool`.

## Steps

### 1. Revise serial error API

Replace `ErrorCounts` (int32 fields) with `ReadErrors` (bool fields). Update callers:

- `term.GetErrorCounts() ErrorCounts` becomes `term.ReadErrors() ReadErrors`
- `ReadErrors.IsZero()` returns true when all fields are false
- `gpsio.TermError` stores `ReadErrors` instead of `ErrorCounts`
- `gpsio.TermError.FramingErrs() int` becomes `HasFramingErr() bool`
- `gpscfg.SerialError` interface: `FramingErrs() int` becomes `HasFramingErr() bool`
- `gpscfg.mh.invalid()`: instead of accumulating `err.FramingErrs()`, just increment by 1 when `HasFramingErr()` is true (the accumulated count was only used for diagnostics)
- Remove `SerialICounter` from the public API; it becomes an internal implementation detail on Linux

This step can be done and tested on Linux/macOS before writing any Windows code.

### 2. Build tags and package structure

Current file layout:
- `term.go` -- core implementation (uses `golang.org/x/sys/unix`)
- `term_linux.go`, `term_bsd.go`, `term_darwin.go` -- Unix variant-specific code

Linux is the primary platform. Keep `term.go` as-is and add a `//go:build !windows` tag to it (and any other files that import `unix`). `term_windows.go` provides an alternative implementation of the same public API using Win32 calls. Shared types that need to be visible on all platforms (`ReadErrors`, `DevKind`, etc.) go in a new `types.go` with no build tag. Rename the existing `types.go` (which just defines the `unixspeed` type alias) to `unixspeed.go`.

### 3. Windows serial I/O (`term_windows.go`)

Add `gps/lib/term/term_windows.go` implementing the same `Term` API using Win32 calls:

- **Open/Init:** `windows.CreateFile` on `\\.\COMn` with `shareMode=0` for exclusive access. Configure with `GetCommState`/`SetCommState` on the `DCB` struct (baud rate, 8N1, no flow control). Set timeouts with `SetCommTimeouts`.
- **Read/Write:** `windows.ReadFile`/`windows.WriteFile`.
- **Speed:** map to DCB `BaudRate` field. Windows supports arbitrary baud rates natively (no Bxxx constants needed); `IsValidSpeed` can be more permissive or use the same table.
- **Flush:** `PurgeComm` with `PURGE_RXCLEAR | PURGE_TXCLEAR`.
- **Close/Restore:** `CloseHandle`. No termios to restore; save/restore DCB if needed.
- **Lock:** exclusive access is handled by `CreateFile` with `shareMode=0`.
- **DevKind:** return `DevUSB` or `DevUSBtoUART` based on USB VID/PID from the registry, or `DevUnknown`.
- **Path sanitisation:** prepend `\\.\` to COM port names (required for `COM10` and above, harmless for lower numbers).

#### Error detection on Windows

- **`ReadErrors`:** call `ClearCommError` after each read. Map flags: `CE_FRAME` -> `Framing`, `CE_RXPARITY` -> `Parity`, `CE_OVERRUN` -> `Overrun`.
- On Linux, keep `TIOCGICOUNT` delta logic internally, but return `ReadErrors{Framing: delta.Frame > 0, ...}`.
- On macOS, return zero struct (no error info).

Optional: `Buffered()` can use `ClearCommError`'s `cbOutQue`. `ModemStatus()` can use `GetCommModemStatus`. These can be deferred.

### 4. Wails Windows build

Verify the Wails build toolchain works on Windows:
- `wails build` should produce a `.exe`
- Test the embedded frontend works in the Windows WebView2 runtime

### 5. Installer/packaging (optional)
- Wails can produce a Windows installer (NSIS)
- Ensure the app icon and metadata are set in `wails.json`
- Code signing can be deferred

## Testing (Playwright)

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
The app builds and runs on Windows with serial error detection. Combined with phase 8 enumeration, users can select COM ports from a dropdown and connect to GPS receivers.

## Files changed
- `gps/lib/term/term.go` -- add `//go:build !windows` tag
- `gps/lib/term/types.go` -- shared types with no build tag (`ReadErrors`, `DevKind`, etc.); rename existing `types.go` to `unixspeed.go`
- `gps/lib/term/term_windows.go` -- new Win32 serial implementation
- `gps/app/gpsio/serial.go` -- update to use `ReadErrors`
- `gps/app/gpscfg/gpscfg.go` -- update `SerialError` interface
- `wails.json` -- Windows metadata
- `desktop/Makefile` or build scripts -- Windows build target

## Out of scope (future work)

**satpulsed on Windows:** the daemon has a hardcoded default log directory of `/var/log/satpulse` (`time/app/daemon/config.go`), which would need a platform-appropriate default on Windows (e.g. `%ProgramData%\SatPulse\logs`). More significantly, running as a Windows service requires the `golang.org/x/sys/windows/svc` package or similar -- different from the current Unix daemon model. This is separate from the desktop app port.
