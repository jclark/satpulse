# VRS NMEA send (send position as NMEA)

## Goal

Add VRS support to the Corrections tab: upload the receiver's position to an
Ntrip caster as an NMEA GGA sentence, so a VRS (virtual reference station)
caster can serve corrections for the receiver's location. The daemon already
does this (root `plan/vrs.md`, `stream.pull.ntrip` VRS mode); this brings the
same capability to the desktop GUI.

MVP scope: a read-only, live Position display plus a "Send position as NMEA"
checkbox that gates connection on having a position. Manual position override
is a deferred follow-up (see [Deferred](#deferred)).

## Background

A VRS caster needs the rover's approximate position before it can generate
corrections, so the client periodically uploads a GGA sentence. A GGA is a full
NMEA sentence -- UTC time, fix quality, satellite count, HDOP, height, geoid
separation, checksum -- not just lat/lon, and the caster wants a current one.
The receiver may not emit NMEA GGA itself, so the position is *synthesized* into
a GGA from the receiver's gpsprot navigation messages.

The reusable pieces already exist in `gps/app/stream` (landed via the gga-synth
and vrs PRs):

- `nmeasyn.Synth` -- a `gpsprot.MsgHandler` that builds a live
  `nmeamsg.GGASentence` each epoch from `Time`/`PosGeo`/`NavEpoch` and calls a
  sink.
- `stream.GGASelector` -- `NewGGASelector()`, `Packet(pkt) bool` (offer an
  original receiver GGA), `Msg(m nmeamsg.GNSSTalkerIDMsg, phase)` (it implements
  the `nmeasyn.Sink` interface, so the synth writes synthesized GGAs here),
  `Packets() <-chan scan.Packet` (the selected-GGA feed). It prefers an original
  receiver GGA over a synthesized one for the same UTC.
- `stream.NewPull(source, lg, pw, portLock, pktFormats, nmeaSendInterval)` plus
  `Pull.Run(ctx, selectedGGA, onState)` -- the pull uploads from the
  `selectedGGA` feed via an internal `GGASender` (resend every interval;
  `WaitReady` waits for the first usable GGA before connecting). A `nil` feed
  means no NMEA send.

The desktop already decodes the receiver stream into gpsprot messages in
`packetWorker` (a `msgHandler` handling `Time`/`PosGeo`/`PosECEF`/`NavEpoch`), so
synthesis is straightforward to wire in.

## Design

### UX

- A "Send position as NMEA" checkbox in the Corrections tab, meaningful only for
  the Ntrip source mode (VRS casters are Ntrip); hidden or disabled for TCP.
- An always-visible, read-only "Position:" field showing live Lat, Lon. It is
  empty until there is a usable fix, then updates live.
- When the checkbox is on, the Connect/Start control is greyed out until the
  Position field shows a value. This makes "waiting for a fix" visible instead
  of a silent connect-time stall.
- Manual position entry (override) is out of scope for the MVP.

### The single-owner invariant

The Position field must show a value if and only if a connect would succeed
without delay -- with no race between the displayed position and the position
the uploader actually has. The race to avoid is two independent derivations of
"ready": the frontend judging readiness from one position source while the
`GGASender` judges it from another (e.g. a `gps:epochPVT`-style feed vs the
selector).

Rule: there is one owner of readiness, and it is the **selected-GGA feed
itself** -- the exact GGA the uploader would send. The Position display, the
Connect gate, and the connect-without-delay guarantee are all the same one
backend fact read three times, never recomputed.

Because the desktop must show the position live even when no corrections session
is running, it interposes a single backend component between `sel.Packets()` and
`Pull.Run` (where the daemon hands `sel.Packets()` straight to the sender):

- **GGA monitor** (connection-scoped, created in `packetWorker`): the sole
  reader of `sel.Packets()`. It parses each GGA with public `nmeamsg` (which it
  needs anyway for the lat/lon display) and treats one as *usable* when quality
  > 0 with lat/lon set -- the same condition `GGASender` applies before
  uploading, so the gate and the upload agree without sharing any code. On
  change it emits a `gps:nmeaPosition` event ({lat, lon}, or cleared). When an
  NMEA-send session is active it forwards the same feed into the channel passed
  to `Pull.Run`, primed with the current latest.

`StartCorrections` owns the connect-without-delay guarantee and enforces it
atomically: with the checkbox on, it starts the session only if the monitor
currently holds a usable GGA -- in which case the sender's feed is already
primed and the upload goes out immediately (`WaitReady` returns at once).
Otherwise it refuses with "waiting for a position fix" rather than starting and
blocking. The frontend's box and greyed Connect are reflections of
`gps:nmeaPosition`, i.e. UX, not authority; a fix dropping between the last event
and the click resolves as a clean rejection, not a hang. `WaitReady` remains
only as a backstop the design never hits.

### Synthesis wiring (always-on)

In `packetWorker`, create `sel := stream.NewGGASelector()` and
`synth := nmeasyn.New(sel)`, install handlers as
`gpsprot.SetAllMsgHandlers(procs, gpsprot.NewMultiHandler(mh, synth))`, and feed
`sel.Packet(pkt)` for each packet in the processing loop (mirroring the daemon's
dispatcher). The synth and selector run for the whole connection, not just
during a session: the handler set is fixed at connect, and the cost is
negligible (one GGA per epoch into a capacity-1 channel). The monitor and `sel`
are stored on `App` so `StartCorrections` can reach them.

If the receiver emits neither NMEA GGA nor position messages, the synth produces
no usable GGA, the Position field stays empty, and Connect stays gated -- the
missing-position condition is visible rather than silent. (For the F9P, position
output is on by default.)

## Implementation

### Prerequisites

- The `gps/app/stream` VRS pieces described under Background (`nmeasyn.Synth`,
  `stream.GGASelector`, and the `NewPull`/`Run` selected-GGA API) are present in
  this branch's tree -- that is, the vrs PR stack (#331 nmea-decode,
  #332 gga-synth, #333 vrs) has landed on master and been merged into
  desktop-gui. This branch does not have them yet.
- The `StartCorrections` pull call migrates to the new shapes regardless of NMEA
  send: `stream.NewPull(source, lg, conn, portLock, pktFormats, interval)` plus
  `Run(ctx, selectedGGA, onState)`. (Shared with the spartn entry in
  `issues.md`, which also switches `pktFormats` to
  `gpsreg.CreateCorrectionFormats()`.)

No `gps/app/stream` changes are required. The monitor judges usability by
parsing the feed itself with public `nmeamsg` (see Backend), so there is nothing
to add or export on the stream side.

### Backend (`desktop/app.go`)

- `packetWorker`: create `sel`/`synth`, install via `NewMultiHandler`, feed
  `sel.Packet(pkt)`, start the GGA monitor goroutine, store `sel`/monitor on
  `App`.
- GGA monitor: sole reader of `sel.Packets()`; tracks the latest usable GGA;
  emits `gps:nmeaPosition`; provides a `ready()` check and a session-feed
  registration (prime plus forward) for `StartCorrections`.
- `CorrectionSource`: add `NMEASend bool` (interval defaults to
  `stream.DefaultNMEASendInterval`); valid only with `Mode == "ntrip"`.
- `StartCorrections`: migrate to the new `NewPull`/`Run`; when `NMEASend`, gate
  on `monitor.ready()` (reject if not), register the session feed, and pass it
  as `selectedGGA`; otherwise pass `nil`.
- Add `gps:nmeaPosition` to the emitted events.

### Frontend (Corrections tab)

- Always-visible read-only "Position:" field, rendered from `gps:nmeaPosition`
  (lat, lon), cleared on the cleared event and on `disconnected`.
- "Send position as NMEA" checkbox, shown for Ntrip mode; pass `NMEASend` in the
  start request.
- Disable Connect/Start while the checkbox is on and `gps:nmeaPosition` has no
  value.

### Files changed

- `desktop/app.go` (selector/synth/monitor wiring, `CorrectionSource`,
  `StartCorrections`, new event)
- the Corrections tab frontend component(s) (Position field, checkbox, Connect
  gating)

## Deferred

Editable Position (manual override): let the user type a position that
permanently replaces the synth-deduced position (useful for testing against a
caster without a live fix). This needs its own UX to make the override state
obvious, and a backend path that substitutes the typed lat/lon into the
otherwise-live synthesized GGA. Out of scope for the MVP.

## Testing

Playwright plus manual, with a live VRS caster (e.g. u-blox PointPerfect) and
the F9P:

- Checkbox off: corrections behave as today; no Position gating.
- Checkbox on, no fix yet: Position empty, Connect greyed.
- On first usable fix: Position shows live lat/lon, Connect enables.
- Connect with checkbox on: session starts immediately (no stall), GGA uploaded,
  caster serves corrections, receiver reaches RTK.
- Lose fix between display and click (forced): backend rejects with "waiting for
  a position fix", no hang.
- Disconnect: Position clears.
