# NMEA GGA support: synthesis and proxy (#329)

## Motivation

GGA is the NMEA sentence that carries a receiver position fix. We need better
GGA handling in several places:

- `satpulsetool ntrip --gga` needs to upload a literal GGA sentence to test VRS
  casters.
- `nmeamsg` needs typed GGA parsing and serialisation as the first step toward
  more general fieldenc-based NMEA handling.
- The daemon should be able to synthesize GGA from decoded `gpsprot` messages
  when the receiver is not emitting NMEA GGA.
- A TCP proxy serving NMEA should be able to include synthesized GGA when the
  receiver stream does not already include a real GGA for that UTC.

This plan is for the general NMEA/GGA work. Everything after stage 1 depends on
the NMEA decode layer (#330), which provides the typed `nmeamsg` GGA and sentence
types. NMEA send pull support remains in `plan/nmea-send.md` and depends on #330 and stages 2
and 3 here.

## Stage 1: `ntripcmd --gga` option (done)

Standalone diagnostic: forward a caller-supplied GGA so the ntrip pull path can
be exercised against a VRS caster (such as PointPerfect) without a live receiver
or position synthesis. The caller pastes a real GGA captured from a receiver;
`ntripcmd` validates it and sends it verbatim.

- Add `--gga sentence`. The argument is a full NMEA GGA sentence without a line
  terminator (e.g. `$GPGGA,...*47`); a trailing CRLF from copy-paste is
  tolerated.
- Validation uses the existing `nmeamsg` syntax helpers:
  `nmeamsg.CheckSyntax(...).IsValidApprovedNMEA()` for a well-formed approved
  sentence, that the address is a `GGA` sentence, and that the `*hh` checksum
  matches `nmeamsg.Checksum`. On success it returns the wire bytes with a single
  CRLF appended.
- Mechanism: once the v1 handshake succeeds (`ICY 200 OK`), `NtripSource` sends
  the GGA to the caster as a separate write on the accepted stream (the spec
  allows a GGA after the request). `ntripcmd` constructs the source with the
  validated bytes.
- Per the spec one GGA suffices to start the stream; `ntripcmd` does not resend.

This stage depends only on existing NMEA syntax support. It proves the upload
mechanism independently of GGA synthesis. The later NMEA send plan supersedes the
static `NtripSource.GGA` field by routing `--gga` through the shared
post-handshake GGA sender; this stage describes the completed first diagnostic,
not the final writer architecture.

## Stage 2: NMEA GGA synthesis from `gpsprot`

Add a reusable `gpsprot.MsgHandler` that synthesizes GGA from decoded GPS
messages and sends typed GGA sentences to a sink. This produces candidate GGA
sentences; it does not decide whether a real receiver GGA should win.

Placement is fixed by the layering in `docs/internals/packages.md`: `nmeasyn` is a
domain-layer package (`gps/nmeasyn`), alongside `gpsprot`. It uses the `gpsprot`
domain abstraction but has no goroutines and does no logging. It imports
`gpsprot`, so it is too high for `gps/lib/`, and it is wired from `time/`, so it
cannot be `gps/internal/`. Implementation must add an entry under the `### gps/`
section of `docs/internals/packages.md`.

```go
type Phase int

const (
	PhaseImmediate Phase = iota
	PhaseEpoch
)

type Sink interface {
	Msg(m nmeamsg.GNSSTalkerIDMsg, phase Phase)
}

func New(sink Sink) *Synth // *Synth implements gpsprot.MsgHandler
```

The sink is deliberately general, not GGA-specific. `nmeamsg.GNSSTalkerIDMsg`
is the interface that every typed NMEA sentence (`nmeamsg.Sentence[F]`, e.g. GGA
and RMC) implements, and `nmeamsg.SerializeMsg` already serializes any of them.
A sink consumer recovers the concrete sentence with a type assertion (e.g.
`m.(nmeamsg.GGASentence)`). This lets the same synthesizer and sink carry RMC
(stage 5) without an interface change.

`Phase` distinguishes the timeliness contract of an emission:

- `PhaseEpoch` is emitted at the end of a navigation epoch, with that epoch's
  full metadata. It is what stage 2 emits.
- `PhaseImmediate` (stage 6) is emitted as soon as the epoch has a time and a
  position, favouring timeliness over fidelity by carrying slow-changing
  metadata (HDOP, satellites, fix quality) forward from the last completed epoch.
  It exists because `NavEpochMsg` is only prompt for protocols with an explicit
  end-of-epoch marker (e.g. UBX-NAV-EOE); without one it is deferred to the next
  cycle, which is too late for some consumers (notably RMC over a TCP proxy).

Stage 2 emits only `PhaseEpoch` GGA. The synthesizer never decides which phase a
consumer wants; the selector (stage 3) applies that policy.

Behavior:

- Embed `gpsprot.DefaultHandler` and implement only the callbacks it needs.
- Accumulate per-epoch state from `TimeMsg` and `PosGeoMsg`/`PosECEFMsg`.
- On `NavEpochMsg`, build a `nmeamsg.Sentence[nmeamsg.GGAFields]` and call
  `sink.Msg(gga, PhaseEpoch)`.

This relies on the `gpsprot` epoch ordering contract: `NavEpochMsg` is emitted
once at the end of a navigation epoch, after the time, position, and velocity
messages for that epoch have been dispatched. The synthesizer treats
`NavEpochMsg` as the end-of-epoch trigger and uses the `TimeMsg` and position
state accumulated before that callback. If a future packet processor cannot
provide that ordering, it must adapt its output before using this synthesizer.

Field sources:

- time from `TimeMsg`;
- lat/lon from `PosGeoMsg.LatLon`;
- ecef from `PosECEFMsg.Pos`, bridged to `geopos.ECEF` and converted via
  `geopos.WGS84.ECEFtoLLH`, used only if there is no lat/lon;
- fix quality (GGA field 6) from `NavEpochMsg.FixLevel`, refined by
  `Correction` and `AuxSrc`:
  - below `FixLevelCode` with `AuxSrc&AuxSrcDR != 0` -> 6 (dead reckoning);
  - below `FixLevelCode` otherwise -> 0 (no fix);
  - `FixLevelCode` -> 1 (GPS), or 2 (DGPS) when `Correction&CorrUsed != 0`;
  - `FixLevelCarrierFloat` -> 5 (RTK float);
  - `FixLevelCarrierFixed` -> 4 (RTK fixed);
- HDOP from `NavEpochMsg.DOP.Hor`;
- satellites-used from `NavEpochMsg.NumSVUsed`;
- height and differential-data fields are intentionally empty initially.

The synthesizer emits a GGA every epoch. Quality is `0` when there is no fix, so
consumers that care about validity read `gga.Fields.Quality`. Optional source
values that are not available remain unset in `GGAFields` and serialize as empty
NMEA fields.

The initial synthesizer intentionally emits only GGA quality values that
`gpsprot` can represent without inventing information. It can synthesize quality
6 from `AuxSrcDR`, but it does not synthesize manual-input/simulator qualities 7
and 8 because `FixLevelNotMeasured` does not preserve that distinction.

Tests:

- Synthetic callbacks in `TimeMsg`, `PosGeoMsg`, `NavEpochMsg` order -> expected
  GGA fields (time, position, quality, HDOP, satellites-used), pinning the
  epoch ordering contract.
- No-fix epoch -> quality 0.
- ECEF-only epoch exercises the conversion.
- Later real NMEA selection is not tested here; this package only synthesizes
  candidates.

## Stage 3: selected GGA feed

Add the shared selector that chooses the best GGA stream for consumers that
need position upload. This stage provides only the reusable selected-GGA core
needed by `plan/nmea-send.md`; it does not change the daemon, dispatcher, stream pull,
or proxy wiring.

The selector belongs in `gps/app/stream` as `GGASelector`: it owns a capacity-1
selected-GGA channel and is reusable by both `satpulsed` and the desktop GUI.
`gpsevent` must receive it through a small interface rather than depending on
the concrete stream type.

Inputs:

- receiver packets that a later caller has already accepted through the normal
  packet processor. The selector itself filters these down to valid NMEA GGA
  packets;
- synthesized GGA candidates from stage 2.

Output:

- a selected GGA packet channel, carrying `scan.Packet` values that are valid
  NMEA GGA wire packets. This is a capacity-1 latest-value channel, not a FIFO:
  the selector publishes without blocking, and when the channel already contains
  a pending GGA it drops that old packet before trying to store the newer one.

For selector input, "valid receiver GGA candidate" means checksum-valid approved
NMEA GGA that also passes the #330 typed GGA parser, including the explicit GGA
field-count check. Non-GGA packets return false and do not enter the selected
feed. A packet that the legacy parser accepts only because it is permissive must
not enter the selected-GGA output.

The selector owns the selected-output channel, implements the stage 2
`nmeasyn.Sink`, and has two input methods:

- `Packet(pkt scan.Packet)` forwards a valid receiver GGA packet immediately and
  records its UTC field when it is comparable. It returns false for non-GGA or
  invalid packets.
- `Msg(m nmeamsg.GNSSTalkerIDMsg, phase nmeasyn.Phase)` is the `nmeasyn.Sink`
  method. It ignores anything that is not a `PhaseEpoch` `nmeamsg.GGASentence`
  (the same GGA-only gate the packet path applies), then serializes the sentence,
  wraps it as a valid NMEA `scan.Packet`, and forwards it only if it is not for
  the same UTC as the last directly emitted original GGA.

The selector is single-threaded. It does not need internal locking; the later
NMEA send integration must call it from one owner goroutine, after normal packet
processing has accepted a receiver packet and from the stage 2 synthesizer sink
for synthesized GGA.

Publishing selected GGA must never block the caller. The selector's output
operation is latest-wins: try to send, and if the capacity-1 channel is full, do
a nonblocking receive to discard the pending old GGA, then try to send the newer
GGA. If the consumer races with this replacement, dropping a selected GGA is
acceptable; GGA is current-position state, not a history stream.

The GGA comparison is intentionally simple. GGA has the fixed prefix `$xxGGA,`,
so the UTC field's integer-second digits are at `pkt.Data[7:13]` for a normal
approved GGA sentence with a non-empty `hhmmss` time. Use a helper that returns
`(utc string, ok bool)` and checks both the fixed prefix and the six time
digits. `Packet` forwards every valid receiver GGA immediately, but records the
UTC only when that helper succeeds. `Msg` suppresses a synthesized
packet only when the last directly emitted original GGA and the synthesized
candidate both have comparable UTC seconds and those strings are equal; if
either time is empty or malformed, the synthesized packet is forwarded. There is
no need to decide whether one GGA is "after" another, so midnight wraparound
does not need special ordering logic. If a future use needs ordering, a
`ggaAfter` helper can compare the same fixed-position substrings with explicit
midnight handling.

Same-UTC suppression does not require a separate ordering mechanism. Synthesized
GGA is emitted only from the `NavEpochMsg` end-of-epoch callback, after the
messages that make up that epoch have been processed. Therefore any original
receiver GGA in the epoch has already reached `Packet` before `Msg`
can offer the synthesized candidate for that epoch.

Wrapping synthesized output as a `scan.Packet` likely needs `gpsreg` to
re-export the NMEA packet format, just as it already re-exports
`RTCMPacketFormat`.

Tests:

- Original GGA is forwarded immediately.
- Original GGA followed by synthesized GGA with the same UTC suppresses the
  synthesized packet.
- Original GGA with an empty UTC field is forwarded but does not suppress a
  later synthesized GGA.
- Synthesized GGA with a different UTC is forwarded.
- Synthesized output is a checksum-valid NMEA `scan.Packet`.
- Invalid receiver packets and non-GGA packets are rejected before entering the
  selected feed.
- Publishing selected GGA with a blocked consumer does not block and keeps the
  newest pending GGA.

## Stage 4: `proxy.tcp` NMEA synthesis option

Add a `synth` option to `[[proxy.tcp]]`. It applies only when
`protocol = "NMEA"`. It uses the selected-GGA core from stage 3 but is unrelated
to NMEA send.

Initially this is epoch-only, exactly as stages 2 and 3 stand: synthesized GGA
is emitted at end of epoch. Lower-latency immediate-phase synthesis for the
proxy is deferred to stage 6.

Example:

```toml
[[proxy.tcp]]
listen = "127.0.0.1:2006"
protocol = "NMEA"
synth = true
```

Validation:

- `synth = true` is valid only for TCP proxy services with `protocol = "NMEA"`.
- It affects only the packets written to proxy clients.
- Other protocol-specific proxy services keep using the raw receiver packet
  broadcast.

Architecture:

- The daemon keeps the existing raw receiver packet broadcast (`pb`) fed by
  `startScan`.
- When a TCP NMEA proxy has `synth = true`, the daemon also creates a selected
  NMEA packet channel plus a second `bcast.Bcast[scan.Packet]`.
- `proxy.Start` grows access to both broadcasts. A TCP service with
  `protocol = "NMEA"` and `synth = true` subscribes to the selected NMEA
  broadcast. Other services subscribe to the raw receiver broadcast.
- `gpsevent.Dispatcher` gets an optional NMEA selector path. It feeds valid
  original NMEA packets from `handlePacket` to the selected NMEA output and
  feeds original GGA plus synthesized GGA candidates through the stage 3
  selected-GGA core.

For proxy clients, the NMEA selector output is broader than the stage 3
selected-GGA feed:

- non-GGA original NMEA packets pass through immediately;
- original GGA packets pass through immediately and update the shared
  selected-GGA state;
- synthesized GGA packets are inserted only when stage 3 does not suppress them
  for the same UTC as an original receiver GGA.

Tests:

- Config validation accepts `synth = true` only for `protocol = "NMEA"` on
  `[[proxy.tcp]]`.
- Selected NMEA bcast passes original NMEA packets through.
- Proxy NMEA output uses the same-UTC suppression as the selected-GGA core.
- TCP NMEA proxy with `synth = true` subscribes to selected NMEA; other proxy
  services stay on the raw packet stream.
- Add a `smoketest/scenarios/proxy/tcp-synth` scenario (`.py`, `.toml.in`,
  `SCENARIOS` entry, and README list entry) for `protocol = "NMEA"` with
  `synth = true`, checking that a live TCP proxy client receives original NMEA
  and synthesized GGA fill-ins.

## Stage 5: RMC synthesis

Extend `nmeasyn.Synth` to also synthesize RMC, emitting it through the same
`sink.Msg(m, phase)` with `m` an `nmeamsg.Sentence[nmeamsg.RMCFields]`. RMC
carries time, status, lat/lon, speed, course, date and mode. Its sources are
`TimeMsg` (time and date), `PosGeoMsg` (lat/lon), `VelGeoMsg` (speed and course
over ground), and `NavEpochMsg.FixLevel` (status and mode). It omits GGA's
dilution and satellite-count metadata (HDOP, satellites), but its status and
mode derive from fix level, which is itself slow-changing metadata -- so like
GGA, an immediate-phase RMC (stage 6) must carry the last known fix level
forward rather than wait for the epoch's `NavEpochMsg`.

Because the general sink and `Phase` are already in place from stage 2, this
stage adds no interface change: it adds the RMC builder and the per-epoch
velocity accumulation. Selectors choose which sentence types they care about via
the same type assertion the GGA selector uses, so an RMC consumer ignores GGA
and vice versa.

At this point the synthesizer still emits only `PhaseEpoch`; RMC at end of epoch
is correct but not yet timely for protocols without a prompt end-of-epoch marker.
That timeliness is stage 6.

## Stage 6: immediate-phase synthesis

This is the substantial new piece. It has two halves: the synthesizer gains a
`PhaseImmediate` emission, and the proxy gets a selector that races the immediate
candidate against the epoch one with a short deadline.

Synthesizer: emit `PhaseImmediate` as soon as an epoch has a time and a position
(time being the field that must be timely), carrying the slow-changing metadata
(GGA quality/HDOP/satellites, RMC fix-level status/mode) forward from the last
completed epoch. The `PhaseEpoch` emission is unchanged. Both are emitted every
epoch; the synthesizer stays policy-free.

Proxy selector -- a deadline race that leverages EOE adaptively. When the
selector receives a `PhaseImmediate` candidate it arms a short timer (say 0.1s)
and holds the candidate.

- If the `PhaseEpoch` candidate for that epoch arrives before the timer expires,
  it is sent immediately and the held immediate one is dropped. Protocols with a
  prompt end-of-epoch marker (UBX-NAV-EOE) take this path, so the higher-fidelity
  epoch sentence is sent with no added latency.
- If the timer expires first, the immediate candidate is sent. Protocols that
  defer the epoch to the next cycle take this path, bounding latency to the
  timeout rather than a full epoch.

Invariant: at most one sentence is emitted per epoch per sentence type (GGA,
RMC). The winner is whichever of the receiver sentence, the epoch candidate, or
the timed-out immediate candidate is sent first; the others for that epoch and
type are then suppressed. The timer goroutine is not cancelled -- on wake it
simply does not send if a sentence for that epoch and type has already gone out.
A real receiver sentence arriving in the window wins the same way, since the
proxy already passes original NMEA through immediately. The race is tracked per
sentence type, so a pending immediate GGA and a pending immediate RMC are
independent.

Concurrency: make the proxy NMEA selector an actor. A single goroutine owns all
selector state: pending immediate candidates, which epoch/type has already won,
and the selected NMEA output channel. No timer goroutine and no dispatch-facing
input method publishes directly.

The dispatch-facing input methods are adapters. They turn receiver NMEA packets
and synthesized `PhaseImmediate`/`PhaseEpoch` candidates into events and send
those events to the selector goroutine. When the selector goroutine receives a
`PhaseImmediate` candidate, it holds the candidate and starts a short timer; the
timer's only job is to send a timeout event back to the selector goroutine. If a
receiver sentence or `PhaseEpoch` candidate wins first, the later timeout event
is ignored by the goroutine that owns the state.

This keeps the race serialized in one place while still using channels for the
concurrent boundary. The selected NMEA packet channel produced by this actor
feeds the broadcast used by TCP NMEA proxy services with `synth = true`.
Services without `synth = true` continue to subscribe to the raw receiver packet
broadcast.

This selector is a separate implementation from the current selected-GGA feed
used by GGA/VRS consumers. The selected-GGA feed keeps its existing latest-GGA
semantics; the proxy NMEA selector owns the stream-level race needed by stage 6.

## Open decisions

- Whether to add `synth` to Unix socket proxy services later. Stage 4 only covers
  `[[proxy.tcp]]`.
