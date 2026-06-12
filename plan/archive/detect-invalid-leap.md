# Invalid leap-second detection

Address #175: detect the cold-start NMEA leap-second-count failure mode
once `phcsync` has locked onto a UTC-source message type, and force a
return to reset mode when it occurs. This is the UTC-time subset of
#182; buffer-overflow / no-message detection (the other half) is out of
scope here, as is TAI-source receiver discontinuity (TAI is not subject
to the stale-firmware effect).

## Goal

`phcsync` currently only consults time messages during reset. Once
locked, a cold-started NMEA receiver that subsequently decodes the
correct leap-second count will silently jump its reported UTC by an
integer number of seconds (the inevitable consequence of out-of-date
firmware updating its leap-count knowledge), and we keep tracking with
a wrong-second alignment until something else (carrier loss, drift-rate
limit, etc.) eventually trips. The check here closes that gap by
detecting the jump shortly after it occurs and forcing a fresh reset.

A real example of the failure mode is captured in
[gps/testdata/packets/u-blox/LEA-6T/coldstart.jsonl](gps/testdata/packets/u-blox/LEA-6T/coldstart.jsonl):
NMEA `GPRMC` UTC steps from `11:11:08` back to `11:11:06` after the
firmware learns the correct leap count.

## The check

The detection logic lives in `timemsg`, operating on the buffer of
recent time messages. It is parameterised by the `(tag, nativeMsgID)`
of the locked-on time message type and a duration threshold
`minDelta`, and returns a bool indicating whether a stale-firmware
leap-second event has been detected.

The check runs immediately after each buffer append, and follows this
rule:

1. The newest buffer entry must be the locked `(tag, nativeMsgID)` and
   have a populated `UTCTime`. If not, the check does nothing and
   returns false.
2. Walk backward through the buffer, skipping entries whose
   `(tag, nativeMsgID)` does not match, until a same-type entry is
   reached or the buffer is exhausted.
3. If the buffer is exhausted before finding a same-type entry, the
   check returns false.
4. If the same-type entry reached at step 2 has nil `UTCTime`, the
   check stops and returns false. It does not look further back for
   a same-type entry that does have `UTCTime`; the most recent
   same-type entry is the one being compared against, and a nil there
   means the comparison cannot be made.
5. Otherwise the check has two endpoints (newest matching entry and
   most recent prior matching entry, both with `UTCTime`) and fires
   in either of the following cases:

- **Backwards UTC step** -- the later message reports an earlier UTC
  than the earlier message. This is unambiguously a stale-firmware
  correction. Real receivers in normal operation never report UTC
  going backwards by >=1s; real positive leap seconds advance through
  `23:59:60` to `00:00:00`, and real negative leap seconds (never
  observed) would also advance.
- **Duplicate UTC** with a corrected `tRead` gap >= `minDelta` -- the
  two messages report identical UTC, and their monotonic-clock read
  times (after applying the per-message `ReadDelay` correction) are
  separated by at least `minDelta`. The only way 1 Hz messages report
  identical UTC is if firmware just subtracted one more leap second
  from GPS time. The `tRead`-gap floor discriminates against the
  receiver-double-emission case (small gap -> "suspicious duplicate",
  info log, no trigger).

Both symptoms are specific to the documented failure mode and are not
produced by serial-buffer backpressure, missed messages, or real leap
seconds, so no `kB+kA` cluster confirmation or quarter-end pre-filter
is needed. This relies on `ptime.UTCTime.Sub` treating an explicit
positive leap-second label (`TimeOfDay >= 24h`, i.e. `23:59:60`) as
one second before the next day's `00:00:00`; the detector has a
regression test for that boundary.

The function logs internally: a `Warn` on detection (one form per
case, with diagnostic context) and an `Info` on suspicious-duplicate.
The caller acts on the bool.

## Configuration

A new top-level `[sync.share]` section under `[sync]`. Mode
sub-sections (`reset`, `converge`, `track`) are mode-named; `share`
makes it explicit that these parameters are not specific to any one
mode. Two fields:

- **`detectInvalidLeap`** (bool, default true): user-facing kill switch for
  the detector. When false, the check is skipped regardless of mode.
- **`minInvalidRepeatInterval`** (float seconds, default 0.8, validated
  `>0,<1.0`): the threshold supplied to the check as `minDelta`. Below
  this interval, a repeated UTC value is treated as a receiver glitch
  rather than a leap-second correction.

The bool is independent of the threshold so users can disable the
detector without losing the threshold value, and can tune the
threshold without re-enabling. `minInvalidRepeatInterval` is a direct
duration rather than a derived `1s - jitter` so that bumping it does
not silently couple to other receiver-jitter knobs.

`Config` (in `phcsync`) gains a `Share SharedConfig` field;
`DefaultConfig` initialises it.

## Wiring

Mirrors the existing `pulseInfo` plumbing in `controller.go`. Three
concerns:

### timemsg side

`GetPostTimeMessages` gains a third return value: a closure that
performs the leap-detection check for the locked message type.

- Non-nil only when the locked type was UTC-source (i.e. the
  representative message's `TAITime.IsZero()`). For TAI-source locks
  the closure is nil: `TAITime` is already on the TAI scale and
  bypasses this UTC-staleness detector entirely. The stale-leap-second
  failure mode is by construction a UTC-side effect.
- Captures `(tag, nativeMsgID)` from the message identified as the
  epoch-start at the time `GetPostTimeMessages` returned successfully.
- Takes a `minDelta time.Duration`; returns a bool. Internally calls
  the existing buffer-side detection function.
- Adds no new state to `Buffer`; reads the current buffer contents on
  each invocation.

The `TimeMsgBuffer` interface in `phcsync` widens to include the new
return. The closure's parameter and result types are
`time.Duration` -> `bool`; no `gpsprot` / `gpsreg` types appear in the
interface. This preserves the existing layering rule that `phcsync`
does not depend on `gpsprot`.

### resetSampleGenerator

Holds the closure across the reset->converging transition. When
`genSample` produces a sample successfully, it captures the closure
returned by `GetPostTimeMessages` for the controller to extract on
mode change. Capture is gated on success so that a transient alignment
failure cannot leak a stale closure to the controller.

The generator does not bake the threshold into the closure or wrap it
further; the threshold lives in `SharedConfig` and is supplied by the
controller at call time.

A new accessor (analogous to `getPulseInfo`) exposes the captured
closure to the controller.

### Controller

Stores the captured closure. Two existing seams in `changeMode`
extend:

- On leaving reset, alongside the `pulseInfo` extraction, the
  controller pulls the closure off the reset-mode generator.
- On entering reset, the closure is cleared.

In `TimeMessage`, before processing the present sample, the controller
runs the check when all of (a) `cfg.Share.DetectInvalidLeap` is true,
(b) the closure is non-nil. If the check fires, the controller
transitions to `ModeReset` and skips sample processing for this
message. The buffer-side function logs the diagnostic; `changeMode`
logs the transition; no additional logging in `TimeMessage`.

`Tick` is not involved; the check is message-driven, so all gating
happens at message arrival.

The non-nil-closure guard handles two of the three "skip the check"
conditions (in-reset, TAI-source); the `DetectInvalidLeap` bool handles the
third (user disable).

## Testing

Algorithm-level coverage is owned by `timemsg`. A table-driven test
exercises:

- The two firing modes (backwards step and duplicate-with-real-gap),
  including the real LEA-6T sequence as one row.
- The non-firing cases: normal forward UTC, single message, empty
  buffer, last entry of the wrong type, last entry with nil
  `UTCTime`, prior entry with nil `UTCTime`, and a real positive leap
  second advancing from `23:59:60` to the next day's `00:00:00`.
- The threshold boundary (gap exactly equal to `minDelta` fires;
  strictly less does not).
- `ReadDelay` correction (raw gap above threshold but corrected gap
  below should not fire).
- Walk-back over interleaved other-type messages to find the
  matching prior entry.

Integration coverage lives in
`time/internal/phcsync/controller_test.go` and uses a fake
`TimeMsgBuffer` rather than the existing simulation suite. The
simulator (`syncsim`) generates only TAI-source messages, so it cannot
exercise the UTC-source detector path end-to-end; extending the
simulator to inject UTC discontinuities is out of scope here. A fake
buffer that satisfies the widened interface keeps the test focused on
controller wiring and is enough to cover what `timemsg`'s algorithm
test does not.

The fake exposes:

- A `GetPostTimeMessages` whose `(lastSec, tRead)` and closure are
  test-controlled. The closure can be: nil (TAI-source lock), a
  function returning false (no discontinuity), or a function returning
  true (discontinuity detected). The test swaps which between
  scenarios.
- `GetPulseCorrection` and `WaitForPulseCorrection` as no-ops
  returning zero / false; the discontinuity check does not consult
  them.

Each case drives the controller through a successful reset using a
"closure returning false" feed, then mutates the fake to set up the
condition under test and calls `c.TimeMessage()`. Assertions are on
`c.Mode()` and on whether the captured closure is still installed.

Cases:

1. **Capture and clear.** UTC-source feed (closure non-nil). Drive
   through reset success; assert the controller has captured a non-nil
   closure. Force re-entry into reset (via `c.Pause()`); assert the
   captured closure is cleared.
2. **TAI-source bypass.** Fake's `GetPostTimeMessages` returns a nil
   closure. Drive through reset success; assert no closure is
   captured. Subsequent `TimeMessage` calls do not consult the check
   even when the fake closure (held but not installed) would return
   true.
3. **End-to-end reset on discontinuity.** UTC-source feed; drive
   through reset success. Swap the fake's closure to one returning
   true. Call `c.TimeMessage()`. Assert `c.Mode() == ModeReset` and
   the captured closure has been cleared.
4. **Kill switch.** Same setup as case 3, but with
   `cfg.Share.DetectInvalidLeap = false`. The closure-returning-true is not
   consulted; mode unchanged.
5. **No-discontinuity steady state.** UTC-source feed; closure
   returning false throughout. After reset success, mode advances
   normally; closure remains captured.

Config validation: separate table-driven test in the same file.
Defaults validate cleanly; `minInvalidRepeatInterval = 0` and
`minInvalidRepeatInterval >= 1.0` are rejected; both `DetectInvalidLeap = true`
and `DetectInvalidLeap = false` are accepted.

## Interaction with the tracking-mode persisted sample

The persisted-sample / drift-rate-limit mechanism from
[reset-drift-limit.md](reset-drift-limit.md) (issue #193) caches the
last good tracking sample after `track.persistThreshold` seconds of
stable tracking (default 900 = 15 min) and uses it in reset mode to
reject any candidate alignment whose implied drift relative to the
persisted sample exceeds `reset.driftRateLimit` (default 100 ppm).
Its purpose is to prevent re-locking to a bad phase after PPS-side
faults, not to detect stale-firmware leap corrections.

This matters here because a stale-firmware leap correction would
itself look like an impossible drift (~1s of slip over the elapsed
interval, far above 100 ppm). If a persisted sample existed when the
leap detector forced a reset, the drift-rate check in reset would
then reject the corrected (1s-shifted) realignment, leaving the
controller stuck in reset.

The 900s default for `track.persistThreshold` is what closes this
gap. It was chosen to accommodate the 12.5-minute worst case for GPS
to broadcast the full leap-second subframe to a cold-started
receiver: by the time tracking has been stable long enough to
persist a sample, the firmware has already learnt the correct leap
count, so the leap detector either fires before there is a
persisted sample to block recovery, or never fires at all. The two
features therefore do not interfere in any realistic operating
regime.

## Out of scope

- Buffer-overflow / no-message detection (the other half of #182, and
  #77). Separate design.
- TAI-source discontinuity detection. The stale-firmware case is
  UTC-only; while a TAI receiver could in principle glitch, that is a
  different failure mode that can be addressed when a concrete case
  arises.
- Cross-checking against system / NTP time (#181 and the separate
  [reset-mode-ntp-check.md](reset-mode-ntp-check.md) plan).
- Time-window after reset during which the check is active. An earlier
  iteration of this plan proposed a window to silence the detector
  after firmware has had time to learn the leap; with the present
  check the false-positive rate is low enough that an unbounded window
  is acceptable, and `DetectInvalidLeap=false` is the escape hatch if needed.
- Buffer sizing. See [timemsg-buffer-window.md](timemsg-buffer-window.md);
  the new check only consults the most recent two same-type entries,
  well within any reasonable window.

## Files touched

- `time/internal/timemsg/timemsg.go` -- third return value on
  `GetPostTimeMessages`.
- `time/internal/timemsg/timemsg_test.go` -- algorithm tests.
- `gps/ptime/ptime.go` and `gps/ptime/ptime_test.go` -- ensure
  `UTCTime.Sub` handles an explicit `23:59:60` endpoint when
  subtracting across the next midnight.
- `time/internal/phcsync/controller.go` -- new `SharedConfig` type
  and `defaultSharedConfig`, widened `TimeMsgBuffer` interface,
  `Share SharedConfig` field on `Config`, captured closure on
  `Controller`, set/clear in `changeMode`, invoke in `TimeMessage`.
- `time/internal/phcsync/reset.go` -- captured closure on
  `resetSampleGenerator`, accessor.
- `time/internal/phcsync/controller_test.go` -- fake
  `TimeMsgBuffer`, integration cases for closure capture / clear /
  TAI-bypass / end-to-end reset / kill-switch, and config-validation
  cases for the new `[sync.share]` fields.
- `configs/config-schema.json` -- new `[sync.share]` section.
- Other callers of `GetPostTimeMessages` -- adapt to the new
  signature.
