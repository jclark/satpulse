# UTC time discontinuity detection

Address the UTC-time subset of #182, focused on detecting the cold-start
NMEA leap-second-count failure mode described in #175. After reset has
locked onto a UTC-source (NMEA) message type, monitor consecutive
same-type messages for an integer-second discontinuity between reported
UTC and elapsed monotonic time, and trigger a return to reset mode when
detected.

## Goal

`phcsync` currently only consults time messages during reset. Once
locked, a cold-started NMEA receiver that subsequently decodes the
correct leap-second count will silently jump its reported UTC by an
integer number of seconds (the inevitable consequence of out-of-date
firmware updating its leap-count knowledge), and we keep tracking with
a wrong-second alignment until something else (carrier loss,
drift-rate limit, etc.) eventually trips. The check here closes that
gap by detecting the jump shortly after it occurs and forcing a fresh
reset.

The check is bounded to the UTC-source case. TAI-source receivers (UBX
and friends) are not subject to the stale-firmware effect; their
`TimeMsg.TAITime` is authoritative and we do not check them here.
Buffer overflow / no-message detection (the other half of #182) is out
of scope here but would naturally live alongside this check (see
"Configuration").

## Failure mode

When firmware ships with a stale leap-second count (call it `L_fw`)
and the true count is `L`, the receiver computes `UTC = GPS - L_fw`
until it decodes the broadcast leap value, after which `UTC = GPS - L`.
At the correction event, reported UTC steps by `L_fw - L` seconds
while monotonic time keeps advancing. Across two consecutive same-type
messages spanning the correction, `|Δ_tRead - Δ_UTC|` is at least 1 s
(specifically `|L - L_fw|` seconds, integer). Sub-second jitter in
normal tracking is well under 100 ms, so an integer-second jump is a
robust signal.

The feature is: detect a sustained step of close to a nonzero integer
number of seconds in the relationship between same-type UTC reports
and elapsed monotonic time, with a leap-second exception. The
integer-second restriction is deliberate: the documented mechanism
(stale-firmware leap-count correction) is exactly integer-valued, and
restricting to that pattern avoids false alarms from sustained
non-integer offset shifts that can arise from non-GPS causes -- e.g.
serial-buffer backpressure on the host, where messages bunch and then
settle at a slightly shifted effective tRead-vs-UTC offset for
multiple messages. Resetting on those would be useless (we would
re-lock to the same source with the same offset shift) and disruptive.

## Design overview

The discontinuity check is consequential -- a positive triggers a
return to reset mode. To avoid acting on a one-off message anomaly,
the check looks for a clean *step* pattern: a configurable number of
consecutive messages at one offset, followed by a configurable number
at a different offset, with the step magnitude close to a nonzero
integer second. Detection is delayed by a few message intervals
relative to the actual transition; that delay is acceptable.

Architectural split:

- **`time/internal/timemsg/timemsg.go`** exposes a typed query
  primitive. It does not know about discontinuity, leap seconds, or
  cross-mode behaviour. `GetPostTimeMessages` returns an extra closure
  that, when invoked, returns the most recent N messages of the
  locked type as `(tReads, utcs)` slices.
- **`time/internal/phcsync/shared.go`** (new) owns the `[sync.shared]`
  config section, the `checkDiscontinuity` algorithm, and the
  `*discontinuityError` type. Other cross-mode checks (e.g. a future
  stale-msg detector) belong here too.
- **`time/internal/phcsync/controller.go`** stores the closure
  extracted from `resetSampleGenerator` on the reset->converging
  transition and invokes `checkDiscontinuity` from `TimeMessage` in
  non-reset modes.

## Configuration

New top-level section under `[sync]`. Mode sub-sections (`reset`,
`converge`, `track`) are mode-named; the new section is for cross-mode
parameters and is named `shared` to make that explicit.

```toml
[sync.shared]
discontinuityMsgVariation  = 0.2  # max msg-to-msg offset variation (s)
discontinuityCountBefore   = 2    # confirmation count at old offset
discontinuityCountAfter    = 2    # confirmation count at new offset
discontinuityWindow        = 900  # active period after leaving reset (s); 0 = unlimited
```

```go
// in time/internal/phcsync/shared.go

// SharedConfig contains tunable parameters for cross-mode checks.
type SharedConfig struct {
    // DiscontinuityMsgVariation must be < 0.5: it is the half-width
    // of the integer-second acceptance band around each integer in
    // the boundary check, so values >= 0.5 would let the bands for
    // adjacent integers (and around zero) overlap, allowing a 0.5 s
    // sub-second shift to trigger as a "1 s discontinuity".
    DiscontinuityMsgVariation float64 `toml:"discontinuityMsgVariation" check:">0,<0.5" comment:"Max msg-to-msg offset variation (s)"`
    DiscontinuityCountBefore  int     `toml:"discontinuityCountBefore"  check:">=1,<100" comment:"Same-offset msgs at old offset to confirm a discontinuity"`
    DiscontinuityCountAfter   int     `toml:"discontinuityCountAfter"   check:">=1,<100" comment:"Same-offset msgs at new offset to confirm a discontinuity"`
    DiscontinuityWindow       float64 `toml:"discontinuityWindow"       check:">=0,<=86400" comment:"Time after leaving reset during which discontinuity check is active (s); 0 = unlimited"`
}

func defaultSharedConfig() SharedConfig {
    return SharedConfig{
        DiscontinuityMsgVariation: 0.2,
        DiscontinuityCountBefore:  2,
        DiscontinuityCountAfter:   2,
        DiscontinuityWindow:       900,   // 15 min, sufficient for receiver to learn TAI offset from GPS broadcast
    }
}
```

`Config` (controller.go) gains a `Shared SharedConfig` field;
`DefaultConfig` initialises it from `defaultSharedConfig()`.

All four fields use TOML conventions native to the project: durations
are `float64` in seconds (not strings).

- **`DiscontinuityMsgVariation`**: default `0.2`. Matches the
  magnitude of `ResetConfig.DelayVariation` -- they bound the same
  physical jitter -- but is a separate parameter and is free to
  evolve independently. Bound `<0.5` is mandatory (see Go doc
  comment).
- **`DiscontinuityCountBefore`** / **`DiscontinuityCountAfter`**:
  default `2` each. The number of consecutive same-offset messages
  required at the old / new offset to confirm a step.
- **`DiscontinuityWindow`**: default `900` seconds (15 min, the
  rough timescale for a GPS receiver to learn the true TAI offset
  from the broadcast leap-second value). After this many seconds
  since leaving reset, the check goes silent until the next reset.
  `0` means "no time limit" -- the check stays active indefinitely.

Field descriptions in the toml tags above are placeholders; they
will be sharpened during implementation.

## The query primitive (timemsg side)

`GetPostTimeMessages` gains a closure return:

```go
// In time/internal/timemsg
func (buf *Buffer) GetPostTimeMessages(n int) (
    lastSec ptime.Time,
    tRead   []time.Time,
    recent  func(n int) (tReads []time.Time, utcs []ptime.UTCTime),
)
```

Semantics of `recent`:

- Non-nil only when the locked message type's `TAITime.IsZero()`
  (i.e. UTC was the source of truth for those messages). For
  TAI-source locks `recent` is nil.
- Captures `(tag, msgID) := (start.Tag, start.NativeMsgID)` from the
  message identified by `epochStartMsg` at the time
  `GetPostTimeMessages` returned successfully. `Ref` is a property
  of the message type (NMEA RMC is always `PostPulse`-like, UBX-TIM-TP
  is always `PrePulse`, etc.), so `(tag, msgID)` uniquely fixes it
  -- no separate `Ref` capture is needed.
- On invocation, walks `buf.validEntries()` from newest to oldest and
  collects the most recent `n` entries whose `msg.Tag == tag`,
  `msg.NativeMsgID == msgID`, and `msg.UTCTime != nil`. Returns
  `tReads[i] = entry.tRead.Add(-time.Duration(entry.msg.ReadDelay))`
  (matching the `ReadDelay` correction `GetPostTimeMessages` already
  applies at
  [timemsg.go:173](time/internal/timemsg/timemsg.go#L173)) and
  `utcs[i] = *entry.msg.UTCTime`, in chronological (oldest-first)
  order. If fewer than `n` matching entries are present, returns
  slices of length < `n`.
- No new state on `Buffer`: the closure reads from `buf.entries` on
  each call.
- No discontinuity logic, leap-second handling, or `phcsync`-specific
  knowledge lives in `timemsg`.

The `TimeMsgBuffer` interface in `controller.go` widens to expose
`recent`; the closure type is
`func(int) ([]time.Time, []ptime.UTCTime)`. `phcsync` already imports
`ptime`, so no new dependency. `gpsprot` / `gpsreg` types still do not
appear in the interface.

## Buffer sizing (prerequisite)

`timemsg.Buffer` keeps messages within a `readWindow` (currently
hardcoded to `5 * time.Second` at
[gpsevent/dispatcher.go:55](time/internal/gpsevent/dispatcher.go#L55)).
This is already a latent bug: reset mode requests
`cfg.Reset.PulseWindow` messages (default 5) via
`GetPostTimeMessages`, and at 1 Hz the 5 s window only just-barely-
coincidentally holds 5 messages. Slow-startup message rates or any
configured `PulseWindow > 5` would silently underfill the request.

The fix is to size the buffer from phcsync's actual needs. Adding the
discontinuity check makes the issue visible (default
`before+after = 4` plus reset's `PulseWindow = 5` and we're already
over the baseline), so the prerequisite is to land the sizing fix
**first**, covering only the existing reset need; this PR then
extends the calculation to include the discontinuity counts.

### Prerequisite PR

Introduce an exported method on `Controller`:

```go
// RequiredMsgWindow returns the minimum time-message buffer window
// the controller needs based on its current config.
func (c *Controller) RequiredMsgWindow() time.Duration {
    n := c.cfg.Reset.PulseWindow
    return time.Duration(n+2) * time.Second   // +2 s margin
}
```

Wire into both buffer construction sites:

**`gpsevent/dispatcher.go`** -- take max of the existing 5 s baseline
(for the controller-less / serial-timing path) and the controller's
reported requirement:

```go
readWindow := 5 * time.Second
if controller != nil {
    if w := controller.RequiredMsgWindow(); w > readWindow {
        readWindow = w
    }
}
timeMsgBuffer := timemsg.NewBuffer(lg, readWindow, ls, gpsprot.GPS)
```

**`time/internal/syncsim/syncsim.go`** -- has its own hardcoded
`5 * time.Second` at [syncsim.go:235](time/internal/syncsim/syncsim.go#L235),
the same latent bug. `NewController` does not depend on the buffer
existing (`SetTimeMsgBuffer` is a separate call), so reorder to
create the controller first, then size the buffer:

```go
// Create controller first
ctrl, err := phcsync.NewController(...)
if err != nil { ... }

// Size the buffer from controller requirements
readWindow := 5 * time.Second
if w := ctrl.RequiredMsgWindow(); w > readWindow {
    readWindow = w
}
timeMsgBuf := timemsg.NewBuffer(lg, readWindow, ls, gpsprot.GPS)
ctrl.SetTimeMsgBuffer(timeMsgBuf)
```

**`time/internal/gpsevent/event_log_replay.go`** and
**`time/internal/gpsevent/replay_test.go`** -- both create the
controller before the buffer, so no reorder needed; just replace the
hardcoded `5 * time.Second` (at
[event_log_replay.go:157](time/internal/gpsevent/event_log_replay.go#L157)
and [replay_test.go:30](time/internal/gpsevent/replay_test.go#L30))
with the same `max(5s, ctrl.RequiredMsgWindow())` pattern. The test
uses `DefaultConfig`, so the required window is deterministic and
larger than the previous 5 s baseline.

### This PR

Extend `RequiredMsgWindow` to also include the discontinuity counts:

```go
func (c *Controller) RequiredMsgWindow() time.Duration {
    n := c.cfg.Reset.PulseWindow
    if m := c.cfg.Shared.DiscontinuityCountBefore + c.cfg.Shared.DiscontinuityCountAfter; m > n {
        n = m
    }
    return time.Duration(n+2) * time.Second
}
```

No dispatcher change needed; the wiring landed in the prerequisite.
The `<100` upper bound on the config check stays loose -- the buffer
scales to whatever the user picks.

## ptime additions

Two helpers added to `gps/ptime/ptime.go`:

```go
// Sub returns a - b.
//
// WARNING: This computation assumes the interval [b, a] does not
// cross a leap second. If a leap second has been inserted or deleted
// between b and a, the result is off by the leap-second magnitude
// (typically 1s) because Sub is a pure date+time-of-day arithmetic
// and has no leap-second knowledge. Callers whose interval may
// straddle a leap-eligible UTC day boundary (see IsLeapEligibleDate)
// must detect that case and handle it before calling Sub.
func (a UTCTime) Sub(b UTCTime) time.Duration {
    return a.Date.Sub(b.Date) + (a.TimeOfDay - b.TimeOfDay)
}

// IsLeapEligibleDate reports whether t is a UTC date at the end of
// which a leap second can occur in IERS practice. The returned set
// is the IERS *preferred* boundaries (end of each UTC quarter:
// Mar 31, Jun 30, Sep 30, Dec 31) rather than the strict standard
// (which technically permits any month end). The narrower set is a
// deliberate choice: every leap second to date has been inserted at
// a quarter end, and over-suppressing on never-used boundaries (e.g.
// Feb 28) would mask real receiver-internal discontinuities for no
// real-world gain.
func IsLeapEligibleDate(t time.Time) bool {
    _, m, d := t.UTC().Date()
    switch {
    case m == time.March     && d == 31: return true
    case m == time.June      && d == 30: return true
    case m == time.September && d == 30: return true
    case m == time.December  && d == 31: return true
    }
    return false
}
```

## The check (phcsync/shared.go)

```go
// in time/internal/phcsync/shared.go

const leapWindow = 2 * time.Second

func checkDiscontinuity(
    recent func(n int) (tReads []time.Time, utcs []ptime.UTCTime),
    cfg    SharedConfig,
) error {
    kB, kA := cfg.DiscontinuityCountBefore, cfg.DiscontinuityCountAfter
    tReads, utcs := recent(kB + kA)
    if len(tReads) < kB+kA {
        return nil   // not enough data yet
    }

    // Quarter-end pre-filter: if any UTC in the window is within
    // leapWindow of a leap-eligible day's midnight boundary, skip --
    // we cannot distinguish a real leap from a stale-firmware
    // correction here, and ptime.UTCTime.Sub is unreliable across
    // the leap.
    for _, u := range utcs {
        if inLeapWindow(u) {
            return nil
        }
    }

    tol := time.Duration(cfg.DiscontinuityMsgVariation * float64(time.Second))

    // dRead from monotonic-bearing time.Time, dRefTime from direct
    // UTCTime subtraction (safe because we pre-filtered leap windows).
    dRead    := make([]time.Duration, kB+kA-1)
    dRefTime := make([]time.Duration, kB+kA-1)
    for i := 1; i < kB+kA; i++ {
        dRead[i-1]    = tReads[i].Sub(tReads[i-1])
        dRefTime[i-1] = utcs[i].Sub(utcs[i-1])
    }

    // step[i] = how much "more" monotonic time elapsed than UTC
    // advanced. Stable run: ~0. Stale-firmware correction at index i:
    // ~integer second.
    step := make([]time.Duration, kB+kA-1)
    for i := range step {
        step[i] = dRead[i] - dRefTime[i]
    }

    // Cluster-tightness: every intra-cluster step is small.
    // The "boundary" step lives between the before and after clusters
    // at index kB-1.
    for i := 0; i < kB-1; i++ {
        if absDur(step[i]) > tol {
            return nil   // before cluster not stable
        }
    }
    for i := kB; i < kB+kA-1; i++ {
        if absDur(step[i]) > tol {
            return nil   // after cluster not stable
        }
    }

    // Boundary step: must be close to a nonzero integer second.
    // Round to the nearest integer second and require the residual
    // within tol. A sustained non-integer offset shift (e.g. caused
    // by serial-buffer backpressure on the host) is NOT flagged --
    // the documented stale-firmware mechanism is exactly integer.
    boundary := step[kB-1]
    rounded := time.Duration(math.Round(boundary.Seconds())) * time.Second
    if rounded == 0 {
        return nil   // sub-second jitter at the boundary
    }
    if absDur(boundary-rounded) > tol {
        return nil   // sustained non-integer shift; not the target failure mode
    }

    return &discontinuityError{
        boundaryStep:    boundary,
        boundaryRounded: rounded,
        dRead:           dRead,
        dRefTime:        dRefTime,
        utcs:            utcs,
    }
}

// inLeapWindow reports whether u is within leapWindow of the end of a
// leap-eligible UTC day (i.e. a window inside which a leap second
// could be inserted or deleted, or has just been).
func inLeapWindow(u ptime.UTCTime) bool {
    if ptime.IsLeapEligibleDate(u.Date) && u.TimeOfDay > 24*time.Hour-leapWindow {
        return true
    }
    if u.TimeOfDay < leapWindow && ptime.IsLeapEligibleDate(u.Date.AddDate(0, 0, -1)) {
        return true
    }
    return false
}
```

`absDur(d)` is `time.Duration` absolute value, a file-local helper in
`shared.go`.

`*discontinuityError` is a new type in `shared.go` implementing the
`loggableError` interface (which moves from `reset.go` to
`controller.go` -- see "Other moves"). Its `log(lg)` emits at `Warn`
level with the boundary step plus the surrounding `dRead` and
`dRefTime` arrays for diagnostic context.

The function takes no logger -- it returns the error and the caller
(`Controller.TimeMessage`) does the logging via the standard
`loggableError` pattern.

## Quarter-end pre-filter

Real leap seconds occur at the end of a UTC quarter per IERS:

- **Positive leap** (insertion): `23:59:58 → 23:59:59 → 23:59:60 →
  00:00:00`. Looks like a stale-firmware correction with step ≈ −1 s.
- **Negative leap** (deletion): `23:59:57 → 23:59:58 → 00:00:00`
  (`23:59:59` omitted). Looks like a step ≈ +1 s.

Two reasons we skip the discontinuity check entirely when any UTC in
the window is within `leapWindow` of a leap-eligible day's midnight
boundary, rather than trying to apply a leap-second exception:

1. We can't reliably distinguish a real leap from a stale-firmware
   correction that happens to coincide with a quarter end. The
   alignment is essentially impossible -- firmware corrections occur
   during cold-start lock-on, not at calendar boundaries -- so this
   coincidence is not a real loss.
2. `ptime.UTCTime.Sub` (which we use directly, without going through
   `SysTime`) silently gives the wrong answer across a leap second.
   The pre-filter ensures every `Sub` we do is on a leap-free
   interval. The warning on `ptime.UTCTime.Sub` documents this
   contract.

The check is calendar-based (via `ptime.IsLeapEligibleDate`), not
consulting `buf.ls`: the Buffer's leap-second knowledge can itself be
stale during the cold-start scenario that motivates this check, so we
cannot use it to validate the exception. An away-from-quarter-end
step ≈ ±1 s is the stale-firmware correction (or some other
receiver-internal discontinuity) and gets reported.

`leapWindow` (a constant in `shared.go`, default 2 s) is wide enough
to cover `23:59:58`, `23:59:59`, the positive-leap `23:59:60`, and a
sliver after `00:00:00`. With `kB + kA ≈ 4` 1 Hz messages spanning a
~3 s window, at least one message will fall in the leap zone if the
window straddles a leap-eligible midnight.

## Closure lifecycle and storage

Mirror the existing `pulseInfo` plumbing in `controller.go`:

1. **In `resetSampleGenerator`** (`time/internal/phcsync/reset.go`):
   - Add a field `recent func(n int) ([]time.Time, []ptime.UTCTime)`.
   - In `genSample` (not `genSampleForMessages` -- only `genSample`
     calls `GetPostTimeMessages`, and `genSampleForMessages` stays
     pure for unit testing), capture the closure returned by
     `GetPostTimeMessages` and assign it to `g.recent` only after
     `genSampleForMessages` returns a non-nil sample.
   - Add an exported
     `getRecent() func(int) ([]time.Time, []ptime.UTCTime)` analogous
     to `getPulseInfo()`.

2. **In `Controller`** (`time/internal/phcsync/controller.go`):
   - Add fields `recent func(n int) ([]time.Time, []ptime.UTCTime)`
     and `recentStartSys time.Time` (the system time at which the
     check window started, used to enforce
     `cfg.Shared.DiscontinuityWindow`).
   - In `changeMode`, when leaving reset (existing
     [controller.go:347-351](time/internal/phcsync/controller.go#L347)
     block that extracts `pulseInfo`), additionally:
     `c.recent = rsg.getRecent()` and
     `c.recentStartSys = c.lastSample.Sys` (the sys time of the
     sample that caused the transition). Each successful reset
     restarts the window naturally.
   - When entering reset (the `case ModeReset:` branch), clear:
     `c.recent = nil`. (`recentStartSys` does not need clearing --
     it is only consulted while `recent != nil`.)
   - In `TimeMessage`
     ([controller.go:204](time/internal/phcsync/controller.go#L204)),
     handle window expiration up front, then run the check:

     ```go
     func (c *Controller) TimeMessage() {
         if c.recent != nil {
             win := time.Duration(c.cfg.Shared.DiscontinuityWindow * float64(time.Second))
             if win > 0 && c.lastSample.Sys.Sub(c.recentStartSys) > win {
                 c.recent = nil   // window expired
             } else if err := checkDiscontinuity(c.recent, c.cfg.Shared); err != nil {
                 if le, ok := err.(loggableError); ok {
                     le.log(c.lg)
                 } else {
                     c.lg.Warn(err.Error())
                 }
                 c.changeMode(ModeReset)
                 return
             }
         }
         sample := c.sampleGen.timeMessageSample()
         c.processPresentSample(sample)
     }
     ```

     `Tick` is not involved -- the discontinuity check is
     message-driven, so all gating happens at message arrival.
     `lastSample.Sys` is the elapsed-time reference; it is at most
     ~`sampleIntervalMax` (1.5 s) stale, negligible for a window of
     hundreds of seconds. `DiscontinuityWindow == 0` means "no time
     limit"; the check stays active until the next reset.

   - The `c.recent != nil` guard covers all three "skip the check"
     conditions: in-reset (cleared on entering reset), TAI-source
     (closure was nil from `GetPostTimeMessages`), and window-expired
     (cleared inside `TimeMessage` itself).

3. **In the `TimeMsgBuffer` interface** (`controller.go:31`): widen
   the `GetPostTimeMessages` signature to include the new return
   value. The closure type is plain `func(int) ([]time.Time,
   []ptime.UTCTime)`; no `gpsprot` / `gpsreg` types appear in the
   interface.

## Other moves

- **`loggableError` interface** moves from `reset.go` to
  `controller.go`. It is now a package-wide pattern used by `reset.go`
  and the new `shared.go`. The concrete error implementations
  (`*limitError`, `*logMsgError`) stay in `reset.go` -- they are
  only used there.

- **`shared.go` is for `[sync.shared]` only.** It is not a general
  utilities file. Future cross-mode checks (e.g. stale-msg detection)
  add their config fields to `SharedConfig` and their logic to
  `shared.go`. Package-wide infrastructure (interfaces, shared types)
  continues to live in `controller.go`.

## Testing

### `time/internal/timemsg/timemsg_test.go`

Tests on the `recent` closure returned by `GetPostTimeMessages`:

- TAI-source lock returns `recent == nil`.
- UTC-source lock returns non-nil closure.
- `recent(n)` returns at most the `n` most recent matching entries,
  in chronological order.
- Different-type interleaved messages do not appear in the result:
  `recent` only returns entries matching the locked
  `(Tag, NativeMsgID)`.
- Entries with `UTCTime == nil` are skipped.
- **`ReadDelay` correction.** Build matching entries with distinct,
  non-uniform `ReadDelay` values (e.g. 80 ms and 120 ms). Assert
  `tReads[i] == entry.tRead.Add(-time.Duration(entry.msg.ReadDelay))`
  for each. Distinct values ensure the test catches an implementation
  that skips the correction or applies a single constant.

### `gps/ptime/ptime_test.go`

Unit tests for the new pure helpers:

- `UTCTime.Sub` correct across regular midnights, regular month
  boundaries, year boundaries.
- `UTCTime.Sub` documented to be wrong across a leap second; one
  test asserts the off-by-1s behaviour to lock down the contract.
- `IsLeapEligibleDate` returns true exactly for `{Mar 31, Jun 30,
  Sep 30, Dec 31}` across several years; false for everything else.

### `time/internal/phcsync/controller_test.go` (new)

The single integration test for both wiring and algorithm. A mock
`TimeMsgBuffer` implements the widened interface; its
`GetPostTimeMessages` returns canned `(lastSec, tRead)` plus a
test-controlled `recent` closure the test can swap between scenarios.
The mock's `GetPulseCorrection` and `WaitForPulseCorrection` are
no-ops returning zero / false.

Each case drives the controller through reset success (using a
stable-cluster `recent`), then swaps `recent` and calls
`c.TimeMessage()` (and/or `c.Tick(now)`) to exercise the algorithm.
Assertions are on `c.Mode()` and `c.recent`.

**Wiring cases:**

1. **Capture and clear.** Drive through reset success on a
   UTC-source feed. Assert `c.recent != nil` after the transition.
   Drive back into reset via `c.Pause()`. Assert `c.recent == nil`.
2. **TAI-source: no check fires.** `GetPostTimeMessages` returns
   `recent == nil`. Drive through reset success. Assert
   `c.recent == nil` and that subsequent `TimeMessage` calls don't
   consult the check.

**Algorithm cases (post-reset, mock `recent` is swapped):**

3. **Stable cluster.** All offsets within `tol`. `c.Mode()`
   unchanged.
4. **Step ≈ +1 s, away from quarter end.** `c.Mode() == ModeReset`,
   `c.recent == nil`.
5. **Step ≈ -1 s, away from quarter end.** Same assertion.
6. **Step ≈ +2 s, away from quarter end.** Same assertion.
7. **Step = 1.4 s sustained, both clusters tight.** Non-integer; not
   the target failure mode. `c.Mode()` unchanged. (Serial-backpressure
   scenario.)
8. **Boundary noise.** Step just inside `1s - tol` of zero:
   unchanged. Just outside (toward 1 s): reset. Just outside
   `1s + tol` toward 2 s: unchanged.
9. **Cluster not tight.** Before-cluster has an outlier; after is
   tight. No reset.
10. **Quarter-end pre-filter.** A utc in the window falls in the leap
    zone. Even with a discontinuity-shaped step, no reset.
11. **Asymmetric `kB`/`kA`.** Configure with e.g. `before=3`,
    `after=2`. Discontinuity-shaped data still triggers correctly.

**Time-window cases:**

12. **Window expiration.** Small `DiscontinuityWindow` (e.g. 5 s).
    Drive through reset success. Advance `c.lastSample.Sys` past
    `recentStartSys + DiscontinuityWindow` (by feeding samples
    through subsequent pulse edges). Call `c.TimeMessage()` with a
    discontinuity-shaped mock `recent`. Assert `c.recent == nil` and
    `c.Mode()` unchanged (no reset on a check that was skipped due
    to expiration).
13. **`DiscontinuityWindow == 0`.** Drive through reset success;
    advance `c.lastSample.Sys` arbitrarily far. `c.recent` stays
    non-nil; a later discontinuity still triggers reset.

This single test file replaces what would otherwise be three:
direct algorithm tests on `checkDiscontinuity` (pure unit) and
`getRecent()` capture tests on `resetSampleGenerator`. The
controller-level integration covers all of it via the mock, with one
piece of test infrastructure (the fake buffer) instead of three.

**Config validation cases** (table-driven, separate `Test` function
in the same file). Most of `Config.Validate` coverage comes from the
existing `check:` tag framework; these tests pin down the boundary
behaviours that a typo could regress:

- Defaults (`DefaultConfig()`) validate cleanly.
- `discontinuityMsgVariation = 0.49`: accepted.
- `discontinuityMsgVariation = 0.5`: rejected (strict `<0.5`
  ensures integer-second trigger bands don't overlap).
- `discontinuityMsgVariation = 0`: rejected (strict `>0`).
- `discontinuityWindow = 0`: accepted ("unlimited" semantics).
- `discontinuityWindow = -1`: rejected.
- `discontinuityWindow = 86400` and `86401`: accepted / rejected,
  hitting the upper bound.
- `discontinuityCountBefore = 0`, `discontinuityCountAfter = 0`:
  each rejected (`>=1`); `1` accepted.

## Out of scope

- Buffer-overflow / no-message detection. This is the other half of
  #182 (and #77) and warrants its own design pass. It would naturally
  reuse the `recent` closure (or a sibling primitive in `timemsg`)
  and add a field to `SharedConfig`.
- TAI-source discontinuity detection. The stale-firmware case is
  UTC-only; while a TAI receiver could in principle glitch, that is a
  different failure mode that can be addressed when a concrete case
  arises.
- Cross-checking against system / NTP time (covered by #181 and the
  separate `reset-mode-ntp-check.md` plan).
- `gpsprot.TimeMsg.PrePulse` messages. `GetPostTimeMessages` already
  filters via `bestEntries`; the `recent` closure inherits that
  constraint by construction (it queries the locked type).

## Files touched

- Modified: `time/internal/timemsg/timemsg.go` -- third return value
  on `GetPostTimeMessages` (the `recent` closure). No discontinuity
  logic in this package.
- Modified: `time/internal/phcsync/controller.go` -- widened
  `TimeMsgBuffer` interface, new `Shared SharedConfig` field on
  `Config` (with `defaultSharedConfig` wired into `DefaultConfig`),
  new `recent` and `recentStartSys` fields on `Controller`,
  set/extract on reset->converging in `changeMode`, clear on
  entering reset, invoke `checkDiscontinuity` in `TimeMessage` with
  `DiscontinuityWindow`-based expiration up front, extend
  `RequiredMsgWindow` to include the discontinuity counts.
  `loggableError` interface moves here from `reset.go`.
- Modified: `time/internal/phcsync/reset.go` -- new `recent` field on
  `resetSampleGenerator`, captured in `genSample` (after
  `genSampleForMessages` returns a non-nil sample), exposed via new
  `getRecent()` method. `loggableError` moves out (concrete
  `*limitError`/`*logMsgError` stay).
- New: `time/internal/phcsync/shared.go` -- `SharedConfig`,
  `defaultSharedConfig`, `checkDiscontinuity`, `*discontinuityError`,
  `inLeapWindow`, file-local helpers (`absDur`).
- Modified: `gps/ptime/ptime.go` -- new `UTCTime.Sub` method (with
  leap-second warning) and `IsLeapEligibleDate` function.
- Modified: `gps/ptime/ptime_test.go` -- tests for the two new
  helpers.
- Modified: `time/internal/timemsg/timemsg_test.go` -- new tests for
  `recent` (closure semantics + ReadDelay correction).
- New: `time/internal/phcsync/controller_test.go` -- single
  integration test covering wiring (closure capture, clear,
  TAI-source bypass, time-window expiration) and algorithm coverage
  (cluster checks, integer-second boundary, quarter-end pre-filter)
  via a mock `TimeMsgBuffer`.
- Modified: `time/internal/gpsevent/dispatcher.go` -- size the time
  message buffer from `controller.RequiredMsgWindow()` (with 5 s
  floor preserved). [Lands in prerequisite PR.]
- Modified: `time/internal/syncsim/syncsim.go` -- reorder to create
  the controller before the buffer; size buffer from
  `ctrl.RequiredMsgWindow()`. [Lands in prerequisite PR.]
- Modified: `time/internal/gpsevent/event_log_replay.go` -- size
  buffer from `ctrl.RequiredMsgWindow()` (with 5 s floor). [Lands in
  prerequisite PR.]
- Modified: `time/internal/gpsevent/replay_test.go` -- same.
  [Lands in prerequisite PR.]
- Modified: `time/internal/gpsevent/event_log_replay.go` and any
  other callers of `GetPostTimeMessages` -- adapt to the new
  signature (likely just discard the third return value).
- Modified: `configs/config-schema.json` -- new `[sync.shared]`
  section with four properties (`discontinuityMsgVariation`,
  `discontinuityCountBefore`, `discontinuityCountAfter`,
  `discontinuityWindow`), matching `SharedConfig` toml tags and
  `check` constraints.
