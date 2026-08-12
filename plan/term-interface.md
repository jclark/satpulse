# Terminal implementation interface

Refactor `gps/lib/term` so that `Term` is the public interface for a
configurable serial terminal rather than the name of a platform-specific
concrete struct. This is infrastructure work independent of serial PPS and
D2XX support.

## Motivation

The package currently exposes `*term.Term`, but that name denotes unrelated
concrete types on Unix and Windows. The Unix definition contains termios and
file-descriptor state, including a Linux-only serial error counter represented
by a dummy type on Darwin and FreeBSD. The Windows definition instead contains
a Windows handle, DCB, and timeout state.

At the next layer, `gpsio.SerialConn` already stores a small `ioFile`
interface. It distinguishes a terminal from non-terminal fallbacks by
asserting that the value is specifically a `*term.Term`. Consequently, a new
terminal transport cannot participate as a peer implementation: it must be
hidden inside the existing concrete struct or specially wrapped in `gpsio`.

The refactoring makes the capability boundary explicit. Native Unix and
Windows terminals become private implementations of a public `Term`
interface. Non-terminal inputs remain implementations of the smaller
file-like interface in `gpsio` and do not pretend to be terminals.

## Scope

This change preserves the behavior of the existing native terminal and
fallback paths. It does not add D2XX, serial PPS, modem-change waiting, or any
other transport or capability. Those changes are rebased onto this work
later.

This interface separates implementations at the I/O and lifecycle boundary;
it does not make terminal configuration transport-neutral. On Unix,
`AttrSetter` still mutates an `Attr` backed by `unix.Termios`. A future Darwin
implementation such as D2XX must interpret the resulting termios fields --
including baud rate, character size, parity, stop bits, flow control, and
`VMIN`/`VTIME` -- and translate them to its native API. Replacing `Attr` with a
transport-independent configuration model would be a separate refactoring.

The existing `OpenFallback` behavior and the division between `term.File`
and `gpsio.pollingFile` remain unchanged. Redesigning that API is a separate
concern.

## Public interface

Define `Term` in the existing build-independent `types.go` as the common,
required terminal surface:

```go
type Term interface {
	io.ReadWriteCloser

	Path() string
	Buffered() (int, error)
	Change(...AttrSetter) error
	Speed() int
	TransmitTime(int) time.Duration
	DevKind() DevKind
	Flush() error
	Drain() error
	Restore() error
}
```

`Open` returns `(Term, error)` rather than `(*Term, error)`. Calls that infer
the result type and use only the required `Term` methods continue to work
unchanged. An inferred result that calls a concrete-only or optional method
must add the corresponding capability assertion; `pollpps` is the in-tree
example because `ModemStatus` is deliberately absent from `Term`. Functions,
fields, and variables that explicitly name `*term.Term` change to `term.Term`;
a pointer to an interface is never used.

The platform-specific `Open` functions also return `(Term, error)`, not a
concrete pointer. Each allocates its concrete implementation, calls its private
`init` method, and explicitly returns `nil, err` when initialization fails.
This preserves nil-on-error semantics: converting a nil `*unixTerm` or
`*windowsTerm` directly to `Term` would produce a non-nil interface.

`Attr` and `AttrSetter` remain platform-specific opaque configuration
machinery. The refactoring does not attempt to give Unix `Termios` and Windows
`DCB` a shared representation.

### Optional capabilities

Operations that are not supported by every terminal implementation do not
belong in `Term` with a boolean availability method and an unsupported stub.
They are represented by separate interfaces and discovered with type
assertions.

The existing Unix-only raw modem status operation becomes such a capability:

```go
type ModemStatusReader interface {
	ModemStatus() (int, error)
}
```

`ModemStatusReader` remains in the non-Windows source alongside the existing
`MODEM_CTS`, `MODEM_DCD`, `MODEM_DSR`, and `MODEM_RI` constants. Its result is
explicitly a Unix `TIOCM_*`-encoded mask, not a portable modem-state encoding.
Any non-termios implementation that chooses to implement this transitional
capability must translate its native flags into those bits.

The non-Windows `pollpps` diagnostic asserts `ModemStatusReader` before using
it. If the assertion fails, it exits with a clear unsupported-capability error
before starting the monitoring loop. Windows does not need to implement an
artificial `ModemStatus` method.

The same rule applies to later serial-PPS work: blocking modem-control change
notification will be a separate interface implemented only by backends that
provide it. It will not add `CanWaitModemControlLineChange` to `Term`.

## Concrete implementations

Replace the current exported concrete type with private implementations named
according to what they contain:

- `unixTerm` owns the POSIX file descriptor, termios attributes, saved
  attributes, path, and attribute lock.
- `windowsTerm` owns the Windows handle, DCB, communication timeouts, and
  saved configuration.

Both have compile-time assertions that they implement `Term`. Their methods
retain the current public method names because those methods satisfy the
interface; their implementation-only helpers remain private.

The platform-specific `Open` functions allocate and initialize the appropriate
concrete type and return it as `Term`. The current exported `Init` method
becomes the private `init` constructor helper. There is no meaningful zero
value for an interface-backed terminal, and the repository has no caller of
`Init` other than `Open`.

### Unix platform state

Keep the termios mechanics shared between Linux, Darwin, and FreeBSD, but do
not put a Linux structure directly in `unixTerm`. Replace the current
`iCount *serialICounter` plus the BSD dummy `serialICounter` with a semantic,
build-tagged serial-error state:

- On Linux, the state retains the previous `TIOCGICOUNT` counters and computes
  the same per-read deltas.
- On Darwin and FreeBSD, the state is empty and reports no kernel serial error
  information.

The common `unixTerm` contains this platform-defined error state, not a dummy
Linux ABI type. Linux ioctl structures stay in Linux-only files. The Linux
state includes explicit baseline validity: it starts invalid, becomes valid
when the first `TIOCGICOUNT` succeeds, and is invalidated whenever that ioctl
fails. The first success after initialization or a transient failure stores a
new baseline without reporting a delta; it must not compare against zero or
stale counters and report a spurious error burst.

Platform-specific `Flush`, `Drain`, attribute ioctls, exclusivity checks, baud
handling, and `DevKind` classification remain in their current build-tagged
files with receivers changed to the appropriate private concrete type.

### Platform construction

Each platform implements the public constructor directly. For example, the
Unix implementation is:

```go
func Open(path string, opts ...AttrSetter) (Term, error) {
	t := new(unixTerm)
	if err := t.init(path, opts...); err != nil {
		return nil, err
	}
	return t, nil
}
```

The Windows implementation follows the same pattern with `windowsTerm`.
Platform file selection provides the appropriate `Open`; there is no separate
build-independent wrapper or `openTerm` layer. A concrete initialization error
is converted to `return nil, err` before returning through the interface,
avoiding a typed-nil result.

Opening errors retain their current behavior, including `ErrNotATTY`, device
locking errors, and restoration/cleanup after partial initialization.

## `gpsio` integration

Retain `gpsio.ioFile` as the minimum common interface for all readable GPS
inputs:

```go
type ioFile interface {
	io.ReadWriteCloser
	Path() string
	Buffered() (int, error)
}
```

The implementations continue to relate as follows:

```text
gpsio.ioFile
|-- term.Term
|   |-- *unixTerm
|   `-- *windowsTerm
|-- *term.File
`-- *gpsio.pollingFile
```

Change the terminal capability assertion from the concrete type to the public
interface:

```go
func (c *SerialConn) term() term.Term {
	t, _ := c.file.(term.Term)
	return t
}
```

Speed changes, transmit-time calculations, flush, drain, and restoration then
operate on any full terminal implementation. FIFO and non-termios fallbacks
continue to return `nil` from this assertion and retain their current behavior.

Update comment references from `*term.Term` to `term.Term` where required by
the type change; otherwise retain the existing terminology.

## Source organization

- Put the public `Term` interface in the existing build-independent `types.go`
  alongside the other build-independent public types. Keep platform-specific
  capability interfaces in their build-tagged files.
- Rename `term.go` to `term_unix.go` and keep its existing `!windows` build
  tag. It retains `Attr`, `AttrSetter`, the attribute setters and speed helpers,
  the Unix modem constants and `ModemStatusReader`, `File`, `openFallback`, and
  the existing Unix implementation code. Rename the concrete type and all its
  receivers to `unixTerm`, rename `Init` to the private `init` helper, and
  change its exported `Open` to return `Term`.
- Rename the concrete type and receivers in `term_windows.go` to
  `windowsTerm`; retain the Windows `Attr` machinery there, rename `Init` to
  the private `init` helper, and change its exported `Open` to return `Term`.
- Update the Linux, BSD, Darwin, and FreeBSD receiver declarations and tests
  to use the private Unix type.
- Keep fallback file types separate from the full terminal implementations.

Avoid compatibility aliases that expose either concrete implementation. Such
an alias would preserve the concrete-type coupling that this change is meant
to remove.

## Compatibility

This is intentionally a source-level API change for callers that explicitly
use `*term.Term`, construct `term.Term{}`, or call `Init`. In-tree use is
limited to `gpsio`, `pollpps`, package methods, an internal Darwin test, and
`term_linux_test.go`. All are updated in the same commit. The Linux test calls
`Open`, asserts that the returned `Term` is a `*unixTerm`, and uses that
concrete value for its package-internal file-descriptor checks.

The return-type change also affects callers that infer the result but then use
a method outside the new core interface, and callers that assign `term.Open`
itself to a function value with the old concrete return signature. The former
must assert an optional capability; the latter must adopt the new function
type or wrap `Open` explicitly.

The following remain stable:

- `term.Open` as the construction entry point.
- Attribute setters and their behavior.
- Read, write, timeout, error, flush, drain, restore, and close semantics.
- Device classification and fallback selection.
- Error wrapping and `errors.Is`/`errors.As` behavior.

## Testing

Add compile-time interface assertions for each native implementation. Update
both the Darwin and Linux tests that construct or inspect the Unix concrete
type internally. Add two new `gpsio` test fakes: one non-native fake
implementing `term.Term`, to verify that terminal behavior is selected by
interface satisfaction rather than concrete type identity, and one implementing
only `ioFile`, to verify the non-terminal fallback path. There is no existing
fallback fake to retain.

Run:

- `go test ./...` on macOS.
- `./unix-build.sh` on macOS.
- cgo-disabled build checks for affected packages.
- `GOOS=linux CGO_ENABLED=0 go vet ./gps/lib/term ./gps/app/gpsio` so
  Linux-only terminal tests and the new `gpsio` interface fakes are
  type-checked for Linux while running on macOS.
- `GOOS=freebsd CGO_ENABLED=0 go vet ./gps/lib/term ./gps/app/gpsio` and
  `GOOS=windows CGO_ENABLED=0 go vet ./gps/lib/term ./gps/app/gpsio`, so the
  target-specific test files are type-checked under their actual platform
  APIs.
- Linux, FreeBSD, and Windows cross-build checks for `gps/lib/term` and
  `gps/app/gpsio`; include `cmd/pollpps` where its build tags allow it. The
  cross-builds remain useful binary checks, but the target-specific vet steps
  are what ensure `_test.go` files are also type-checked.

No hardware behavior should change, but opening and communicating with an
attached native serial receiver is a useful smoke test when hardware is
available.

## Follow-up integration

After this refactoring is committed independently, rebase the serial-PPS and
D2XX branches onto it. The follow-up changes then:

- add modem-control state reading as an optional capability where appropriate;
- add blocking modem-change notification as a second optional capability,
  without a `CanWait...` boolean;
- implement D2XX as another concrete `Term`, rather than placing it behind an
  internal backend field in `unixTerm`; and
- map the termios-backed Darwin `Attr` settings onto D2XX configuration calls,
  unless configuration itself is redesigned in a separate prerequisite.

Those follow-up changes and their hardware validation remain part of the
serial-PPS/D2XX work, not this plan.
