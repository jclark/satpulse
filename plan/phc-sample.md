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

### Motivation: faster startup

`phcsample` begins emitting samples within a few seconds of startup — roughly `MinMsgSpan` + the handful of labelled edges the PHC calibration window needs (around 3–5 s with defaults). `phcsync`, by contrast, must first step the PHC into reset, then pass through converging mode, and only reaches tracking — the point at which SOCK samples are emitted — after the servo's convergence criteria are met, which takes significantly longer. Because `phcsample` is a measurement pipeline rather than a servo, "ready" is just "enough observations for a stable fit"; there is no closed-loop settling to wait for. This is an operational win for any deployment where the server is expected to be providing good time to clients shortly after boot.

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

When `phc.freeRunning` is true, `phcsync` does not run and `[phcsample]` configures this module instead; otherwise `[sync]` applies and `[phcsample]` is ignored. Both `[sync]` and `[phcsample]` may be present in a file; `phc.freeRunning` selects which one applies.

## Module interface

The module exposes a single type, `phcsample.Generator`, with a small surface:

```go
// PulseEdge carries the PHC-side data the Generator needs from a
// single edge event. The dispatcher adapts ts.Event into this,
// dropping Kind / ResumeFunc / TReadWall; ts.Event.Kind filtering
// (Pause, Resume) and stale-era filtering are dispatcher
// responsibilities.
type PulseEdge struct {
    Timestamp phctime.Time   // PHC timestamp of the pulse edge
    TRead     phctime.Sample // monotonic-bearing PHC/system read sample
}

// IsZero reports whether edge is the zero value. Used inside phcWindow
// to mark entries that pre-admission filtering has rejected, without
// disturbing positional alignment across dual-edge polarity streams.
func (edge PulseEdge) IsZero() bool

// Pulse records a pulse-edge. The Generator labels it with a UTC
// second via the labelling regression over MsgUTCTime inputs and, if
// the edge passes pre-admission filtering, adds it as a (PHC, UTC)
// entry to the PHC calibration window.
func (g *Generator) Pulse(edge PulseEdge)

// Generate returns the offset (true time - sys) in seconds at the moment
// the cross-sample was taken. It returns a non-nil error when no sample
// can be produced; callers can distinguish the "still warming up" case
// from other failures using errors.Is against the sentinels below.
func (g *Generator) Generate(phc ptime.Time, sys time.Time) (offset float64, err error)

// ErrNotReady indicates phcsample is not yet able to produce a sample.
// It is returned when the wallClock window has not warmed up (too few
// messages, or messages do not yet span enough real time) or when the
// phcWindow has not yet accumulated enough labelled edges. This is the
// expected error during startup and after a gap in edges or messages;
// callers treat it as a quiet transient state rather than something to
// log loudly.
var ErrNotReady = errors.New("phcsample: not ready")
```

Other error returns (e.g., `phc` too far outside the PHC calibration window to extrapolate safely) will be defined as additional sentinels as the need arises. `ErrNotReady` is privileged because callers want to treat it as a normal transient state, not something to log loudly.

In addition, the Generator implements the `MsgUTCTimer` sink on `timemsg.Buffer` — `MsgUTCTime` in phase 1, adding `Leap` in phase 2. See the "MsgUTCTimer interface" subsection for the method signatures.

**No lifecycle methods.** `Generator` has no `Pause`, `Resume`, or `Reset`. On `ts.PauseEvent` the dispatcher drops the current Generator; on the next edge event after the new era, it constructs a fresh `phcsample.NewGenerator(...)`. This works because the Generator holds nothing worth preserving across a pause: the wallClock regression re-warms quickly from subsequent `MsgUTCTime` calls, and any pre-pause PHC calibration window entries are invalid anyway (the PHC may have stepped during pause). Stale-era filtering stays in the dispatcher, same as for phcsync. Contrast with `phcsync.Controller.Pause()`, which preserves servo state (`freq`, `estimatedFreq`), the persistent tracking sample, and the PTP grandmaster reference; phcsample has no analogues to preserve.

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

`Generate` does not touch the system clock except as a value passed in, and has no leap-second state of its own. True-time labels come from `timemsg.Buffer`, which already carries the leap reference; the Generator requests them in UTC. The sink interface the Generator implements on `timemsg.Buffer` is described in "Labelling pulse-edges with UTC" below.

This shape is what makes the module simulatable: the evaluation rig (see "Testing") knows ground-truth true time and can feed synthetic `sys` values, with no refclock or leap-second machinery in the sim loop.

## Sample emission

On each successful `Generate`, the Generator reports the resulting sample through a one-method observability interface:

```go
// Sampler receives samples emitted by Generator.Generate.
// Only successful calls are reported; ErrNotReady and other
// errors produce no sample.
type Sampler interface {
    NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time)
}
```

`Generator` takes a `Sampler` at construction. `Generate` calls `NTPSample` exactly once per successful return; `ErrNotReady` and other error returns are silent. `sys` is the CLOCK_REALTIME reading paired with the PHC cross-sample, `offset` is the seconds offset returned by `Generate` (true time minus `sys`), `leap` is the leap-second state passed to the refclock, and `phc` is the PHC timestamp at which the offset was computed.

The method is named `NTPSample` rather than `Sample` for two reasons:

1. **No name collision.** `phcsync.Sampler` already declares `Sample(phcsync.Sample)`. Go does not permit two embedded interfaces in `obs.Observer` to share a method name with different signatures, so this interface must pick a different method name.
2. **Names the consumer.** These samples are what chrony (the NTP side) ultimately receives via the SOCK refclock; `NTPSample` says so on the tin.

**Why separate arguments and not a shared struct.** Passing values individually rather than packaging them into a named `Sample` struct means no single package has to own the type. `phcsample.Sampler` declares `NTPSample(time.Time, float64, ptime.LeapSecondKind, ptime.Time)` here; `obs.Observer` declares the same method directly alongside its other methods. By Go's structural typing any `obs.Observer` also satisfies `phcsample.Sampler` with no import relationship between the two packages. This is what lets the Dispatcher — which already holds an `obs.Observer` — emit `NTPSample(sys, offset, leap, phc)` from the serial-timing and PHC-disciplined paths without importing `phcsample`, while the Generator calls the same method on its own `phcsample.Sampler` argument. A shared struct type, by contrast, would force one package to own it and the other to import it, pushing the dependency in a direction that crosses the producer/aggregator boundary.

In the serial-timing path `phc` is `ptime.Time{}` (no PHC in play); in the PHC-disciplined and free-running paths it carries the PHC timestamp the offset was computed at. Observers that want to distinguish can check `phc.IsZero()`.

The `phcsample/sim` rig installs its own `Sampler` implementation to capture the stream and score each sample against ground-truth true time, with no refclock plumbing in the sim loop.

## Inputs available

- **Pulse events** from the PHC driver: each carries a PHC timestamp of the PPS edge (`ptime.Time`), a monotonic read-time `TReadMono`, and a wall/PHC cross-sample `TReadWall` captured near the event. Both read-times are used, for different purposes - see "Read-time usage" below.
- **Time messages** from the GPS receiver, identifying the GPS second of each edge, consumed through `timemsg.Buffer`. Each message also carries a monotonic read-time, used as the independent variable of the labelling regression. In this mode we request UTC: the Generator works in UTC `time.Time` throughout.

Pulse correction (sawtooth) is a phase 2 input; see "Phasing".

## Read-time usage

Pulse events carry two distinct read-times. They are used for different purposes and must not be conflated:

- **`TReadMono` (monotonic read-time)** is used for all read-order and approximate-edge-time reasoning: evaluating the labelling regression at each edge, backing out approximate edge monotonic time on batching hardware (CM4/CM5), and any ordering comparison between edges and messages. The wall clock is not safe for these uses - it can step, jump across NTP adjustments, or wobble under chrony discipline.
- **`TReadWall` (wall/PHC cross-sample)** is used only for the final sample-emission path: the `sys` argument to `Generate(phc, sys)` comes from `TReadWall`, because that is where we need the best available PHC↔system-clock correspondence. The wall side of `TReadWall` is the CLOCK_REALTIME reading paired with the PHC reading in a single kernel cross-sample, which is precisely the quantity chrony wants an offset against.

Labelling cannot be done from `TReadWall`; sample emission cannot use `TReadMono`. Mixing them silently would be a subtle bug that looks correct in steady state but fails across clock steps.

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

Suppression is a **phase 2** addition (see "Phasing"). Phase 1 is leap-unaware: `MsgUTCTimer` has no `Leap` method and the regression windows are not reset on a transition, so output will be wrong across a leap second and the regressions will take time to re-converge afterwards. This is an accepted phase-1 gap: another leap second may never occur.

Three explicit behaviors (phase 2):

1. **The leap-second edge is neither labelled nor emitted.** Edges whose GPS second falls inside an insertion/deletion are not admitted to the window, and no sample is emitted for them.
2. **No regression across a leap-second boundary.** The PHC calibration window is reset (or the leap-straddling pre-transition entries are discarded) when a transition occurs, so the fit never mixes pre- and post-transition UTC labels. Allowing such a mix would inject a ~1 s jump into the fitted slope.
3. **`Generate` returns `ErrNotReady` until post-transition PHC calibration is re-established.** Rebuilding the window is governed by the same minimum-edges precondition as warm-up.

Detection is the responsibility of the timemsg / dispatcher side, which already knows about leap events through `ptime.LeapSecond` and GPS almanac messages. The Generator sees the transition as an explicit `Leap(kind ptime.LeapSecondKind)` callback on the timemsg sink (see "Labelling pulse-edges with UTC") and uses it to reset both the labelling regression window and the PHC calibration window. The Generator itself holds no leap-second counter and no TAI state.

## Labelling pulse-edges with UTC

To put a UTC label on each pulse-edge the Generator maps the edge's monotonic time to the UTC second it falls inside. The precision required is sub-second: we only need to identify which GPS second the edge belongs to.

The conceptual model is a continuous mapping, not a pairwise edge/message correlation. The `MsgUTCTimer` callback stream gives us a running measurement of (monotonic `tRead` → UTC) sampled once per eligible message. Fitting a linear model over a window of recent `(tRead, utc)` pairs yields `utc ≈ a + b * tRead`, evaluable at any monotonic time — including the recovered edge-monotonic time — to recover the GPS second.

This formulation is resilient to real-world conditions that break a strict pairing approach:

- **Missing or late messages.** A few dropped or delayed messages do not break labelling; the remaining points in the window still fit.
- **Unequal message and pulse rates.** Messages at 1 Hz, 5 Hz, or higher flow into the fit verbatim regardless of the edge rate. No pairwise match is required.
- **Per-message `tRead` jitter.** Serial latency variation is averaged out by the fit.

Labelling uses `TReadMono` on both sides (see "Read-time usage"). Message `tRead` values come from `MsgUTCTime` with `ReadDelay` already subtracted. The edge event's `TReadMono` records when the edge timestamp was *read from the PHC*, not when the edge itself occurred; delivery of edge events can be delayed by up to ~0.25 s (e.g., Raspberry Pi CM4/CM5) or by a few microseconds (typical x86). Following the pattern in `phcsync/reset.go`'s `pulseTimes`, the approximate edge monotonic time is recovered by taking the PHC delta between the edge timestamp and `TRead.PHC`, scaling it to real-time by the locally-estimated PHC-to-real rate, and subtracting from `TRead.Mono`. Microsecond-level error in the recovered edge time is far better than needed for sub-second labelling.

`TReadWall` is not used here - it is reserved for the sample-emission path.

The labelling regression evaluates at the edge's recovered monotonic time and rounds to the nearest second to produce the integer-second UTC label. If the predicted UTC lies further from an integer second than a tolerance well under 0.5 s, the edge is not admitted to the PHC calibration window. Leap transitions break the labelling regression (UTC jumps by ±1 s at the boundary); `Leap(kind)` on `MsgUTCTimer` resets the labelling regression window.

The labelling regression is deliberately separate from, and much coarser than, the PHC calibration regression described in a later section:

| | Labelling regression | PHC calibration regression |
|---|---|---|
| Purpose | Identify which GPS second | PHC-to-UTC at cross-sample |
| Input stream | `MsgUTCTime` callbacks | Window of labelled edges |
| X (independent) | Corrected monotonic `tRead` | PHC `ptime.Time` |
| Y (dependent) | Message UTC | Integer-second UTC label |
| Precision needed | Sub-second | Sub-microsecond |

Fit method (OLS vs. recency-weighted, window length) is tunable per regression; the sub-second precision goal for labelling is loose enough that plain OLS over a short window is expected to be sufficient.

### MsgUTCTimer interface

The push-based interface that already delivers UTC seconds to the serial SOCK path is generalized to serve phcsample too. The existing `timemsg.SerialSampler` / `SetSerialSampler` pair is renamed `MsgUTCTimer` / `SetMsgUTCTimer`. The name identifies the data: UTC time arriving on the GPS message channel (distinct from `gpsprot.TimeMsg`, the raw message, and from edge-derived time on the `ts.Event` channel). The existing serial timing behavior is preserved in substance: the same stream of timing-message notifications continues to flow to the serial SOCK consumer, including higher-rate (e.g. 5 Hz) messages, with `ReadDelay` already subtracted from `tRead`.

```go
type MsgUTCTimer interface {
    MsgUTCTime(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind)
    Leap(kind ptime.LeapSecondKind) // phase 2
}

func (buf *Buffer) SetMsgUTCTimer(t MsgUTCTimer)
```

- **`MsgUTCTime`** replaces today's `SerialSample`. Called once per eligible ms-rounded UTC value. `tRead` has `ReadDelay` already applied, consistent with the existing `timemsg` convention (see `GetPostTimeMessages` and `serialSample`). Each call feeds directly into the labelling regression; sub-second and non-aligned values are welcome (they constrain the fit just like on-second values).
- **`Leap`** (phase 2) fires when `timemsg.Buffer` observes a leap-second transition. phcsample uses this to reset both the labelling regression window and the PHC calibration window and stay in `ErrNotReady` until PHC calibration is re-established; the serial SOCK path ignores it. Phase 1 omits this method entirely — see "Leap-second handling".

`gpsprot.Ref` (PrePulse / PostPulse / NavSolution) stays internal to `timemsg.Buffer`: the buffer may use it for internal message selection but does not surface it on this interface. `PulseOffset` is a separate path — today `GetPulseCorrection`, phase 2 a UTC-keyed `float64`-ns accessor — and is not folded into `MsgUTCTime`.

## Why interpolation between two adjacent edges is not enough

The natural first design is: hold a window of labelled edges, and for a cross-sample at PHC time `P_c`, linearly interpolate between the two edges bracketing it. This is wrong in two ways:

1. **Latency.** The edge *after* `P_c` may not arrive for up to a second. `Generate` should answer now, not later.
2. **Noise.** Even if the bracketing pair is available, GPS PPS carries tens of nanoseconds of jitter per edge, and PHC timestamps have their own sampling noise. A two-point "rate" is dominated by this pair-specific noise. Chrony can filter noise *between* samples, but it cannot repair a sample whose offset is arithmetically corrupted by bad endpoints.

The fix is to fit a local model over a window of recent labelled edges - *past* edges only, for zero latency - and evaluate the model at `P_c`. Pre-admission filtering (see "Pre-admission filtering") guarantees that every edge in the window is correctly labelled; the regression is a separate concern, operating on already-validated inputs.

## PHC calibration regression

The Generator maintains a window of recent labelled edges. Each entry is a pair `(P_i, T_i)` of PHC timestamp (`ptime.Time`) and true UTC label (`time.Time`). Window entries are correctly labelled by construction - the pre-admission filter is what makes this assumption safe.

Over this window, fit a linear model mapping PHC to UTC:

```
T_utc(P) ≈ T_ref + a + b * (P - P_ref)
```

where `P_ref`, `T_ref` are a recent anchor edge (kept to stay in integer nanosecond arithmetic), `b` is the local PHC-to-UTC rate (dimensionless, very close to 1), and `a` is a small offset in seconds. To answer `Generate`, evaluate this model at `P = phc`.

The fit uses edges with `P_i <= phc` only. `Generate` never waits. The model extrapolates forward from the last labelled edge to `phc`, over a gap of at most ~1 pulse interval, using a slope estimated from a longer history.

The method of fitting is an open decision. **Likely starting point: ordinary least squares.** Candidates to evaluate against the clocksim rig include recency-weighted least squares and other simple non-robust variants. Statistically robust fits (MAD-based rejection, Theil-Sen) are explicitly not the direction: the window is clean by construction, chrony handles residual outliers at the sample level, and robustness machinery is unnecessary weight.

Window length is likewise open, to be tuned in simulation.

## Pre-admission filtering

This is the most load-bearing part of the design. Its job is to keep wrong-second, wrong-polarity, and (in phase 2) wrong-PulseOffset edges out of the PHC calibration window.

It is load-bearing for three purposes:

1. **Label integrity.** The labelling regression answers "what UTC second is this edge?", but rounds to the nearest second. An edge whose PHC timestamp has drifted far from the expected stride — or that is not a timing edge at all — can round to the wrong second. The stride check ensures only legitimate, on-stride timing edges reach the rounding step.
2. **Pulse-offset correction (phase 2).** Pulse-offset lookup is keyed by GPS second, so the same label-integrity requirement applies.
3. **Dual-edge polarity selection.** In dual-edge mode only same-polarity edges are ~1 second apart. The stride check is what distinguishes timing edges from the other polarity.

**A mislabelled edge cannot be recovered from once it enters the window.** It is not noise that the regression can average out; it is a bad `(P_i, T_i)` pair, with a one-second or one-pulse-width error in the true-time column. Chrony's downstream filtering cannot repair it either - it operates on emitted samples, not on window entries. So the filtering here must be strong enough that *every edge admitted to the window is correctly labelled*; the PHC calibration regression assumes that precondition.

The mechanism is a relative-variation check on edge PHC timestamps, matching reset.go's `PulseVariation`: with `edgesPerPulse` known from config (1 for single-edge, 2 for dual-edge), compute the stride-`edgesPerPulse` interval between each pair of successive same-polarity edges in the window, and check the PPB variation between the shortest and longest of those intervals against `PulseVariation`.

In single-edge mode this is consecutive intervals. In dual-edge mode it is the stride-2 interval between same-polarity edges; intervals between alternating polarities contain one pulse-width and one gap duration, which differ markedly from 1 second and therefore blow the relative-variation limit, which is how the non-timing edge gets filtered.

When the stride variation exceeds the limit, the offending edge is not added to the window. Wait for subsequent edges to re-establish stride consistency.

`PulseVariation` must be tight enough that mis-labelling is impossible in practice (variations that would move a PHC interval more than a small fraction of a second from 1 true second must be rejected; well under any plausible sawtooth magnitude for phase 2), and loose enough to admit every legitimately-timed edge even under worst-case PHC jitter and drift. reset.go's default (500 PPB) is a starting point; the final value is to be determined against real hardware and the clocksim rig.

## Dual-edge timing-edge selection

In dual-edge mode only one of the two edges marks the top of the GPS second. V1 makes a deliberate product choice: because satpulse is what configures the GPS receiver, it can require that the configured pulse width be far enough from 50 percent duty that edge selection is unambiguous from timing alone. The short/long pattern of consecutive different-polarity intervals then directly reveals which edge is the rising edge (top of second): the interval less than 0.5 s is the pulse width, and the edge preceding it is the timing edge. A few good edges after startup are enough.

This is a stance about the product, not a technical limitation. Prior art: `reset.go`'s `filterEdgeListsByPulseWidth` has a message-alignment fallback for ambiguous duty cycles used by `phcsync`; that approach could be adopted here as future work if it turns out some receiver cannot be configured away from ~50% duty. V1 does not carry it forward.

Explicit `gps.pulseWidth` configuration is not required; the pattern reveals itself.

## Module structure

Module name: `phcsample` (matches the config section).

Three pieces with narrow jobs:

1. **Labelling regression**: fit a linear `tRead → UTC` model over a sliding window of `MsgUTCTime` calls. Evaluate at each edge's recovered monotonic time and round to the second to produce its label. Validation of the fit (enough points, enough span, staleness, slope, scatter) is wholly the wallClock's responsibility; see "Implementation design / wallClock".

2. **PHC calibration window**: maintain recent labelled edges that pass pre-admission filtering, each with a `ptime.Time` PHC position and a UTC `time.Time` label. Apply the stride-`edgesPerPulse` consistency filter. In dual-edge mode, track which polarity is the timing edge and discard the other. Separately, discard edges whose predicted UTC is too far from an integer second — this per-edge confidence check runs over the wallClock's output and is a phcWindow responsibility, not a wallClock gate.

3. **PHC calibration fit**: on `Generate`, fit a linear model over the PHC calibration window (likely plain least squares; see the "PHC calibration regression" section) and evaluate at the requested PHC timestamp. Return the offset in seconds.

## Implementation design

This section pins down the internal shapes the module structure above compiles to. The first subsection captures the config surface; subsequent subsections describe individual internal types.

### Config

Although `phcsample.Config` shares some field names with `phcsync.ResetConfig`, the semantics are not identical. Reset-mode validation is based on explicit pulse/message correlation: it takes a matched sequence of pulse times and post-pulse message read times and asks pair-wise questions about absolute delays. phcsample's wallClock does not do pair-wise matching — it fits a continuous monotonic-to-UTC mapping from the stream of `MsgUTCTime` observations, which may contain more messages than pulses and which has no unique "this message belongs to this pulse" relationship. Reset-style absolute-delay checks therefore do not carry over literally.

Only the parameters whose meanings survive the change in observables are reused verbatim; the rest are either dropped or replaced by parameters shaped around what phcsample actually sees. Docstring style (a multi-line comment above each field explaining observable behaviour, plus a short `comment:` tag for TOML generation) matches reset.go.

Carried over verbatim from `phcsync.ResetConfig` — same units, same toml tag, same semantics:

- **`PulseWindow`** — number of pulses kept for the PHC calibration window. Used only for phcWindow; the wallClock sizes its message window independently (see "wallClock window sizing" below).
- **`PulseVariation`** — max relative PPB variation between stride-`edgesPerPulse` intervals in phcWindow (pre-admission filter).
- **`PulseWidthDetectLimit`** — dual-edge polarity auto-detection threshold.

Carried over with a narrower role:

- **`ExpectedDelay`** — expected pulse-to-message delay in seconds. In phcsample it is used only as a centering shift subtracted from each `MsgUTCTime` sample's `tRead` before feeding the wallClock regression, so predictions centre on the true pulse-mono time rather than on the later message read time. No absolute-window check is built on top of it.

New to phcsample (observables that exist only in the message-stream model):

- **`MsgWindow`** — length of the message history retained for the wallClock's regression, in integer seconds. Messages older than this are discarded. A generously-sized window gives a tighter slope estimate, greater tolerance of occasional outliers (dilution rather than dominance), and more jitter averaging; there is no meaningful cost since neither memory nor compute scales problematically at typical message rates. Default ~30 s.
- **`MaxMsgGap`** — how long phcsample will continue answering after the most recent message arrived, in seconds. While messages are flowing the fitted mapping stays fresh; when they stop, the wallClock projects forward until the gap reaches this limit, then stops producing samples.
- **`MinMsgSpan`** — minimum elapsed time (seconds) that the observed messages must cover before the wallClock's fit is treated as usable. Larger values give more stable slope estimates but delay the first sample after startup. Recommended ~3 s.
- **`ClockRateLimit`** — maximum tolerated rate mismatch between the system monotonic clock and the UTC time advertised by the GPS message stream, as a dimensionless fraction (e.g. `0.1` ≡ 10 %). When the two clocks appear to tick at rates differing by more than this, phcsample treats the stream as unreliable and stops producing samples. A safety bound; expected to be very generous (default around `0.1`) so it catches pathological conditions without interfering with normal crystal drift.
- **`MsgTimingVariation`** — tolerated inconsistency in message timing, expressed as a fraction of 1 second. Measured as the median absolute deviation of observations from the best-fit line, so occasional badly-timed messages (e.g., the slower of two interleaved message types, or bursts on a 9600-baud link) do not trip it — up to ~50 % of messages can be off without firing. Deliberately lenient; a safety gate against genuinely broken streams, not a per-sample quality filter.
- **`EdgeSecondTolerance`** — max distance in seconds from the fit-predicted UTC of an edge to the nearest integer second, for the edge to be admitted to the PHC calibration window. Must be well under 0.5 s so that rounding to the nearest second is unambiguous, and loose enough to accept legitimate edges under worst-case message-to-pulse delay jitter. Distinct from `MsgTimingVariation`: this field caps an edge-to-second-boundary distance in the fit's *output*, whereas `MsgTimingVariation` caps residual scatter of message observations around the fit *input*.
- **`IgnoreSawtoothCorrection`** — when true, disables the use of PrePulse `PulseOffset` corrections even if they are available. This mirrors `phcsync`'s tracking-mode knob and exists primarily for testing, comparison, and field fallback: it lets operators and the sim rig compare corrected vs uncorrected behaviour without changing the message stream. Phase 1 ignores the field because there is no sawtooth path yet; phase 2 respects it by passing a nil pulse-corrector into the PHC-calibration pipeline.

Dropped from `phcsync.ResetConfig` (semantics do not survive):

- **`DelayConfidenceWindow`** — a pair-based absolute-delay gate; wallClock observes a message stream and cannot infer absolute pulse-to-message delay from it alone (an OLS fit absorbs any constant delay into the intercept).
- **`DelayVariation`** — spread of pair-wise delays; replaced by `MsgTimingVariation`, which measures a related but distinct observable (median scatter around the fit, not pair-delay spread).
- **`StepThreshold`** — phcsample does not step a clock.
- **`DriftRateLimit`** — phcsample does not validate candidate steps against a persisted sample.

### Generator

`Generator` is the public type. All three entry points are thin pass-throughs to the internal collaborators; it owns no state beyond the collaborators themselves.

```go
type Generator struct {
    cfg  Config
    wc   wallClock
    win  phcWindow
    smp  Sampler
    leap ptime.LeapSecondKind // latest value from MsgUTCTime, forwarded to NTPSample
    lg   *slog.Logger
}

func NewGenerator(cfg Config, smp Sampler, edgesPerPulse int, lg *slog.Logger) *Generator {
    return &Generator{
        cfg: cfg,
        wc:  *newWallClock(&cfg),
        win: *newPhcWindow(&cfg, edgesPerPulse),
        smp: smp,
        lg:  lg,
    }
}

// MsgUTCTime implements the MsgUTCTimer sink. The most recent leap
// value is retained and forwarded on the next NTPSample call; phase 2
// adds a separate Leap method.
func (g *Generator) MsgUTCTime(utc, tRead time.Time, leap ptime.LeapSecondKind) {
    g.wc.Add(tRead, utc)
    g.leap = leap
}

func (g *Generator) Pulse(edge PulseEdge) {
    g.win.Pulse(edge)
}

func (g *Generator) Generate(phc ptime.Time, sys time.Time) (float64, error) {
    off, err := g.win.TrueTimeOffset(phc, sys, &g.wc, nil, g.lg)
    if err != nil { return 0, err }
    g.smp.NTPSample(sys, off, g.leap, phc)
    return off, nil
}
```

The `wallClock` reference is passed to `TrueTimeOffset` per call rather than stored inside `phcWindow`. This keeps phcWindow free of lifetime-bound pointers and makes each call self-contained. `Pulse` never consults the wallClock — edges are recorded cheaply and all labelling work happens lazily inside `TrueTimeOffset` against the wallClock state at the moment of the query. The `pulseCorrector` argument is `nil` in phase 1, also `nil` in phase 2 when `cfg.IgnoreSawtoothCorrection` is true, and otherwise `timemsg.Buffer`'s phase-2 `GetUTCPulseCorrection` accessor; phase-1 code therefore exercises the same arithmetic path as the "sawtooth present but ignored" configuration, so phase 2 adds no new control-flow shape.

### phcWindow

`phcWindow` holds recent pulse edges, labels them via the wallClock at query time, admits those that pass pre-admission filtering, fits the PHC-to-UTC regression, evaluates it, and combines with the cross-sample's `sys` reading to produce the refclock offset. The only method called per edge, `Pulse`, is trivial: it appends the raw edge. All labelling, admission, fitting, and arithmetic happens lazily inside `TrueTimeOffset`.

```go
// pulseCorrector is the interface implemented by timemsg.Buffer's
// phase-2 UTC-keyed pulse-correction accessor. Phase 1 passes nil.
// Harmonised with timemsg.Buffer's existing GetPulseCorrection
// style; both return (value, ok) rather than error.
type pulseCorrector interface {
    GetUTCPulseCorrection(refTime time.Time) (float64, bool)
}

func newPhcWindow(cfg *Config, edgesPerPulse int) *phcWindow

// Pulse records the edge for later processing.
func (w *phcWindow) Pulse(edge PulseEdge)

// TrueTimeOffset returns the refclock offset in seconds:
//
//     offset = true_time_at(phc) - sys
//
// A positive value means true time is ahead of sys — i.e., the
// system clock is behind real time and needs to advance by this
// amount. A negative value means the opposite. This matches the
// sign convention chrony expects from a SOCK refclock.
//
// Internally: processes recorded edges via wc (with phase-2 pulse
// correction via po when non-nil), runs pre-admission filtering,
// fits the PHC-to-UTC regression over admitted entries, evaluates
// at phc, and combines with sys. Sub-nanosecond precision from the
// pulse-correction accessor is carried through arithmetic at this
// level, not exposed in the return type. Returns ErrNotReady until
// enough admissible data is available; other sentinels for
// extrapolation-range failures etc. The Generator forwards errors
// to its caller without wrapping.
func (w *phcWindow) TrueTimeOffset(phc ptime.Time, sys time.Time, wc *wallClock, po pulseCorrector, lg *slog.Logger) (offset float64, err error)

// Reset clears state (phase 2, for leap handling).
func (w *phcWindow) Reset()
```

**Top-level sketch of `TrueTimeOffset`.** The body is a straight pipeline through four internal collaborators, one per sub-step. Stateless across calls: no sticky state other than the raw `Pulse` buffer.

```go
func (w *phcWindow) TrueTimeOffset(phc ptime.Time, sys time.Time, wc *wallClock, po pulseCorrector, lg *slog.Logger) (float64, error) {
    timing, medianInterval, err := timingEdges(w.buf, w.edgesPerPulse, w.cfg)   // 6a + 6b
    if err != nil {
        return 0, err
    }
    basePHC := w.buf[0].Timestamp.T
    entries, err := mapEdgesToUTC(timing, medianInterval, basePHC, wc, po)      // 6c
    if err != nil {
        return 0, err
    }
    return fitAndEvaluate(entries, basePHC, phc, sys, w.cfg.maxExtrapolation()) // 6d
}
```

`timingEdges` wraps 6a (stride-consistency filter, applied once in single-edge mode and once per polarity in dual-edge mode) and 6b (polarity selection in dual-edge mode). Internals (buffering strategy, fit method, sub-ns carry representation) are deferred to the per-step subsections below.

#### timingEdges (6a + 6b)

```go
func timingEdges(buf []PulseEdge, edgesPerPulse int, cfg *Config) (edges []PulseEdge, medianInterval time.Duration, err error) {
    if edgesPerPulse == 1 {
        raw, medianInterval := consistentEdges(buf, cfg.PulseVariation)
        edges = removeZeroEdges(raw)
        if len(edges) == 0 {
            return nil, 0, ErrNotReady
        }
        return edges, medianInterval, nil
    }
    a, b := splitAlternating(buf)
    ea, ma := consistentEdges(a, cfg.PulseVariation)
    eb, mb := consistentEdges(b, cfg.PulseVariation)
    return selectTimingStream(ea, eb, ma, mb, cfg.PulseWidthDetectLimit)
}
```

`medianInterval` is the PHC duration spanning one real second, sourced from the chosen stream's same-polarity stride. Downstream steps (`mapEdgesToUTC`) use it as the PHC-to-real scaling factor and so do not need to recompute it.

Stateless: polarity is re-decided every call from the current buffer. `selectTimingStream` returns `ErrNotReady` when either stream is empty or when the short/long pattern is not yet unambiguous.

#### consistentEdges (6a)

```go
func consistentEdges(stream []PulseEdge, tolPPB float64) (edges []PulseEdge, medianInterval time.Duration)
```

`consistentEdges` takes a chronological polarity stream and returns a same-length slice where rejected edges are replaced by the `PulseEdge` zero value (admitted edges are passed through unchanged), along with the stream's median interval. Keeping the slice length fixed preserves positional alignment between the two polarity streams in dual-edge mode, so `selectTimingStream` can pair `A[i]` with `B[i]` without the shift that would otherwise arise when either stream loses entries. `selectTimingStream` calls `removeZeroEdges` on the chosen stream before returning, so callers downstream of `timingEdges` never see zero entries. `medianInterval` is the PHC duration corresponding to one real second for this stream. Rejection is against `tolPPB` (sourced from `Config.PulseVariation`). Algorithm:

1. Compute PHC intervals between consecutive edges in the stream.
2. Find the median interval.
3. If any interval is ≥1.5× median (a gap, e.g. a missing pulse), zero out every entry before the gap and restart from step 1 on the post-gap suffix.
4. Flag intervals deviating from the median by more than `tolPPB`. An edge adjacent to two flagged intervals — or adjacent to a single flagged interval at the stream boundary — is an outlier and is zeroed. Survivors are left in place at their original indices.

Median-based so a single bad edge does not pull the center away from the true cluster. Drift across the window (tens of µs at phase-1 window sizes) is orders of magnitude below outlier magnitudes and below `PulseVariation` tolerance, so a window-wide median is drift-safe; it would need to become local if windows were ever tuned to tens of seconds or more.

Returns a fully-zeroed slice (surfaced as `ErrNotReady` by callers) when the window is too short to compute a meaningful median, or when the classifier cannot identify a consistent majority.

#### selectTimingStream (6b)

```go
func selectTimingStream(a, b []PulseEdge, aMed, bMed time.Duration, pulseWidthLimit float64) ([]PulseEdge, time.Duration, error) {
    // a and b are same-length (guaranteed by splitAlternating +
    // consistentEdges). Pair by index, skipping any pair where either
    // side is zero. If crossPolarityGap finds no admissible pair
    // (e.g. both streams are fully zeroed), it returns ok=false and
    // this function returns ErrNotReady.
    avgMedian := (aMed + bMed) / 2
    pulseWidthPHC, ok := crossPolarityGap(a, b)
    if !ok {
        return nil, 0, ErrNotReady
    }
    pulseWidth := float64(pulseWidthPHC) * (float64(time.Second) / float64(avgMedian))
    // The chosen stream has its zero entries removed before return:
    // zeros served their purpose inside selectTimingStream (preserving
    // A↔B index alignment), and mapEdgesToUTC receives a simple slice.
    switch {
    case pulseWidth <= pulseWidthLimit*float64(time.Second):
        return removeZeroEdges(b), bMed, nil
    case (1-pulseWidthLimit)*float64(time.Second) <= pulseWidth:
        return removeZeroEdges(a), aMed, nil
    default:
        // Ambiguous — pulse width near 50% duty. V1 product rule
        // forbids this; receiver must be configured outside the
        // ambiguous band.
        return nil, 0, ErrNotReady
    }
}
```

The short/long discriminator is inherited from [`phcsync/reset.go` `filterEdgeListsByPulseWidth`](time/internal/phcsync/reset.go) (line 374 onwards), but the surrounding machinery is different: a zero-sentinel alignment scheme that preserves positional pairing between the two polarity streams through `consistentEdges`'s filtering, and `crossPolarityGap` skipping zeroed pairs. The ambiguous-pulse-width fallback ("both — try message alignment") is also dropped: V1's product rule — satpulsed configures the receiver, so the pulse width can be kept well outside the ambiguous band — makes it unnecessary, and the simpler surface is preferred.

#### mapEdgesToUTC (6c)

```go
// calibEntry is one point on the PHC-to-true-time ruler.
// X is PHC nanoseconds relative to basePHC (the caller's per-call anchor).
// Y is the top-of-second UTC at that edge, as an exact time.Time — no
// anchoring needed on this side since time.Time is already nanosecond-
// precise and arithmetic through time.Duration stays exact. Phase 2's
// sub-ns pulse-offset correction modifies X (it shifts the PHC-side
// ruler mark from the physical pulse edge to the true top-of-second),
// not Y.
type calibEntry struct {
    X float64   // (P_i - basePHC) + pulseOffsetPHCNs_i, nanoseconds
    Y time.Time // integer-second UTC of this edge
}

func mapEdgesToUTC(edges []PulseEdge, medianInterval time.Duration, basePHC ptime.Time, wc *wallClock, po pulseCorrector) ([]calibEntry, error)
```

`mapEdgesToUTC` turns each stride-admitted edge into a ruler mark. `X` is the edge's PHC position relative to `basePHC` (a per-call anchor supplied by the caller — typically `w.buf[0].Timestamp.T`), adjusted by the pulse-offset correction after scaling it from true-time nanoseconds into PHC nanoseconds using `medianInterval`, so that the mark lands at the *true* top of the GPS second rather than at the physical pulse edge. `Y` is the exact integer UTC second that top-of-second corresponds to.

`basePHC` exists to keep `X` bounded to window-span magnitude (tens of seconds), so float64 ULP stays in the femtosecond range regardless of process uptime. There is no corresponding UTC anchor: `time.Time` handles nanosecond arithmetic exactly, and the fractional-ns precision from pulse correction is carried on the PHC side via `X`.

Algorithm:

1. For each edge, derive the edge-occurrence monotonic time: shift `TRead.Mono` back by the PHC-side delivery gap (`TRead.PHC.T - Timestamp.T`) scaled to real time by `medianInterval` (the PHC duration corresponding to one real second, already computed by `consistentEdges` — no separate rate estimate here).
2. Ask `wc.SecondAt(edgeMono)` for the integer UTC second. Three outcomes:
   - **Success** → admit the edge.
   - **`errors.Is(err, ErrNotReady)`** → wallClock has no fit that covers this edge's time. The whole window is blocked on the message side. Return `ErrNotReady` immediately.
   - **`errors.Is(err, errStale)`** → this edge is newer than wallClock's usable range. All subsequent edges (in chronological order) are also stale, so stop iterating and return the already-admitted prefix as the result. Stale on its own is not an error to the caller — the admitted prefix is a valid answer.
   - **Any other error** (rate, scatter) → the fit is globally broken. Return the error.
3. Drop edges whose fit-predicted UTC (before rounding) lies further from an integer second than `cfg.EdgeSecondTolerance`. Guards against rounding an edge that falls near the half-second mark. (Details of surfacing the unrounded residual out of `wallClock` are an internal change alongside this step.)
4. Let `pulseOffsetNs` be `0.0` in phase 1, or the sub-ns `float64` returned by `po.GetUTCPulseCorrection(integer_utc)` in phase 2. Convert it to PHC nanoseconds with `pulseOffsetPHCNs := pulseOffsetNs * float64(medianInterval) / float64(time.Second)`. Compute `X = float64((edge.Timestamp.T - basePHC).Nanoseconds()) + pulseOffsetPHCNs` and `Y = integer_utc`. Append `{X, Y}` to entries. The sign here is the opposite of `tracking.go`'s `refTime = sec - PulseOffset`: there the correction moves the time label of the physical pulse, while here `Y` stays the integer UTC second and the correction moves the PHC-side ruler mark to the true top-of-second. Phase 2 is still additive in structure because the arithmetic path already carries the fractional-ns term.

Returns `ErrNotReady` if the surviving entries are too few for the 6d fit (threshold tied to a small constant; minimum meaningful window for OLS is 3).

#### fitAndEvaluate (6d)

```go
// errExtrapolation indicates phc is too far past the last entry's PHC
// position for a trusted fit evaluation.
var errExtrapolation = errors.New("phcsample: phc beyond extrapolation range")

func fitAndEvaluate(entries []calibEntry, basePHC ptime.Time, phc ptime.Time, sys time.Time, maxExtrapolation time.Duration) (offsetSeconds float64, err error)
```

`fitAndEvaluate` fits an OLS line to the ruler marks, evaluates it at `phc` to get the true UTC, subtracts `sys`, and returns the result as `float64` seconds. The entire pipeline stays in float64 nanoseconds from 6c through the final return, preserving the phase-2 sub-ns pulse-offset precision into the refclock output. No intermediate `time.Time` materialisation of the true time — `time.Time` has 1 ns resolution and would round the sub-ns precision away.

Algorithm:

1. If `len(entries) < minFitEntries`, return `ErrNotReady`.
2. Compute `X_query := float64((phc - basePHC).Nanoseconds())`. `phc` is the PHC timestamp paired with the current edge's cross-sample, so it is at or just past the last entry's `X`; the evaluation is strictly forward. If `X_query - entries[last].X > float64(maxExtrapolation.Nanoseconds())`, return `errExtrapolation`.
3. Anchor Y internally: let `yRef := entries[0].Y`. Transform to `Y_i := float64(entries[i].Y.Sub(yRef).Nanoseconds())` for OLS. Values stay bounded to window-span magnitude so float64 ULP remains in the femtosecond range.
4. Plain OLS over `(X_i, Y_i)`: means, `S_xy`, `S_xx`, slope `b := S_xy/S_xx`, intercept `a := meanY - b·meanX`.
5. Evaluate: `Y_query_ns := a + b·X_query`. This is the true-time offset from `yRef` in nanoseconds, as a float64 carrying sub-ns precision.
6. Compute the offset against `sys`: `offsetNs := float64(yRef.Sub(sys).Nanoseconds()) + Y_query_ns`. The `yRef.Sub(sys)` is an exact `time.Duration` (int64 ns), small in magnitude because cross-samples pair `sys` with the recent PHC window. Return `offsetNs * 1e-9`.

Plain OLS is the starting point; recency-weighted and other non-robust variants are candidates to compare in simulation (see "Open decisions"). `minFitEntries` is a small package constant — meaningful OLS needs at least 3 points.

`wallClock` is an unexported type inside `phcsample` that models UTC as a function of monotonic time, synthesised from the `MsgUTCTime` stream. Given a monotonic instant — including a recovered edge-mono time — it returns the integer UTC second that time falls inside, or an error that explains why no answer is available.

```go
// wallClock maps monotonic time to integer-second UTC from the
// MsgUTCTime stream. Internally it maintains a sliding linear
// regression over (tRead - expectedDelay, utc) pairs.
type wallClock struct {
    // captured from Config at construction in convenient internal form
    expectedDelay time.Duration // Config.ExpectedDelay
    msgWindow     time.Duration // Config.MsgWindow (seconds -> Duration)
    maxGap        time.Duration // Config.MaxMsgGap
    minSpan       time.Duration // Config.MinMsgSpan
    rateLimit     float64       // Config.ClockRateLimit (dimensionless)
    timingVar     time.Duration // Config.MsgTimingVariation expressed as a Duration
    // regression state follows
}

func newWallClock(cfg *Config) *wallClock

// Add observes a MsgUTCTime sample. tRead is the monotonic read
// time (with ReadDelay already subtracted by timemsg.Buffer); utc
// is the ms-rounded UTC reported for that message.
func (c *wallClock) Add(tRead, utc time.Time)

// SecondAt returns the integer UTC second the wall clock reads at
// the given monotonic instant. On failure it returns an error
// describing which gate rejected the fit. The sentinel ErrNotReady
// is returned when there isn't yet enough observation to answer
// (too few points, or observations span too little real time);
// callers treat that as quiet transient state.
func (c *wallClock) SecondAt(mono time.Time) (utc time.Time, err error)

// Reset clears the window (used on leap transition in phase 2).
func (c *wallClock) Reset()
```

**Construction and type conversion.** `newWallClock` takes `*Config` and captures only the fields it uses, converted to internal forms. The `Config` surface expresses sub-second seconds as `float64` (so fractional values are natural in TOML), fractions as `float64` in `[0, 1]`, and `MsgWindow` as an `int` number of whole seconds (values of practical interest are tens of seconds, and integer TOML renders more cleanly than `"30s"`-style duration strings). wallClock internally prefers `time.Duration` for quantities that combine with `time.Time` values; the conversion happens once, at construction, so `SecondAt` and `Add` operate entirely in `time.Duration` without re-parsing `Config` fields.

**Delay handling.** A message's `tRead` lags its pulse by the receiver's internal pulse-to-message delay (typically 50–250 ms). The regression operates on `(tRead - expectedDelay, utc)` pairs so predictions centre on the true pulse-mono time. wallClock does not attempt to measure the absolute delay — an OLS fit absorbs any constant offset into the intercept, and there is no external reference for the monotonic-to-UTC offset from within wallClock's inputs.

**Validation.** `SecondAt` applies these gates; failure returns an error describing which gate tripped:

- **Not enough data.** Fewer than the minimum number of observations, or observations that do not yet cover `MinMsgSpan` of real time. Both conditions return `ErrNotReady` — a quiet transient state during startup and after a reset. The Generator forwards the same sentinel to its caller.
- **Stale query.** The query's monotonic instant is more than `MaxMsgGap` past the most recent observation. The fit is too old to project from. Returns the sentinel `errStale` (wrapped via `fmt.Errorf` with the numeric detail) so callers can distinguish it from `ErrNotReady` and from the global-fit failures below.
- **Clock rate mismatch.** The fitted slope differs from unity by more than `ClockRateLimit`. Indicates a pathologically broken message stream or clock — a safety gate, expected to fire very rarely.
- **Message timing scatter.** The median absolute residual of observations around the fit line exceeds `MsgTimingVariation` (interpreted as a fraction of 1 s). Tolerant of a minority of badly-timed messages by construction; catches streams where most messages are inconsistent with each other.

All gates are purely local to wallClock's state and do not require external information.

**Internal state.** Sliding window of `(tRead, utc)` pairs anchored at the oldest entry so the regression operates in small deltas rather than absolute nanoseconds. Plain OLS starting point; recency-weighted and other non-robust variants are candidates to compare in simulation (see "Open decisions").

## Relationship to existing code

`freeRunning=true` introduces a **third dispatcher runtime mode**, not merely a "no controller" variant of PHC-disciplined mode. The three modes are mutually exclusive:

| Mode | PHC | `phcsync.Controller` | `phcsample.Generator` |
|------|-----|----------------------|-----------------------|
| Serial timing | absent | nil | nil |
| PHC disciplined | present | present | nil |
| PHC free-running | present | nil | present |

Implications for the dispatcher and daemon wiring:

- **`controller == nil` must stop being a synonym for serial timing.** The current code in `time/internal/gpsevent/dispatcher.go` uses the nil-controller check to route GPS time messages into `SerialSample`, and PHC event handling assumes a controller exists. Both sites must become three-way: `controller != nil` (disciplined) vs. `generator != nil` (free-running) vs. both nil (serial). The correct conditions for serial SOCK sampling and for PHC event delivery must be made explicit, not implicit in `controller == nil`.
- **PHC event path in free-running mode.** The dispatcher adapts `ts.Event` into `phcsample.PulseEdge` (see "Module interface") and calls `generator.Pulse(edge)` for each `EdgeEvent`. On `PauseEvent` it drops the Generator; on the next edge after the era advances it constructs a fresh one. Stale-era filtering stays in the dispatcher. Cross-samples from edge events go to `generator.Generate(phc, sys)` using the `TReadWall` side; the dispatcher owns the `rc.Sample` call.
- **Serial SOCK sampling is disabled in free-running mode**, just as it is in disciplined mode - the refclock samples come from `phcsample`, not from GPS messages. `Buffer.SetMsgUTCTimer` (today `SetSerialSampler`; see "MsgUTCTimer interface") is wired to the dispatcher in serial timing mode and to `phcsample.Generator` in free-running mode. In disciplined mode no `MsgUTCTimer` is installed.
- The `ts.Event` feed and `timemsg.Buffer` already provide everything this module needs on the input side, once the generalized `MsgUTCTimer` interface replaces today's `SerialSampler` (see "MsgUTCTimer interface").
- The `refclock.ProxyRefClock` / `sockrefclock` path already provides the output. No changes required there.
- `phcsync` itself is untouched. This module reuses none of its state machine, servo, or MAD filter.
- **Observability.** `obs.Observer` gains an `NTPSample(sys time.Time, offset float64, phc ptime.Time)` method declared directly in its method set. By structural typing any `obs.Observer` then satisfies `phcsample.Sampler`; no import relationship between `obs` and `phcsample` is introduced (see "Sample emission"). `obs.MultiObserver` gains an `NTPSample` method that fans out by type-asserting handlers, and `obs.DefaultObserver` gains a no-op implementation, mirroring how each handles `phcsync.Sampler.Sample`. Concrete observer consumers (`statsobs`, `logobs`, `promobs`) do not observe the new stream automatically — they each consume `phcsync.Sample` today and would need their own `NTPSample` methods added to start consuming phcsample output. That wiring is a follow-up, not part of the phcsample bring-up.

## What is not inside the Generator

Deliberately absent:

- **System clock work.** `sys` is an opaque parameter, used only via `time.Time` subtraction at the end of `Generate`. No cross-sample pair rate, no sys-vs-PHC rate estimate.
- **Leap-second arithmetic.** True-time labels are UTC throughout, supplied by `timemsg.Buffer`. The Generator has no `ptime.LeapSecond` field and performs no TAI-to-UTC conversion. In the phase-2 design it suspends across a leap transition rather than bridging (see "Leap-second handling"); phase 1 is leap-unaware.
- **TAI.** `ptime.Time` appears only for PHC timestamps, where it denotes PHC ticks, not TAI seconds.

## Phasing

Three phases. Phase 0 lands two small preliminaries that are independent of the rest of the design and worth shipping first. Phase 1 gets us something that works with chrony. Phase 2 refines, polishes, and adds sawtooth correction for the full receiver-specific product. Each step is self-contained, reviewable, and leaves the build green.

### Phase 0 — preliminaries (done)

Two standalone additions that the later phases build on but do not themselves depend on phcsample. Either can ship on its own.

**Status: landed.** Both steps below are complete; this section is retained as a record of what shipped.

1. **Rename in `timemsg`.** `SerialSampler` → `MsgUTCTimer`, `SerialSample` → `MsgUTCTime`, `SetSerialSampler` → `SetMsgUTCTimer`. Update the one implementer (`Dispatcher.SerialSample`) and the existing tests. No `Leap` method yet — it is added in phase 2.

2. **Add `NTPSample` observability.** Declare `NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time)` on `obs.Observer` (see "Sample emission"), extend `obs.MultiObserver` to fan out and `obs.DefaultObserver` to no-op. Fix `SSEObserver` to embed `obs.DefaultObserver` (it currently embeds only `gpsprot.DefaultHandler` — see `time/internal/sseobs/sse.go`), so it picks up `NTPSample` and any future Observer additions for free. In `time/internal/gpsevent/dispatcher.go`, call `d.obs.NTPSample(...)` only on the success branch of `rc.Sample(...)` in both refclock-sample paths:
   - `MsgUTCTime` (serial timing mode): `sys = tRead`, `offset = utc.Sub(tRead).Seconds()`, `leap = leap`, `phc = ptime.Time(0)`.
   - `sysSample` (PHC-disciplined mode): `sys = sys`, `offset = offset`, `leap = leap`, `phc = ref`.

   The `rc.Sample` calls are unchanged; the observer hook is purely additive. Concrete observer consumers (`statsobs`, `logobs`, `promobs`, `sseobs`) pick up no-op `NTPSample` from `DefaultObserver` for now and are wired up as separate follow-ups outside this plan.

### Phase 1 — working with chrony

**Status: landed.** All steps (3–7) complete; free-running mode is usable with chrony end-to-end.

Throughout phase 1:

- UTC time messages only (no sawtooth correction).
- Pulse-edge labels placed at the physical edge; sub-nanosecond quantization error from the receiver flows into the PHC calibration regression as noise.
- Supports single-edge and dual-edge (with the 50 percent duty restriction).
- Past-only windowed linear regression for PHC calibration (method per "PHC calibration regression"; likely plain least squares).

Steps:

3. **Implement `wallClock` and `phcsample.Config` with unit tests.** Create the `phcsample` package. Land `phcsample.Config` with the fields named in "Implementation design / Config". Implement the `wallClock` type per "Implementation design / wallClock". Unit tests (using the `go-unit-test` skill) cover each gate: `ErrNotReady` when too few observations or the window span is below `MinMsgSpan`; correct integer-second identification across typical pulse-to-message delays (50–250 ms); the stale-query gate (`MaxMsgGap`); the clock-rate gate (`ClockRateLimit`); the message-timing-scatter gate (`MsgTimingVariation`, confirming the median-based check tolerates a minority of offset points); and `Reset()` behaviour. This is the first concrete phcsample code to land.

4. **Implement `Generator` and the `phcWindow` interface.** Add the public `Generator` type per "Implementation design / Generator", with its method bodies as actual pass-throughs (`MsgUTCTime` forwards to `wallClock.Add`; `Pulse` forwards to `phcWindow.Pulse`; `Generate` calls `phcWindow.TrueTimeOffset` and emits via the `Sampler`). Add the `phcWindow` surface (`Pulse`, `TrueTimeOffset`, `Reset`) with `Pulse` appending the edge to an internal buffer, `Reset` clearing it, and `TrueTimeOffset` stubbed to return `ErrNotReady`. Add `PulseEdge`, `ErrNotReady`, the `Sampler` interface, and the `pulseCorrector` interface. The tree compiles; the Generator actually routes calls; the only missing piece is `TrueTimeOffset`'s body.

5. **Build `phcsample/sim` and integration unit tests.** Implement `phcsample/sim` paralleling `syncsim` — event loop, clocksim plumbing, ground-truth scoring. Add table-driven Generator-level unit tests covering single-edge, dual-edge, missing messages, missing edges, gross outliers, and startup. Tests compile; most fail because `TrueTimeOffset` still returns `ErrNotReady`.

6. **Implement `phcWindow.TrueTimeOffset`; get tests passing.** Fills in `TrueTimeOffset`'s body. This is where the heavy lifting lives: edge labelling via the passed `wallClock` (and the phase-1-nil `pulseCorrector`), pre-admission filtering (stride-`edgesPerPulse` consistency check), dual-edge polarity selection, the PHC calibration fit, evaluation at `phc`, and combination with `sys`. To be broken down into its own sub-plan. No sawtooth correction and no leap-second handling. Tests pass; the sim rig produces clocksim statistics on par with `syncsim`'s output for `phcsync`.

7. **Wire into the daemon.** Add the `phc.freeRunning` config field. In the daemon and `time/internal/gpsevent/dispatcher.go`, add the third runtime mode (free-running): `controller == nil` stops being a synonym for serial timing; the three-way split between `controller`, `generator`, and neither becomes explicit. Wire `SetMsgUTCTimer` to the generator in free-running mode and to the dispatcher in serial mode; neither in disciplined mode.

End of phase 1: the system is usable with chrony.

### Phase 2 — refine, polish, and add sawtooth correction

**Status: step 10 landed; first-sample info log landed out-of-band.** PrePulse sawtooth correction, `IgnoreSawtoothCorrection` knob, and the sim-rig acceptance test are in. A `logobs.NTPSampleLogObserver` now emits "generated first NTP refclock sample" info on the first `Observer.NTPSample` call, covering the step-12 warmup log in a mode-neutral way (see step 12 for the reduced scope). Steps 8, 9, 11, 12, 13, and 14 are still to do.

8. **Add leap-second handling.** Extend `MsgUTCTimer` with `Leap(kind ptime.LeapSecondKind)`. `timemsg.Buffer` fires it on observed leap-second transitions. `phcsample.Generator` resets both regression windows on `Leap` and returns `ErrNotReady` until re-warmed. Implement the three behaviors from "Leap-second handling". Add sim-rig tests covering leap transitions.

9. **Wire the `[phcsample]` config section.** Parse the TOML `[phcsample]` section into the Generator's `Config` struct, including the phase-2 `ignoreSawtoothCorrection` knob. Revisit field names, types, units, and descriptions now that the implementation constrains what's actually tunable. Update `docs/man/satpulse.toml.5.md`.

10. **Add sawtooth correction (PrePulse only).**
    - Consume PrePulse pulse-correction messages and apply `PulseOffset` to each pulse-edge calibration pair so the PHC-side ruler mark lands at the exact top of the second rather than at the physical PPS edge.
    - The calibration pair remains `Y = message UTC` at the exact integer second; the correction is applied on the `X` side by converting `PulseOffset` from true-time nanoseconds into PHC nanoseconds and adding it to the edge's PHC coordinate. This is the opposite sign from `tracking.go`, which subtracts `PulseOffset` from the reference-time label because it is solving for the GPS time of the physical pulse.
    - Pulse-offset lookup is done in UTC: `timemsg.Buffer` is extended to accept a UTC key for pulse-correction access. **No leap-second or TAI arithmetic enters `phcsample`** — the UTC-in / UTC-out boundary established in phase 1 is preserved.
    - Respect `ignoreSawtoothCorrection`: when set, `Generator` must behave exactly as phase 1 did even if PrePulse correction messages are present. This is useful both as an operator escape hatch and as an A/B toggle for testing.
    - **Only PrePulse corrections apply.** The new UTC-keyed accessor ignores PostPulse correction messages — the correction for pulse N in PostPulse mode arrives *after* pulse N's edge event, which requires a different pipeline. See step 14 for PostPulse handling.
    - The accessor returns `float64` true-time nanoseconds (not `time.Duration`). Phase 1 already carries sub-ns precision end-to-end: `phcWindow.mapEdgesToUTC` scales that value into PHC nanoseconds without rounding, and `phcWindow.fitAndEvaluate` preserves the resulting fractional-ns precision through to the returned `float64`-seconds offset. The only outstanding piece is that `timemsg.Buffer`'s representation must keep the pulse-offset correction as a separate `float64` alongside the nanosecond-resolution `time.Time` label, rather than folding it in and rounding (as today's `validatePulseOffset` does).
    - Extend `phcsample/sim` so sawtooth-enabled runs can deliver the same PrePulse correction path through `timemsg.Buffer` that production uses. This gives us an end-to-end acceptance test for the step: with sawtooth enabled, runs with `ignoreSawtoothCorrection=true` should match the uncorrected behaviour, while runs with it false must show a concrete measurable improvement in the residual statistics (at least stddev and/or absMax, ideally back toward the phase-1 baseline). A wrong-sign or unscaled implementation should fail this comparison and show a residual correlated with the simulated sawtooth.
    - Expected gain: cleaner regression input, tighter chrony convergence.

11. **Unify the pulse-edge sink interface (cleanup).** `phcsync.Controller` and `phcsample.Generator` each declare their own exported `PulseEdge` struct with the same two fields (`Timestamp phctime.Time`, `TRead phctime.Sample`) and take it via differently-named methods (`PulseEdge` vs. `Pulse`). Introduce a shared `gpsevent.PulseReceiver` interface with a single method `Pulse(timestamp phctime.Time, tRead phctime.Sample)`. Rename `phcsync.Controller.PulseEdge` to `Pulse(ts, tr)` and change `phcsample.Generator.Pulse(edge)` to `Pulse(ts, tr)`; both build their own (now unexported) `pulseEdge` internally. `gpsevent.NewDispatcher` takes a single `PulseReceiver` argument instead of separate `controller` and `generator` parameters; the Dispatcher keeps both a `pulse PulseReceiver` field (for the shared `Pulse` call) and typed `controller` / `generator` fields (populated by a one-shot type-switch in the constructor) for the mode-specific paths (Pause, sysSample / genSample, Close, ticker, MsgUTCTime). Update callers: `replay.go`, `syncsim.go`, `phcsample/sim/sim.go`, and the daemon wiring. Also touches phcsync's internal `PulseEdge` uses (tracking.go, reset.go, converging.go, tests) as a mechanical rename. Pure cleanup — no behaviour change.

12. **Add debug logging inside `phcsample`.** The `*slog.Logger` is plumbed through `Generator` and `phcWindow.TrueTimeOffset` today but nothing inside the package uses it. Two tiers:
    - **Debug, per successful `Generate`**: a small handful of interesting stats — likely window size, fit residual, estimated PHC rate. Keep the attribute list short; this fires at the edge rate.
    - **Debug, when things go wrong**: the non-`ErrNotReady` failure paths in `TrueTimeOffset` (stride rejection, ambiguous polarity, `errExtrapolation`, wallClock rate / scatter gates). These don't surface nicely through the error return today — each gate becomes a targeted `Debug` log with the relevant numeric detail.

    The "first successful `Generate`" info log originally scoped here is already handled generically by `logobs.NTPSampleLogObserver`, which fires once per process lifetime on the first `Observer.NTPSample` call across all three dispatcher modes. Re-firing on each `NewInstance` (PHC era transition) is not covered there; if operators want per-era warmup visibility, it needs an observability hook (e.g. a `Pause` event) that today does not exist.

    Exact attribute sets and wording TBD in the step itself.

13. **Surface `NTPSample` in the web UI via SSE.** `sseobs.SSEObserver` currently emits `SampleSSE` off `phcsync.Sampler.Sample` — so in free-running mode the offset field in the UI is empty. Add an `NTPSample` method on `SSEObserver` that emits an SSE with the `offset` (sys-vs-true-time in seconds, the quantity already carried by `NTPSample`), and any other values worth surfacing. This offset is *system-clock* offset rather than PHC offset, and is itself interesting — it's what chrony is ultimately disciplining, so operators can read it directly from the UI. Fires in all three dispatcher modes (serial, disciplined, free-running), so the same SSE channel becomes informative across modes. Minor web-side work to render it. Separate-from-but-parallel-to the `Sample` wiring that observes PHC offset.

14. **Handle PostPulse correction messages.** In PostPulse-mode receivers the pulse-correction data for pulse N arrives *after* pulse N's edge event (see `lastPostCorrMsg` in `time/internal/timemsg/timemsg.go`); step 10's UTC-keyed accessor intentionally returns no correction in that case. To use PostPulse corrections the pipeline must either wait for the correction before admitting the edge to the PHC calibration window, or record a "pending edge" whose admission is deferred until its correction arrives. Two candidate approaches:
    - **Deferred admission.** `phcWindow.Pulse` records the raw edge as today; `mapEdgesToUTC` skips edges whose correction is still missing on this `Generate` call and re-tries them on subsequent calls. Each freshest edge in PostPulse mode therefore contributes to the fit one call late; older edges pick up their corrections as the next PostPulse message arrives.
    - **Explicit wait.** The dispatcher defers `generator.Generate` until `timemsg.Buffer` signals that the PostPulse correction for the freshest edge has arrived, via an extension of the existing `WaitForPulseCorrection` mechanism.

    Either approach needs the UTC-keyed accessor to distinguish "correction not yet received" from "no correction for this GPS second". Must preserve step 10's UTC-in / UTC-out boundary and sub-ns precision guarantees. Details TBD.

End of phase 2: fully configurable, tested, documented, production-grade, and delivers the receiver-specific sawtooth-correction advantage described in the motivation.

## Not in v1 (any phase)

- Pulse widths near 50 percent duty in dual-edge mode. This is a deliberate product constraint: satpulse configures the receiver, and v1 simply requires a pulse width far enough from 50% duty for timing-only edge selection (see "Dual-edge timing-edge selection"). It is not a gap to be filled.
- Taking cross-samples more frequently than edge events. Today `ts.Event` delivers one `TReadWall` per edge, so we emit at the edge rate (1 Hz single-edge, 2 Hz dual-edge); a higher sample rate would need an independent cross-sampler.
- Holdover-style behavior during GPS outage. If edges stop, `Generate` returns `ErrNotReady` (or an extrapolation-range error); chrony falls back to its own sources.

## Open decisions

- Specific fit (plain vs. recency-weighted least squares, or another simple non-robust variant) and window length. Drive with clocksim results.
- Absolute tolerance for the stride-`edgesPerPulse` consistency check (likely low-microseconds; measure on real hardware and in simulation).
- Minimum number of good edges before the first non-error return (at least enough for a stable fit; probably a handful more for dual-edge polarity confidence).
- Whether to log regression diagnostics (residual stddev, rejected-edge rate, estimated local rate) as free telemetry.

## Testing

`phcsample` is designed so the main work - edge-to-UTC labelling and PHC-to-UTC calibration - can be exercised in the existing `time/clocksim` simulation framework, at the same scope as `time/internal/syncsim`.

A sub-package `phcsample/sim`, paralleling `syncsim` in scope, drives the Generator:

- Reuses `clocksim.RawClock`, `clocksim.VirtualClock`, oscillator simulators (`ConstantDrift`, `WhiteFreqNoise`, etc.), and GPS simulators (jitter, sawtooth, excursions, outliers) unchanged.
- Reuses `syncsim`'s Go-level configuration types for oscillator, GPS, pulse, and fault (outages/excursions/outliers) parameters. There is no TOML loader; tests and evaluation scripts construct configs programmatically.
- The `phcsample`-specific configuration is intentionally a small surface compared to `phcsync`'s. `phcsync` is a servo with many knobs (PI gains, MAD thresholds, convergence criteria, tracking windows, reset thresholds); `phcsample` is essentially a window size, a stride tolerance, and the choice of regression method. A handful of fields.
- Event loop analogous to `syncsim`: merged pulse / message streams. `gen.Generate(phc, sys)` is called once per edge event, synchronously with the cross-sample the simulator produced for that edge — matching production, where `Generate` runs on edge events and `phc` is always the latest edge's PHC timestamp. The sim does not call `Generate` at independent cadences; there is no higher-rate cross-sampler in v1.
- For phase 2, the sim should also be able to inject sawtooth correction messages through the same `timemsg.Buffer` path used in production. That makes the sawtooth step directly testable: compare zero-sawtooth baseline, non-zero sawtooth with `ignoreSawtoothCorrection=true`, and non-zero sawtooth with PrePulse correction enabled. The ignored-correction case should match the uncorrected behaviour; the corrected case is not just expected to look cleaner, it should show a concrete measurable improvement over the ignored-correction case in the reported residual stats (at least stddev and/or absMax), ideally recovering toward the zero-sawtooth baseline. Sign or timescale mistakes should be obvious in the same comparison.
- The simulator's ground-truth true time is known, so the "true offset" is known exactly. Each emitted `(sys, offset)` is scored against it.
- Stats: offset-vs-true mean / stddev / absMax / Allan deviation, plus Generator-internal diagnostics (residual stddev, rejected-edge rate, estimated local rate).
- No refclock, no `rc.Sample` call, no leap-second plumbing in the sim loop. The UTC labels enter via `timemsg.Buffer` exactly as in production; `sys` is a synthesized `time.Time`; the subtraction at the end of `Generate` works identically.

No `satpulsetool` subcommand is planned for this. The package is an internal evaluation artifact; it is what lets us tune the regression window, stride tolerance, and method choice against realistic, reproducible noise and fault conditions, but it does not need to be user-facing.

Unit tests cover deterministic cases (single-edge, dual-edge, missing edges, gross outliers, missing messages, startup) on top of the same infrastructure.
