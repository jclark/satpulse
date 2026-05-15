# PHC sample mode

Issue: #256

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

// ErrNotReady indicates the Generator does not yet have enough
// labelled edges to answer. This is the expected error during startup
// and after a gap in edges or messages.
var ErrNotReady = errors.New("phcsample: not enough labelled edges")
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

The mechanism is an absolute consistency check on edge PHC timestamps. With `edgesPerPulse` known from config (1 for single-edge, 2 for dual-edge), the check is: the PHC interval between edge n and edge n - `edgesPerPulse` should be within an absolute tolerance of 1 true second.

In single-edge mode this is consecutive intervals. In dual-edge mode it is the stride-2 interval between same-polarity edges; intervals between alternating polarities contain one pulse-width and one gap duration, which are not equal to 1 second and automatically fail the check - which is how the non-timing edge gets filtered.

When a stride interval falls outside tolerance, the offending edge is not added to the window. Wait for subsequent edges to re-establish stride consistency.

The tolerance must be chosen to be tight enough that mis-labelling is impossible in practice (well under 0.5 s for the round-to-second; well under any plausible sawtooth magnitude for phase 2), and loose enough to admit every legitimately-timed edge even under worst-case PHC jitter and drift. Numbers to be determined against real hardware and the clocksim rig.

## Dual-edge timing-edge selection

In dual-edge mode only one of the two edges marks the top of the GPS second. V1 makes a deliberate product choice: because satpulse is what configures the GPS receiver, it can require that the configured pulse width be far enough from 50 percent duty that edge selection is unambiguous from timing alone. The short/long pattern of consecutive different-polarity intervals then directly reveals which edge is the rising edge (top of second): the interval less than 0.5 s is the pulse width, and the edge preceding it is the timing edge. A few good edges after startup are enough.

This is a stance about the product, not a technical limitation. Prior art: `reset.go`'s `filterEdgeListsByPulseWidth` has a message-alignment fallback for ambiguous duty cycles used by `phcsync`; that approach could be adopted here as future work if it turns out some receiver cannot be configured away from ~50% duty. V1 does not carry it forward.

Explicit `gps.pulseWidth` configuration is not required; the pattern reveals itself.

## Module structure

Module name: `phcsample` (matches the config section).

Three pieces with narrow jobs:

1. **Labelling regression**: fit a linear `tRead → UTC` model over a sliding window of `MsgUTCTime` calls. Evaluate at each edge's recovered monotonic time and round to the second to produce its label. Discard edges whose predicted UTC is too far from an integer second.

2. **PHC calibration window**: maintain recent labelled edges that pass pre-admission filtering, each with a `ptime.Time` PHC position and a UTC `time.Time` label. Apply the stride-`edgesPerPulse` consistency filter. In dual-edge mode, track which polarity is the timing edge and discard the other.

3. **PHC calibration fit**: on `Generate`, fit a linear model over the PHC calibration window (likely plain least squares; see the "PHC calibration regression" section) and evaluate at the requested PHC timestamp. Return the offset in seconds.

## Implementation design

This section pins down the internal shapes the module structure above compiles to. The first subsection captures the config surface; subsequent subsections describe individual internal types.

### Config

`phcsample.Config` reuses a **subset of `phcsync.ResetConfig` properties verbatim**. Names, units, toml tags, check constraints, and semantics match `time/internal/phcsync/reset.go`; do not introduce parallel names for things reset.go already names. The subset:

- **`PulseWindow`** — number of pulses kept for the PHC calibration window.
- **`PulseVariation`** — max PPB relative variation on the stride-`edgesPerPulse` interval check (pre-admission filter).
- **`PulseWidthDetectLimit`** — dual-edge polarity auto-detection threshold.
- **`ExpectedDelay`** — expected pulse-to-message delay. Subtracted from each `MsgUTCTime` sample's `tRead` before feeding the wallClock regression so predictions centre on the true pulse-mono time rather than lagging by the delay.
- **`DelayConfidenceWindow`** — validates the wallClock fit's implied delay (recovered from the fit) against `ExpectedDelay`.
- **`DelayVariation`** — max residual spread of wallClock fit points around the fitted line.

From `ResetConfig`, these do **not** apply:

- `StepThreshold` — phcsample does not step a clock.
- `DriftRateLimit` — phcsample does not validate candidate steps against a persisted sample.

Phcsample-specific fields are introduced only when a reset.go property cannot be reused. Open: whether the wallClock regression needs a separate window size (messages arrive at a different rate from pulses) or can derive one from `PulseWindow`; whether minimum-edges warm-up needs its own field or falls out of `PulseWindow`.

### Generator

`Generator` is the public type. All three entry points are thin pass-throughs to the internal collaborators; it owns no state beyond the collaborators themselves.

```go
type Generator struct {
    cfg Config
    wc  wallClock
    win phcWindow
    smp Sampler
    lg  *slog.Logger
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

The `wallClock` reference is passed to `TrueTimeOffset` per call rather than stored inside `phcWindow`. This keeps phcWindow free of lifetime-bound pointers and makes each call self-contained. `Pulse` never consults the wallClock — edges are recorded cheaply and all labelling work happens lazily inside `TrueTimeOffset` against the wallClock state at the moment of the query. The `pulseCorrector` argument is `nil` in phase 1 and `timemsg.Buffer`'s phase-2 `GetUTCPulseCorrection` accessor in phase 2; phase-1 code exercises the full arithmetic path with a zero correction, so phase 2 adds no new data flow.

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

Internals (buffering strategy, stride/polarity state, fit method, sub-ns carry representation) are deferred to a later design pass.

### wallClock

`wallClock` is an unexported type inside `phcsample` that models UTC as a function of monotonic time, synthesised from the `MsgUTCTime` stream. Given a monotonic instant — including a recovered edge-mono time — it returns the integer UTC second that time falls inside.

```go
// wallClock maps monotonic time to integer-second UTC from the
// MsgUTCTime stream. Internally it maintains a sliding linear
// regression over (tRead - expectedDelay, utc) pairs.
type wallClock struct {
    // captured from Config at construction in convenient internal form
    expectedDelay time.Duration // Config.ExpectedDelay as Duration
    minDelay      time.Duration // lower bound of DelayConfidenceWindow
    maxDelay      time.Duration // upper bound of DelayConfidenceWindow
    maxSpread     time.Duration // DelayVariation expressed as a Duration
    windowSize    int           // derived from PulseWindow
    // regression state follows
}

func newWallClock(cfg *Config) *wallClock

// Add observes a MsgUTCTime sample. tRead is the monotonic read
// time (with ReadDelay already subtracted by timemsg.Buffer); utc
// is the integer-second UTC reported for the corresponding pulse.
func (c *wallClock) Add(tRead, utc time.Time)

// SecondAt returns the integer UTC second the wall clock reads at
// the given monotonic instant. ok=false while the window is warming
// up or when the fit's implied delay or residual spread fails the
// DelayConfidenceWindow / DelayVariation validation.
func (c *wallClock) SecondAt(mono time.Time) (utc time.Time, ok bool)

// Reset clears the window (used on leap transition in phase 2).
func (c *wallClock) Reset()
```

**Construction and type conversion.** `newWallClock` takes `*Config` and captures only the fields it uses, converted to internal forms. `Config` is the external/TOML-facing surface and matches `ResetConfig` by using `float64` seconds and proportions; wallClock internally prefers `time.Duration` for quantities that combine with `time.Time` values. The conversion happens once, at construction — `SecondAt` and `Add` then operate entirely in `time.Duration` without re-parsing `Config` fields. (The `minDelay` / `maxDelay` pair corresponds to reset.go's `ResetConfig.DelayBounds(1.0)`.)

**Delay handling.** A message's `tRead` lags its pulse by the receiver's internal pulse-to-message delay (typically 50–250 ms). The regression operates on `(tRead - expectedDelay, utc)` pairs so predictions centre on the true pulse-mono time.

**Validation.** `SecondAt` returns `ok=false` when either gate fails:

- The fit's implied delay (recovered from the fit, back-shifted by `ExpectedDelay`) falls outside `Config.DelayConfidenceWindow` around `ExpectedDelay`.
- The residual spread of points around the fitted line exceeds `Config.DelayVariation`.

These match reset.go's identically-named checks and use identical arithmetic on the same quantities.

**Warm-up.** Under the minimum-points threshold, `SecondAt` returns `ok=false`. Threshold tied to `PulseWindow` (or a separate field, pending the open question in "Config").

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

Throughout phase 1:

- UTC time messages only (no sawtooth correction).
- Pulse-edge labels placed at the physical edge; sub-nanosecond quantization error from the receiver flows into the PHC calibration regression as noise.
- Supports single-edge and dual-edge (with the 50 percent duty restriction).
- Past-only windowed linear regression for PHC calibration (method per "PHC calibration regression"; likely plain least squares).

Steps:

3. **Implement `wallClock` and `phcsample.Config` with unit tests.** Create the `phcsample` package. Land `phcsample.Config` with the reset.go-subset fields named in "Implementation design / Config". Implement the `wallClock` type per "Implementation design / wallClock". Unit tests (using the `go-unit-test` skill) cover: warm-up gating, correct integer-second identification across typical pulse-to-message delays (50–250 ms), `ok=false` when the implied delay falls outside `DelayConfidenceWindow`, `ok=false` when residual spread exceeds `DelayVariation`, and `Reset()` behaviour. This is the first concrete phcsample code to land.

4. **Implement `Generator` and the `phcWindow` interface.** Add the public `Generator` type per "Implementation design / Generator", with its method bodies as actual pass-throughs (`MsgUTCTime` forwards to `wallClock.Add`; `Pulse` forwards to `phcWindow.Pulse`; `Generate` calls `phcWindow.TrueTimeOffset` and emits via the `Sampler`). Add the `phcWindow` surface (`Pulse`, `TrueTimeOffset`, `Reset`) with `Pulse` appending the edge to an internal buffer, `Reset` clearing it, and `TrueTimeOffset` stubbed to return `ErrNotReady`. Add `PulseEdge`, `ErrNotReady`, the `Sampler` interface, and the `pulseCorrector` interface. The tree compiles; the Generator actually routes calls; the only missing piece is `TrueTimeOffset`'s body.

5. **Build `phcsample/sim` and integration unit tests.** Implement `phcsample/sim` paralleling `syncsim` — event loop, clocksim plumbing, ground-truth scoring. Add table-driven Generator-level unit tests covering single-edge, dual-edge, missing messages, missing edges, gross outliers, and startup. Tests compile; most fail because `TrueTimeOffset` still returns `ErrNotReady`.

6. **Implement `phcWindow.TrueTimeOffset`; get tests passing.** Fills in `TrueTimeOffset`'s body. This is where the heavy lifting lives: edge labelling via the passed `wallClock` (and the phase-1-nil `pulseCorrector`), pre-admission filtering (stride-`edgesPerPulse` consistency check), dual-edge polarity selection, the PHC calibration fit, evaluation at `phc`, and combination with `sys`. To be broken down into its own sub-plan. No sawtooth correction and no leap-second handling. Tests pass; the sim rig produces clocksim statistics on par with `syncsim`'s output for `phcsync`.

7. **Wire into the daemon.** Add the `phc.freeRunning` config field. In the daemon and `time/internal/gpsevent/dispatcher.go`, add the third runtime mode (free-running): `controller == nil` stops being a synonym for serial timing; the three-way split between `controller`, `generator`, and neither becomes explicit. Wire `SetMsgUTCTimer` to the generator in free-running mode and to the dispatcher in serial mode; neither in disciplined mode.

End of phase 1: the system is usable with chrony.

### Phase 2 — refine, polish, and add sawtooth correction

8. **Add leap-second handling.** Extend `MsgUTCTimer` with `Leap(kind ptime.LeapSecondKind)`. `timemsg.Buffer` fires it on observed leap-second transitions. `phcsample.Generator` resets both regression windows on `Leap` and returns `ErrNotReady` until re-warmed. Implement the three behaviors from "Leap-second handling". Add sim-rig tests covering leap transitions.

9. **Wire the `[phcsample]` config section.** Parse the TOML `[phcsample]` section into the Generator's `Config` struct. Revisit field names, types, units, and descriptions now that the implementation constrains what's actually tunable. Update `docs/man/satpulse.toml.5.md`.

10. **Add sawtooth correction.**
    - Consume pulse-correction messages and apply `PulseOffset` to each pulse-edge label so it lands at the exact top of the second.
    - The pulse-edge label becomes (UTC top-of-second) = (message UTC) + (pulse-offset adjustment).
    - Pulse-offset lookup is done in UTC: `timemsg.Buffer` is extended to accept a UTC key for pulse-correction access. **No leap-second or TAI arithmetic enters `phcsample`** — the UTC-in / UTC-out boundary established in phase 1 is preserved.
    - Expected gain: cleaner regression input, tighter chrony convergence.
    - **Desirable but not required: preserve sub-nanosecond precision.** `TimeMsg.PulseOffset` is `*float64` nanoseconds (see `gps/gpsprot/msg.go`), so pulse-offset corrections can have fractional-ns resolution, and the exact top-of-second need not fall on a nanosecond boundary. Today's `timemsg.Buffer.validatePulseOffset` returns `time.Duration` (rounded to whole ns via `math.Round`), which discards that precision before phcsync ever sees it. The new UTC-keyed pulse-correction accessor planned for `timemsg.Buffer` should return `float64` ns rather than `time.Duration`, letting phcsample carry sub-ns precision through to the `float64`-seconds offset returned by `Generate`. This requires the internal representation to keep the pulse-offset correction as a separate `float64` alongside the nanosecond-resolution `time.Time` label, rather than folding it in and rounding. Implementing this is a quality-of-output improvement over phcsync, not a correctness requirement.

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
- Event loop analogous to `syncsim`: merged pulse / message / tick streams. On each tick (or chosen cadence) it synthesizes a `sys time.Time` and a `phc ptime.Time` from the simulator, calls `gen.Generate(phc, sys)`, and records the returned offset.
- The simulator's ground-truth true time is known, so the "true offset" is known exactly. Each emitted `(sys, offset)` is scored against it.
- Stats: offset-vs-true mean / stddev / absMax / Allan deviation, plus Generator-internal diagnostics (residual stddev, rejected-edge rate, estimated local rate).
- No refclock, no `rc.Sample` call, no leap-second plumbing in the sim loop. The UTC labels enter via `timemsg.Buffer` exactly as in production; `sys` is a synthesized `time.Time`; the subtraction at the end of `Generate` works identically.

No `satpulsetool` subcommand is planned for this. The package is an internal evaluation artifact; it is what lets us tune the regression window, stride tolerance, and method choice against realistic, reproducible noise and fault conditions, but it does not need to be user-facing.

Unit tests cover deterministic cases (single-edge, dual-edge, missing edges, gross outliers, missing messages, startup) on top of the same infrastructure.
