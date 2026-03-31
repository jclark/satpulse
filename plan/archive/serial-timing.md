# Serial timing for chrony (#77)

Generate chrony SOCK samples from serial GPS data alone (no PHC). This lets satpulsed work with chrony on a Raspberry Pi or PC with just a serial GPS receiver. Chrony uses a PPS refclock for precise top-of-second and locks it to a SOCK refclock from satpulsed to identify which second.

Depends on [refclock-refactor.md](refclock-refactor.md).

## Chrony setup

```
refclock PPS /dev/pps0 lock NMEA refid GPS1
refclock SOCK /var/run/chrony.clk.ttyS0.sock offset 0.5 delay 0.2 refid NMEA noselect
```

The SOCK refclock is `noselect` (chrony uses it only to label PPS pulses, not for time discipline). The `offset 0.5` accounts for the time message arriving roughly half a second after the PPS edge.

## Mode selection

Serial timing and PHC timing are mutually exclusive. If PHC is configured, chrony samples go through the existing PHC-based path. If no PHC is configured, they go through the serial path. Configuration decides which.

## Architecture

### UTCTime.SysTime()

Add a `SysTime()` method to `ptime.UTCTime` that returns `time.Time`:

```go
func (ut UTCTime) SysTime() time.Time {
    return ut.Date.Add(ut.TimeOfDay)
}
```

This replaces the `ut.Date.Add(ut.TimeOfDay)` pattern used in several places.

### Sink interface in timemsg

`timemsg.Buffer` defines the interface it calls:

```go
type SerialSampler interface {
    SerialSample(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind)
}

func (buf *Buffer) SetSerialSampler(sink SerialSampler)
```

When no sink is set (PHC mode), `Buffer.Time()` does no extra work. When a sink is set (serial mode), `Buffer.Time()` checks each incoming message and calls the sink when it has a new eligible sample.

### Message eligibility in Buffer.serialSample()

On each incoming message, if a sampler is set:

1. Message must have `UTCTime` set. Messages with only `TAITime` are ignored. In serial mode, the receiver is configured to report UTC (no `PVTMsgTAI`), so all time messages should have `UTCTime`. There is no TAI fallback — this avoids any dependency on `buf.ls` and the risk of stale or incorrect leap second state producing wrong UTC times.
2. Leap second (23:59:60, i.e. `TimeOfDay >= 24h`) is skipped.
3. The UTC time is rounded to the nearest millisecond to recover the nominal time. Native messages often report navigation solution time that differs from the nominal epoch time by a few microseconds. NMEA already rounds to the nearest millisecond. This rounding also means sub-second messages (e.g. from a 5Hz receiver) produce distinct samples rather than being deduplicated.
4. The rounded time must be strictly after the last sent time (monotonic dedup). First eligible message for a given time wins (lowest latency).

Leap state is computed via `buf.ls.UTCStateAt(ut).LeapTonight`. This works entirely in UTC (no TAI conversion): it compares the message's UTC date with the leap second date (`ls.Date()`) and checks whether `TimeOfDay >= 12h` (matching the 12-hour window used by `StateAt` for TAI). Negative `TimeOfDay` (GLONASS) is normalized before comparison.

### Dispatcher implements the sink

The dispatcher implements `SerialSampler`:

```go
func (d *Dispatcher) SerialSample(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind) {
    offset := utc.Sub(tRead).Seconds()
    err := d.rc.Sample(tRead, offset, leap)
    if err != nil {
        d.lg.Warn("refclock sample failed", "err", err)
    }
}
```

The daemon wiring calls `buf.SetSerialSampler(d)` during setup only when both conditions hold: no PHC is configured (`clk == nil`), and `ntp.sock.path` is set (so `d.rc` is non-nil). If neither condition is met, the sink is not set and `Buffer.Time()` does no extra work.

## Configuration

- Socket path: reuse the existing `ntp.sock.path` key. The two modes are mutually exclusive, so the same key serves both.
- GPS message configuration: serial mode uses `NoTimePulsePVTMsgFlags` (the existing no-PHC message set). This is a different set from the PHC path, which uses `TimePulsePVTMsgFlags`. The two sets may be revisited after experimentation.

## Verify

- Unit tests for `timemsg.Buffer` serial sink path: eligible message triggers sink call, duplicate second is suppressed, TAI-only message ignored, leap second skipped, sink=nil does no work
- `make test` passes
- Integration: serial-only config sends samples to chrony socket

