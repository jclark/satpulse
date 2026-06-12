# PHC-base-clock refclock mode (#257)

## Introduction

This document describes a mode of satpulsed operation in which the PTP hardware clock is left free-running (`phc.sync = false`) and satpulsed feeds chrony reference-clock samples whose timestamps are PHC readings rather than system-clock readings. Each SOCK sample's `tv` is the PHC timestamp of a PPS pulse edge, treated as TAI; the offset is the difference between GPS-derived TAI at that edge (with sawtooth correction) and the PHC reading. Chrony maps the PHC to the system clock on its side, using its multi-clock support with a `refclock SOCK` bound to the PHC.

This is one of two free-running flavors. The other (#256) has satpulse do the PHC-to-system translation internally and ship system-referenced samples; it stacks on this work, shares the machinery described here, and adds the `ntp.clock` key to select between the two. This document covers the shared machinery and the PHC-referenced flavor.

### Motivation: chrony hardware timestamping

The driving reason for the free-running modes is to let chrony use **hardware timestamping** for its NTP / PTP peers on the same NIC whose PHC is fed by GPS. Hardware timestamping asks the NIC to stamp packet send/receive times against its PHC, and chrony then uses those stamps for its own filtering and discipline. This only works if satpulsed never steps or slews the PHC, as `phcsync` does.

The PHC-base-clock flavor is the optimal arrangement when chrony itself disciplines the PHC (multi-clock): GPS-derived offsets are never re-expressed in the system-clock domain inside satpulse. Chrony performs the PHC-to-system translation in the same domain its hardware-timestamped packet measurements already live in, so the cross-domain noise that an internal translation would incur is simply absent. The sample stream has the same shape that ptp4l produces when used with a SOCK refclock, but constructed from the GPS receiver instead of a PTP network.

### Motivation: don't feed chrony the output of an upstream servo

This mode also avoids feeding chrony the output of an upstream servo. In the PHC-disciplined SOCK path, satpulse first filters, gates, and disciplines the PHC, and only then derives refclock samples from that already-controlled clock. The resulting sample stream is shaped by another controller's decisions and dynamics — not the statistically cleanest input for chrony, whose own filtering and estimation are designed to operate on measurements. Here, every sample is a direct measurement: a PHC timestamp and the GPS receiver's word for what TAI time that edge marks.

### Motivation: receiver-specific sawtooth correction

Compared with chrony's own PHC-with-`extpps` refclock, satpulse can apply the receiver's `PulseOffset` correction when constructing each sample, labelling each pulse with the receiver's corrected top-of-second rather than treating the physical PPS edge as the exact second boundary. A generic chrony `extpps` setup does not have this receiver-specific information in the PPS construction path.

### Motivation: faster startup

Samples begin flowing within a few seconds of startup — `MinMsgSpan` of message history plus enough edges for the admissibility filter (3–5 s with defaults). `phcsync`, by contrast, must step the PHC into reset, pass through converging mode, and reach tracking before SOCK samples are emitted. This is a measurement pipeline, not a servo: "ready" is just "enough observations to label an edge".

### Shape of the problem

The `phcsync` path must get every decision right, because its output *is* the product. Feeding chrony is a different setting: chrony has extensive infrastructure for filtering outliers and averaging samples, so satpulsed should provide relatively clean samples with minimum latency and let chrony do its work. The one thing satpulse must never do is emit a *mislabelled* sample — one whose offset is off by a whole second — because that is not noise chrony can average out. The admissibility filtering below exists to make mislabelling impossible in practice.

## Configuration

```toml
[phc]
interface = "enp1s0"
sync = false

[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

`phc.sync` defaults to `true`. When it is `false`, `phcsync` does not run and `[sample.phc]` tunes this module instead; `[sync]` is ignored. `phc.sync = false` requires `ntp.sock.path` (the samples exist only for the SOCK refclock) and excludes `ntp.shm` (the SHM protocol carries system-clock timestamps by definition). With #256 stacked on top, `ntp.clock` selects between the two free-running flavors.

## Inputs

- **Pulse events** from the PHC driver: each carries a PHC timestamp of the PPS edge (`ptime.Time`) and a monotonic read-time sample `TReadMono`. The wall/PHC cross-sample (`TReadWall`) that the disciplined and system-referenced modes consume is **not used** in this mode: satpulse never relates the PHC to the system clock here.
- **Time messages** from the GPS receiver, consumed through `timemsg.Buffer`, which delivers per-second TAI labels (converting from UTC via the leap-second state when the message itself is UTC-only). Each delivery carries a monotonic read-time with `ReadDelay` already subtracted.
- **PrePulse pulse-correction messages** supplying the sawtooth `PulseOffset` for each second, looked up by TAI label.

## The per-edge model

Everything reduces to one question, asked once per admitted pulse edge:

> Which TAI second does this edge mark, and how far is the PHC reading from it?

Each admitted edge becomes its own sample, emitted immediately:

```
offset = (label - correction) - edge_phc        # in seconds, as float64
tv     = edge_phc                               # the SOCK sample timestamp
```

where `label` is the integer TAI second the edge marks, and `correction` is the receiver's `PulseOffset` (true-time nanoseconds satisfying `true_second = pulse_time + correction`, so true time *at the physical edge* is `label - correction`). The potentially huge TAI-to-PHC distance is computed as an int64 `time.Duration` and converted to float64 seconds only at the end; the SOCK wire carries the offset as a double anyway, so this loses nothing.

There is no regression over the samples and no extrapolation: the PHC timestamp *is* the measurement instant, and chrony's filtering operates on the per-edge stream. This is the structural simplification relative to #256, which must fit a PHC-to-true-time model in order to evaluate it at a cross-sample taken away from the edges.

## Labelling pulse edges with TAI

To label an edge, the module maps the edge's monotonic time to the TAI second it falls inside. The precision required is sub-second: only the identity of the second matters, since the precise timing comes from the PHC timestamp itself.

The conceptual model is a continuous mapping, not a pairwise edge/message correlation. The `MsgTAITime` callback stream gives a running measurement of (monotonic `tRead` → TAI), sampled once per eligible message. Fitting a linear model over a window of recent pairs yields a mapping evaluable at any monotonic time — including the recovered edge-monotonic time. This formulation is resilient to missing or late messages, unequal message and pulse rates, and per-message `tRead` jitter, none of which break a fit the way they break strict pairing.

The edge event's `TReadMono` records when the edge timestamp was *read from the PHC*, not when the edge occurred; delivery can be delayed by up to ~0.25 s on batching hardware (Raspberry Pi CM4/CM5). The approximate edge monotonic time is recovered by taking the PHC delta between the edge timestamp and the read sample's PHC reading, scaling it to real time by the median pulse interval, and subtracting from the monotonic read time.

The fit (the `wallClock` type) evaluates at the edge's recovered monotonic time; the prediction is rounded to the nearest second to produce the label. If the unrounded prediction lies further from an integer second than `EdgeSecondTolerance`, the edge is not emitted — rounding would be ambiguous.

### wallClock

`wallClock` maintains a sliding window (`MsgWindow`) of `(tRead - ExpectedDelay, ref)` pairs and fits plain OLS. Its Y axis is the domain-neutral `ntime.Time` (#258); in this mode TAI flows through it. The constant pulse-to-message delay is absorbed into the intercept; `ExpectedDelay` is only a centering shift. Validation gates, each returning a descriptive error:

- **Not enough data** (fewer than 2 points, or span below `MinMsgSpan`) → `ErrNotReady`, the quiet warm-up state.
- **Stale query** (more than `MaxMsgGap` past the newest observation) → samples stop until messages resume.
- **Backward coverage** (query too far before the oldest observation) → `ErrNotReady`; prevents labelling stale pulse history after recovery from a message gap.
- **Clock rate mismatch** (fitted slope vs unity beyond `ClockRateLimit`) and **message timing scatter** (median absolute residual beyond `MsgTimingVariation`) → safety gates against pathologically broken streams.

Because the fit extrapolates forward within `MaxMsgGap`, in steady state an edge is labellable — and its sample emitted — at the moment the pulse arrives, without waiting for the messages that follow it.

### MsgTAITimer interface

`timemsg.Buffer` gains a TAI sink alongside the existing (unchanged) `MsgUTCTimer` used by serial timing mode:

```go
type MsgTAITimer interface {
    MsgTAITime(tai ptime.Time, tRead time.Time, leap ptime.LeapSecondKind)
}
```

Called once per eligible ms-rounded TAI second, monotonically increasing across message types. PrePulse messages are excluded: they arrive before the pulse and carry the time of the *upcoming* second, so pairing them with their read time would feed misaligned points into the fit. The `leap` value is the pending-leap state; it rides along to chrony in the SOCK sample's leap field.

Pulse corrections are a separate path: `GetTAIPulseCorrection(refTime ptime.Time) (float64, bool)` returns the PrePulse `PulseOffset` for a TAI top-of-second as unrounded float64 nanoseconds. PostPulse correction messages are not consulted — in PostPulse mode the correction for pulse N arrives after pulse N's edge event, which would require a deferred-admission pipeline (future work, shared with #256).

## Pre-admission filtering

The most load-bearing part of the design: a mislabelled edge cannot be recovered from once emitted — it is off by a whole second, and chrony's filtering operates on samples, not on their construction. Filtering must be strong enough that every emitted edge is correctly labelled.

The mechanism is a relative-variation check on edge PHC timestamps (`consistentEdges`): compute the stride-`edgesPerPulse` intervals between successive same-polarity edges in the window (`PulseWindow` pulses), take their median, and flag intervals deviating from the median by more than `PulseVariation` PPB. An edge adjacent to two flagged intervals — or to a single flagged interval at a stream boundary — is rejected. Rejected entries are zeroed in place rather than removed, preserving positional alignment between the two polarity streams in dual-edge mode.

A **PHC discontinuity detector** (`firstDiscontinuity`) handles another process stepping the PHC (e.g. chrony disciplining it, which is precisely the multi-clock arrangement, or an operator running `phc_ctl`): an interval whose absolute deviation from the median is at least `DiscontinuityThreshold` (default 1 ms), with the neighbouring intervals each below that threshold, cuts the stream; admissibility restarts from the post-discontinuity suffix. This covers forward gaps (missing pulses), forward steps, and backward steps. The threshold is chosen to match the step/don't-step threshold of the external disciplining system.

### Dual-edge timing-edge selection

In dual-edge mode only one polarity marks the top of the second. Because satpulse configures the receiver, the pulse width is required to be far enough from 50% duty that selection is unambiguous from timing alone: the average cross-polarity gap, scaled to real time, is either clearly less than `PulseWidthDetectLimit` (the following polarity is the timing edge) or clearly more (the leading one is). Near-50% duty is rejected as a product rule, not handled.

## Emission

`phcsample.Generator` buffers edges cheaply on `Pulse` and runs admissibility + labelling lazily, on each pulse and each `MsgTAITime` delivery. Newly labellable edges are emitted oldest-first through a one-method sink:

```go
type Sampler interface {
    PHCSample(phc ntime.Time, offset float64, leap ptime.LeapSecondKind)
}
```

The dispatcher implements `Sampler` and forwards to the refclock. Consume semantics: each edge is emitted at most once (tracked by the newest emitted PHC timestamp); an edge rejected by `EdgeSecondTolerance` remains retryable on later calls until a newer edge is emitted, after which it is skipped permanently; a stale wallClock stops iteration for the remaining (newer) edges without discarding them. After warm-up the buffered backlog (~`PulseWindow` edges) emits in one burst, giving chrony immediate history.

`Observer.NTPSample` does not fire in this mode: it reports sys-vs-true offsets, and satpulse never computes one here.

## Leap seconds

TAI is continuous through a leap second, so the labelling fit is undisturbed and no suspension or window reset is needed — the machinery for that simply does not exist in this mode. The pending-leap flag from the message stream is forwarded to chrony in each sample, which is what chrony needs to handle the UTC side itself.

## Relationship to existing code

`phc.sync = false` introduces a third dispatcher runtime mode, mutually exclusive with the others:

| Mode | PHC | `phcsync.Controller` | `phcsample.Generator` |
|------|-----|----------------------|-----------------------|
| Serial timing | absent | nil | nil |
| PHC disciplined | present | present | nil |
| PHC free-running | present | nil | present |

- The dispatcher delivers edges to `generator.Pulse(ts, tRead)` and installs the `MsgTAITimer` sink; delivery is indirected through the dispatcher so that era transitions — which replace the Generator with a fresh instance (`NewInstance`), since pre-pause edges reference a possibly-stepped PHC — cannot leave the buffer pointing at a discarded instance.
- Serial SOCK sampling (`MsgUTCTimer` → dispatcher) is installed only in serial timing mode; in this mode the refclock samples come exclusively from `phcsample`.
- The refclock path carries `ntime.Time` timestamps end to end (#258); `sockrefclock` writes the PHC reading into the SOCK `tv` unchanged.
- `phcsync` is untouched. The module reuses none of its state machine, servo, or filters.
- ptp4l grandmaster wiring is skipped in free-running mode: the controller owns the ptp4l worker's lifecycle, and without a controller there is nothing to drive it.

## Testing

- Unit tests cover the wallClock gates, the admissibility filter (including discontinuities), dual-edge selection, per-edge emission semantics (warm-up burst, no re-emission, label-tolerance rejection), and sawtooth sign/magnitude.
- Because the PHC is free-running and there is no estimation loop inside satpulse, the same recorded event log deterministically produces the same samples; event-log replay against a recorded baseline is the planned regression vehicle, in place of the simulation rig that scores #256's regression error.
- End-to-end validation against multi-clock chrony on real hardware is the binary go/no-go gate before this mode ships.

## Open items

- Operator-facing rejection logging (which TOML parameter caused samples to stop); the design sketched for #256 applies to the shared pipeline and should land with or after it.
- PostPulse pulse-correction support (deferred admission), shared with #256.
