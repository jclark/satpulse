# RMC and prompt NMEA synthesis (#429)

## Motivation

`gps/nmeasyn` currently produces GGA only when a navigation epoch completes.
Extend it to produce RMC as well. RMC is a time-bearing sentence used by timing
consumers, so waiting for `NavEpochMsg` is not sufficient on protocols where
the end of an epoch is recognized only when the following epoch begins.

The synthesizer should therefore offer a prompt sentence as soon as the current
epoch has time and position, while continuing to offer an epoch-complete
sentence with the current epoch's full metadata. The prompt mechanism applies
to GGA as well as RMC because it falls naturally out of the same state and
emission model.

This work is independent of, but related to, the synthesized GGA TCP service in
[nmea-gga-tcp.md](nmea-gga-tcp.md) (#329). The synthesizer and selection policy
are transport-independent; an NMEA TCP service is one consumer of their output.

## Existing foundation

The implemented design in [archive/nmea-gga.md](archive/nmea-gga.md) already
provides the general interfaces needed here:

```go
type Phase int

const (
	PhaseImmediate Phase = iota
	PhaseEpoch
)

type Sink interface {
	Msg(m nmeamsg.GNSSTalkerIDMsg, phase Phase)
}
```

`nmeamsg.GNSSTalkerIDMsg` admits both typed GGA and RMC sentences. `PhaseEpoch`
is implemented for GGA; `PhaseImmediate` exists but is not emitted.

## RMC synthesis

Extend `nmeasyn.Synth` to synthesize RMC and emit it through the existing
`Sink.Msg` interface.

RMC fields come from protocol-neutral messages:

- time and date from `TimeMsg`;
- latitude and longitude from `PosGeoMsg`, or converted from `PosECEFMsg` when
  geographic position is absent;
- speed and course over ground from `VelGeoMsg`;
- status and mode from `NavEpochMsg.FixLevel`.

Missing optional source values remain empty rather than being invented. The
mapping from fix level to RMC status and mode must be explicit and tested.

At navigation-epoch completion, emit a `PhaseEpoch` RMC containing all current
fields accumulated for that epoch. GGA `PhaseEpoch` behavior remains unchanged.

## Prompt synthesis

Emit `PhaseImmediate` GGA and RMC as soon as an epoch has both time and
position. Time is the field whose delivery must not wait for a delayed epoch
boundary.

Fields that normally arrive only with `NavEpochMsg` are slow-changing state.
The prompt form carries their last values from the preceding completed epoch:

- GGA quality, HDOP, and satellites used;
- RMC fix-level-derived status and mode.

Speed and course are included when current-epoch velocity has arrived before
the prompt emission; otherwise they remain empty. The later `PhaseEpoch` RMC
contains the complete current-epoch values.

Both phases are emitted every epoch. `nmeasyn` reports candidates and remains
free of consumer policy.

## Timely sentence selection

Consumers that expose a single NMEA stream need at most one sentence of each
type per epoch. Add a transport-independent selected-NMEA actor under
`gps/app/stream` that serializes the race among:

- a real receiver sentence;
- a synthesized `PhaseImmediate` candidate;
- a synthesized `PhaseEpoch` candidate;
- expiry of a short deadline after the prompt candidate.

When a prompt candidate arrives, hold it and arm a short timer, initially
0.1 seconds:

- a real receiver sentence wins immediately;
- a `PhaseEpoch` candidate arriving before the deadline wins immediately;
- otherwise the deadline releases the prompt candidate.

This adapts to explicit end-of-epoch messages such as UBX-NAV-EOE: when the
complete candidate is prompt, it adds no delay; when epoch completion would be
deferred to the next cycle, output latency is bounded by the deadline.

The invariant is at most one emitted sentence per epoch and sentence type. GGA
and RMC races are independent.

One goroutine owns pending candidates, winners, and the selected output channel.
Dispatch-facing methods and timer callbacks only enqueue events. A timer need
not be cancelled after another candidate wins; its later event is ignored by
the owner goroutine.

The existing latest-value `GGASelector` used by Ntrip client-GGA upload remains
separate. That consumer wants the newest usable position rather than a complete
ordered NMEA stream.

## Testing

RMC synthesis:

- time, date, position, velocity, status, and mode map to expected RMC fields;
- ECEF-only position exercises coordinate conversion;
- missing optional fields remain empty;
- the serialized result is checksum-valid NMEA.

Prompt synthesis:

- GGA and RMC are emitted after time and position without waiting for
  `NavEpochMsg`;
- prompt output carries the preceding epoch's quality and status metadata;
- current velocity is used when already available and omitted otherwise;
- `PhaseEpoch` still contains current complete metadata.

Selection:

- a receiver sentence beats both synthesized phases;
- a prompt `PhaseEpoch` beats the held immediate candidate;
- deadline expiry emits the immediate candidate when epoch completion is late;
- late candidates and timer events do not produce duplicates;
- GGA and RMC are tracked independently;
- the actor remains nonblocking under concurrent dispatcher and timer input.
