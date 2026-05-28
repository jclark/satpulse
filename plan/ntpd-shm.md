# NTP SHM refclock (#300)

## Introduction

Add support for the classic ntpd/NTPsec SHM refclock interface
(driver 28). This is a second NTP publication path alongside the
existing chrony SOCK refclock:

- Chrony SOCK: satpulsed writes samples to a Unix datagram socket.
- NTP SHM: satpulsed writes samples to a SysV shared-memory segment.

The SHM refclock carries time-of-day samples only. satpulsed will not
publish PPS through SHM; users that want sub-second discipline with
ntpd/NTPsec should configure a separate PPS/ATOM refclock.

SHM is a peer of SOCK, not a replacement. Both can be configured at the
same time. The dispatcher constructs one NTP sample and publishes it
independently to each configured sink.

## Design goals

- Keep NTP SHM in a small `time/lib/ntpshm` package with no logging and
  no goroutines.
- Keep SHM publication synchronous. A write is a few memory stores and
  atomic operations; the proxy/worker pattern used for SOCK would add
  machinery for a failure mode SHM writes do not have.
- Do not force SHM through `refclock.RefClock`. SOCK naturally works in
  `(sys, offset, leap)` form and can block on socket I/O. SHM naturally
  writes `(clockTime, receiveTime, leap)` into memory and only has an
  attach-time failure.
- Keep precision policy out of `ntpshm.Writer`. The library stores and
  publishes the current precision, while daemon/gpsevent code decides
  where that precision comes from.
- Match the NTP SHM C ABI rather than hand-designing a Go layout.

## Package plan

Add `time/lib/ntpshm`:

- `ntpshm.go`: public `Writer`, `Attach`, `ErrUnsupported`,
  `Precision(time.Duration) int8`, and the public writer methods.
- `shm_linux.go`: Linux SysV SHM attach, write, close, mode selection,
  and the compile-time size assertion.
- `shm_stub.go`: non-Linux stub returning `ErrUnsupported`.
- `types_linux.go`: `cgo -godefs` input.
- `ztypes_linux.go`: generated Go layout, checked in.
- package tests.

`Writer` stores the current SHM precision. Callers use `SetPrecision`
when the value changes. `Write` does not take a precision argument; it
publishes the current stored value with the sample. This keeps the
sample publication API focused on the sample itself and lets gpsevent
encapsulate calibration.

The public API is intentionally concrete. `ntpshm.New` returns
`*ntpshm.Writer`; the consumer package (`gpsevent`) defines the
interface it needs.

## ABI layout plan

The NTP SHM segment is defined by a C `struct shmTime`, so the Go code
must match that C layout. Use `go tool cgo -godefs` to generate the Go
struct from an inline C definition.

`types_linux.go` declares `struct shmTime` itself rather than including
an ntpd/NTPsec header. Those headers are not necessarily installed on
the build or development machine, and the generated Go file is checked
in. Developers only need to regenerate if the inline C struct changes.

The nanosecond fields are C `int`, matching chrony's SHM layout and
the generated Go `int32` fields. They are not unsigned in our input.

Keep a compile-time size assertion in the Linux implementation. The
expected Linux size is 96 bytes; if the generated layout drifts, the
package should fail to compile.

## SHM lifecycle plan

Use the standard NTP SHM key convention:

- key: `0x4e545030 + unit`;
- unit 0/1 mode: `0600`;
- unit 2+ mode: `0666`.

Attach with `IPC_CREAT`. Do not call `IPC_RMID` in normal operation.
The segment should persist across satpulsed restarts, like gpsd and
chrony's SHM writer. Removing the segment at attach or close time is
unsafe: on Linux, a later `shmget(key, IPC_CREAT)` by ntpd can create a
new segment with the same key while satpulsed still writes the old
attached segment.

`Close` only detaches with `shmdt`. Operators can remove stale segments
manually with `ipcrm`, or a future tool can automate that.

At attach, initialise the segment into mode 1 and invalidate the slot
before any new sample is published. This prevents a reader from
accepting stale data left by a previous writer.

## Write protocol plan

Use the NTP SHM mode-1 count/valid protocol:

1. store `Valid = 0`;
2. increment `Count`;
3. write timestamps, leap, precision, and both usec/nsec fields;
4. increment `Count`;
5. store `Valid = 1`.

Use `sync/atomic` for `Valid` and `Count`. The atomics document the
cross-process synchronization protocol and give the compiler ordering
needed around the plain field stores. The cost is negligible at one
sample per second.

Populate both the legacy microsecond fields and the nanosecond fields.
Mode and sample count are set at attach; precision is stored in
`Writer` and written with every sample.

## Dispatcher plan

Add a consumer-side SHM interface in `time/internal/gpsevent`:

- `Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind)`;
- `Close() error`.

`Dispatcher` gets two SHM-related fields:

- `shm SHMWriter`;
- `sps samplePrecisionSetter`, non-nil only when the writer is the
  calibration wrapper.

Add `gpsevent.NewSHMWriter(w *ntpshm.Writer, precision *int8)` as the
adapter between daemon configuration and dispatcher use:

- nil writer returns nil;
- explicit precision sets the concrete writer precision and returns the
  writer directly;
- nil precision returns a `calibratingSHMWriter`.

This keeps `Dispatcher` independent of the concrete ntpshm type except
at construction time. It also keeps `SHMWriter` small: it is only the
sample sink, not a precision-control interface. The calibration wrapper
has the extra unexported duration method used by `Dispatcher`.

Publish SHM samples from both existing NTP sample paths:

- PHC tracking mode: `sysSample` writes `(clockTime, sys, leap)`;
- serial timing mode: `MsgUTCTime` writes `(utc, tRead, leap)`.

Guard SOCK and SHM independently. A SOCK error must not skip the SHM
write or the observer callback. In serial timing mode, register the UTC
timer when either SOCK or SHM is configured.

In `Dispatcher.Run`, close the SHM writer on exit and log detach
errors at the app layer.

## Precision plan

The SHM `Precision` field is an NTP `log2(seconds)` exponent.
`ntpshm.Precision(d)` converts a duration to the smallest exponent
whose duration is at least `d`, using ceil rounding and clamping to the
`int8` range.

There are three precision modes:

- Explicit config: `[ntp.shm] precision = N` uses `N` unchanged and no
  calibration runs.
- Serial timing mode: when no PHC interface is configured,
  `Config.shmFixedPrecision()` returns a fixed `-1`.
- PHC tracking mode: when a PHC interface is configured and no explicit
  precision is set, `Config.shmFixedPrecision()` returns nil and
  gpsevent uses the calibrating wrapper.

Derive PHC precision from the PHC/system cross-timestamp:

- `phc.MultiSample.Extract` and `Reduce` return the selected sample
  precision as a `time.Duration`;
- the duration is the selected system-clock bracket interval, floored
  at 5 ns;
- `ts.Event` carries that duration to gpsevent.

The calibration wrapper sets an initial precision on the first valid
PHC sample, collects `precisionCalibrationSamples` durations, then sets
a final median-based precision and releases the window. After that, the
precision stays fixed for the run. This gives ntpd a useful value from
the first sample without continually changing the SHM metadata.

## Configuration plan

Extend `[ntp]` with an optional `[ntp.shm]` table:

- `unit`: `0..255`; pointer-backed so `0` is distinguishable from
  absent;
- `precision`: optional `int8`; pointer-backed so `0` is
  distinguishable from absent.

Example:

```toml
[ntp]
shm.unit = 2
# shm.precision = -23
```

`NTPConfig.NewSHMWriter` lives in the daemon package. It attaches to
the configured segment and logs success. It returns nil when SHM is not
configured. If SHM is explicitly configured and attach fails, startup
fails so service supervision can detect that the requested NTP output is
inactive.

`Config.shmFixedPrecision()` determines the effective fixed precision
from config alone:

- explicit `[ntp.shm] precision` wins;
- no PHC interface means serial fallback `-1`;
- configured PHC interface means nil, so PHC calibration runs.

Document the permission consequence of the unit choice. Units 0 and 1
create private `0600` segments and are normally usable only when
satpulsed and ntpd run as the same user. Units 2 and above create
`0666` segments and are the recommended cross-user configuration.

## Testing plan

Unit tests in `time/lib/ntpshm`:

- `Precision` rounding, clamping, zero, and negative inputs;
- write round-trip against an in-memory `shmTime`;
- attaching twice to the same unit and observing writes through the
  second mapping.

Dispatcher tests:

- serial-mode SHM writes;
- SOCK and SHM both receiving the same event;
- calibration wrapper lifecycle;
- explicit precision bypassing calibration.

Daemon config tests:

- parse `ntp.shm.unit` and `ntp.shm.precision`;
- compute effective fixed precision from explicit config, serial mode,
  and PHC mode.

PHC tests:

- selected sample precision from `MultiSample.Extract` and `Reduce`;
- 5 ns floor behavior.

Run the normal repository checks with `make test` and `make`.

## Documentation plan

Update:

- `configs/config-schema.json`;
- `configs/satpulse.toml`;
- `docs/man/satpulse.toml.5.md`;
- `docs/internals.md`.

The manual should explain:

- what SHM unit means;
- permissions for unit 0/1 vs 2+;
- precision override semantics;
- that PPS must be configured separately in ntpd/NTPsec.

## Follow-ons

- Add systest coverage against ntpd or NTPsec.
- Add FreeBSD or macOS SHM support by generating native
  `ztypes_*.go` files on those operating systems and broadening the
  build tags.
- Add a setup guide covering chrony SOCK, ntpd/NTPsec SHM, and PPS
  configuration.
- Consider an operator helper for removing stale SHM segments.
