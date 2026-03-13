# Correction support in satpulsed (#221)

Add a `[correction]` configuration section to satpulsed that receives
correction data from an external source and feeds it to the GPS
receiver.  The `corrsink` package (see `plan/archive/corrsink.md`)
handles the network/serial plumbing; this plan covers the daemon
integration and the observability event that fires on each received
correction packet.

## Prerequisite: split rtcm into lib package

See `plan/rtcm-split.md`.  The RTCM packet format code moves to
`gps/lib/rtcmbin`; `gps/internal/rtcm` retains only the
`PacketProcessor`.

## CorrectionPacket observer event

The event uses a generic name (`CorrectionPacket`) rather than
`RTCMMessage` so it can cover future correction protocols (SPARTN,
etc.) without adding new methods.

### CorrectionPacketMsg struct

Add to `gps/gpsprot/msg.go`:

```go
// CorrectionPacketMsg carries metadata about a received correction packet.
type CorrectionPacketMsg struct {
    Protocol        Tag             `json:"protocol"`
    MsgType         string          `json:"msgType"`
    MultipleMessage bool            `json:"multipleMessage,omitempty"`
    RefStationID    opt.Val[uint16] `json:"refStationID,omitzero"`
}
```

- `Protocol` is the packet format tag (e.g. `"RTCM"`, `"SPARTN"`).
- `MsgType` is the human-readable message ID string (e.g. `"1077"`),
  matching `PacketFormat.MsgID`.
- `MultipleMessage` is true when more packets follow for this epoch
  (MSM multiple-message bit).
- `RefStationID` is set when the packet contains a reference station
  ID (RTCM DF003).  `opt.Val` because not all message types or
  protocols carry one.

No `Dispatch` method -- this is not a `gpsprot.Msg` routed through
`MsgHandler`.  It goes through Observer only.

### Observer interface

Add to `time/internal/obs/observer.go`:

```go
// CorrectionPacket delivers metadata for each correction packet
// received from the correction source.
CorrectionPacket(msg *gpsprot.CorrectionPacketMsg, tRead time.Time)
```

Update `DefaultObserver` (no-op), `MultiObserver` (fan-out with
type assertion, same pattern as `Tick` / `NavEpochPV`).

### Event log

Add to `gpsevent.LogEvent`:

```go
CorrectionPacket *gpsprot.CorrectionPacketMsg `json:"correctionPacket,omitempty"`
```

## Dispatcher

Add an optional correction event channel to `Dispatcher.Run`:

```go
func (d *Dispatcher) Run(tsCh <-chan ts.Event, pktCh <-chan scan.Packet,
    corrCh <-chan CorrectionEvent)
```

where

```go
// CorrectionEvent pairs a correction packet with its read time.
type CorrectionEvent struct {
    Msg   gpsprot.CorrectionPacketMsg
    TRead time.Time
}
```

Update the loop condition to include `corrCh`:

```go
for tsCh != nil || pktCh != nil || corrCh != nil {
```

Add to the select:

```go
case ce, ok := <-corrCh:
    if ok {
        d.obs.CorrectionPacket(&ce.Msg, ce.TRead)
        d.logEvent(LogEvent{T: ce.TRead, CorrectionPacket: &ce.Msg})
    } else {
        corrCh = nil
    }
```

When correction is not configured, `corrCh` is nil from the start
and does not affect the loop condition or select.

## Daemon wiring

### Configuration

Add `Correction CorrectionConfig` to `Config` in
`time/app/daemon/config.go`.

```go
type CorrectionConfig struct {
    TCP    CorrectionTCPConfig
    Serial CorrectionSerialConfig
}

type CorrectionTCPConfig struct {
    Address string
}

type CorrectionSerialConfig struct {
    Device string
    Speed  int
}
```

Transport is selected by which keys are present (TCP first, NTRIP
later).  Serial defaults to the main GPS port.

Put correction-specific daemon code in a new file
`time/app/daemon/correction.go`.

### Startup in `run()`

Correction setup is split into two phases to avoid a startup
deadlock.  `run()` has many fallible steps between early setup and
the final `d.Run(...)` call that starts consuming channels.  If the
adapter started producing before the dispatcher was running, it
could block on `corrCh <-` with no consumer, and a subsequent
error-path `cancel()` + `wg.Wait()` would hang.

**Phase 1 -- prepare (before any fallible steps that follow):**

If `cfg.Correction.TCP.Address != ""`:

1. Create `corrsink.NewSink()`.
2. Create `corrsink.TCPSource{Addr: cfg.Correction.TCP.Address}`.
3. Determine correction serial port: if `cfg.Correction.Serial.Device`
   is set, open a separate serial connection and create a new
   `OutPortLock`; otherwise reuse the main `portLock`.
4. Create `corrCh := make(chan CorrectionEvent)`.

**Phase 2 -- start (immediately before `d.Run`, after all fallible
steps):**

5. Subscribe to `sink.Packets` and start the adapter goroutine that
   owns `corrCh` and converts `scan.Packet` to `CorrectionEvent`:
   - Skip packets where `pkt.Format == nil` (malformed input,
     timeout markers, reconnect noise -- the bcast delivers these
     before the internal queue filters them).
   - `tag := pkt.Format.Tag()` for `Protocol`
   - `pkt.Format.MsgID([]byte(pkt.Data))` for `MsgType`
   - Type-assert `pkt.Format` to `MultiPacketFormat` for
     `IsMultipleMessage`
   - Call `rtcmbin.ReferenceStationID([]byte(pkt.Data))` for
     `RefStationID` (import `gps/lib/rtcmbin`)
   - Send on `corrCh`
   - The adapter **closes `corrCh`** when the bcast subscription
     channel closes (corrsink shutdown), signalling the dispatcher
     that no more correction events will arrive.
6. Start `sink.Run` in a goroutine (via `wg.Go`).
7. Pass `corrCh` to `Dispatcher.Run`.

### Shutdown ordering

Channel ownership: the adapter goroutine owns `corrCh` and is
responsible for closing it.

Shutdown sequence:

1. Daemon cancels the corrsink context.
2. `Sink.Run` drains its internal pipeline (reader, queue, writer),
   then calls `Packets.Close()` which closes all bcast subscriber
   channels.
3. The adapter goroutine sees its subscription channel close, closes
   `corrCh`, and exits.
4. The dispatcher sees `corrCh` closed, nils it out, and (once
   `tsCh` and `pktCh` are also nil) exits its loop.

The dispatcher loop condition `tsCh != nil || pktCh != nil ||
corrCh != nil` ensures it stays alive to drain `corrCh` even if the
GPS channels close first.  Conversely, because `Sink.Run` is
ctx-based, the daemon must cancel that context before or concurrently
with closing the GPS channels to avoid the dispatcher blocking
indefinitely on a never-closed `corrCh`.

### Connection state logging

`corrsink.Sink.Run` takes an `onState func(State, error)` callback.
The daemon passes a callback that logs state transitions via slog:

```go
func(st corrsink.State, err error) {
    switch st {
    case corrsink.Connecting:
        lg.Info("correction source connecting", "addr", addr)
    case corrsink.Connected:
        lg.Info("correction source connected", "addr", addr)
    case corrsink.Reconnecting:
        lg.Warn("correction source reconnecting", "addr", addr, "err", err)
    }
}
```

## Implementation order

1. Split `gps/internal/rtcm` into `gps/lib/rtcmbin` +
   `gps/internal/rtcm` (see `plan/rtcm-split.md`).
2. `CorrectionPacketMsg` in `gps/gpsprot/msg.go`.
3. `CorrectionPacket` on Observer interface + DefaultObserver +
   MultiObserver.
4. `CorrectionEvent` + dispatcher changes.
5. `CorrectionConfig` + daemon wiring + adapter goroutine.
6. Tests.

## Testing

- Unit test `CorrectionPacketMsg` JSON marshaling (verify `omitzero`
  omits unset `RefStationID`, `omitempty` omits false
  `MultipleMessage`).
- Unit test adapter goroutine: feed `scan.Packet` with known RTCM
  data, verify `CorrectionEvent` fields.
- `make test` for no regressions.
- Manual: add `[correction]` to `/etc/satpulse.toml` pointing at a
  base station, verify `correctionPacket` entries in the event log.
