# Windows support for `gps/lib/term`

## Goal
Add Windows serial I/O to `gps/lib/term`, matching the Linux level of
control including serial error detection.

## Prerequisites

This plan assumes `plan/serial-refactor.md` is complete. In particular:

- **Step 8**: `term.Error` with an `ErrFlags` bitmask is the portable
  representation of read-time serial errors, with an optional
  `*ErrorCounts` for platforms that expose per-category kernel
  counters. Windows will populate `Flags` only (`Counts` nil).
- **Step 5**: `term.OpenPolling` is a Linux-only feature; all
  non-Linux platforms share a stub in `polling_stub.go`
  (`//go:build !linux`) that returns "polling not supported on this
  platform". Windows inherits this stub unchanged.
- **Step 2**: `term.ErrNotATTY` is the sentinel returned from
  `getAttr` when the underlying ioctl reports `ENOTTY`. Windows has
  no termios so this does not apply there; `CreateFile` on a
  non-comm handle simply fails at open time.

## Current state

`gps/lib/term` works on Linux and macOS/BSD:
- Linux: full support including per-error counters via `TIOCGICOUNT`,
  and `term.OpenPolling` for non-TTY char devices (e.g. `/dev/gnss0`).
- macOS/BSD: full termios I/O; `term.Error` carries only `Flags`;
  `term.OpenPolling` is stubbed via `polling_stub.go`
  (`//go:build !linux`) returning "polling not supported on this
  platform".
- Windows: **not supported** -- no `term_windows.go`.

## Approach

Roll our own `term_windows.go` using Win32 APIs (`windows.CreateFile`,
`GetCommState`/`SetCommState`, `ReadFile`/`WriteFile`,
`ClearCommError`). This matches the Linux level of control and lets
us surface serial errors through the `term.Error` type from
serial-refactor step 8. A third-party serial library would hide that
information.

For `term.OpenPolling`, Windows has no equivalent of the Linux GNSS
subsystem or the `/tmp/fifo` replay workflow, so it's covered by the
shared `polling_stub.go` (`//go:build !linux`) with no
Windows-specific code.

## Steps

### 1. Build tags and package structure

For Windows:

- Add `//go:build !windows` to files that import
  `golang.org/x/sys/unix`: `term.go`, `term_test.go`.
- Widen `types.go`'s build tag from `!freebsd` to
  `!freebsd && !windows` (it defines the Unix-only `unixspeed`
  alias). Optionally rename it to `unixspeed.go` for clarity.
- Add `term_windows.go` for the Windows implementation. The
  `_windows` filename suffix selects it automatically.
- `polling_stub.go` (`//go:build !linux`) already covers Windows --
  no new polling file needed.

### 2. Windows serial I/O (`term_windows.go`)

Implement the same public API as `term.go` using Win32:

- **Open/Init:** `windows.CreateFile` on `\\.\COMn` with
  `shareMode=0` for exclusive access (no separate `flock` step
  needed). Configure via `GetCommState`/`SetCommState` on the `DCB`
  struct (baud rate, 8N1, no flow control). Set read timeouts with
  `SetCommTimeouts` to match the Linux ~100ms VTIME behaviour.
- **Read/Write:** `windows.ReadFile`/`windows.WriteFile`.
- **Error detection (the reason for the DIY approach):** after each
  successful `ReadFile`, call `ClearCommError` and map the returned
  flags to `term.ErrFlags`:
  - `CE_FRAME`    -> `ErrFraming`
  - `CE_RXPARITY` -> `ErrParity`
  - `CE_OVERRUN`  -> `ErrOverrun`
  - `CE_BREAK`    -> `ErrBreak`
  - `CE_RXOVER`   -> `ErrBufOverrun`
  When any flag is set, return
  `(n, &term.Error{Path: t.path, Flags: ..., Counts: nil})`.
  Windows does not report per-category counters, so `Counts` stays
  nil and `(*Error).Error()` falls back to rendering the flag names.
- **Speed:** write directly to `DCB.BaudRate`. Windows accepts
  arbitrary baud rates; keep the existing `baudRates` table for
  `IsValidSpeed` so accepted speeds remain consistent across
  platforms.
- **Flush:** `PurgeComm(PURGE_RXCLEAR | PURGE_TXCLEAR)`.
- **Buffered:** `ClearCommError`'s `COMSTAT.cbOutQue`.
- **Close/Restore:** snapshot the DCB at Open, restore it at Close,
  then `CloseHandle`.
- **DevKind:** look up USB VID/PID via the registry and return
  `DevUSB` / `DevUSBtoUART` / `DevUnknown`.
- **Path sanitisation:** prepend `\\.\` to COM port names so
  `COM10` and above work.

## Testing

- Run existing tests on Linux and macOS after the build-tag changes to
  confirm nothing regressed.
- `GOOS=windows go vet ./gps/lib/term` to confirm the package builds.
- On a Windows host with a real GPS, point `satpulsed` at the COM
  port and verify that `term.Error` flows through `scan.Scan` exactly
  as on Linux (`TemporaryError` handling; log-line trigger via
  `(*Error).SerialFraming()`).

## Result
`gps/lib/term` compiles and works on Windows with serial error
detection, surfacing errors through the platform-agnostic
`term.Error` type defined in serial-refactor step 8.

## Files changed
- `gps/lib/term/term.go` -- add `//go:build !windows`
- `gps/lib/term/term_test.go` -- add `//go:build !windows`
- `gps/lib/term/types.go` -- widen tag to `!freebsd && !windows`
  (optionally rename to `unixspeed.go`)
- `gps/lib/term/term_windows.go` -- new Win32 serial implementation
