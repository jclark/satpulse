# Windows support for `gps/lib/term`

## Goal

Add Windows serial I/O to `gps/lib/term`, matching the Unix serial-port
API used by `gps/app/gpsio` after `plan/serial-refactor.md`, including
read-time serial error reporting.

## Prerequisites

This plan assumes `plan/serial-refactor.md` is complete through step 7.
The important resulting contracts are:

- `gpsio.OpenSerial` already returns the effective speed, so Windows
  only needs to expose `(*term.Term).Speed()` internally to `gpsio`
  during open, as the Unix implementation does.
- `gpsio.SerialConn` reads through an `ioFile` interface and calls
  `Read` directly; serial error detection belongs inside `term.Read`,
  not in `gpsio`.
- `term.Error` with `ErrFlags` is the portable representation of
  read-time serial errors. `Counts` is optional and nil on platforms
  that do not expose per-category counters.
- `term.OpenPolling` is Linux-only. Windows uses the shared
  `polling_stub.go` (`//go:build !linux`) unchanged. FIFO/read-only
  support from serial-refactor step 8 is not a Windows prerequisite.
- `term.ErrNotATTY` is still a portable sentinel, but it is only used
  by Unix termios open paths. Windows does not return it; `CreateFile`
  on a non-COM path simply fails.

## Current state after serial-refactor

- Linux: termios serial support, `TIOCGICOUNT`-based serial error
  counts, `/dev/gnss0` polling-file fallback, and FIFO read-only
  replay support.
- macOS/BSD: termios serial support; no serial error detection; polling
  is stubbed by `polling_stub.go`.
- Windows: not supported; there is no `term_windows.go`.

## Approach

Implement `term_windows.go` directly with Win32 APIs:
`windows.CreateFile`, `GetCommState`/`SetCommState`,
`ReadFile`/`WriteFile`, `ClearCommError`, `PurgeComm`, and
`SetCommTimeouts`.

A third-party serial library is not a good fit because the Windows
implementation needs access to `ClearCommError` after reads so it can
surface `term.Error` consistently with Linux.

## Steps

### 1. Split shared types from platform implementations

Before adding the Windows implementation, make the serial-refactor file
layout explicit so the shared API is not trapped in a platform file.

- Use `types.go` for shared exported package types and sentinels:
  `ErrNotATTY`, `Error`, `ErrFlags`, `ErrorCounts`, and any related
  methods such as `SerialFraming`.
- Rename the current `types.go` file that defines the Unix-only
  `unixspeed` alias to `unixspeed.go`.
- Give `unixspeed.go` the build tag needed by the Unix termios files,
  excluding Windows.
- Keep `term.go` as the main non-Windows implementation file after
  serial-refactor step 7. Linux-specific files still provide the
  Linux-only helpers and `TIOCGICOUNT`-backed `term.Error`
  construction.
- When adding Windows, make sure the non-Windows implementation does
  not collide with `term_windows.go`; add `//go:build !windows` to
  `term.go` if it does not already have that constraint.
- Leave `polling_linux.go` and `polling_stub.go` as selected by their
  existing Linux/non-Linux tags. Keep the stub signature matched to
  the Linux `OpenPolling` signature that exists at the point Windows
  support lands; the FIFO/read-only signature expansion can land
  independently.

### 2. Add the Windows term implementation

Create `gps/lib/term/term_windows.go` with the same public API used by
`gpsio`:

- `Open(path string, opts ...AttrSetter) (*Term, error)`
- `(*Term).Init(path string, opts ...AttrSetter) error`
- `(*Term).Read`, `Write`, `Close`, `Restore`, `Flush`, `Buffered`,
  `Path`, `Speed`, `TransmitTime`, and `DevKind`
- `Speed(int) AttrSetter`, `ReadTimeout(time.Duration) AttrSetter`,
  `RawMode`, `Local`, `NoFlowControl`, and `IsValidSpeed`

`Attr` on Windows should wrap the Win32 `DCB` plus timeout settings.
Snapshot the original DCB and COMMTIMEOUTS at open, apply 8N1/no-flow
configuration, and restore both on close.

### 3. Opening and path handling

- Normalize COM names before opening. Bare `COM1` through `COM9` work
  with Win32, but always converting to `\\.\COMn` keeps `COM10` and
  above working too.
- Use `windows.CreateFile` with `GENERIC_READ|GENERIC_WRITE`,
  `shareMode=0` for exclusive access, `OPEN_EXISTING`, and no
  overlapped flag initially.
- Windows exclusive open replaces the Unix `flock` step; no separate
  lock helper is needed.

### 4. Speed and timeout behaviour

- Write the configured baud rate directly to `DCB.BaudRate`.
- Keep accepted speeds consistent with Unix by preserving the existing
  baud-rate table semantics for `IsValidSpeed`, even though Windows can
  accept more arbitrary rates.
- Configure read timeouts with `SetCommTimeouts` so a no-data read
  returns after roughly the same interval as the Unix VTIME path.
- Match serial-refactor step 6: a read timeout should return
  `(0, *os.PathError{Err: os.ErrDeadlineExceeded})` or another error
  whose `Timeout() bool` is true, so `scan.Scan` handles it exactly as
  it handles Unix and polling-file timeouts.

### 5. Read, write, and serial errors

- `Read` calls `windows.ReadFile`.
- After each successful read, call `ClearCommError` and map error flags
  to `term.ErrFlags`:
  - `CE_FRAME` -> `ErrFraming`
  - `CE_RXPARITY` -> `ErrParity`
  - `CE_OVERRUN` -> `ErrOverrun`
  - `CE_BREAK` -> `ErrBreak`
  - `CE_RXOVER` -> `ErrBufOverrun`
- If any mapped flag is set, return
  `(n, &term.Error{Path: t.path, Flags: flags, Counts: nil})`.
- `Write` loops until the full buffer is written, matching the Unix
  implementation's all-bytes-or-error behaviour.
- `Buffered` should use `ClearCommError`'s `COMSTAT.CbOutQue`.
- `Flush` should call `PurgeComm(PURGE_RXCLEAR | PURGE_TXCLEAR)`.

### 6. Device kind detection

Implement `DevKind` on a best-effort basis:

- Prefer registry/device-interface lookup for USB VID/PID and map known
  native USB CDC devices to `DevUSB` and USB-to-UART bridges to
  `DevUSBtoUART`.
- Return `DevUnknown` when lookup fails. This is acceptable because
  Windows support should not depend on perfect device classification.

## Testing

- Run normal Linux tests after the build-tag split:
  `go test ./gps/lib/term ./gps/app/gpsio ./gps/app/gpscfg ./time/app/daemon ./internal/gpscmd`
- Cross-compile the term package:
  `GOOS=windows go test ./gps/lib/term`
- Cross-compile the serial users:
  `GOOS=windows go test ./gps/app/gpsio ./gps/app/gpscfg ./time/app/daemon ./internal/gpscmd`
- On a Windows host with a real receiver, point `satpulsed` at the COM
  port and verify that read timeouts, configuration writes, and
  `term.Error.SerialFraming()` flow through `scan.Scan` and `gpscfg`.

## Result

`gps/lib/term` compiles on Windows and provides the same high-level
serial API as Unix. Windows serial faults are reported through
`term.Error` with flag-only detail (`Counts == nil`), while Linux keeps
its richer counter-backed error strings.

## Files changed

- `gps/lib/term/types.go` -- shared `ErrNotATTY`, `Error`, `ErrFlags`,
  `ErrorCounts`, and related methods
- `gps/lib/term/unixspeed.go` -- Unix-only `unixspeed` alias, renamed
  from the old `types.go`
- `gps/lib/term/term.go` -- main non-Windows term implementation
- `gps/lib/term/term_test.go` -- mark Unix-only if it depends on Unix
  devices or termios details
- `gps/lib/term/term_windows.go` -- new Win32 serial implementation
