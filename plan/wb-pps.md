# Workbench PPS tab

Integrate serial PPS with the workbench: a new PPS tab that configures
edge detection on the connected receiver's serial port, starts and
stops it, and shows the received edges as a table, an offset graph,
and summary statistics.

Branch: `wb-pps-proto`. No issue filed yet; add the number to this
heading when one exists.

## Status: prototype

The `wb-pps-proto` branch holds a working end-to-end prototype of this
plan, verified against replayed recordings and live hardware (FT232H,
CTS pin, CASIC receiver). It is a reference, not a merge candidate:
the production work proceeds bottom-up on separate branches off
master, adapting code from the prototype rather than merging it, in
this order:

1. gpsio: redesign the SerialConn pin-watch API around an explicit
   watch object, so that watch creation (the arming point, where
   ErrUnsupported/ErrUnavailable surface) is observable and the
   lazy-creation races and result-ownership handshake inside
   WaitModemControlPinChange dissolve. The design is not settled; the
   prototype does not attempt it.
2. serialpps: method-used reporting built on the new API. The
   prototype's `selected` callback parameter on Detect is provisional
   and stands in for this; the agreed direction is a channel carrying
   the method once it is armed (watch created; for poll, the first
   state read of Poll's init), never for failed fallback rungs.
3. session: the PPS engine, events, and replay source (largely as
   prototyped).
4. satpulsewb and frontend: endpoints, transport capability, the tab
   (largely as prototyped), plus e2e tests and the NEWS entry.

## Decisions taken

- The PPS engine lives in the session (`gps/app/session`), mirroring
  the corrections model: start/stop methods, a sticky state event, a
  snapshot accessor, per-edge events. Both satpulsewb and the desktop
  frontend get it through the transport.
- Detection runs only on the session's connected serial port. The
  session's `conn` from `SerialOpener` is a `*gpsio.SerialConn`, which
  already implements `serialpps.StateReader`/`ChangeWaiter`, and the
  packet pipeline keeps the port drained (required by the wait
  method). The tab is disabled when disconnected or connected over a
  socket.
- The graph plots the frontend-computed mod-1s offset: edge timestamp
  minus the nearest integral second. Every edge is shown; no UTC
  labelling via `serialpps.Generator` (a possible later view).
- The table is separate from the graph and shows, per edge: UTC time,
  local time, offset from top of second, uncertainty, settling. All
  derived in the frontend from the edge timestamp.
- Edge history, stats, and Clear are frontend-only. History is capped;
  a reload or late-joining window starts empty. Clear is a local
  button (works on read-only windows).
- Config fields: pin (cts/dcd/dsr/ri), method (auto/poll/wait/kernel),
  poll prewarm, invert polarity. No max wakeup latency in the UI.
- Stats are computed since Clear over all retained edges (no sliding
  window).
- Simulation for testing: replay a JSONL file recorded by
  `satpulsetool serial -p -j`, fed through the session analogously to
  packet-log replay.

## Open questions

- **Replay retiming details** (proposal below): confirm during
  implementation that rebasing preserves what matters.

## Decisions resolved during prototyping

- **Method-used reporting**: `Detect` gained a
  `selected func(gpsio.PPSMethod)` parameter, called where the
  "serial PPS method selected" log line is emitted; it fires again on
  each fallback, and the session forwards the latest into the
  `gps:pps` state event. The UI shows "Method: poll" etc. while
  running.
- **Statistics** (replacing the earlier jitter question): follow the
  PHC stats model (`time/internal/statsobs`, `systest/clocklog.py`),
  adapted for a nonzero bias: over settled edges only -- settling
  edges are excluded the way out-of-sync PHC samples are, and counted
  separately -- report mean (the bias), std dev about the mean, max
  |offset|, and 95% |offset|, all in microseconds. Displayed in the
  scatter panel's headed StatRow style beside the edge table.
- **No config persistence**: the panel fields are session state only,
  no localStorage. The pin defaults from the selected device -- CTS
  for a USB serial adapter (identified by the `usb` field the ports
  endpoint already serializes), DCD for a native port -- until the
  user picks one. Pin options show DB9 pin numbers: CTS (8), DCD (1),
  DSR (6), RI (9).
- **Layout**: one config row (pin, method, pre-warm, invert polarity,
  Start/Stop, status dot and method/error text); full-width offset
  graph with round-number microsecond ticks and a left-aligned 60 s
  minimum window; below it the edge table (chronological,
  auto-scrolling unless the user scrolls up, packet-panel styling)
  with the statistics block beside it and Clear at its bottom.

## Backend

### serialpps

- Method-used notification per the open question above; nothing else.
  `PollStats` stays a logging concern. Detection errors are returned
  from `Detect` as today; the session turns them into state events.

### session (`gps/app/session`)

New PPS engine mirroring corrections:

- `PPSConfig` (JSON-tagged, from the frontend): pin name, invert
  polarity, method name (empty = auto), poll prewarm seconds.
  Validation reuses `gpsio.ParsePPSMethod` and a pin parser (serialcmd
  has one; either export or duplicate the four-case switch).
- `StartPPS(cfg PPSConfig) error`: requires `StateConnected` and a
  conn that type-asserts to `serialpps.StateReader`; refuses socket
  connections and while an exclusive operation holds the port is not
  a concern (detection reads pin state only, it does not use the
  packet stream). Starting while running restarts with the new
  config (stop first), like corrections. Runs `serialpps.Detect` in a
  goroutine under a context parented on `runCtx`, registered so
  disconnect stops it; goroutines bridge into `connWg` via a captured
  wg, as `StartCorrections` does.
- `StopPPS() error`: cancel and wait, no-op when not running,
  mirroring `StopCorrections` including the stopping-in-progress
  guard.
- `PPSState() PPSEvent` snapshot accessor for late joiners.
- Events:
  - `gps:pps` (`PPSEvent`): `state` ("stopped", "running", "failed"),
    the active config, `method` (the method in use, from the
    notification hook; may update after start on fallback), `error`
    (set when failed, e.g. detection returned an error or the conn
    cannot do PPS).
  - `gps:ppsedge` (`PPSEdgeEvent`): timestamp (RFC3339 with
    microseconds, matching serialcmd's `ppsEvent` rounding),
    `uncertainty` seconds (omitzero), `settling` bool (omitzero) --
    the same information as `satpulsetool serial -p -j` minus the
    device. 1 Hz; no `Wants` gating needed.
- Lifecycle: disconnect (and reconnect's run teardown) stops
  detection; the session emits `gps:pps` stopped. PPS does not
  auto-restart after reconnect (user restarts; keeps v1 simple).
- Interaction with exclusive operations (configuring, sending):
  detection keeps running; it does not touch the packet stream or the
  out-port lock. Verify the poll method's ioctls are safe alongside
  concurrent reads/writes (they are for serialcmd, which drains
  concurrently).

### Edge replay source

For hardware-free testing (unit, e2e, and UI prototyping):

- A replay source that parses the `serial -p -j` JSONL
  (`{"device","t","uncertainty","settling"}`) and re-emits
  `serialpps.CandidateEdge`s in real time.
- Retiming: recorded timestamps are in the past. Rebase the sequence
  onto the current clock so that each edge keeps its fractional-second
  offset and the inter-edge spacing is preserved: compute the
  whole-second shift once from the first edge (round the delta to an
  integral number of seconds), apply it to all edges, and sleep until
  each rebased time. Loop at EOF, shifting by the recording's span
  rounded up to whole seconds, so short recordings replay
  indefinitely.
- Seam in the session: `StartPPS` runs the replay source instead of
  `serialpps.Detect` when one is installed. Installation is a session
  option (e.g. an `EdgeSource` field on `Options`, or a
  `SetPPSSimulation(path)` method) -- decide at implementation time;
  keep it out of `PPSConfig`, which is frontend-facing. When
  simulating, `gps:pps` reports method "replay" (or similar) so the
  UI shows the truth. The connected-serial-port requirement is
  relaxed under simulation so e2e can drive it from a FIFO/ubxsim
  session.
- satpulsewb gets a hidden flag (e.g. `--pps-replay path`) wiring the
  source, following the pattern of its existing test-only knobs; the
  e2e harness and manual HMR prototyping use it.
- Concurrent packet and PPS logs: one recording run produces both
  (`satpulsetool serial -d <dev> -p <pin> -j --packet-log <path>`
  writes the packet log while printing edges), so the two files share
  a timeline. A simulated session replays the packet log through the
  existing FIFO mechanism (receiver side: time messages, PVT) and the
  PPS log through the edge source, giving a coherent fake receiver
  with edges. The mod-1s view only needs each edge's fractional
  second, so the two replays need not be phase-locked; tight
  cross-stream alignment (same whole-second rebase for both) becomes
  necessary only if Generator-based UTC labelling is added later --
  note it as a constraint on that future work, not on v1.

### satpulsewb (`cmd/satpulsewb`)

- Endpoints, following the existing adapter pattern:
  - `GET /api/pps` -> `PPSState()` snapshot.
  - `POST /api/pps/start` (writer) -> `StartPPS` with the JSON body.
  - `POST /api/pps/stop` (writer) -> `StopPPS`.
- SSE: `gps:pps` and `gps:ppsedge` flow through the existing stream;
  the latest-event-per-name cache primes late joiners with the state
  and (harmlessly) the last edge.

## Frontend

All in `webui/packages/workbench` plus the transport implementation in
`workbench-http`; regenerate the embedded assets (`go generate
./cmd/satpulsewb`) in the same change.

### Transport

- Extend `Transport` in `transport.ts` with an optional capability,
  like `connection` and `msgFile`:
  `pps?: {start(cfg): Promise<void>; stop(): Promise<void>;
  getState(): Promise<PPSEvent>}`. Events come through `eventsOn` as
  usual. The tab renders only when `transport.pps` is present, like
  the Message file tab, so the desktop frontend is unaffected until
  its bindings implement the capability.
- `workbench-http` implements it over the three endpoints.

### PPS tab (`pps-panel.tsx`)

New tab between Corrections and Configuration. Layout follows the
corrections panel's conventions:

- Config rows at the top: pin `Select` (CTS/DCD/DSR/RI), method
  `Select` (Auto/Poll/Wait/Kernel), prewarm `Input` (seconds; enabled
  only when method is Auto or Poll), Invert polarity checkbox, status
  dot (muted/success/danger), Start/Stop `Button` right-aligned.
  Values persisted in localStorage. Fields disabled while running,
  when disconnected, or read-only; the state display stays live.
- Status line: "Method: kernel" once known (from `gps:pps`), errors in
  `text-danger`.
- State sync like the corrections panel: subscribe to `gps:pps`,
  reconcile with `getState()` on mount/connect, event-seq guard
  against races.
- Edge intake: subscribe to `gps:ppsedge`; append to a capped ring
  (a few thousand edges) that feeds table, graph, and stats. Clear
  empties it locally.

### Stats strip

Row of figures above the graph, Clear button at the right: edge
count, median offset, max |offset|, jitter (metric open -- implement
behind a small function so candidates can be compared during
prototyping). Computed since Clear over the retained edges; if the
ring cap is ever hit, either raise the cap or note the truncation --
decide when prototyping.

### Graph

Hand-rolled SVG like the scatter panel (no chart library): x = wall
time, y = mod-1s offset with auto-scaled units (ns/us/ms), zero line
centered. Settling edges visually distinct (hollow or muted). Whether
to draw uncertainty as error bars is a prototyping question.

### Table

Newest first, monospace data, scrolling like the packet panel.
Columns: UTC time, local time, offset from top of second,
uncertainty, settling. All derived from the edge timestamp in the
frontend.

## Testing

- Go: session PPS lifecycle tests using the replay source (start,
  edges arrive as events, restart with new config, stop, disconnect
  stops it, socket conn refused). serialpps method-notification test.
  Replay source retiming test with a canned JSONL fixture.
- e2e (Playwright, `webui/packages/e2e`): drive satpulsewb with
  `--pps-replay` and a recorded fixture; assert the tab starts,
  edges populate table/graph, stats update, Clear resets, stop works,
  read-only window sees state but cannot start.
- Manual prototyping: HMR dev server against satpulsewb with
  `--pps-replay` (see `webui/CLAUDE.md`), comparing graph and jitter
  treatments before finalizing.

## Order of work

1. Backend minimum to light up the pipeline: session PPS engine with
   events, replay source, satpulsewb flag and endpoints. (The replay
   source is what unblocks all frontend work without hardware.)
2. Frontend: transport capability, tab with config/table first, then
   graph and stats prototyping over replayed edges.
3. serialpps method notification (resolve the open question when the
   session work makes the trade-off concrete).
4. Tests and e2e; regenerate embedded assets; NEWS.md entry (this is
   a user-facing feature) in the same change as the implementation.
