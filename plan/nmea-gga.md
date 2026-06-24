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
types. VRS pull support remains in `plan/vrs.md` and depends on #330 and stages 2
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
mechanism independently of GGA synthesis. The later VRS plan supersedes the
static `NtripSource.GGA` field by routing `--gga` through the shared
post-handshake GGA sender; this stage describes the completed first diagnostic,
not the final writer architecture.

## Stage 2: NMEA GGA synthesis from `gpsprot`

Add a reusable `gpsprot.MsgHandler` that synthesizes GGA from decoded GPS
messages and sends typed GGA sentences to a sink. This produces candidate GGA
sentences; it does not decide whether a real receiver GGA should win.

Placement is fixed by the layering in `docs/internals.md`: `nmeasyn` is a
domain-layer package (`gps/nmeasyn`), alongside `gpsprot`. It uses the `gpsprot`
domain abstraction but has no goroutines and does no logging. It imports
`gpsprot`, so it is too high for `gps/lib/`, and it is wired from `time/`, so it
cannot be `gps/internal/`. Implementation must add an entry under the `### gps/`
section of `docs/internals.md`.

```go
type Sink interface {
	GGASentence(gga nmeamsg.Sentence[nmeamsg.GGAFields])
}

func New(sink Sink) *Synth // *Synth implements gpsprot.MsgHandler
```

Behavior:

- Embed `gpsprot.DefaultHandler` and implement only the callbacks it needs.
- Accumulate per-epoch state from `TimeMsg` and `PosGeoMsg`/`PosECEFMsg`.
- On `NavEpochMsg`, build a `nmeamsg.Sentence[nmeamsg.GGAFields]` and call
  `sink.GGASentence(gga)`.

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
needed by `plan/vrs.md`; it does not change the daemon, dispatcher, stream pull,
or proxy wiring.

The selector belongs in `time/internal/gpsevent` as `GGASelector`: it owns a
capacity-1 selected-GGA channel and sits at the application-layer join point
where receiver packets and synthesized GPS events will later meet.

Inputs:

- original receiver GGA packets that a later caller has already accepted
  through the normal NMEA packet processor;
- synthesized GGA candidates from stage 2.

Output:

- a selected GGA packet channel, carrying `scan.Packet` values that are valid
  NMEA GGA wire packets. This is a capacity-1 latest-value channel, not a FIFO:
  the selector publishes without blocking, and when the channel already contains
  a pending GGA it drops that old packet before trying to store the newer one.

For selector input, "valid original GGA candidate" means checksum-valid approved
NMEA GGA that also passes the #330 typed GGA parser, including the explicit
GGA field-count check. A packet that the legacy parser accepts only because it is
permissive must not enter the selected-GGA output.

The selector owns the selected-output channel and has two input methods:

- `OriginalGGA(pkt scan.Packet)` forwards a valid original GGA packet
  immediately and records its UTC field when it is comparable.
- `GGASentence(gga nmeamsg.Sentence[nmeamsg.GGAFields])` serializes the sentence,
  wraps it as a valid NMEA `scan.Packet`, and forwards it only if it is not for
  the same UTC as the last directly emitted original GGA.

The selector is single-threaded. It does not need internal locking; the later
VRS integration must call it from one owner goroutine, after normal packet
processing has accepted an original GGA and from the stage 2 synthesizer sink
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
digits. `OriginalGGA` forwards every valid original GGA immediately, but records
the UTC only when that helper succeeds. `GGASentence` suppresses a synthesized
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
receiver GGA in the epoch has already reached `OriginalGGA` before `GGASentence`
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
- Invalid original GGA candidates are rejected before entering the selected feed.
- Publishing selected GGA with a blocked consumer does not block and keeps the
  newest pending GGA.

## Stage 4: `proxy.tcp` NMEA synthesis option

Add a `synth` option to `[[proxy.tcp]]`. It applies only when
`protocol = "NMEA"`. It uses the selected-GGA core from stage 3 but is unrelated
to VRS.

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

## Open decisions

- Whether to add `synth` to Unix socket proxy services later. Stage 4 only covers
  `[[proxy.tcp]]`.
