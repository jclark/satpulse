# Correction sink package: `gps/app/corrsink`

Connects to a correction source, scans packets, and writes them to a
serial port.  It handles reconnection internally and exposes a
`bcast.Bcast` of scanned packets so callers can subscribe for UI
updates, logging, or other purposes without affecting the network read
or serial write paths.

This package lives in `gps/app/` alongside `gpsio` and `gpscfg`
(application layer, see `docs/internals.md`).

Related: GitHub issue #221 adds a `[correction]` section to satpulsed
for the daemon equivalent.

## Source interface

The `Source` interface abstracts the network transport.  A `Source`
establishes a connection and returns a `net.Conn` that delivers raw
correction data.  Different implementations handle different
protocols (TCP, NTRIP, etc.).

```go
// Source provides a network connection for correction data.
type Source interface {
    // Connect establishes a connection to the correction source and
    // returns a net.Conn that delivers raw correction data.  Connect
    // must respect ctx cancellation.
    Connect(ctx context.Context) (net.Conn, error)
}
```

A TCP source is trivial:

```go
type TCPSource struct {
    Addr string  // "host:port"
}

func (s *TCPSource) Connect(ctx context.Context) (net.Conn, error) {
    return (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
}
```

An NTRIP source would perform the HTTP handshake in `Connect` and
return the underlying connection.

## Sink

```go
// State represents the connection state of a Sink.
type State int

const (
    Connecting   State = iota
    Connected
    Reconnecting
)

// Sink reads correction packets from a Source and writes them to a
// serial port via a pruning queue.  Scanned packets are broadcast
// to subscribers so that subscriber timing reflects true
// network-receive time.
type Sink struct {
    // Packets broadcasts every scanned packet from the correction
    // source.  The caller should subscribe before calling Run.
    // The bcast lives for the entire duration of Run, surviving
    // reconnects -- subscribers are not affected by network drops.
    Packets *bcast.Bcast[scan.Packet]
}

// NewSink creates a Sink.  The caller should subscribe to
// s.Packets before calling Run.
func NewSink() *Sink

// Run connects to the correction source, scans packets, and writes
// each to serialConn via WritePacket.  On network error, Run
// reconnects with exponential backoff (1s, 2s, 4s, ... capped at
// 30s).  It calls onState on each connection state change
// (Connecting, Connected, Reconnecting).  Run blocks until ctx is
// cancelled or serialConn errors.  On cancellation, Run waits for
// all internal goroutines to exit before returning.
func (s *Sink) Run(ctx context.Context, lg *slog.Logger,
    source Source,
    serialConn *gpsio.SerialConn,
    portLock gpsio.OutPortLock,
    pktFormats []gpsprot.PacketFormat,
    onState func(State, error)) error
```

`Run` owns the reconnect loop.  The bcast is created once in
`NewSink` and lives for the entire `Run` call, so subscribers are
not disrupted by reconnects.  The `onState` callback is called on
state transitions; it is safe to call from the reader goroutine
because state changes are infrequent and the callback is lightweight.

## Internal cancellation

`Run` creates an internal context via `context.WithCancel` derived
from the caller's ctx.  All goroutines use this internal context.
Any goroutine that hits a fatal error (writer serial error, or an
unrecoverable reader error) calls the internal cancel, which triggers
shutdown of the other goroutines.  The reader also monitors the
internal context to close the live network connection and unblock any
pending `Scan` call.  `Run` waits for all goroutines to exit before
returning.

Network read errors are not fatal -- the reader reconnects.  Serial
write errors are fatal -- the writer cancels the internal context,
which stops the reader and queue.

## Internal goroutines

`Run` starts four goroutines and waits for all of them before
returning:

**Bcast goroutine:**

Runs `s.Packets.Run(ctx, lg)`.  The bcast requires its own goroutine
(`bcast.Run` blocks) and exits when both its input channel is closed
and `Close()` has been called.  `Run` ensures both conditions are met
during shutdown:

- The reader closes the bcast input channel when it exits (step 10).
- After the reader, queue, and writer goroutines have all exited,
  `Run` calls `s.Packets.Close()` and then waits for the bcast
  goroutine to finish.

This ordering guarantees that subscriber channels are closed by the
bcast goroutine during its shutdown, so external subscribers (e.g.
the UI event goroutine in the desktop app) see a clean channel close
and can exit.  Note: bcast closes subscriber channels as soon as any
shutdown trigger fires (ctx cancel, input close, or Close()), so
some already-queued packets may be dropped before subscribers see
them.  This is acceptable -- correction packets are ephemeral and
losing a few during shutdown has no impact.

Concretely, `Run` uses two WaitGroups: one (`pipelineWg`) for the
reader, queue, and writer; one (`bcastWg`) for the bcast goroutine.
After `pipelineWg.Wait()` returns, `Run` calls `s.Packets.Close()`
and then `bcastWg.Wait()`.

**Reader goroutine** (owns the reconnect loop):

1. Calls `onState(Connecting, nil)`.
2. Calls `source.Connect(ctx)`.
3. On dial error: calls `onState(Reconnecting, err)`, backs off
   with `select { case <-ctx.Done(): return; case <-time.After(d): }`,
   goes to step 1.
4. On success: calls `onState(Connected, nil)`.  Starts a cancel
   goroutine that waits on `ctx.Done()` and closes the connection
   to unblock any pending `Scan` (same pattern as `gpsio.Scan`).
5. Creates a `scan.Scanner` on the connection with `pktFormats`.
6. Scans a packet.
7. Sends the packet into the bcast input channel.
8. Repeats from step 6.
9. On read error: closes the connection.  If ctx is not cancelled,
   calls `onState(Reconnecting, err)`, backs off, goes to step 1.
10. On context cancellation (external or internal): closes the
    connection, closes the bcast input channel, returns.

Backoff: 1s, 2s, 4s, ... capped at 30s, reset on successful
connection.  Backoff waits always use `select` with `ctx.Done()` so
shutdown is never blocked by a sleep.

**Pruning queue goroutine:**

Subscribes to the bcast and mediates between the reader and the
writer.  It maintains a FIFO ordered by insertion time, plus a map
from message type to queue position for O(1) lookup.  On enqueue, if
a packet of the same message type is already in the queue, the old
entry is removed.  On dequeue, the front packet is sent to the
writer.

The queue receives from its bcast subscription channel.  It sends to
the writer via another channel; the send is conditional on the queue
being non-empty (nil channel when empty).  When the subscription
channel closes (bcast shutting down), the queue discards any
remaining buffered packets, closes the writer channel, and returns.
It does not attempt to drain to the writer, because the writer may
have already exited (e.g. after a fatal serial error).

**Writer goroutine:**

1. Receives a packet from the queue channel.  If the channel is
   closed, returns.
2. Acquires `portLock` with cancellation:
   `select { case <-ctx.Done(): return; case port := <-portLock: ... }`.
3. Calls `serialConn.WritePacket(pkt.Data, pkt.Format)`.
4. On write error: releases `portLock`, calls the internal cancel,
   returns.  This triggers shutdown of the reader and queue.
5. Releases `portLock` (`portLock <- port`).
6. Repeats from step 1.

All `portLock` acquires use `select` with `ctx.Done()` to avoid
deadlock during shutdown.  Every successful acquire must have a
`defer portLock <- port` release so the token cannot leak on error
or early return.

## Shutdown sequence

The following describes the full shutdown sequence, whether triggered
by context cancellation or a fatal writer error:

1. Internal context is cancelled (either externally or by the writer).
2. Reader: closes the network connection, closes the bcast input
   channel, returns.
3. Bcast goroutine: sees ctx cancellation, closes all subscriber
   channels (including the queue's subscription).  It continues
   running until both Close() is called and the input channel is
   closed.
4. Queue: sees its subscription channel close, discards buffered
   packets, closes the writer channel, returns.
5. Writer: sees the writer channel close (or has already exited due
   to a serial error), returns.
6. `pipelineWg.Wait()` returns (reader, queue, writer all done).
7. `Run` calls `s.Packets.Close()`.
8. `bcastWg.Wait()` returns (bcast goroutine exits because both
   conditions are met: input channel closed by reader, Close()
   called by Run).
9. `Run` returns.

## Daemon reuse (issue #221)

When the daemon adds correction support, it creates a `corrsink.Sink`
and subscribes to `s.Packets` from the dispatcher's select loop, the
same way the dispatcher already consumes `scan.Packet` from the GPS
receiver's bcast.  The dispatcher routes received RTCM packets
through the observer interface (e.g. an `RtcmPacket` observer event),
keeping the daemon's event/logging pipeline consistent with how GPS
packets are handled today.  The `Source` interface lets the daemon
support TCP, NTRIP, or other transports without changes to corrsink.

## Testing

Tests use `net.Pipe` for the network side and a mock serial
connection.  Key scenarios:

- Packets flow from source to serial port.
- Pruning queue drops stale packets of the same message type.
- Reconnection on network error with backoff.
- Clean shutdown on context cancellation.
- Serial write error triggers shutdown of all goroutines.
- `portLock` is acquired/released around each write.

## Implementation order

1. `Source` interface, `TCPSource`, `State` type.
2. `Sink` struct with `NewSink`/`Run`, reader goroutine, pruning
   queue goroutine, writer goroutine.
3. Tests using `net.Pipe`.
