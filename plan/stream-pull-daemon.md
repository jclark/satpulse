# Stream pull daemon integration (#221, #99)

Integrate `stream.pull` into satpulsed.  The `gps/app/stream`
package handles the network/serial correction data path; this plan
covers daemon configuration and lifecycle wiring only.

RTCM observability is a separate concern; see `plan/rtcm-obs.md`.

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

A separate serial port for corrections is a follow-on phase.  The
first phase shares the receiver's serial port; see "Phase 2
(follow-on): separate serial port" below.

## Data path

The pull path is network-to-receiver:

```text
NTRIP/TCP source
  -> stream.Pull reader
  -> RTCM packet scanner
  -> pruning queue
  -> serial writer
  -> receiver correction input
```

`stream.Pull.Packets` continues to broadcast scanned packets inside
`gps/app/stream`; this plan does not connect that broadcast to the
daemon dispatcher.  `plan/rtcm-obs.md` adds optional observation of
those packets.

## Phase 1: shared serial port

The first phase writes corrections to the same serial port the daemon
already uses for the receiver, sharing its `OutPortLock`.  This
covers the common single-receiver setup and lets the rest of the
daemon wiring (config, lifecycle, logging) land before the
multi-port plumbing.

### Configuration

Add `Stream stream.Config` to `Config` in
`time/app/daemon/config.go`.  The `stream.Config` type is defined
in `gps/app/stream` and matches the `[stream.pull]` TOML table
described above.

Put stream-specific daemon code in a new file
`time/app/daemon/stream.go`.

### Startup in `run()`

Stream setup is split into prepare and start phases so the daemon
does not launch a long-running stream goroutine until the remaining
daemon setup has succeeded.

**Prepare step:**

If `[stream.pull]` is configured:

1. Create `stream.NewPull()`.
2. Create the appropriate `Source` (`stream.TCPSource` or
   `stream.NtripSource`) based on which transport keys are present.
3. Reuse the main receiver `portLock` and `SerialConn` as the
   correction output port.  Reject `[stream.pull.serial]` in
   config validation; it is reserved for the follow-on phase.
4. Build the packet format list for correction input scanning.  For
   now this is RTCM.

**Start step:**

Immediately before starting the dispatcher, after all fallible
setup has completed, start `pull.Run` in a goroutine via `wg.Go`.
Pass it the prepared source, packet writer, output port lock,
packet formats, and state callback.

If `pull.Run` returns a non-cancel error (e.g. serial write failure),
the goroutine logs it and exits.  Stream pull errors must not cancel
the daemon: time/PHC sync is independent of corrections, and the
daemon should continue degraded rather than tear down on a
correction-side fault.

No dispatcher API change is required by this plan.  The dispatcher
continues to run with its existing GPS packet and timestamp inputs.

### Shutdown ordering

The daemon context owns stream shutdown.

1. Daemon cancellation cancels the context passed to `Pull.Run`.
2. `Pull.Run` stops the reader, queue, and writer pipeline.
3. `Pull.Run` closes `Pull.Packets` after its internal pipeline has
   drained.

Because stream pull is context-based and independent of dispatcher
channel lifetime, shutdown does not require the dispatcher to drain
any stream-specific channel.

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

### Implementation order

1. `stream.Config` TOML integration in daemon `Config`, including a
   validation error for `[stream.pull.serial]`.
2. `time/app/daemon/stream.go` prepare/start helpers.
3. Wire `run()` to prepare stream pull before final daemon setup and
   start it immediately before `d.Run(...)`.
4. Tests.

### Testing

- Unit tests for `[stream.pull]` config parsing: TCP vs Ntrip,
  mutual exclusion, required fields, and rejection of
  `[stream.pull.serial]`.
- Unit tests for daemon stream helper behavior where practical
  (configured vs not configured, source selection).
- Existing `gps/app/stream` pull tests continue to cover reader,
  scanner, pruning queue, writer, reconnect, and Ntrip transport.
- `make test` for no regressions.
- Manual: add `[stream.pull]` to `/etc/satpulse.toml` pointing at a
  correction source, verify connection-state logs and receiver
  correction status.

## Phase 2 (follow-on): separate serial port

Allow `[stream.pull]` to drive a second serial port dedicated to
corrections, separate from the main receiver port.  This is useful
for receivers with a dedicated correction input channel.

### TOML

```toml
[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
serial.device = "/dev/ttyUSB1"
serial.speed = 115200
```

`serial.device` is required when `[stream.pull.serial]` is present;
`serial.speed` follows the same `0 = use current speed` semantics as
the main `[serial]` table.

### Configuration

Lift the Phase 1 validation that rejects `[stream.pull.serial]`.
Reject configs where `serial.device` equals the main `[serial]`
device (would conflict with the main port lock).

### Startup

Replace Phase 1 step 3 with: if `[stream.pull.serial]` is present,
open a second `*gpsio.SerialConn` via `gpsio.OpenSerial(device,
speed)`, build a fresh `OutPortLock` for it, and use that connection
as both the `PacketWriter` and the locked port.  The daemon `run()`
defers `Close` on this connection.  Otherwise, fall back to the
shared-port wiring from Phase 1.

### Shutdown

Adds one step to the Phase 1 sequence: after `Pull.Run` returns, the
daemon closes the separate `SerialConn` (via the deferred close).

### Testing

- Config parsing: separate `serial.{device,speed}` fields, conflict
  with main `[serial].device`.
- Daemon helper: separate `SerialConn` is opened when
  `serial.device` is set and closed on shutdown.
