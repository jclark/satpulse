# NMEA GGA synthesis and selection (#329)

This plan records the implemented protocol-neutral GGA synthesizer and the
selected-GGA feed that prefers a receiver sentence over a synthesized sentence
for the same UTC. The typed NMEA sentence layer was implemented separately by
#330, and the Ntrip client-GGA consumer is recorded in [vrs.md](vrs.md) (#325).

The unfinished applications and extensions have their own active plans:

- [nmea-gga-tcp.md](../nmea-gga-tcp.md) (#329) adds synthesized GGA to an NMEA
  TCP service.
- [nmea-rmc-synth.md](../nmea-rmc-synth.md) (#429) adds RMC and prompt GGA/RMC
  synthesis.

## NMEA GGA synthesis from `gpsprot`

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
in the follow-on [RMC synthesis plan](../nmea-rmc-synth.md) without an interface change.

`Phase` distinguishes the timeliness contract of an emission:

- `PhaseEpoch` is emitted at the end of a navigation epoch, with that epoch's
  full metadata. It is what this implementation emits.
- `PhaseImmediate` (planned in [nmea-rmc-synth.md](../nmea-rmc-synth.md)) is emitted as soon as the epoch has a time and a
  position, favouring timeliness over fidelity by carrying slow-changing
  metadata (HDOP, satellites, fix quality) forward from the last completed epoch.
  It exists because `NavEpochMsg` is only prompt for protocols with an explicit
  end-of-epoch marker (e.g. UBX-NAV-EOE); without one it is deferred to the next
  cycle, which is too late for some consumers (notably RMC over a TCP proxy).

The implemented synthesizer emits only `PhaseEpoch` GGA. The synthesizer never
decides which phase a consumer wants; the selector below applies that policy.

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

## Selected GGA feed

Add the shared selector that chooses the best GGA stream for consumers that
need position upload. This is the reusable selected-GGA core used by the Ntrip
client-GGA integration recorded in [vrs.md](vrs.md).

The selector belongs in `gps/app/stream` as `GGASelector`: it owns a capacity-1
selected-GGA channel and is reusable by both `satpulsed` and the desktop GUI.
`gpsevent` must receive it through a small interface rather than depending on
the concrete stream type.

Inputs:

- receiver packets that a later caller has already accepted through the normal
  packet processor. The selector itself filters these down to valid NMEA GGA
  packets;
- synthesized GGA candidates from the synthesizer above.

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

The selector owns the selected-output channel, implements the
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
processing has accepted a receiver packet and from the synthesizer sink
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
