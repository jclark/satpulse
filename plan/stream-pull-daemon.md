# Stream support in satpulsed (#221, #99, #126)

Integrate `stream.pull` and `stream.push` into satpulsed.  The
`gps/app/stream` package handles the
network/serial plumbing; this plan covers the daemon integration
and the observability event that fires on each received correction
packet.

## Prerequisite

- `plan/corrsink-rename.md` (rename corrsink to stream).
- `plan/stream-backoff.md` (adaptive backoff).
- `plan/ntrip-client.md` (NTRIPSource and ntriphdr library) if
  NTRIP transport is needed.

## TOML configuration

`stream.pull` is a single table (one correction source per
receiver).  Transport is selected by which dotted keys are present
(`tcp.address` vs `ntrip.*`), mutually exclusive.

```toml
# Plain TCP
[stream.pull]
tcp.address = "10.56.65.82:2006"

# OR NTRIP client
[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
ntrip.username = "user"
ntrip.password = "pass"
```

### stream.pull with separate serial port

If the receiver has a second serial input for corrections:

```toml
[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
serial.device = "/dev/ttyUSB1"
serial.speed = 115200
```

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

Add `Stream stream.Config` to `Config` in
`time/app/daemon/config.go`.  The `stream.Config` type is defined
in `gps/app/stream` and matches the `[[stream.pull]]` /
`[stream.pull]` TOML table described above.

Put stream-specific daemon code in a new file
`time/app/daemon/stream.go`.

### Startup in `run()`

Stream setup is split into two phases to avoid a startup deadlock.
`run()` has many fallible steps between early setup and the final
`d.Run(...)` call that starts consuming channels.  If the adapter
started producing before the dispatcher was running, it could block
on `corrCh <-` with no consumer, and a subsequent error-path
`cancel()` + `wg.Wait()` would hang.

**Phase 1 -- prepare (before any fallible steps that follow):**

If `[stream.pull]` is configured:

1. Create `stream.NewPull()`.
2. Create the appropriate `Source` (`stream.TCPSource` or
   `stream.NTRIPSource`) based on which transport keys are present.
3. Determine serial port: if `serial.device` is set, open a
   separate serial connection and create a new `OutPortLock`;
   otherwise reuse the main `portLock`.
4. Create `corrCh := make(chan CorrectionEvent)`.

**Phase 2 -- start (immediately before `d.Run`, after all fallible
steps):**

5. Subscribe to `pull.Packets` and start the adapter goroutine that
   owns `corrCh` and converts `scan.Packet` to `CorrectionEvent`:
   - Skip packets where `pkt.Format == nil` (malformed input,
     timeout markers, reconnect noise -- the bcast delivers these
     before the internal queue filters them).
   - `tag := pkt.Format.Tag()` for `Protocol`
   - `pkt.Format.MsgID([]byte(pkt.Data))` for `MsgType`
   - Call `rtcmbin.MultipleMessageBit([]byte(pkt.Data))` for
     `MultipleMessage`
   - Call `rtcmbin.ReferenceStationID([]byte(pkt.Data))` for
     `RefStationID` (import `gps/lib/rtcmbin`)
   - Send on `corrCh`
   - The adapter **closes `corrCh`** when the bcast subscription
     channel closes (stream shutdown), signalling the dispatcher
     that no more correction events will arrive.
6. Start `pull.Run` in a goroutine (via `wg.Go`).
7. Pass `corrCh` to `Dispatcher.Run`.

### Shutdown ordering

Channel ownership: the adapter goroutine owns `corrCh` and is
responsible for closing it.

Shutdown sequence:

1. Daemon cancels the stream context.
2. `Pull.Run` drains its internal pipeline (reader, queue, writer),
   then calls `Packets.Close()` which closes all bcast subscriber
   channels.
3. The adapter goroutine sees its subscription channel close, closes
   `corrCh`, and exits.
4. The dispatcher sees `corrCh` closed, nils it out, and (once
   `tsCh` and `pktCh` are also nil) exits its loop.

The dispatcher loop condition `tsCh != nil || pktCh != nil ||
corrCh != nil` ensures it stays alive to drain `corrCh` even if the
GPS channels close first.  Conversely, because `Pull.Run` is
ctx-based, the daemon must cancel that context before or concurrently
with closing the GPS channels to avoid the dispatcher blocking
indefinitely on a never-closed `corrCh`.

### Connection state logging

`stream.Pull.Run` takes an `onState func(State, error)` callback.
The daemon passes a callback that logs state transitions via slog:

```go
func(st stream.State, err error) {
    switch st {
    case stream.Connecting:
        lg.Info("stream pull connecting", "addr", addr)
    case stream.Connected:
        lg.Info("stream pull connected", "addr", addr)
    case stream.Reconnecting:
        lg.Warn("stream pull reconnecting", "addr", addr, "err", err)
    }
}
```

## Implementation order

1. `CorrectionPacketMsg` in `gps/gpsprot/msg.go`.
2. `CorrectionPacket` on Observer interface + DefaultObserver +
   MultiObserver.
3. `CorrectionEvent` + dispatcher changes.
4. `stream.Config` + daemon wiring + adapter goroutine.
5. Tests.

## Testing

- Unit test `CorrectionPacketMsg` JSON marshaling (verify `omitzero`
  omits unset `RefStationID`, `omitempty` omits false
  `MultipleMessage`).
- Unit test adapter goroutine: feed `scan.Packet` with known RTCM
  data, verify `CorrectionEvent` fields.
- `make test` for no regressions.
- Manual: add `[stream.pull]` to `/etc/satpulse.toml` pointing at
  a base station, verify `correctionPacket` entries in the event
  log.
