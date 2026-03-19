# Read delay

## Goal

Add a `ReadDelay` field to `gpsprot.TimeMsg` that captures the latency between the
start of the navigation epoch and the moment the time message itself was read.
This is the difference `tRead(TimeMsg) - tRead(first message in epoch)`.

The information is useful for diagnosing and compensating for serial port latency.

## Background

Each backend (ubx, casic, as, nmea, unc, nov, sdbp) processes a group of messages
that belong to the same navigation epoch.  The first message in each epoch is
identified by a change in the epoch identifier (iTOW, RunTime, week/ms, etc.) and
its tRead is stored in `curNavEpochMsg.StartTime` (type `time.Time`).

The `TimeMsg` for that epoch arrives some milliseconds later.  The delay equals
`tRead - curNavEpochMsg.StartTime`.

Currently `TimeMsg` carries no such field.  `tRead` is passed alongside the message
through `MsgHandler.Time(msg *TimeMsg, tRead time.Time)` but the epoch start time
is not exposed outside the backend.

## Design

### Field name

`ReadDelay gpsprot.Duration` -- the duration between the start of the epoch and the
read time of this message.  Zero means unknown or not applicable (e.g. the time
message itself was the first message in the epoch, or the backend does not track
epochs).

Using `time.Duration` (not `*time.Duration`) keeps the struct simple: zero is the
natural "not set" value.

### Where ReadDelay is set

Each backend sets `ReadDelay` on the `TimeMsg` it emits, computed as:

    delay := gpsprot.Duration(tRead.Sub(p.curNavEpochMsg.StartTime))

For nav-class time messages (e.g. UBX NAV-TIMEGPS, CASIC NAV-SOL, SDBP DAT-GPST),
the message carries an epoch identifier (iTOW / RunTime / LocalTimestamp), so
`handleNavEpoch` always runs before the dispatch path reaches the time message
handler.  This guarantees `curNavEpochMsg` is non-nil and its `StartTime` is the
tRead of the first message in this epoch (which may be the time message itself,
giving a delay of zero).

For messages that do NOT carry an epoch identifier -- specifically PrePulse
messages such as UBX TIM-TP and SDBP DAT-TPPS -- there is no associated nav
epoch, so `ReadDelay` should be left as zero.

For NMEA, the epoch's `StartTime` field (`epoch.StartTime`) is set by
`handleEpoch` before any message is dispatched, so the same pattern applies.

For nov and unc, the epoch start time is in `curEpochMsg.StartTime` of the
shared `packetProcessor` struct.

### gpsprot changes

Add field to `TimeMsg` in `gps/gpsprot/msg.go`:

```go
ReadDelay Duration `json:"readDelay,omitempty"`
```

No interface changes are needed.

### timemsg changes

In `GetPostTimeMessages`, when building the `tRead` slice, subtract the
message's `ReadDelay` from its `tRead` before appending:

    tRead = append(tRead, e.tRead.Add(-time.Duration(e.msg.ReadDelay)))

This back-dates each read time to the start of the epoch (i.e. when the first
message of that second arrived), rather than when the time message itself was
read.  The result is a smaller and more consistent delay between the PPS pulse
and the reported read time, because the epoch-start message arrives closer to
the pulse than the time message does.

## Implementation steps

1. **gpsprot** -- add `ReadDelay time.Duration` to `TimeMsg`.

2. **timemsg** -- in `GetPostTimeMessages`, subtract `ReadDelay` from each
   entry's `tRead` before returning it, so the returned times reflect the
   epoch start rather than the time message arrival.

3. **ubx** -- in `ubxtime.go`, pass `curNavEpochMsg.StartTime` to the time
   message constructor or set the field after construction.  The `PacketProcessor`
   struct already holds `curNavEpochMsg`.  The `Dispatch` function (ubx.go) calls
   `h.Time(time, tRead)` -- the time message is built inside the dispatch table;
   set `ReadDelay` in `ubxtime.go` helper functions by accepting the epoch start
   time as a parameter (or set it in the dispatch path after the helper returns).

4. **unc (unicore)** -- same approach using `p.curEpochMsg.StartTime`.

5. **casic** -- same approach using `p.curNavEpochMsg.StartTime`.

6. **as (allystar)** -- same approach using `p.curNavEpochMsg.StartTime`.

7. **nmea** -- use `p.curNavEpoch.StartTime` (set by `handleEpoch`).  When
   `p.curNavEpoch` is nil the delay is unknown; leave it zero.

8. **nov (novatel)** -- same approach using `p.curEpochMsg.StartTime` from the
   shared `packetProcessor`.

9. **sdbp** -- same approach using `p.curNavEpochMsg.StartTime`.

## Notes

- PrePulse messages (e.g. UBX TIM-TP) are not necessarily part of the nav epoch
  group; their `ReadDelay` may be zero or small.  That is fine -- it is still
  informative.
- The field is intentionally `omitempty` in JSON so existing log consumers are
  not affected when the delay is zero.
- No test changes are strictly required for step 1.  Each backend step should
  update or add a test to verify `ReadDelay` is set correctly.
