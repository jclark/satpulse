# Reset mode NTP cross-check

Address #181: cross-check the GPS-derived second chosen during reset mode
against an externally supplied wall-clock estimate (typically NTP), so
that occasional wrong-second picks from the GPS pipeline (#175 cold-start
NMEA leap-second errors, buffered-message delays) can be detected and
optionally rejected.

## Goal

When reset mode produces a candidate sample whose `Sample.Ref` (TAI) is
about to be committed, validate that the candidate's chosen second is
consistent with an independent wall-clock estimate. The check operates
at integer-second granularity: it does not attempt to validate sub-second
precision (PPS+GPS is the higher-precision source).

## Layering

`phcsync` must not import `gpsprot`. The existing `time/internal/timemsg`
package is the canonical example: `phcsync` defines its own narrow
interface (`TimeMsgBuffer`) in pure `time`/`gps/ptime` terms; `timemsg`
imports `gpsprot` and adapts. We follow the same pattern.

## The interface

`phcsync` exposes a single-method interface; the caller asks for a TAI
estimate at a chosen monotonic instant, and supplies the leap-second
state to use. All projection, offset-source selection, and UTC->TAI
conversion is the implementation's responsibility -- `phcsync` consumes
TAI time and accuracy, nothing else.

```go
// in time/internal/phcsync
type TimeEstimater interface {
    // EstimateTime returns an estimate of TAI time at the given monotonic
    // instant and the estimate's stated accuracy. ls is the leap-second
    // state the caller considers authoritative; the implementation may
    // use it to derive the TAI-UTC offset when its own source is
    // unavailable or unreliable. ok is false when no usable estimate
    // is available (e.g. NTP unsynchronised, in a leap-second window);
    // callers must abstain in that case.
    EstimateTime(at time.Time, ls ptime.LeapSecond) (t ptime.Time, accuracy time.Duration, ok bool)
}
```

`reset.go` calls this with `sample.Sys` (the monotonic instant of the
pulse) and the controller's leap-second state. No projection, leap-second
arithmetic, or TAI conversion lives in `reset.go`.

## Configuration

Two new fields in `ResetConfig` ([time/internal/phcsync/reset.go:17](time/internal/phcsync/reset.go#L17)),
flat (no nested struct):

```go
NTPCheckLevel       NTPCheckLevel `toml:"ntpCheckLevel" comment:"NTP cross-check posture"`
NTPDiscrepancyLimit float64       `toml:"ntpDiscrepancyLimit" check:">=0.0,<1.0" comment:"NTP cross-check slack added to Accuracy (s)"`
```

`NTPDiscrepancyLimit` is the additional slack, in seconds, added to the
time estimate's stated accuracy to form the threshold above which the
candidate's reference time is judged to disagree with the time estimate
by enough to indicate a wrong-second pick and is rejected.

`NTPCheckLevel` is a typed int with `UnmarshalText`/`MarshalText`/
`String`, so the toml decoder converts the string in the file directly
to the typed value (no string field on the config struct, no string
handling in `phcsync` proper):

```go
type NTPCheckLevel int

const (
    NTPCheckNone       NTPCheckLevel = iota // do not run the check
    NTPCheckWarn                            // run the check; log warning on contradiction; reset still succeeds
    NTPCheckConsistent                      // estimate must not contradict; reset stays in reset on contradiction; abstain if estimate too imprecise
    NTPCheckConfirm                         // estimate must affirmatively confirm; both Accuracy and discrepancy within NTPDiscrepancyLimit
)

func (l NTPCheckLevel) String() string                 { ... }
func (l NTPCheckLevel) MarshalText() ([]byte, error)   { ... }
func (l *NTPCheckLevel) UnmarshalText(text []byte) error { ... } // returns error on unknown value
```

Defaults in `defaultResetConfig`:
- `NTPCheckLevel: NTPCheckConsistent`
- `NTPDiscrepancyLimit: 0.4`

Schema entries in `configs/config-schema.json` matching the existing
`ResetConfig` style: `string` with `enum`
(`"none"`/`"warn"`/`"consistent"`/`"confirm"`) for the level (the JSON
schema sees the text form, since the toml decoder uses
`UnmarshalText`); `number` with `minimum`/`exclusiveMaximum` for the
limit.

## The check

The check is part of *sample generation*, not sample processing. All
existing reset-time validations (`checkPulseIntervals`,
`checkAlignment`, etc.) live on `resetSampleGenerator` and run inside
`genSampleForMessages`. The chosen-second check is conceptually another
such validation -- "is the second this candidate represents
plausible?" -- and belongs alongside the others. The processor
(`resetSampleProcessor`) is unchanged.

A new function on `resetSampleGenerator`, `checkChosenSecond`, is
called from `genSampleForMessages` after a candidate sample has been
built but before it is returned. `genSampleForMessages` does no
logging itself (so it remains directly unit-testable); all logging
happens in `genSample`, which inspects the returned error.

The check has four possible outcomes, distinguished by the returned
error:

1. **Pass / disabled.** Return `(sample, stats, nil)`. Same as today.
2. **Warn-only rejection** (`NTPCheckWarn` and the disagreement
   exceeds threshold). Return `(sample, stats, ntpWarnErr)`.
   `ntpWarnError` is a `loggableError` whose `log` emits at Warn
   level. `genSample` logs and **still returns the sample**.
   Reset succeeds.
3. **Imprecise abstain** (`NTPCheckConsistent` and
   `accuracy + NTPDiscrepancyLimit >= 1s`). Return
   `(sample, stats, ntpAbstainErr)`. `ntpAbstainError` is a
   `loggableError` whose `log` emits at Info level. `genSample`
   logs and **still returns the sample**. Reset succeeds.
4. **Hard rejection** (`NTPCheckConsistent` /
   `NTPCheckConfirm` failure). Return `(nil, nil, err)` where
   `err` is a `loggableError` describing the failure. `genSample`
   logs and returns nil -- the existing "discard, wait for more
   data" path. Reset stays in reset.

Cases (2) and (3) share the "log + keep sample" behaviour and are
distinguished from (4) by a marker interface:

```go
// keepSampleError marks errors that genSample should log but treat as
// non-fatal (sample still flows through).
type keepSampleError interface {
    error
    keepSample()
}
```

`*ntpWarnError` and `*ntpAbstainError` both implement
`keepSample()`. Other `loggableError`s (existing `*limitError`,
`*logMsgError`) do not -- they remain hard-rejection.

```go
// inside genSampleForMessages, after the alignment loop builds sample:

err := g.checkChosenSecond(sample)
if err != nil {
    if _, ok := err.(keepSampleError); ok {
        return sample, stats, err   // log-and-keep: warn or abstain
    }
    return nil, nil, err            // hard rejection: discard
}
return sample, stats, nil
```

```go
// in genSample, the existing loggableError dispatch already handles
// logging for both ntpWarnError (Warn) and ntpAbstainError (Info)
// since both implement loggableError. The only change is to avoid
// returning nil when the error is a keepSampleError:

sample, stats, err := g.genSampleForMessages(lastSec, tRead)
if err != nil {
    if le, ok := err.(loggableError); ok {
        le.log(g.lg)
    } else if errors.Is(err, errLastPulseNoMessage) {
        g.lg.Debug(err.Error())
    } else if !errors.Is(err, errNotEnoughTimestamps) {
        g.lg.Info(err.Error())
    }
    if _, ok := err.(keepSampleError); !ok {
        return nil
    }
    // keepSampleError: fall through to success log + return sample
}

if sample != nil {
    g.lg.Info("reset mode succeeded", ...)
}
return sample
```

```go
// checkChosenSecond validates that sample.Ref (the GPS-derived TAI
// second) is consistent with the externally supplied time estimate.
// Returns nil if the check passes, the check is disabled, or no
// estimate is available (silent abstain). Returns *ntpWarnError
// (warn case, keeps sample) or *ntpAbstainError (imprecise abstain
// at NTPCheckConsistent, keeps sample) for log-and-keep paths.
// Returns another loggableError when the chosen second is rejected
// and reset should stay in reset.
func (g *resetSampleGenerator) checkChosenSecond(sample *Sample) error {
    if g.cfg.NTPCheckLevel == NTPCheckNone || g.timeEst == nil {
        return nil
    }
    limitDur := time.Duration(g.cfg.NTPDiscrepancyLimit * float64(time.Second))

    estTAIAtPulse, accuracy, ok := g.timeEst.EstimateTime(sample.Sys, g.leapSecond)
    if !ok {
        if g.cfg.NTPCheckLevel == NTPCheckConfirm {
            return &logMsgError{ /* "could not confirm chosen second: no estimate" */ }
        }
        return nil
    }

    discrepancy := absDuration(time.Duration(sample.Ref - estTAIAtPulse))

    switch g.cfg.NTPCheckLevel {
    case NTPCheckWarn:
        if discrepancy > accuracy + limitDur {
            return &ntpWarnError{discrepancy, accuracy, limitDur, sample.Ref, estTAIAtPulse}
        }
        return nil
    case NTPCheckConsistent:
        threshold := accuracy + limitDur
        if threshold >= time.Second {
            // estimate too imprecise to distinguish neighbouring seconds; abstain.
            // genSample logs at Info level and keeps the sample.
            return &ntpAbstainError{accuracy: accuracy, limit: limitDur}
        }
        if discrepancy > threshold {
            return &logMsgError{ /* "reset sample rejected: NTP cross-check indicates wrong second" */ }
        }
    case NTPCheckConfirm:
        if accuracy > limitDur {
            return &logMsgError{ /* "could not confirm chosen second: estimate too imprecise" */ }
        }
        if discrepancy > limitDur {
            return &logMsgError{ /* "reset sample rejected: NTP cross-check indicates wrong second" */ }
        }
    }
    return nil
}

// ntpWarnError is a keepSampleError + loggableError for the warn case.
// log emits at Warn level.
type ntpWarnError struct {
    discrepancy time.Duration
    accuracy    time.Duration
    limit       time.Duration
    sampleRef   ptime.Time
    estTAI      ptime.Time
}

func (e *ntpWarnError) Error() string { return "NTP cross-check indicates wrong second" }
func (e *ntpWarnError) keepSample() {}
func (e *ntpWarnError) log(lg *slog.Logger) {
    lg.Warn(e.Error(),
        "discrepancy", e.discrepancy,
        "accuracy", e.accuracy,
        "limit", e.limit,
        "sampleRef", e.sampleRef,
        "estTAI", e.estTAI)
}

// ntpAbstainError is a keepSampleError + loggableError for the
// imprecise-abstain case at NTPCheckConsistent. log emits at Info level.
type ntpAbstainError struct {
    accuracy time.Duration
    limit    time.Duration
}

func (e *ntpAbstainError) Error() string { return "NTP cross-check abstaining: estimate too imprecise" }
func (e *ntpAbstainError) keepSample() {}
func (e *ntpAbstainError) log(lg *slog.Logger) {
    lg.Info(e.Error(), "accuracy", e.accuracy, "limit", e.limit)
}
```

The `nil`-`TimeEstimater` case is short-circuited at the top of the
function: it behaves identically to `NTPCheckNone`. No stub needed
on the controller side.

Tests on `genSampleForMessages` can assert on returned error types
without involving a logger.

### Rationale for the threshold formulae

For `NTPCheckWarn` and `NTPCheckConsistent`: the estimate says true UTC
at the pulse is in `[estTAIAtPulse - accuracy, estTAIAtPulse + accuracy]`.
The candidate is consistent with that interval iff
`|discrepancy| <= accuracy`. We add `NTPDiscrepancyLimit` of
user-configured slack to absorb system-clock noise, scheduling jitter,
and over-confident NTP. If `|discrepancy|` exceeds
`accuracy + NTPDiscrepancyLimit`, the disagreement is real and large
enough to indicate a wrong-second pick.

The `threshold >= 1s` abstain rule for `NTPCheckConsistent`: when the
threshold is at least one full second, the check cannot distinguish the
right second from a neighbour, so it has nothing useful to say.

For `NTPCheckConfirm`: the user is asking for affirmative confirmation,
not just absence of contradiction. The estimate must be precise enough
to pin down the second on its own (`accuracy <= NTPDiscrepancyLimit`),
and the candidate must agree (`discrepancy <= NTPDiscrepancyLimit`).
At default `NTPDiscrepancyLimit = 0.4s`, a wrong-by-1s candidate yields
`discrepancy >= 0.6s` (lower bound assumes maximum-allowed accuracy of
0.4s), well above the 0.4s threshold -- rejected. A right-second
candidate has `discrepancy <= accuracy <= 0.4s` -- passes. The 0.2s
margin between right-second-acceptance and wrong-second-rejection at
the boundary is the design comfort.

## Wiring

### `phcsync.NewController`

Add a `TimeEstimater` parameter to the constructor (after `gm`, before
`cfg`). The parameter is allowed to be `nil`; passing `nil` disables
the cross-check entirely regardless of `NTPCheckLevel`.

Stored on the `Controller`, threaded into `newResetSampleGenerator`
on every `changeMode(ModeReset)` along with `c.leapSecond`. The
generator stores both and passes the leap-second to
`timeEst.EstimateTime` on each check. The processor
(`resetSampleProcessor`) is unchanged.

### Mid-reset leap-second updates

`Controller.LeapSecond` ([controller.go:266](time/internal/phcsync/controller.go#L266))
must also push the new value into the active reset generator's cached
field (mirroring the type-assertion pattern already used at
[controller.go:348](time/internal/phcsync/controller.go#L348)):

```go
func (c *Controller) LeapSecond(ls ptime.LeapSecond) {
    c.leapSecond = ls
    c.lg.Debug("leap second updated", "leapSecond", ls)
    if rsg, ok := c.sampleGen.(*resetSampleGenerator); ok {
        rsg.leapSecond = ls
    }
    c.gmUpdate()
}
```

Reasoning: the initial `c.leapSecond` may be the config default (or a
stale `OffChangeTime`) until the receiver delivers a leap-second
message; if that arrives mid-reset, the generator's cached value would
otherwise stay stale and the `ntpTimeEstimater` fallback would compute
a wrong TAI offset. Recreating the generator on each leap update would
discard accumulated reset state, which is unacceptable; pushing the
update into the live cache preserves the state. The `sampleGenerator`
interface is not widened -- the type assertion is local to the
controller.

### Daemon-side implementation

The implementation lives in a new `time/app/daemon/systime.go`,
keeping all NTP / kernel knowledge out of `time/internal/phcsync`.
It does not go in `internal/gpscmd/systime.go` -- that package is for
the `satpulsetool` CLI, and the estimater is daemon-only. The existing
`gpscmd.EstimateSystemTime` stays where it is. Sketch:

```go
type ntpTimeEstimater struct{}

func (e *ntpTimeEstimater) EstimateTime(at time.Time, ls ptime.LeapSecond) (ptime.Time, time.Duration, bool) {
    pre := time.Now()
    state, err := ntptime.Get()
    post := time.Now()
    if err != nil || !state.Synchronized {
        return 0, 0, false
    }
    captureMono := pre.Add(post.Sub(pre) / 2)
    estUTCAtAt := state.Time.Add(at.Sub(captureMono))

    // Abstain when estUTCAtAt falls inside the leap-second window of a
    // pending leap, since state.TAIOffset is ambiguous there.
    todUTC := estUTCAtAt.Sub(estUTCAtAt.Truncate(24 * time.Hour))
    var leapEnd time.Duration
    switch state.LeapSecondStatus {
    case ntptime.LeapSecondInsTonight:
        leapEnd = 24 * time.Hour              // positive: [23:59:60, 24:00:00)
    case ntptime.LeapSecondDelTonight:
        leapEnd = 24*time.Hour - time.Second  // negative: [23:59:58, 23:59:59)
    case ntptime.LeapSecondInProgress:
        return 0, 0, false
    }
    if leapEnd > 0 && todUTC >= leapEnd-time.Second && todUTC < leapEnd {
        return 0, 0, false
    }

    // Decide offset source. Trust the kernel TAIOffset only when it has
    // been configured (non-zero). Otherwise, fall back to the
    // caller-supplied leap-second state via SysToTime.
    var estTAI ptime.Time
    if state.TAIOffset != 0 {
        estTAI = ptime.Time(estUTCAtAt.UnixNano() + int64(state.TAIOffset)*int64(time.Second))
    } else {
        var ok bool
        estTAI, ok = ls.SysToTime(estUTCAtAt)
        if !ok {
            return 0, 0, false
        }
    }

    // accuracy is the kernel's MaxError, already aged by the kernel
    // up to the moment of the syscall. We do not age further: the
    // caller's `at` is sub-millisecond after `captureMono` in practice.
    return estTAI, state.MaxError, true
}
```

The kernel ages its internal `maxerror` per `MAXFREQ` (typically
500 ppm) between syscalls; calling `ntp_adjtime` on every check
ensures we get the correctly-aged value. Cost: one syscall per
sample-process call, bounded by the reset-sample cadence (a few per
second at most). No state stored on the estimater between calls.

The estimater is constructed in
[time/app/daemon/daemon.go:301](time/app/daemon/daemon.go#L301)
(`NewDispatcher`) and passed to `phcsync.NewController` at
[time/app/daemon/daemon.go:306](time/app/daemon/daemon.go#L306).

### syncsim

`syncsim.Simulate` ([time/internal/syncsim/syncsim.go:248](time/internal/syncsim/syncsim.go#L248))
supplies a fake `TimeEstimater` (or `nil`) when constructing the
controller. Default fake returns `ok=false` (preserves current
behaviour: no check). New test scenarios construct a fake that returns
controlled `(ptime.Time, accuracy, true)` to exercise the new check
paths.

## `gpsprot.TimeEstimate`

No structural change. It already serves the receiver-config / u-blox
MGA path correctly, including its `Trusted` field and
`LeapSecondState`. We do not introduce a `ptime.TimeEstimate` value
type -- the new design uses an interface call returning TAI directly,
not a value-typed estimate. `gpsprot.TimeEstimate` and the existing
`EstimateSystemTime` ([internal/gpscmd/systime.go:12](internal/gpscmd/systime.go#L12))
remain as-is.

## Logging

- Wrong-second detected (`NTPCheckWarn`, warn level): `"NTP cross-check
  indicates wrong second"` with `discrepancy`, `accuracy`, `limit`,
  `sampleRef`, `estTAIAtPulse`.
- Wrong-second detected (`NTPCheckConsistent`/`NTPCheckConfirm`,
  info level, reject): `"reset sample rejected: NTP cross-check
  indicates wrong second"` with the same fields. Same log shape as
  the existing drift-rate-limit rejection
  ([reset.go:771](time/internal/phcsync/reset.go#L771)).
- Estimate too imprecise (`NTPCheckConsistent` abstain, info level):
  `"NTP cross-check abstaining: estimate too imprecise"` with
  `accuracy`, `limit`. Logged every time the abstain occurs; reset
  succeeds and the generator is destroyed shortly after, so the log
  fires at most once per reset.
- `NTPCheckConfirm` rejection due to no estimate or imprecise estimate
  (info level): `"reset sample rejected: NTP cross-check could not
  confirm chosen second"` with reason. Logged every time, matching
  existing rejection logging in this file.

## Testing

### Unit tests in `phcsync` (`reset_test.go`)

Tests use a fake `TimeEstimater` with controlled return values.

- `NTPCheckNone`: check never fires regardless of estimate.
- `NTPCheckWarn`: contradiction logs warning, sample passes through.
- `NTPCheckConsistent`:
  - matching estimate -> pass
  - wrong-by-1s estimate within precision -> reject (stay in reset)
  - wrong-by-1s estimate but `accuracy + limit >= 1s` -> abstain, pass
  - `ok=false` -> abstain, pass
- `NTPCheckConfirm`:
  - matching estimate, tight accuracy -> pass
  - matching estimate, accuracy > limit -> reject
  - `ok=false` -> reject
  - wrong-by-1s estimate, tight accuracy -> reject

### Integration tests in `syncsim`

Add scenarios under `time/internal/syncsim/`:
- Inject wrong-second condition in the simulated GPS feed; verify
  `NTPCheckConsistent` keeps reset, `NTPCheckWarn` allows but logs,
  `NTPCheckNone` is silent.
- Verify `NTPCheckConfirm` requires precise estimate.

The existing `synctest`-based tests in `phcsync` cover the
sample-processing path; the new tests slot in alongside.

## Out of scope

- Converging->tracking transition cross-check (mentioned in #181).
  Defer; the reset-time check addresses #175 and the buffered-message
  failure mode, which are the concrete cases on the table.
- Background NTP client / network NTP source. The `TimeEstimater`
  interface is general enough to support this in future without
  changes to `phcsync`.
- Daemon-side config to enable/disable `ntpTimeEstimater`. Initial
  cut wires it unconditionally; if the chrony-as-refclock circularity
  becomes a concern in practice, add a daemon config knob then. The
  per-mode `NTPCheckLevel = NTPCheckNone` already lets the user opt
  out without removing the source.
- Fixing `EstimateSystemTime`'s blind use of kernel `TAIOffset` for
  the u-blox MGA path. That is a pre-existing latent bug and is left
  alone here.

## Files touched

- Modified: `time/internal/phcsync/reset.go` -- `TimeEstimater`
  interface, `NTPCheckLevel` typed int with marshalling, two new
  `ResetConfig` fields, `checkChosenSecond` and its call site in
  `genSampleForMessages`, level constants. New parameters on
  `newResetSampleGenerator` for `timeEst` and `leapSecond`.
- Modified: `time/internal/phcsync/controller.go` -- new constructor
  arg, `nil` allowed, threaded into `newResetSampleGenerator`.
- New: `time/app/daemon/systime.go` -- `ntpTimeEstimater` type and
  `EstimateTime` method.
- Modified: `time/app/daemon/daemon.go` -- construct `ntpTimeEstimater`,
  pass to `phcsync.NewController`.
- Modified: `time/internal/syncsim/syncsim.go` -- pass a fake or `nil`
  `TimeEstimater` to `NewController`.
- Modified: `time/internal/gpsevent/replay_test.go` and
  `time/internal/gpsevent/event_log_replay.go` -- pass `nil`
  `TimeEstimater` at the existing `NewController` call sites
  ([replay_test.go:24](time/internal/gpsevent/replay_test.go#L24),
  [event_log_replay.go:150](time/internal/gpsevent/event_log_replay.go#L150)).
- Modified: `configs/config-schema.json` -- new `ntpCheckLevel`
  (string enum) and `ntpDiscrepancyLimit` (number) fields under
  `sync.reset`.
- New tests: `time/internal/phcsync/reset_test.go` additions; possibly
  syncsim scenario additions.
