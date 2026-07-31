# Terminal UI shell for the GPS session (no issue yet)

## Goal

A terminal-based UI for interactive work with a GPS receiver: a third
shell over `gps/app/session`, alongside the web workbench (master) and
the Wails desktop app (desktop-gui branch). It should cover the same
ground as the workbench where a terminal can do it well, so it is
usable over ssh on a headless box with a receiver attached, with no
browser involved.

The work is done on a separate `tui` branch. This is a first pass:
the aim is a working, tasteful TUI with the core views, not feature
parity with the workbench. Where the workbench does
something a terminal cannot (maps, graphics), drop it or substitute
something simpler rather than forcing it.

## Concept

Use Bubble Tea v2 (`github.com/charmbracelet/bubbletea/v2`), with
`lipgloss/v2` for layout and styling and `bubbles/v2` for standard
components (table, viewport, textinput, spinner). v2 is a breaking
rework of the v1 API and most published example code targets v1:
follow the v2 API documentation, and do not mix in v1 patterns.
Bubble Tea's Elm-style message loop matches the session design
directly:

- The `session.Sink` implementation wraps each `session.Event` in a
  `tea.Msg` and delivers it with `Program.Send`, which is safe from
  session goroutines. Emit must not block; if backpressure is a
  problem in practice, coalesce keep-latest per event name (the render
  loop only ever wants the newest snapshot of high-rate events).
- `Wants` returns false for `gps:packet` unless the Packets view is
  active, mirroring how the workbench gates packet streaming on
  visibility.
- Blocking session methods (`ReadConfig`, `ApplyConfig`) run as
  `tea.Cmd`s; their results come back as messages. `Connect`,
  `SendMsgFile`, and `StartCorrections` are already asynchronous with
  progress delivered as events.

The session is used in-process: construct `session.New` with the TUI
sink, no HTTP or serialization layer. Route slog through
`session.NewLogHandler` with a file (or discard) base handler so
nothing writes to the terminal behind Bubble Tea's back.

The TUI is its own command, `satpulsetui` (`cmd/satpulsetui`),
taking satpulsewb's receiver flags (device, speed, vendor filter,
packet log path). It was first built as `satpulsewb -t`, but that
shorthand already belongs to satpulsewb's `--token`, which cannot
change; see Decided. A new package means an entry in
`docs/internals/packages.md`. The feature needs a NEWS.md entry.

## Structure

Mirror the workbench layout, translated to terminal idiom:

- A persistent header line: connection state, receiver
  identification, port speed, corrections state. This replaces the
  workbench's fixed connection panel; connect/disconnect and
  connection parameters get a small view or overlay of their own.
- A tab bar of full-screen views, switched by key, following the
  workbench tabs: Monitor, Packets, Corrections, Config, Messages
  (Messages only when a message file is loaded, as in the workbench).
- A footer line with the key hints for the active view.

Per view, the corresponding workbench panel sources
(`webui/packages/workbench/src/*.tsx`) are the reference for what to
show, what to label it, and when controls are enabled or disabled --
the TUI consumes the same events and calls the same session methods,
so the workbench code answers most "what should this do" questions.

- Monitor: a scrollable stack of sections like the workbench's
  collapsible sections -- summary, clock, PVT, satellite signals
  (text bar chart), survey, position statistics. Skip the map, the
  sky view (the signals table already carries elevation and azimuth
  per satellite; cf. gpsd's cgps, which also shows a table and leaves
  the pictorial display to xgps), and the scatter dot plot. The
  position statistics section keeps the numbers the workbench scatter
  panel computes -- running mean position and CEP50 / CEP95 /
  horizontal, vertical, and 3D RMS over accumulated fixes (see
  scatter-panel.tsx) -- without the plot.
- Packets: scrolling packet list from `gps:packet` events with a
  detail view for the selected packet (`session.DecodePacket`).
  Pause/resume scrolling.
- Corrections: source form (mode, host, port, mountpoint,
  credentials, NMEA send), start/stop, status and packet summary from
  `gps:corrections` / `gps:corrpacket` / `gps:basearp`.
- Config: read and apply configuration, controls gated on the probed
  `ConfigSupport` flags exactly as the workbench Config tab gates
  its controls.
- Messages: tag list from `SetMsgFile`, send progress and receiver
  responses from `gps:msgsend` / `gps:response`.

A view that is not implemented in the first pass should simply not
appear in the tab bar; do not ship placeholder tabs.

## Prerequisite: payload aliasing check

The existing shells serialize every event payload to JSON at emit
time, which snapshots it. The TUI holds payloads by reference, so
pointer or reference-typed payloads (`gps:time` is
`*gpsprot.TimeMsg`, `MsgEvent.Msg` is `any`, slices and maps inside
event structs) are only safe if the session and packet processors do
not mutate or reuse them after emitting. Before building on the
events, check this for each emitted payload type; where an emitter
does reuse a value, either fix the emitter to allocate per emit or
have the TUI sink copy on receipt. Record the outcome in this file.

### Outcome

Checked every emitted payload type against the emitters. All are
safe to hold by reference after Emit returns; the TUI sink does not
need to copy on receipt. The load-bearing facts:

- scan.Packet.Data is a string copied out of the scanner's reused
  buffer (scan.go `p.Data = string(pkt)`), so everything built from
  it downstream is independently allocated.
- gps:time: TimeTicker.Time emits a fresh local copy per epoch and
  stores its own value copy separately; TimeMsg has no reference
  fields.
- gps:msg: every packet processor allocates a fresh gpsprot.*Msg per
  packet. The processors that accumulate satellite or nav-epoch
  messages across packets (ubx, unc, casic, septentrio) all release
  their reference before emitting: ubx/unc nil their satMsg/sigMsg
  fields before combine-and-emit, casic's accumulator resets with
  `a.svs = nil` (not `[:0]`), and every FlushNavEpoch nils
  curNavEpochMsg before returning it. MergeNavEpoch mutates its
  first argument, but only before the emit.
- gps:epochPVT: PVMsgBundle is four opt.Val values whose message
  types contain only arrays and scalars, so the emitted value copy
  is a deep copy.
- gps:packet: inbound entries copy pkt.Data (MakePacketLogEntry).
  Outbound entries alias the caller's write buffer
  (gpsio.PacketLog.LogOutput), but every current writer passes a
  fresh []byte(string) conversion or a one-shot slice, never a
  reused scratch buffer; this is an invariant to keep, not a bug to
  fix.
- gps:corrpacket allocates per packet and NativeMsg is nilled before
  emit; gps:receiver shares its SignalsSupported map and
  PacketFormats slice with the Session.Receiver() snapshot but they
  are never mutated (the TUI must also treat them as read-only);
  gps:log builds a fresh Attrs map per record; the remaining
  payloads are pure value types.

## Testing

No GPS hardware is needed: the Playwright e2e harness
(`webui/packages/e2e/harness.ts`) already exercises the workbench
against three data sources, and all three sit below the shell, so
they drive `satpulsetui` unchanged:

- FIFO replay: `satpulsetool pack --realtime <factor> <log>` into a
  named pipe, with `satpulsetui -d <fifo> -s 38400`. Read-only;
  drives the Monitor and Packets views (the harness uses
  `gps/testdata/packets/u-blox/ZED-F9P/daemon-sats-pos-38400.jsonl`).
- The u-blox F9P simulator: `satpulsetool ubxsim` behind a pty
  symlink (see the harness's launchUbxsim for the invocation). It
  answers probes and configuration (VALGET/VALSET, saves, resets)
  and consumes writes, so it drives the Config and Messages views
  and the receiver side of Corrections.
- The fake NTRIP caster
  (`smoketest/scenarios/stream/fakesource.py`) streaming a base
  station RTCM capture, for the Corrections source form; pair with
  the ubxsim pty, not the FIFO (correction sessions write back to
  the port).

The message file library for the Messages view comes from
`configs/gpsmsg` via `SATPULSE_GPSMSG_PATH`, as in the harness.

Unit-test the TUI model logic (event handling, coalescing sink,
state rendering) with synthetic session events; use these data
sources for interactive and end-to-end verification. How much of the
Playwright-style scripted testing to reproduce for the terminal
frontend is left to the implementation.

## Non-goals for the first pass

- No map, no sky view, no scatter plot: no raster or
  character-graphics plotting at all in the first pass.
- No multi-client concerns (seats, priming): single window,
  in-process.
- No new session/ API; if the TUI seems to need one, stop and raise
  it rather than adding it.

## Decided

- No sky view: the satellite signals table carries elevation and
  azimuth, which is sufficient in a terminal.
- Position scatter is replaced by its statistics readout (CEP etc.);
  no plotting.
- The TUI is the separate `satpulsetui` binary, for now. The original
  decision was `satpulsewb -t`, but `-t` is satpulsewb's `--token`
  shorthand and cannot be repurposed. Not added to install/deb/rpm
  packaging yet.
- All five views are in scope for the first pass.
- Keybindings: not vim-style; arrows, tab, enter, and similar
  conventional keys.
- No need to fit 80x24; design for a reasonably sized modern
  terminal window. Degrade gracefully (scroll, truncate) rather than
  break when smaller.
- Bubble Tea v2, with matching bubbles/v2 and lipgloss/v2.
