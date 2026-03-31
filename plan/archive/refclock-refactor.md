# Refclock refactor

Move the TAI-to-system-time conversion out of `sockrefclock` and into the caller, so the refclock packages deal only in pre-computed offsets. This is a prerequisite for serial-only chrony timing (#77), where the true time is UTC rather than TAI.

## Current state

The `RefClock` interface is:

```go
Sample(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) error
```

The TAI-to-system-time conversion (`ls.TimeToSys(ref).Sub(sys)`) and leap state determination (`ls.StateAt(ref).LeapTonight`) happen inside `sockrefclock.initSockSample`. This couples both `refclock` and `sockrefclock` to `ptime.Time` and `ptime.LeapSecond`.

## New interface

```go
type RefClock interface {
    io.Closer
    Sample(sys time.Time, offset float64, leap ptime.LeapSecondKind) error
}
```

`offset` is seconds between true time and system time, same semantics as the chrony SOCK `offset` field. Positive means true time is ahead of system time. `leap` uses the existing `ptime.LeapSecondKind` type (None/Positive/Negative).

## Changes by file

### `time/internal/refclock/refclock.go`

- Change `RefClock.Sample` signature to `(sys time.Time, offset float64, leap ptime.LeapSecondKind) error`
- Change `RefClockSample` fields to `Sys time.Time`, `Offset float64`, `Leap ptime.LeapSecondKind`
- Update `ProxyRefClock.Sample`, `RefClockWorker`, `LoggingSockRefClock.Sample`
- Import changes: keep `ptime` (for `LeapSecondKind` only), remove `LeapSecond` and `Time` usage

### `time/sockrefclock/sockpacket.go`

- `sockPacket` and `initSockSample` take `(sys time.Time, offset float64, leap ptime.LeapSecondKind)`
- `initSockSample` sets `s.offset = offset` directly
- Replace `refSockLeap(ref, ls)` with a mapping from `ptime.LeapSecondKind` to `sockLeap`
- Remove `ptime.Time` and `ptime.LeapSecond` usage

### `time/sockrefclock/sockrefclock.go`

- Change `SockRefClock.Sample` signature to match
- Remove `ptime.Time` and `ptime.LeapSecond` usage

### `time/internal/gpsevent/dispatcher.go` (the caller)

`sysSample` computes `offset` and `leap` before calling `rc.Sample`:

```go
func (d *Dispatcher) sysSample(trp ptime.Time, tRead time.Time) {
    if d.rc == nil || trp.IsZero() || d.controller.Mode() != phcsync.ModeTracking {
        return
    }
    offset := d.ls.TimeToSys(trp).Sub(tRead).Seconds()
    leap := d.ls.StateAt(trp).LeapTonight
    err := d.rc.Sample(tRead, offset, leap)
    ...
}
```

### `time/app/daemon/config.go`

No changes needed. `NewRefClock` returns `refclock.RefClock`; callers are unaffected.

## Result

After this refactor, `refclock` and `sockrefclock` depend on `ptime` only for the `LeapSecondKind` type. They no longer need `ptime.Time` or `ptime.LeapSecond`. Any caller that can compute a `(sys, offset, leap)` triple can send chrony samples.

## Verify

- `make test` passes
- `sockrefclock` and `refclock` packages no longer use `ptime.Time` or `ptime.LeapSecond`
- Existing PHC-based chrony samples are bit-identical (same offset computation, just done in a different place)
