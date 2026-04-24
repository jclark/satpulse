# Serial refactor

Refactor `gps/lib/term` and `gps/app/gpsio` to support non-TTY GNSS devices
(Linux GNSS subsystem, e.g. `/dev/gnss0` on Intel ice) and to simplify the
path to Windows support. Issues: #255, #204.

### Platform scope

The polling-file path (steps 3, 5) is Linux-only. BSD/Darwin must
still compile: on those platforms, `term.OpenPolling` is a stub that
returns "polling not supported on this platform". `term.ErrNotATTY`
(step 2) is cross-platform -- each platform's `getAttr` returns it
when its ioctl returns `ENOTTY` -- so the fallback path exists on
BSD/Darwin but always errors out cleanly.

FIFO/pipe support and the read-only-port machinery (step 8) are
separated from the `/dev/gnss0` and Windows-enabling refactors because
they are not needed for either prerequisite path and naturally extend
across platforms.

## Step 1: OpenSerial returns the speed it used

`SerialConn.Speed()` goes away. `OpenSerial` returns the speed it configured
as a second return value. The caller is responsible for filling in the
configured speed when `OpenSerial` returns 0 (i.e. the device has no
meaningful baud rate).

The `speed` value that flows into `createConfigTarget` represents the
effective operating speed used for message-bandwidth gating (see
`GPSConfig.satsMsg` in `time/app/daemon/gps.go`). That value has one
meaning but two possible sources: device-reported (from termios on a
TTY) or user-asserted (from `cfg.Serial.Speed` on a device where the
kernel has no meaningful baud rate). When a user explicitly sets
`serial.speed = 38400` against a non-TTY device, they are asserting
their belief about the effective operating speed; that assertion is a
legitimate input for bandwidth gating.

### Changes

- `gpsio.OpenSerial(path string, speed int) (*SerialConn, int, error)`
  returns the actual speed of the opened device. For a TTY this is the
  termios speed (either the newly set value or the pre-existing one when
  the caller passed 0). For devices without a configurable speed it will
  eventually be 0; for now there is only the TTY path, so this just
  surfaces what `term.Term.Speed()` already returns.
- Delete `SerialConn.Speed()`.
- Update both callers of `gpsio.OpenSerial` for the new signature:
  `time/app/daemon/daemon.go:114` and
  `internal/gpscmd/gpscmd.go:62`. The daemon uses the returned speed
  as described below; `gpscmd` can discard it (the speed-dependent
  code path only runs in the daemon).
- In `time/app/daemon/daemon.go`, capture the returned speed and
  merge with the configured value:

  ```go
  conn, speed, err := gpsio.OpenSerial(cfg.Serial.Device, speed)
  ...
  if speed == 0 && cfg.Serial.Speed != nil {
      speed = *cfg.Serial.Speed
  }
  gct, err := createConfigTarget(lg, cfg, speed, clk != nil)
  ```

  Use the existing `speed` variable throughout; do not introduce a second
  name.

### Scope

This step does not change `term`, does not introduce the `ioFile`
interface, and does not add non-TTY support. It only moves the speed
lookup from a method call on `SerialConn` to a return value from
`OpenSerial`, which removes `Speed` from `SerialConn`'s public surface
ahead of the interface work in later steps.

## Step 2: add ErrNotATTY to term

Surface the "not a tty" condition as a sentinel so callers can branch
on it. No other changes.

### Changes

- `gps/lib/term`: export `ErrNotATTY`. In `term.Term.Init`, if
  `getAttr` fails with `unix.ENOTTY` (checked via `errors.Is`), return
  `ErrNotATTY` (wrapped so `errors.Is(err, term.ErrNotATTY)` works).
  Other `getAttr` failures propagate as-is. `gpsio` checks only
  `term.ErrNotATTY`, so the dependency on `unix.ENOTTY` is confined to
  `term`.

### Scope

Nothing else in `term` changes. No `gpsio` changes. No callers consume
the sentinel yet -- step 5 adds the `pollingFile` fallback that uses it.

## Step 3: factor out term.lock

Factor the `flock` call inside `term.Term.Init` into an unexported
helper so step 5's new `term.OpenPolling` function can reuse it.

### Changes

- `gps/lib/term`: add unexported `func lock(fd int, path string) error`
  that does `unix.Flock(fd, LOCK_EX|LOCK_NB)` and wraps any failure in
  a descriptive error ("could not lock device ...; probably being used
  by another process").
- `term.Term.Init` replaces its inline `unix.Flock` call with `lock`.

### Scope

Trivial refactoring. No behaviour change. No new API.

## Step 4: remove TransmitTime from OutPort

`TransmitTime` is only used on the speed-change path inside
`SerialConn.writeThenChangeSpeed`, which is already TTY-specific.
Nothing outside `gpsio` calls it through the `OutPort` interface.
Removing it from `OutPort` lets the step 5 `ioFile` interface
satisfy `OutPort` without implementations having to provide a
no-op `TransmitTime`.

### Changes

- `gps/app/gpsio/conn.go`: remove `TransmitTime` from the `OutPort`
  interface. Delete `SerialConn.TransmitTime` and `NetConn.TransmitTime`
  and the test fakes' `TransmitTime`. Inside
  `SerialConn.writeThenChangeSpeed`, call `c.term.TransmitTime(n)`
  directly.

### Scope

Independent of the other steps. Can land before or after step 2/3.

## Step 5: minimum to support /dev/gnss0

Get `/dev/gnss0` (and other non-TTY char devices that speak a GNSS
protocol but don't implement termios) working.

### Changes

- `gps/lib/term` (Linux): add `func OpenPolling(path string) (*os.File, error)`.
  Implementation:
  1. `fstat` the path first and dispatch on file type:
     - Regular file, block device, directory, socket, FIFO: reject
       with a clear error. FIFO support arrives in step 8.
     - Char device: open `O_RDWR|O_NOCTTY|O_CLOEXEC|O_NONBLOCK`, then
       probe with a throwaway `epoll_create1` + `epoll_ctl(EPOLL_CTL_ADD)`.
       If the probe fails (`EPERM`, etc.) the driver lacks `.poll`
       support; close and return a clear error.
  2. `lock(fd, path)` (from step 3). On failure, close and return the
     error.
  3. Wrap in `os.NewFile(uintptr(fd), path)` and return.
- `gps/lib/term` (BSD/Darwin): `OpenPolling` is a stub that returns
  "polling not supported on this platform". The signature matches the
  Linux one so callers compile unchanged.
- `gps/app/gpsio/serial.go`: introduce a minimal `ioFile` interface
  satisfied by both `*term.Term` and a new `pollingFile`:

  ```go
  type ioFile interface {
      io.ReadWriteCloser
      Path() string
      Buffered() (int, error)
  }
  ```

  `ioFile` satisfies `OutPort` (after step 4 removed `TransmitTime`).
  `SerialConn` holds `file ioFile` instead of `term *term.Term`. For
  any TTY-specific operation (speed change, flush, restore, error
  counts), `SerialConn` type-asserts to `*term.Term`; on a `pollingFile`
  the assertion fails and the operation is skipped. Skipping is the
  intended behaviour -- these operations are meaningless on a non-TTY
  device.
- `SerialConn.LocalAddr` returns `c.file.Path()`.
- `gps/app/gpsio/serial.go`: add a `pollingFile` implementation that
  uses Go's runtime netpoller for deadline-based timeouts. Constructor
  takes the `*os.File` returned by `term.OpenPolling` and a read
  timeout (passed by the caller -- `gpsio` already has `readTimeout`
  as a package-level constant for the term path).

  `pollingFile.Read` calls `f.SetReadDeadline(time.Now().Add(timeout))`
  before each `f.Read(p)`. When the deadline fires, Go returns
  `*os.PathError{Err: os.ErrDeadlineExceeded}`, which already satisfies
  `scan.TimeoutError` (`*os.PathError` forwards `Timeout() bool = true`
  from the wrapped `ErrDeadlineExceeded`). No translation needed --
  `scan.Scan` handles it identically to the existing
  `gpsio.timeoutError`.

  `Write`/`Close` delegate to the file. `Path` returns `f.Name()`
  (which is the path passed to `term.OpenPolling`). `Buffered` returns
  `(0, nil)`.

  This approach is verified empirically to work on standard TTYs
  (`/dev/ttyACM0` at 38400) and confirmed to work on `/dev/gnss0` by
  reading `drivers/gnss/core.c`: it implements `.poll` and respects
  `O_NONBLOCK`.
- `OpenSerial` dispatches: call `openTerm` first; if it returns
  `ErrNotATTY`, fall back to `term.OpenPolling` and build a
  `SerialConn` around a `pollingFile`. Otherwise proceed as today.
  On BSD/Darwin `term.OpenPolling` is a stub that errors, so the
  fallback surfaces a clear platform error.

### Scope

No changes to `ReadErrors` or error handling. No `DevKind` changes --
`SerialConn.isUART` is computed at open time in the TTY branch (from
`t.DevKind()`) and hard-coded false in the `pollingFile` branch.

`termRead` is renamed to `ioRead` and takes an `ioFile`. It always
does the read; the `GetErrorCounts` / `TermError` / `timeoutError`
accounting runs only when the argument is a `*term.Term` (via type
assertion). For a `pollingFile`, `ioRead` returns whatever the file
returned.

## Step 6: push timeout handling into term.Read

Make `term.Read` report a VTIME timeout as a timeout-flavoured error
directly, matching what `pollingFile.Read` already does. After this
step, `ioRead` no longer needs a zero-byte-read branch -- both `ioFile`
implementations return `(0, timeout-err)` on timeout themselves.

### Changes

- `term.Read`: when `unix.Read` returns `(0, nil)` (VTIME expired with
  no data), return `(0, &os.PathError{Op: "read", Path: t.path, Err:
  os.ErrDeadlineExceeded})`. This matches the error value
  `*os.File.Read` returns after `SetReadDeadline`, so it satisfies
  `scan.TimeoutError` the same way (via `*os.PathError`'s
  `Timeout()` forwarding).
- `gps/app/gpsio/serial.go`: remove the zero-byte-read branch from
  `ioRead`; it no longer has to synthesize `timeoutError`. The
  `gpsio.timeoutError` type is deleted, along with the
  `var _ scan.TimeoutError = timeoutError{}` assertion in
  `gps/app/gpsio/conn.go`.
- `ioRead` retains its `GetErrorCounts` / `TermError` handling for the
  term case. That moves in step 7.

### Scope

Observable behaviour is unchanged for `scan.Scan` -- the timeout error
it receives still satisfies `TimeoutError` and still reports
`Timeout() bool = true`. Only the concrete error type changes. No
changes to `term.Term`'s public API.

## Step 7: move serial error checking into term.Read

Move the `GetErrorCounts` check from `ioRead` into `term.Read` so
`SerialConn.Read` can call `c.file.Read(p)` directly through the
`ioFile` interface. `ioRead` is deleted. Replace `gpsio.TermError`
with a new portable `term.Error` type that also works for Windows.

### Changes

- `gps/lib/term`: introduce an exported `Error` type for serial
  errors reported by `term.Read`:

  ```go
  type Error struct {
      Path   string
      Flags  ErrFlags
      Counts *ErrorCounts  // nil when per-count info is not available
  }

  type ErrFlags uint32
  const (
      ErrFraming ErrFlags = 1 << iota
      ErrParity
      ErrOverrun
      ErrBreak
      ErrBufOverrun
  )

  type ErrorCounts struct {
      Framing, Parity, Overrun, Break, BufOverrun int32
  }
  ```

  `Flags` is the portable truth -- always populated when at least one
  error occurred. `Counts` is richer per-count info available on
  platforms where the kernel reports counts (Linux via `TIOCGICOUNT`);
  `nil` on Windows and macOS.

  `(*Error).Error()` renders `Counts` when present (preserving today's
  Linux format like `"fe=3 oe=1"`); otherwise it renders the set flag
  names. `(*Error).Temporary() bool` returns `true`.

- `term.Read`: on Linux, after `unix.Read` returns successfully, take
  a `TIOCGICOUNT` delta internally; if non-zero, construct and return
  `(n, &Error{Path: t.path, Flags: ..., Counts: &ErrorCounts{...}})`.
  On macOS/BSD (and any platform with no counter support), `term.Read`
  never returns `*Error` -- there's no way to detect serial errors, so
  nothing to report.

- `term.GetErrorCounts`, the exported `ErrorCounts` struct as a
  standalone public counter API, and `SerialICounter` become internal
  to `term` (or are removed entirely if no caller survives outside
  `term.Read`).

- `(*Error).SerialFraming() bool` returns `e.Flags & ErrFraming != 0`.
  This is how `gpscfg` detects framing errors without depending on the
  concrete type.

- `gps/app/gpsio/serial.go`: delete `ioRead` and the local `TermError`
  type. `SerialConn.Read` calls `c.file.Read(p)` directly. Remove the
  `var _ scan.TemporaryError = TermError{}` assertion in
  `gps/app/gpsio/conn.go`.
- Remove the compile-time assertions that reference `gpsio.TermError`:
  `internal/gpscmd/gpscmd.go:204` and
  `time/app/daemon/daemon.go:186`. Replace (or delete) with an
  equivalent assertion against `*term.Error` if the `gpscfg.SerialError`
  interface is still worth asserting from these call sites.

- `gps/app/gpscfg/gpscfg.go`: the unexported `SerialError` interface
  changes from `FramingErrs() int` to `SerialFraming() bool`. The
  three call sites that do `> 0` comparisons become bool checks. The
  `bad.framingErrs` counter (used only to decide which log line to
  emit) increments by 1 per read-with-framing-errors instead of by a
  kernel-reported count; the count value itself is never rendered, so
  this is a log-trigger change only.

### Scope

Observable behaviour: `scan.Scan` still sees a `TemporaryError` on
serial errors and the Linux `Error()` string still carries counts.

## Step 8: FIFO and read-only port support

Extend `OpenPolling` to accept FIFOs (named pipes), classify the
opened device with `term.DevKind`, and plumb two orthogonal predicates
(`ReadOnly` and `Direct`) up through `OutPort` so higher layers can
adapt their behaviour.

This enables the `satpulsetool pack --timing packets.jsonl > /tmp/fifo`
replay workflow (issues #246, #247) against a running satpulsed reading
from `/tmp/fifo` -- a test path for the full daemon pipeline that
doesn't require GPS hardware.

### Concepts

`ReadOnly` and `Direct` are orthogonal properties of a port:

- **ReadOnly**: writes to this port are rejected. Today this is true
  only for FIFOs; in future it could also be true for a
  permission-restricted TTY opened `O_RDONLY`. Consumers: `gpscfg`
  downgrades from probing to listen-only when true.
- **Direct**: the port is known to be a live hardware attachment --
  a classified UART, USB serial receiver, USB-to-UART bridge, or
  Bluetooth RFCOMM device -- that will produce data continuously
  when healthy. The predicate is deliberately whitelist-driven:
  only kinds we have classified from their kernel major number
  return true. Anything else (FIFO, socket, pseudo-terminal,
  unclassified TTY, `/dev/gnss0`) returns false on the conservative
  assumption that we cannot promise prompt input. Char devices
  aren't a reliable proxy for "attached" -- `/dev/ptyN` is a char
  device but clearly not a GPS receiver -- so we opt in rather than
  out. Consumers: none yet, but the predicate is plumbed so that
  `detect()` can later extend or remove its timeout for non-direct
  sources. Callers read `if !port.Direct() { ... wait patiently
  ... }`.

The whitelist default means `/dev/gnss0` is treated as non-direct
today, which is conservative -- if we later want to fast-fail on it
specifically, we can add a `DevGNSS` value classified from its sysfs
subsystem or device path and include it in `Direct()`.

### Changes

- `gps/lib/term`: add `DevFIFO` constant to the existing `DevKind`.
  No other new values: char devices without termios (e.g.
  `/dev/gnss0`) classify as `DevUnknown` under the existing
  major-number scheme, which is accurate -- the UART/USB/BT
  distinctions don't apply.
- `term.OpenPolling` (Linux) signature grows to
  `(*os.File, DevKind, error)`. The existing char-device branch
  returns `DevUnknown`. A new FIFO branch opens
  `O_RDWR|O_NONBLOCK` and returns `DevFIFO`. `O_RDWR` (not
  `O_RDONLY`) is used so that our own fd keeps the write side of the
  pipe open: an `O_RDONLY` FIFO with no writer returns EOF on every
  read, which would shut the scan worker down before any external
  writer connects. Self-feeding is prevented at the application
  layer -- `SerialConn.writeThenChangeSpeed` rejects writes when
  `c.ReadOnly()` is true (see the `SerialConn` bullet below).
- `term.OpenPolling` (BSD/Darwin): stub signature updated to match;
  still returns an error.
- `pollingFile`: unchanged from step 5 -- just `f *os.File` and
  `timeout time.Duration`. It does not need to know the kind: it is
  only constructed and used inside `SerialConn`, and write rejection
  happens at the `SerialConn` layer (see below) before any call
  reaches `pollingFile.Write`.
- `SerialConn`: drops the `isUART bool` field added in step 5 and
  stores `kind term.DevKind` directly. On the TTY open path, the
  kind is `t.DevKind()`; on the polling path, it is whatever
  `OpenPolling` returned (`DevUnknown` for char devices, `DevFIFO`
  for FIFOs). The speed-change branch in `writeThenChangeSpeed`
  tests `c.kind == term.DevUART` (previously `c.isUART`); this
  preserves the existing behaviour of using the `TransmitTime` delay
  for USB-serial converters and other non-UART TTYs.
  `writeThenChangeSpeed` rejects the write up front with a clear
  error when `c.ReadOnly()` returns true, so the read-only guard
  lives in one place and naturally picks up any future read-only
  cases that extend `ReadOnly()`.
- `gps/app/gpsio/conn.go`: add `ReadOnly() bool` and `Direct() bool`
  to the `OutPort` interface. Both are derived inside `gpsio` from
  the stored `term.DevKind`; `term` is not re-exported through the
  `OutPort` interface. `SerialConn.ReadOnly()` returns
  `kind == DevFIFO`. `SerialConn.Direct()` returns true only for
  `DevUART`, `DevUSB`, `DevUSBtoUART`, `DevBT`. `NetConn.ReadOnly()`
  returns false; `NetConn.Direct()` returns false. Test fakes:
  `ReadOnly=false`, `Direct=false`.
- `gps/app/gpscfg/gpscfg.go`: at the top of `Configure`, if
  `port.ReadOnly()` and configuration was wanted, log a warning
  ("device is not writable, not configuring GPS") and downgrade to
  the no-probe path (same path already taken when nothing needs
  configuring). `Direct()` is plumbed but not yet consumed -- the
  existing `listenTimeout` still applies, so a FIFO replay must
  begin within 2 seconds of satpulsed startup. A follow-up step
  can extend detection for non-direct sources.
