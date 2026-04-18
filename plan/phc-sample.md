# PHC sample mode

## Introduction

This document describes a new mode of satpulsed operation in which the PTP hardware clock is left free-running and satpulsed feeds reference-clock samples to chrony (via the existing SOCK refclock) rather than disciplining the PHC itself.

### Motivation: chrony hardware timestamping

The driving reason for this mode is to let chrony use **hardware timestamping** for its NTP / PTP peers on the same NIC whose PHC is being steered by GPS. Hardware timestamping asks the NIC to stamp packet send/receive times against its PHC, and chrony then uses those stamps for its own filtering and discipline of the system clock. This only works if the PHC is **free-running** (monotonic, stable, not stepped) from chrony's point of view. The moment satpulsed steps or slews the PHC - as `phcsync` does - chrony's packet timestamps become inconsistent and hardware timestamping is effectively unusable.

The architecture we want, therefore, is:

- **PHC**: free-running, untouched by satpulsed. Chrony uses it as a stable hardware-timestamp reference for its peers.
- **satpulsed**: reads the PHC and the PPS-pulse PHC timestamps, correlates them with GPS time messages, and feeds **samples** (offset between true time and CLOCK_REALTIME) to chrony via the SOCK refclock - analogously to chrony's own PHC-with-`extpps` refclock, which performs the same construction internally rather than over SOCK.
- **chrony**: combines those samples with its hardware-timestamped NTP/PTP peers and disciplines CLOCK_REALTIME itself.

`phcsync` solves a different problem (disciplining the PHC to be the system's primary time reference) and remains the right choice when hardware timestamping is not in play.

### Motivation: don't feed chrony the output of an upstream servo

`phcsample` also avoids feeding chrony the output of an upstream servo. In the existing PHC-disciplined SOCK path, satpulse first filters, gates, and disciplines the PHC, and only then derives refclock samples from that already-controlled clock. The resulting sample stream is therefore not a direct measurement process; it is shaped by another controller's decisions and dynamics. That is not the statistically cleanest input for chrony, whose own filtering and estimation are designed to operate on measurements rather than on the residuals of a prior servo. With `phcsample`, satpulse constructs true-time-versus-system samples and leaves the estimation and control problem in one place: chrony.

### Motivation: receiver-specific sawtooth correction in the pulse-labelled path

Compared with the combination of satpulse's serial timing SOCK refclock and chrony's own PHC `extpps` mode, `phcsample` has an additional advantage: it can apply the receiver's `PulseOffset` correction when constructing the PHC-labelled sample. That allows satpulse to label each pulse with the receiver's corrected top-of-second, rather than treating the physical PPS edge as the exact second boundary. A generic chrony `extpps` setup can pair pulses with a message-based source, but it does not have this receiver-specific pulse-correction information in the PPS construction path.

### Shape of the problem

The existing `phcsync` path disciplines the PHC: it must get every decision right, because its output *is* the product - there is no downstream filter to clean up after it. Feeding chrony is a very different setting. Chrony already has extensive infrastructure for filtering outliers and averaging samples, so satpulsed should provide relatively clean samples with minimum latency and let chrony do its work.

This mode is the analogue of chrony's own PHC refclock with the `extpps` option, but with the sample construction happening in satpulsed.

## Configuration

A new boolean field on the existing `[phc]` section:

```toml
[phc]
interface = "enp1s0"
freeRunning = true
```

When `phc.freeRunning` is true, `phcsync` does not run. A new section `[phcsample]` configures this module instead. Both `[phcsync]` and `[phcsample]` may be present in a file; `phc.freeRunning` selects which one applies.

## Module interface

The module exposes a single type, `phcsample.Generator`, with two entry points:

```go
// Pulse records a calibrated pulse edge. The Generator pairs it with the
// corresponding GPS time message (via timemsg.Buffer) to build a
// (PHC, true-UTC) calibration point.
func (g *Generator) Pulse(edge ts.Event)

// Generate returns the offset (true time - sys) in seconds at the moment
// the cross-sample was taken. It returns a non-nil error when no sample
// can be produced; callers can distinguish the "still warming up" case
// from other failures using errors.Is against the sentinels below.
func (g *Generator) Generate(phc ptime.Time, sys time.Time) (offset float64, err error)

// ErrNotReady indicates the Generator does not yet have enough
// calibrated pulses to answer. This is the expected error during
// startup and after a gap in pulses or messages.
var ErrNotReady = errors.New("phcsample: not enough calibrated pulses")
```

Other error returns (e.g., `phc` too far outside the calibration window to extrapolate safely) will be defined as additional sentinels as the need arises. `ErrNotReady` is privileged because callers want to treat it as a normal transient state, not something to log loudly.

The two sides of the calculation are deliberately split so that `Generate` does purely the mapping work and the dispatcher owns the refclock plumbing:

```go
off, err := gen.Generate(cs.PHC.T, cs.Sys)
switch {
case err == nil:
    rc.Sample(cs.Sys, off, leap)
case errors.Is(err, phcsample.ErrNotReady):
    // expected during warm-up; stay quiet
default:
    lg.Warn("phcsample.Generate failed", "err", err)
}
```

`Generate` does not touch the system clock except as a value passed in, and has no leap-second state of its own. True-time labels come from `timemsg.Buffer`, which already carries the leap reference; the Generator requests them in UTC. The exact `timemsg` accessor(s) for continuous per-pulse correlation are TBD (see "Correlating pulses with messages" below).

This shape is what makes the module simulatable: the evaluation rig (see "Testing") knows ground-truth true time and can feed synthetic `sys` values, with no refclock or leap-second machinery in the sim loop.

## Inputs available

- **Pulse events** from the PHC driver: each carries a PHC timestamp of the PPS edge (`ptime.Time`), a monotonic read-time `TReadMono`, and a wall/PHC cross-sample `TReadWall` captured near the event. Both read-times are used, for different purposes - see "Read-time usage" below.
- **Time messages** from the GPS receiver, identifying the GPS second of each pulse, consumed through `timemsg.Buffer`. Each message also carries a monotonic read-time, used for correlation. In this mode we request UTC: the Generator works in UTC `time.Time` throughout.

Pulse correction (sawtooth) is a phase 2 input; see "Phasing".

## Read-time usage

Pulse events carry two distinct read-times. They are used for different purposes and must not be conflated:

- **`TReadMono` (monotonic read-time)** is used for all read-order and approximate-edge-time reasoning: pairing a pulse with its time message, backing out approximate edge monotonic time on batching hardware (CM4/CM5), and any ordering comparison between pulses and messages. The wall clock is not safe for these uses - it can step, jump across NTP adjustments, or wobble under chrony discipline.
- **`TReadWall` (wall/PHC cross-sample)** is used only for the final sample-emission path: the `sys` argument to `Generate(phc, sys)` comes from `TReadWall`, because that is where we need the best available PHC↔system-clock correspondence. The wall side of `TReadWall` is the CLOCK_REALTIME reading paired with the PHC reading in a single kernel cross-sample, which is precisely the quantity chrony wants an offset against.

Monotonic correlation cannot be done from `TReadWall`; sample emission cannot use `TReadMono`. Mixing them silently would be a subtle bug that looks correct in steady state but fails across clock steps.

## The key question

Everything the Generator does reduces to one question, asked once per `Generate` call:

> What is the true UTC time at PHC timestamp `phc`?

If we can answer that, the rest is a `time.Time` subtraction and a unit conversion to seconds.

## Types

- **PHC timestamps**: `ptime.Time` throughout. Individual PHC timestamps are meaningful only as differences against other PHC timestamps (the PHC is free-running with no fixed epoch relationship to true time), but `ptime.Time` is the type the rest of the stack already uses and introducing a new "tick count" type would fight `phctime`, `ts.Event`, and the clocksim simulator.
- **True time**: UTC `time.Time`. The Generator consumes messages in UTC via `timemsg.Buffer` and produces UTC labels; no TAI arithmetic appears inside the module.
- **Durations**: `time.Duration` for intervals (PHC-minus-PHC, UTC-minus-UTC). The caveat is that a PHC-derived `time.Duration` is not exactly real-time nanoseconds — the free-running PHC deviates from nominal by tens of ppm. The regression below is what absorbs that.
- **Offset output**: `float64` seconds, matching the refclock interface.

## Leap-second handling

Leap-second transitions are handled by **suppression**, not by TAI arithmetic inside the Generator. This is what makes the UTC-only type choice above viable: at the one place where TAI-vs-UTC would otherwise matter - the leap-second boundary - we do not try to bridge it; we suspend.

Three explicit behaviors, in all phases:

1. **The leap-second pulse is neither labelled nor emitted.** Pulses whose GPS second falls inside an insertion/deletion are not admitted to the ring, and no sample is emitted for them.
2. **No regression across a leap-second boundary.** The ring is reset (or the leap-straddling pre-transition entries are discarded) when a transition occurs, so the fit never mixes pre- and post-transition UTC labels. Allowing such a mix would inject a ~1 s jump into the fitted slope.
3. **`Generate` returns `ErrNotReady` until post-transition calibration is re-established.** Rebuilding the ring is governed by the same minimum-pulses precondition as warm-up.

Detection is the responsibility of the timemsg / dispatcher side, which already knows about leap events through `ptime.LeapSecond` and GPS almanac messages. The Generator sees the transition as an explicit signal (precise API is TBD alongside the continuous per-pulse correlation accessor) or, at minimum, through the absence of a label for the leap second. The Generator itself holds no leap-second counter and no TAI state.

## Why interpolation between two adjacent pulses is not enough

The natural first design is: hold a ring of labelled pulses, and for a cross-sample at PHC time `P_c`, linearly interpolate between the two pulses bracketing it. This is wrong in two ways:

1. **Latency.** The pulse *after* `P_c` may not arrive for up to a second. `Generate` should answer now, not later.
2. **Noise.** Even if the bracketing pair is available, GPS PPS carries tens of nanoseconds of jitter per edge, and PHC timestamps have their own sampling noise. A two-point "rate" is dominated by this pair-specific noise. Chrony can filter noise *between* samples, but it cannot repair a sample whose offset is arithmetically corrupted by bad endpoints.

The fix is to fit a local model over a *window* of recent labelled pulses - *past* pulses only, for zero latency - and evaluate the model at `P_c`. Pre-ring filtering (see "Pre-ring filtering and correlation validation") guarantees that every pulse in the ring is correctly labelled; the regression is a separate concern, operating on already-validated inputs.

## Regression over ring entries

The Generator maintains a ring of recent calibrated pulses. Each is a pair `(P_i, T_i)` of PHC timestamp (`ptime.Time`) and true UTC label (`time.Time`). Ring entries are correctly labelled by construction - the pre-ring filter is what makes this assumption safe.

Over this ring, fit a linear model mapping PHC to UTC:

```
T_utc(P) ≈ T_ref + a + b * (P - P_ref)
```

where `P_ref`, `T_ref` are a recent anchor pulse (kept to stay in integer nanosecond arithmetic), `b` is the local PHC-to-UTC rate (dimensionless, very close to 1), and `a` is a small offset in seconds. To answer `Generate`, evaluate this model at `P = phc`.

The fit uses pulses with `P_i <= phc` only. `Generate` never waits. The model extrapolates forward from the last calibrated pulse to `phc`, over a gap of at most ~1 pulse interval, using a slope estimated from a longer history.

The method of fitting is an open decision. **Likely starting point: ordinary least squares.** Candidates to evaluate against the clocksim rig include recency-weighted least squares and other simple non-robust variants. Statistically robust fits (MAD-based rejection, Theil-Sen) are explicitly not the direction: the ring is clean by construction, chrony handles residual outliers at the sample level, and robustness machinery is unnecessary weight.

Window length is likewise open, to be tuned in simulation.

## Correlating pulses with messages

To put a UTC label on each pulse the Generator needs to pair it with the time message reporting its GPS second (and eventually its sawtooth correction). This is a separate concern from the estimation and has a much weaker precision requirement: we only need to avoid confusing a pulse with the message for an adjacent second. Sub-second accuracy is more than enough.

Correlation uses `TReadMono` on both sides (see "Read-time usage"). The message's monotonic read-time here means the monotonic read time corrected by `ReadDelay`, consistent with existing `timemsg` behavior (see `GetPostTimeMessages`, which subtracts `ReadDelay` from each entry's `tRead`).

The pulse side needs an analogous correction. The pulse event's `TReadMono` records when the pulse timestamp was *read from the PHC*, not when the edge itself occurred; delivery of pulse events can be delayed by up to ~0.25 s (e.g., Raspberry Pi CM4/CM5) or by a few microseconds (typical x86). Following the pattern in `phcsync/reset.go`'s `pulseTimes`, the approximate edge monotonic time is recovered by taking the PHC delta between the pulse timestamp and `TRead.PHC`, scaling it to real-time by the locally-estimated PHC-to-real rate, and subtracting from `TRead.Mono`. This is a single approach, not two; it handles fast and slow delivery uniformly. Microsecond-level error in the recovered edge time is fine for correlation.

`TReadWall` is not used here - it is reserved for the sample-emission path.

If a pulse's message never arrives within a reasonable window, the pulse is discarded: it cannot enter the calibration ring without a label.

**The continuous per-pulse accessor on `timemsg.Buffer` is TBD** - the existing `GetPostTimeMessages` is reset-mode machinery, the wrong shape for this use. That interface deserves its own design pass.

## Pre-ring filtering and correlation validation

This is the most load-bearing part of the design. Its job is to keep wrong-second, wrong-edge, and (in phase 2) wrong-PulseOffset pulses out of the calibration ring.

It is load-bearing for three purposes:

1. **Correlation with time messages.** To assign a UTC label to a pulse we must confidently identify which GPS second it belongs to. A pulse whose PHC timestamp drifts far from the expected stride cannot be reliably paired with its message.
2. **Correlation with pulse-offset corrections (phase 2).** Same requirement: the sawtooth correction is keyed by GPS second.
3. **Dual-edge polarity selection.** In dual-edge mode only same-polarity edges are ~1 second apart. The stride check is what distinguishes timing edges from the other polarity.

**A mislabelled pulse cannot be recovered from once it enters the ring.** It is not noise that the regression can average out; it is a bad `(P_i, T_i)` pair, with a one-second or one-pulse-width error in the true-time column. Chrony's downstream filtering cannot repair it either - it operates on emitted samples, not on ring entries. So the filtering here must be strong enough that *every pulse admitted to the ring is correctly labelled*; the regression section below assumes that precondition.

The mechanism is an absolute consistency check on pulse PHC timestamps. With `edgesPerPulse` known from config (1 for single-edge, 2 for dual-edge), the check is: the PHC interval between edge n and edge n - `edgesPerPulse` should be within an absolute tolerance of 1 true second.

In single-edge mode this is consecutive intervals. In dual-edge mode it is the stride-2 interval between same-polarity edges; intervals between alternating polarities contain one pulse-width and one gap duration, which are not equal to 1 second and automatically fail the check - which is how the non-timing edge gets filtered.

When a stride interval falls outside tolerance, the offending pulse is not added to the ring. Wait for subsequent pulses to re-establish stride consistency.

The tolerance must be chosen to be tight enough that mis-correlation is impossible in practice (well under 0.5 s for message pairing; well under any plausible sawtooth magnitude for phase 2), and loose enough to admit every legitimately-timed pulse even under worst-case PHC jitter and drift. Numbers to be determined against real hardware and the clocksim rig.

## Dual-edge timing-edge selection

In dual-edge mode only one of the two edges marks the top of the GPS second. V1 makes a deliberate product choice: because satpulse is what configures the GPS receiver, it can require that the configured pulse width be far enough from 50 percent duty that edge selection is unambiguous from timing alone. The short/long pattern of consecutive different-polarity intervals then directly reveals which edge is the rising edge (top of second): the interval less than 0.5 s is the pulse width, and the edge preceding it is the timing edge. A few good edges after startup are enough.

This is a stance about the product, not a technical limitation. Prior art: `reset.go`'s `filterEdgeListsByPulseWidth` has a message-alignment fallback for ambiguous duty cycles used by `phcsync`; that approach could be adopted here as future work if it turns out some receiver cannot be configured away from ~50% duty. V1 does not carry it forward.

Explicit `gps.pulseWidth` configuration is not required; the pattern reveals itself.

## Module structure

Module name: `phcsample` (matches the config section).

Three pieces with narrow jobs:

1. **Calibration ring**: maintain recent accepted pulses, each with a `ptime.Time` PHC position and a UTC `time.Time` label. Apply the stride-`edgesPerPulse` consistency filter. In dual-edge mode, track which polarity is the timing edge and discard the other for ring purposes.

2. **Correlation**: pair each pulse with its time message. If the message does not arrive within a reasonable window, drop the pulse.

3. **Regression**: on `Generate`, fit a linear model over the window of past labelled pulses (likely plain least squares; see "Regression over ring entries") and evaluate at the requested PHC timestamp. Return the offset in seconds.

## Relationship to existing code

`freeRunning=true` introduces a **third dispatcher runtime mode**, not merely a "no controller" variant of PHC-disciplined mode. The three modes are mutually exclusive:

| Mode | PHC | `phcsync.Controller` | `phcsample.Generator` |
|------|-----|----------------------|-----------------------|
| Serial timing | absent | nil | nil |
| PHC disciplined | present | present | nil |
| PHC free-running | present | nil | present |

Implications for the dispatcher and daemon wiring:

- **`controller == nil` must stop being a synonym for serial timing.** The current code in `time/internal/gpsevent/dispatcher.go` uses the nil-controller check to route GPS time messages into `SerialSample`, and PHC event handling assumes a controller exists. Both sites must become three-way: `controller != nil` (disciplined) vs. `generator != nil` (free-running) vs. both nil (serial). The correct conditions for serial SOCK sampling and for PHC event delivery must be made explicit, not implicit in `controller == nil`.
- **PHC event path in free-running mode.** `ts.Event` pulses flow to `generator.Pulse(edge)` instead of to the phcsync controller. Cross-samples from those events go to `generator.Generate(phc, sys)`; the dispatcher owns the `rc.Sample` call.
- **Serial SOCK sampling is disabled in free-running mode**, just as it is in disciplined mode - the refclock samples come from `phcsample`, not from GPS messages. The existing `timemsg.Buffer.SetSerialSampler` wiring should be installed only in serial timing mode.
- The `ts.Event` feed and `timemsg.Buffer` already provide everything this module needs on the input side, modulo the continuous per-pulse correlation accessor on `timemsg.Buffer` (TBD).
- The `refclock.ProxyRefClock` / `sockrefclock` path already provides the output. No changes required there.
- `phcsync` itself is untouched. This module reuses none of its state machine, servo, or MAD filter.
- **Observability.** `obs.Observer` is extended with a new method that delivers successfully-generated samples (`phc`, `sys`, `offset`) from `Generate`. Failed `Generate` calls (whether `ErrNotReady` or otherwise) are not observed - only successful samples flow through this hook, analogously to how `phcsync.Sampler` delivers samples produced by phcsync. Existing observer-based logging and stats machinery then applies unchanged. The `phcsample/sim` rig uses the same hook to capture the sample stream and score it against ground-truth true time.

## What is not inside the Generator

Deliberately absent:

- **System clock work.** `sys` is an opaque parameter, used only via `time.Time` subtraction at the end of `Generate`. No cross-sample pair rate, no sys-vs-PHC rate estimate.
- **Leap-second arithmetic.** True-time labels are UTC throughout, supplied by `timemsg.Buffer`. The Generator has no `ptime.LeapSecond` field and performs no TAI-to-UTC conversion; across a leap transition it suspends rather than bridges (see "Leap-second handling").
- **TAI.** `ptime.Time` appears only for PHC timestamps, where it denotes PHC ticks, not TAI seconds.

## Phasing

**Phase 1** - the core module, with its evaluation rig:

- UTC time messages only (no sawtooth correction).
- Ring labels placed at the physical pulse edge; sub-nanosecond quantization error from the receiver flows into the regression as noise.
- Supports single-edge and dual-edge (with the 50 percent duty restriction).
- Past-only windowed linear regression (method per "Regression over ring entries"; likely plain least squares).
- **`phcsample/sim` built alongside the module** (see "Testing"): clocksim-driven rig with realistic oscillator, PPS, and fault models, used to tune the window length, stride tolerance, and regression method, and to set the minimum-pulses threshold for the first non-`ErrNotReady` return. Phase 1 is not considered complete without this rig landing and producing statistics on par with `syncsim`'s output for `phcsync`.

**Phase 2** - add sawtooth correction:

- Consume pulse-correction messages and apply `PulseOffset` to each ring label so it lands at the exact top of the second.
- The ring label becomes (UTC top-of-second) = (message UTC) + (pulse-offset adjustment).
- Pulse-offset lookup is done in UTC: `timemsg.Buffer` will be extended to accept a UTC key for pulse-correction access. **No leap-second or TAI arithmetic enters `phcsample` in phase 2** - the UTC-in / UTC-out boundary established in phase 1 is preserved.
- Expected gain: cleaner regression input, tighter chrony convergence.
- **Desirable but not required: preserve sub-nanosecond precision.** `TimeMsg.PulseOffset` is `*float64` nanoseconds (see `gps/gpsprot/msg.go`), so pulse-offset corrections can have fractional-ns resolution, and the exact top-of-second need not fall on a nanosecond boundary. Today's `timemsg.Buffer.validatePulseOffset` returns `time.Duration` (rounded to whole ns via `math.Round`), which discards that precision before phcsync ever sees it. The new UTC-keyed pulse-correction accessor planned for `timemsg.Buffer` should return `float64` ns rather than `time.Duration`, letting phcsample carry sub-ns precision through to the `float64`-seconds offset returned by `Generate`. This requires the internal representation to keep the pulse-offset correction as a separate `float64` alongside the nanosecond-resolution `time.Time` label, rather than folding it in and rounding. Implementing this is a quality-of-output improvement over phcsync, not a correctness requirement.

Phase 2 is the complete product. Phase 1 is a testable stepping stone: it works, produces correct samples, and lets chrony sync, but it does not yet deliver the receiver-specific sawtooth-correction advantage described in the motivation. It is valuable primarily because it can be brought up and evaluated against the clocksim rig independently of `PulseOffset` plumbing.

## Not in v1 (any phase)

- Pulse widths near 50 percent duty in dual-edge mode. This is a deliberate product constraint: satpulse configures the receiver, and v1 simply requires a pulse width far enough from 50% duty for timing-only edge selection (see "Dual-edge timing-edge selection"). It is not a gap to be filled.
- Taking cross-samples more frequently than pulse events. Today `ts.Event` delivers one `TReadWall` per pulse, so we emit at 1 Hz; a higher sample rate would need an independent cross-sampler.
- Holdover-style behavior during GPS outage. If pulses stop, `Generate` returns `ErrNotReady` (or an extrapolation-range error); chrony falls back to its own sources.

## Open decisions

- Specific fit (plain vs. recency-weighted least squares, or another simple non-robust variant) and window length. Drive with clocksim results.
- Absolute tolerance for the stride-`edgesPerPulse` consistency check (likely low-microseconds; measure on real hardware and in simulation).
- Minimum number of good pulses before the first non-error return (at least enough for a stable fit; probably a handful more for dual-edge polarity confidence).
- The continuous per-pulse accessor on `timemsg.Buffer`.
- Whether to log regression diagnostics (residual stddev, rejected-pulse rate, estimated local rate) as free telemetry.

## Testing

`phcsample` is designed so the main work - pulse-to-UTC calibration and PHC-to-UTC extrapolation - can be exercised in the existing `time/clocksim` simulation framework, at the same scope as `time/internal/syncsim`.

A sub-package `phcsample/sim`, paralleling `syncsim` in scope, drives the Generator:

- Reuses `clocksim.RawClock`, `clocksim.VirtualClock`, oscillator simulators (`ConstantDrift`, `WhiteFreqNoise`, etc.), and GPS simulators (jitter, sawtooth, excursions, outliers) unchanged.
- Reuses `syncsim`'s Go-level configuration types for oscillator, GPS, pulse, and fault (outages/excursions/outliers) parameters. There is no TOML loader; tests and evaluation scripts construct configs programmatically.
- The `phcsample`-specific configuration is intentionally a small surface compared to `phcsync`'s. `phcsync` is a servo with many knobs (PI gains, MAD thresholds, convergence criteria, tracking windows, reset thresholds); `phcsample` is essentially a ring size, a stride tolerance, and the choice of regression method. A handful of fields.
- Event loop analogous to `syncsim`: merged pulse / message / tick streams. On each tick (or chosen cadence) it synthesizes a `sys time.Time` and a `phc ptime.Time` from the simulator, calls `gen.Generate(phc, sys)`, and records the returned offset.
- The simulator's ground-truth true time is known, so the "true offset" is known exactly. Each emitted `(sys, offset)` is scored against it.
- Stats: offset-vs-true mean / stddev / absMax / Allan deviation, plus Generator-internal diagnostics (residual stddev, rejected-pulse rate, estimated local rate).
- No refclock, no `rc.Sample` call, no leap-second plumbing in the sim loop. The UTC labels enter via `timemsg.Buffer` exactly as in production; `sys` is a synthesized `time.Time`; the subtraction at the end of `Generate` works identically.

No `satpulsetool` subcommand is planned for this. The package is an internal evaluation artifact; it is what lets us tune the regression window, stride tolerance, and method choice against realistic, reproducible noise and fault conditions, but it does not need to be user-facing.

Unit tests cover deterministic edge cases (single-edge, dual-edge, missing pulses, gross outliers, missing messages, startup) on top of the same infrastructure.
