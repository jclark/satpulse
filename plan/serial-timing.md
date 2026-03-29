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

### Sink interface in timemsg

`timemsg.Buffer` defines the interface it calls:

```go
type SerialSampler interface {
    SerialSample(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind)
}

func (buf *Buffer) SetSerialSampler(sink SerialSampler)
```

When no sink is set (PHC mode), `Buffer.Time()` does no extra work. When a sink is set (serial mode), `Buffer.Time()` checks each incoming message and calls the sink when it has a new eligible sample.

### Message eligibility in Buffer.Time()

On each incoming message, if a sink is set:

1. Message must have a valid time (prefer `UTCTime`; fall back to `TAITime` converted via `buf.ls`)
2. The UTC time must represent a new second (not already sent for this second)
3. First eligible message for a given second wins (lowest latency)

Buffer computes the leap state from `buf.ls` and the message time, and calls `sink.SerialSample(utc, tRead, leap)`.

### Dispatcher implements the sink

The dispatcher implements `SerialSampler`:

```go
func (d *Dispatcher) SerialSample(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind) {
    offset := utc.Sub(tRead).Seconds()
    d.rc.Sample(tRead, offset, leap)
}
```

The daemon wiring calls `buf.SetSerialSampler(d)` during setup only when both conditions hold: no PHC is configured, and `ntp.sock.path` is set (so `d.rc` is non-nil). If neither condition is met, the sink is not set and `Buffer.Time()` does no extra work.

## Configuration

- Socket path: reuse the existing `ntp.sock.path` key. The two modes are mutually exclusive, so the same key serves both.
- GPS message configuration: same messages as with PHC, just without the `PVTMsgTAI` option. The receiver then reports UTC instead of TAI in the same message types. High-level configuration handles this by not setting the TAI option.

## Verify

- Unit tests for `timemsg.Buffer` serial sink path: eligible message triggers sink call, duplicate second is suppressed, UTC preferred over TAI, sink=nil does no work
- Unit test for leap state computation from UTC time via `buf.ls`
- `make test` passes
- Integration: serial-only config sends samples to chrony socket

