# Corrections tab (consume mode)

Forward correction data from a remote TCP source to the connected GPS
receiver.  The user enters a host and port, clicks Start, and RTCM
packets flow from the network into the receiver's serial port.

Provide mode (base use case, serving corrections to network clients) is
out of scope for this plan.

Related: GitHub issue #221 adds a `[correction]` section to satpulsed
for the daemon equivalent.

## Preconditions

The `gps/app/corrsink` package must be implemented first.  See
`plan/corrsink.md` for the package design.

## Architecture

### Write coordination

Currently the desktop app serializes writes via the state machine: config
and send are mutually exclusive and each writes to `conn` directly.
Corrections run concurrently, so a coordination mechanism is needed.

Use `gpsio.OutPortLock` as a semaphore (not a handle provider).  The
correction consumer holds a direct reference to `*gpsio.SerialConn` for
calling `WritePacket`.  It acquires `OutPortLock` before writing and
releases between packets.  Config and send drain `OutPortLock` before
they start writing.

`OutPortLock` is created in `Connect()` after opening the serial port.
No changes to the `OutPortLock` type are needed.

## Backend changes (`desktop/app.go`)

### New fields on App

```go
portLock     gpsio.OutPortLock    // created in Connect
corrCtx      context.Context      // cancelled to stop corrections
corrCancel   context.CancelFunc
corrWg       sync.WaitGroup       // tracks correction goroutine
corrStopping bool                 // true while stopCorrLocked waits
```

### Changes to Connect

After `gpsio.OpenSerial`:

```go
a.portLock = gpsio.NewOutPortLock(conn)
```

### Changes to closeLocked

Call `stopCorrLocked()` early in `closeLocked()`, before cancelling
`connCtx`.  This synchronously waits for the correction goroutine to
exit, ensuring it is not writing to the serial port when the connection
is closed.  The goroutine is also in `connWg`, so `connWg.Wait()` will
not hang on it.

### Changes to config and send write paths

Config writes happen inside `gpscfg.Configure`, which takes a
`gpsio.Conn`.  The `Conn` interface includes `Write`, and
`gpscfg.Configure` calls it directly.  To coordinate with corrections:

- In `packetWorker`, before calling `gpscfg.Configure`, drain
  `portLock` with cancellation:
  `select { case <-ctx.Done(): ...; case port := <-portLock: ... }`.
  After `Configure` returns, release it (`portLock <- port`).
  This pauses corrections for the duration of the config operation.

- In `sendWorker`, same pattern: drain `portLock` at the start of the
  send session, release when done.  Since send already has exclusive
  access via the state machine, this just adds the corrections pause.

All `portLock` acquires use `select` with `ctx.Done()` so shutdown
cannot deadlock waiting for the token.

### New exported methods

```go
// StartCorrections dials the remote address and starts forwarding
// correction packets to the GPS receiver.
func (a *App) StartCorrections(host string, port int) Result

// StopCorrections stops the correction forwarding.
func (a *App) StopCorrections() Result
```

#### StartCorrections

1. Validate args; require connected state.
2. If corrections already running, call `stopCorrLocked()` which
   cancels `corrCtx` and waits on `corrWg` for the old goroutine to
   fully exit before proceeding.  This prevents races between old and
   new correction sessions.
3. Create `corrCtx, corrCancel` from `connCtx`.
4. Create a `corrsink.Sink` via `corrsink.NewSink()`.
5. Create a `corrsink.TCPSource` with the target address.
6. Subscribe to `sink.Packets` and start a goroutine (tracked in
   both `connWg` and `corrWg`) that reads scanned packets from the
   subscription and emits `gps:corrpacket` Wails events.  This
   goroutine is decoupled from both the network reader and the
   serial writer, so `EventsEmit` latency cannot affect either path.
   It exits when the subscription channel closes (which happens when
   `sink.Run` returns and the bcast shuts down).
7. Spawn a goroutine (tracked in both `connWg` and `corrWg`) that
   calls `sink.Run(corrCtx, lg, source, serialConn, portLock,
   rtcmFormats, onState)`.  The `onState` callback emits
   `gps:corrections` Wails events.
8. Return immediately.

#### StopCorrections

1. Call `stopCorrLocked()`: cancel `corrCtx`, wait on `corrWg`.
2. Emit `gps:corrections` event with stopped status.

#### stopCorrLocked helper

```go
func (a *App) stopCorrLocked() {
    if a.corrCancel == nil {
        return
    }
    a.corrCancel()
    a.corrCancel = nil
    // Set corrStopping so a concurrent StartCorrections/StopCorrections
    // that enters while mu is dropped will see that a stop is in
    // progress and wait rather than proceeding.
    a.corrStopping = true
    a.mu.Unlock()
    a.corrWg.Wait()
    a.mu.Lock()
    a.corrStopping = false
}
```

`StartCorrections` and `StopCorrections` must check `corrStopping` at
entry and return an error (or spin on a condition variable) if true.
The simplest approach: return `Result{Error: "corrections stopping"}`
and let the frontend retry or disable the button during transitions.

### Events

The `onState` callback passed to `sink.Run` emits `gps:corrections`
Wails events on every state change.  `StopCorrections` emits
`stopped` after `corrWg.Wait()` returns.  The payload:

```go
type CorrEvent struct {
    State string `json:"state"`           // "connecting", "connected", "reconnecting", "stopped"
    Host  string `json:"host,omitempty"`
    Port  int    `json:"port,omitempty"`
    Error string `json:"error,omitempty"` // last error (set during reconnecting)
}
```

State transitions:
- `onState(Connecting, nil)` -> emit `connecting`
- `onState(Connected, nil)` -> emit `connected`
- `onState(Reconnecting, err)` -> emit `reconnecting` (with error)
- `StopCorrections` or disconnect -> emit `stopped`

## RTCM packet table

### Packet event: `gps:corrpacket`

In `app.go`, `StartCorrections` subscribes to `sink.Packets` and
starts a goroutine (tracked in both `connWg` and `corrWg`) that
reads from the subscription channel.  For each `scan.Packet`, it
extracts the RTCM message type via `pkt.Format.MsgID(pkt.Data)` and
emits a Wails `gps:corrpacket` event with payload `{msg: "1074"}`.
The goroutine exits when the subscription channel closes (after
`sink.Run` returns and the bcast shuts down), so `corrWg.Wait()` in
`stopCorrLocked` reliably waits for it.

Because this goroutine is a bcast subscriber, it runs independently
of both the network reader and the serial writer.  Packet timing
reflects true network-receive time (`pkt.TRead`), and any latency
in `EventsEmit` cannot stall the reader or writer.  Every packet is
counted -- bcast delivers all packets to all subscribers.

Correction packets also appear in the Packets tab as outgoing RTCM
(via `WritePacket` -> packet log -> `gps:packet`), but the RTCM table
shows the network-receive side.

### Frontend aggregation

`rtcm-panel.tsx` listens for `gps:corrpacket` events and maintains a
`Map<string, MsgRow>` keyed by message type string:

```typescript
interface MsgRow {
    msgType: string;     // e.g. "1074"
    count: number;       // total packets received
    lastTime: number;    // Date.now() of last packet
    prevTime: number;    // Date.now() of second-to-last packet (for rate)
}
```

On each event: look up or create the row, increment `count`, shift
`lastTime` into `prevTime`, set `lastTime = Date.now()`.

### Table columns

| Column      | Source                                      | Format          |
|-------------|---------------------------------------------|-----------------|
| Type        | `msgType`                                   | e.g. `1074`     |
| Description | `rtcmDescription(msgType)` (frontend only)  | e.g. `GPS MSM4` |
| Count       | `count`                                     | integer         |
| Age         | `now - lastTime`                            | `Ns` (seconds)  |
| Rate        | `lastTime - prevTime`                       | `Ns` (seconds)  |

Age and rate are displayed as whole seconds, updated by a 1-second
`setInterval` timer that forces a re-render.

### RTCM description mapping (frontend)

A pure function `rtcmDescription(msgType: string): string` that maps
message type numbers to short descriptions.  MSM types follow a regular
pattern:

```
MSM base: GPS 1070, GLONASS 1080, Galileo 1090, SBAS 1100,
          QZSS 1110, BeiDou 1120, NavIC 1130
MSM offset 1-7 -> MSM1-MSM7
```

So `1074` = GPS (1070) + 4 = `"GPS MSM4"`, `1127` = BeiDou (1120) + 7
= `"BeiDou MSM7"`.

Non-MSM types use a small lookup:

| Type | Description              |
|------|--------------------------|
| 1005 | Station ARP              |
| 1006 | Station ARP + height     |
| 1007 | Antenna descriptor       |
| 1008 | Antenna + serial         |
| 1033 | Receiver + antenna       |
| 1230 | GLONASS bias             |

Unknown types show an empty description.

### Clear behaviour

A Clear button below the table resets `msgRows` to an empty map.  The
table is also cleared automatically on disconnect (`gps:state` ->
`disconnected`).

## Frontend changes

### New tab

Add `'corrections'` to the `TabID` union in `app.tsx`.  Add the tab
button between Config and Messages.  The tab is always visible but its
controls are disabled when disconnected.

### Two components

**`corrections-panel.tsx`** -- the whole Corrections tab.  Contains
connection controls (host/port inputs, start/stop buttons, status line)
and embeds `RtcmPanel`.

Layout follows the same vertical flex pattern as `msgfile-panel.tsx`:
outer `div` is `flex h-full flex-col`.  The controls toolbar sits at
the top with `px-4 pt-4 pb-2` and `items-center gap-3`, matching the
message file toolbar.  Use `Input` from `ui.tsx` for host/port fields,
`Button` for Start/Stop, and `fieldLabelText` for "Host:" / "Port:"
labels.  Status text uses `text-xs`: `text-success` for connected,
`text-info` for connecting, `text-warning` for reconnecting,
`text-text-muted` for stopped.  The `RtcmPanel` fills the remaining
space with `flex-1`.

```
Host:    [___________]    Port: [_____]

[ Start ]  /  [ Stop ]

Status:  Connected to 10.0.0.1:2006
         Connecting to 10.0.0.1:2006...
         Reconnecting: connection refused
         Stopped
```

State:
- `host`, `port`: text input state, persisted to localStorage
- `corrState`: `'stopped' | 'connecting' | 'connected' | 'reconnecting'`,
  from `gps:corrections` events
- `corrError`: string, last error (shown during reconnecting)

Wails bindings:
- Call `StartCorrections(host, port)` and `StopCorrections()`.
- Listen for `gps:corrections` events via `EventsOn`.
- On disconnect (`gps:state` -> `disconnected`), reset corrState.

**`rtcm-panel.tsx`** -- the RTCM packet table.  Embedded inside
`corrections-panel.tsx`.  Listens for `gps:corrpacket` events
independently.

Follow the same visual pattern as the tables in `packet-panel.tsx` and
`msgfile-panel.tsx`.  Table container: `mx-3 mt-3 flex-2 overflow-y-auto
rounded border border-border-subtle bg-surface-2` (same as packet
panel).  Sticky header: `sticky top-0 z-10 bg-surface-2`.  Header row:
`text-left text-text-secondary` with `whitespace-nowrap px-2 py-1.5`.
Body: `font-mono` on `<tbody>`, cells use `px-2 py-0.5`.  Numeric
columns (Count, Age, Rate) are `text-right tabular-nums`.  Text colour:
`text-text-primary` for active rows, `text-text-muted` for stale (age
> some threshold).  Row hover: `hover:bg-surface-3`.  The Clear button
sits in a toolbar below the table: `mx-3 flex shrink-0 items-center
gap-2 py-1.5` with a `flex-1` spacer pushing Clear to the right -- same
layout as the packet panel toolbar.  `text-xs` on the outer `div`.  Use
only semantic tokens; no hardcoded colours or `dark:` prefixes.

```
 Type  | Description    | Count | Age | Rate
 1074  | GPS MSM4       |   142 |  1s |  1s
 1084  | GLONASS MSM4   |   141 |  1s |  1s
 1094  | Galileo MSM4   |   142 |  1s |  1s
 1124  | BeiDou MSM4    |   142 |  1s |  1s
 1005  | Station ARP    |   14  |  7s | 10s
 1230  | GLONASS bias   |   14  |  3s | 10s

                                        [ Clear ]
```

State:
- `msgRows`: `Map<string, MsgRow>`, keyed by RTCM message type
- `tick`: counter incremented every second to re-render age/rate

Listens for `gps:corrpacket` events via `EventsOn`.

Cleared by Clear button, or automatically on disconnect
(`gps:state` -> `disconnected`).

## Implementation order

Precondition: `gps/app/corrsink` package (see `plan/corrsink.md`).

1. Backend: add `portLock` to `App`, create in `Connect`.
2. Backend: wrap config and send write paths with portLock
   acquire/release.
3. Backend: `StartCorrections` / `StopCorrections`.  Subscribe to
   `sink.Packets` for `gps:corrpacket` events; use `onState` for
   `gps:corrections` events.
4. Frontend: `rtcm-panel.tsx` (RTCM packet table) and
   `corrections-panel.tsx` (tab with connection controls + rtcm-panel).
5. End-to-end test: consume corrections from a satpulsed TCP proxy.
