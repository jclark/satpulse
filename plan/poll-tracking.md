# Simplify the serial PPS poll tracking loop (#TBD)

Related: #460 (GPIO polling reuses this loop).

## Overview

The poll method in `gps/app/pps/poll.go` finds PPS leading edges by
repeatedly reading a pin around the time the next pulse is expected. Its
tracking phase adapts a polling window to the measured need. The original
adaptation had a set of interacting rules whose combined behaviour
was hard to predict, and on 2026-09-20 they combined to withhold every
sample from chrony for 5 min 27 s while the pulse was caught every second.

The implemented redesign uses a simpler tracking controller. The window
no longer sets the polling resolution; it is only coverage. A catch can
never widen it, only misses can. The poller marks catches anomalous when
their outer width exceeds four times the median of recent tracking widths.
The consumer forwards a catch only when it is not anomalous and the larger
of its two uncertainty components is within a fixed limit. The anomaly flag
has no effect on tracking or acquisition.

"Revisions made" records completed changes. "Current algorithm" describes
the implementation. "Remaining fixes and validation" records unfinished
work; "Possible follow-up work" records optional algorithm investigations.
The remaining sections preserve the historical design rationale and test
evidence.

## Revisions made

### Tracking controller

Tracking cadence is independent of extent. Catches cannot widen the
extent; misses double it; catches shrink it at 31/32, stopping at eight
midpoint-to-midpoint brackets. The first redesign replaced settled-bracket
history with local rejection tests and removed `Settled`. The subsequent
anomaly-flag revision below has removed those local tests and `Rejected`.

### Asymmetric uncertainty and paired poll widths

The old midpoint-to-midpoint interval understated the possible edge error.
In a recorded Mac run, an edge 1.33 ms late was forwarded with scalar
uncertainty of 746 us. The interface now reports the full outer interval.

`CandidateEdge.Uncertainty` is now `[2]time.Duration`, in `[before, after]`
order. The timestamp remains the midpoint of the two poll midpoints. The
uncertainty components are measured directly from that timestamp to
`prev.start` and `cur.end`, so they report the outer interval without
requiring the estimator to be its midpoint. This bounds the physical edge
when each call samples fresh pin state; it does not include unmeasured
delay from cached status.

The two poll durations are now `CandidateEdge.PollWidths`, also
`[2]time.Duration`, in `[start, end]` order: the off poll followed by the on
poll. The serial tool, `sysPulseEdge` event records and simulator edge
records report `uncertainty: [before, after]` and `pollWidths: [start, end]`
in seconds. The serial tool and event records omit zero pairs for the wait
and kernel methods. These replace the scalar uncertainty and the separate
`startPollWidth` and `endPollWidth` fields. The serial tool's JSON timestamp
now preserves nanoseconds, so rounding the timestamp does not shift the
reported interval; human-readable timestamps still round to microseconds.

The consumer and simulator apply the existing uncertainty limit to the
larger component. Interval validity now requires ordered query endpoints and an outer width strictly between
zero and one period. The controller retains midpoint separation as its
`bracket`, with the same shrink constant and half-error prediction
correction.

### Anomaly flag replaces rejected catches

Implemented: the poller now has only caught and missed outcomes.
`CandidateEdge.Anomalous` replaces `Rejected`, and the serial tool, event
log and simulator expose `anomalous` instead of `rejected`. The local
duration/gap tests, their sleep
gate, opening-read exception and `MinSpacing` floor are removed. There is
no rejected-catch tracking event or failure path.

For outer width `W = before + after`, the poller sets:

```
Anomalous = W > 4 * median(previous tracking widths)
```

The history holds the previous 31 valid tracking catches. Classification
uses the history before insertion; every valid tracking width is then
inserted, including anomalous and over-limit catches. The available history
is used immediately, and an empty history gives `Anomalous = false`.
Acquisition catches are classified against any existing history but never
inserted. Misses and reacquisition retain the history. Its age is measured
in tracking catches, not elapsed seconds.

All catches follow the same prediction and extent rules, reset tracking
failures, and allow acquisition to progress. Only misses count toward the
failure limit. The consumer forwards on
`!Anomalous && max(Uncertainty[0], Uncertainty[1]) <= U`.

The implementation directly imports `time/lib/median` and uses its
`Window[time.Duration]`. This is an intentional temporary exception to the
normal dependency direction; the library has not been moved.
The dependency fix is tracked by [#468](https://github.com/jclark/satpulse/issues/468),
which moves `time/lib/median` to `common/lib/median`.

A replay of recorded Mac catches motivated the simple factor of four.
With 31 previous tracking widths, it withheld seven of 585 catches in the
loaded run, with maximum forwarded absolute error 318 us and a six-second
gap including four recorded misses. It withheld none of 591 quiet catches.
In a snapshot of 8,354 tracking catches from the longer run, it withheld
seven; a factor of two withheld 177 and created a seven-second gap without
improving the maximum forwarded error. These compare forwarding gates on
identical recorded catches, not the changed controller's future behaviour.

## Current algorithm

### Objective

Deliver good edge timestamps to the time consumer with short gaps between
them. An occasional missed pulse is acceptable; a bad timestamp forwarded
is not. Long-term CPU must stay small.

### State

- `prediction`: monotonic time of the next expected leading edge.
- `extent`: width of the window polled around the prediction.
- `failures`: count of consecutive misses.
- `widths`: recent tracking widths, used only for anomaly classification.

### Recorded per query

These are recorded today, in `reading` and `poll`:

- `sched`: when the query was scheduled to start.
- `start`, `end`: when it actually started and returned.
- `slept`: whether the wait for `sched` used a timer. False when `sched`
  was already past, or nearer than the platform can sleep to.
- `value`: pin on or off.

Short intervals, including query durations, gaps and the bracket, use the
existing `clockReading` abstraction. Each platform uses the reading
appropriate to it: the one `time.Now` reading where it carries both wall
and monotonic time, and on Windows the precise measurement stamp rather
than the coarse monotonic reading.

### Polling cadence

Unchanged. Each query is scheduled at the previous query's start plus
`MinSpacing`. The wait sleeps where the platform can and returns at once
where it cannot: on Linux, sub-millisecond waits are truncated to zero,
because the runtime would otherwise round them up to a millisecond. The
loop is therefore timer-paced on hosts that can sleep briefly and
query-paced elsewhere, including on macOS with a USB serial adapter whose
query already exceeds `MinSpacing`. There is no spinning. `MinSpacing` is
a CPU saving where it can be honoured and nothing where it cannot.

### One attempt, once per period

1. `open = prediction - extent/2`, `close = prediction + extent/2`. Wait
   until `open`, with `PreWarm` as today.
2. Poll as today: if the pin is on at the first query, poll through the
   in-progress pulse; then poll until an off-to-on transition is seen or
   the query midpoint passes `close`. A transition takes precedence over
   the deadline. For a valid catch, `classify` requires ordered query
   endpoints and `0 < cur.end - prev.start < period`.
3. The attempt is a **miss** if no valid transition is seen, otherwise a
   **catch**. For a catch between `prev` (off) and `cur` (on), `bracket` is
   their midpoint separation, used by the controller's shrink rule. It is
   distinct from the outer interval `[prev.start, cur.end]` reported to the
   consumer.

4. Update the state:

   ```
   catch:     prediction += period + predictionError/2
              extent      = min(extent, max(extent * 31/32, K * bracket))
              failures    = 0
   miss:      prediction += period
              extent      = 2 * extent
              failures++
   ```

5. Give up and return to acquisition if `failures >= F`, or if the
   doubling in step 4 would make `extent` exceed `MaxExtent`. In the
   second case the doubling is not applied.
6. Send every catch to the consumer with the midpoint of the two poll
   midpoints as its timestamp `T`, `Uncertainty = [T - prev.start,
   cur.end - T]`, `PollWidths = [duration(prev), duration(cur)]`, and a
   computed `Anomalous` flag. The consumer forwards a catch to the time
   daemon only if it is not anomalous and
   `max(Uncertainty[0], Uncertainty[1]) <= U`.
   The reported interval bounds the physical edge if each query samples
   fresh pin state during its call; cached status can add unmeasured delay.

### Acquisition

The spacing halves from `period/64` toward `MinSpacing` on each catch;
the window is 64 spacings. Every catch adopts the caught midpoint as the
prediction and resets the miss count. A catch at `MinSpacing`, or two
consecutive caught windows with no scheduled sleep, completes acquisition.
A slept catch or miss resets the query-paced confirmation. Misses sweep
the poll-grid phase, and an in-progress pulse is polled through.

Coarse or anomalous catches can advance acquisition normally. The extent
handed to tracking is capped at `MaxExtent`.

### Constants

None encodes a hardware timing.

| Constant | Role | Current value |
|---|---|---|
| `MinSpacing` | sleep between queries where the platform can | 50 us, as today |
| `K` | brackets at which shrinking stops | 8 |
| shrink | fraction of the extent kept per catch | 31/32 |
| anomaly ratio | outer width relative to the recent median | 4 |
| history length | previous valid tracking widths | 31 |
| `F` | consecutive misses before giving up | 10 |
| `MaxExtent` | largest tracking extent, a fraction of the period | 1/8 |
| `U` | consumer's uncertainty limit | 1 ms, as today |

Measured quantities: the bracket, the durations of the two queries around
the edge, and the gap before the catching query.

### Consumer

`sysPulseCandidateEdge` in `time/internal/gpsevent/dispatcher.go`
forwards on
`!Anomalous && max(Uncertainty[0], Uncertainty[1]) <= sysPulseMaxUncertainty`.
The consumer logs rate-limited warnings for consistently anomalous catches
and for non-anomalous catches consistently above `U`. The latter is a
configuration problem, not a tracking failure: acquisition cannot make
queries faster.

### Removed from the original controller

- Spacing that scales with the window (`window/InitialPolls`) in tracking.
- The floor `2 * (|predictionError| + bracket)`.
- The 300-catch hold after a short run of misses (`shrinkAfter`,
  `catches`, `absentRun`).
- First-miss growth by two brackets, distinct from later doubling.
- Loss declared by ten misses at the full period (replaced by `F` and
  `MaxExtent`).
- `atFloor` and `Settled`.
- The ring of recent settled brackets and `OutlierRatio`. The new width
  history includes all valid tracking catches, independently of forwarding.

## Remaining fixes and validation

- **Warning counters (review point 8).** An anomalous catch returns before
  resetting the coarse counter, whereas a non-anomalous catch resets the
  anomalous counter. Alternating coarse and anomalous catches can therefore
  produce a coarse warning whose `consecutive` count is not consecutive.
  Make the reset rules match the warning's meaning.
- **Regression comparison.** Compare the previous and current algorithms
  under matching hardware, prewarm and load conditions. The completed runs
  establish current behaviour, but do not by themselves establish whether
  accuracy, forwarding gaps or CPU use have regressed.
- **macOS measurements.** Replace the article's measurements with results
  from the implemented algorithm, covering prewarm on, prewarm off and
  load. The separate raw-versus-chrony-filtered comparison still needs a
  daemon run; serial-tool captures alone do not supply it.

## Possible follow-up work

These are optional investigations, not prerequisites for the implemented
algorithm. Evaluate the current implementation before adding mechanisms;
each must justify its complexity with a concrete problem and measurable
improvement.
The four-times-median anomaly flag and removal of rejected catches are
complete, as recorded under [Revisions made](#anomaly-flag-replaces-rejected-catches).

### Improve acquisition with short pulses

The K901 on abondance's FT232R has an approximately 1 ms pulse and took
67 s to acquire; a repeated startup took 57 s. The initial spacing is
15.625 ms, so polling can repeatedly miss the whole pulse. In 100
simulations per width, varying startup phase and timing with 130 us
queries and the Linux timer model, acquisition took a median 25.6 s and
maximum 111.3 s with 1 ms pulses, versus 5.5 s and 6.0 s with 100 ms
pulses. This supports checking pulse width as the cause on hardware.

Investigate a simple way to acquire short pulses more reliably. Compare
acquisition time across startup phases, restarts and CPU cost, including
the hardware comparison between the existing pulse and a 0.1 s pulse.

### Revisit reacquisition at the maximum extent

Review point 9 remains unresolved: a miss returns to acquisition when
doubling would exceed `MaxExtent`, even before ten consecutive misses.
Acquisition can hand tracking an extent already at `MaxExtent`, in which
case one miss causes reacquisition. From a 15.625 ms extent, four
consecutive misses suffice to reach this exit.

Decide whether to retain this early exit or cap the extent at `MaxExtent`
and continue until the failure limit. Compare total polling cost and
recovery time, including the cost of reacquiring a short pulse; the
ten-failure limit currently does not guarantee ten tracking attempts.

### Consider a separate prediction correction guard

Independently of anomaly classification, consider whether prediction
correction needs protection from imprecise measurements. A proposed local
guard would skip correction when `W > extent`: the observation is then
less precise than the current search scale. On the loaded Mac, a 13 ms
open poll passed the open-read exception, corrected the prediction by
5 ms and cost four misses. This width guard would have stopped that
correction. Width and placement are distinct, though: the guard alone
does not address a narrow catch far from the prediction.

This guard is not yet chosen. It must earn its place as a prediction
update rule, without recreating a special catch category with different
failure counting and acquisition behaviour. The implemented anomaly flag
does not depend on adopting it. Midpoint separation remains the controller's cadence measure;
changing uncertainty to outer width is not a reason to change `K` from
8 to 4.

For context, tests of the former rejection rules produced gaps of 14,
24 and 44 s for ten, twenty and forty stalled catches, including
reacquisition. The longest rejection run seen on hardware was seven before
the absolute floor was added and two since. These results motivate
checking consecutive forwarding gaps, not just anomaly rates, when
evaluating the revised controller and forwarding gate together.

### Evaluate the estimator

Keep the midpoint of poll midpoints for now. On catches the consumer would
forward in the recorded runs, it had about 17 percent less spread than the
outer midpoint on the Mac; the gap midpoint had about 12 percent less
spread again. On the two Linux hosts the three were within noise. The
quiet and loaded Mac runs gave opposite indications about where sampling
occurred during a long start poll.

Any later estimator comparison should use identical candidate sets and
measure bias and upper-tail absolute error as well as spread. The paired
uncertainty interface already permits changing the estimator without
changing what interval is reported.

### Review coverage and pacing

- **Maximum extent.** Is `MaxExtent = period/8` too generous? At a
  one-second period, 125 ms costs about 12 percent of a core on the Mac
  for the few seconds before acquisition takes over.
- **Minimum coverage.** With 3.6 us UART queries, midpoint separation is
  about 4.5 us and extent shrinks to about 35 us. Scheduler jitter above
  roughly 17 us then costs a miss: 11 isolated misses in 20 minutes
  unloaded. Consider an explicit minimum extent as well as the previously
  suggested `K * max(bracket, MinSpacing)` floor, which would give 400 us
  and cost about 90 extra reads per second on the UART. `MinSpacing` is
  a pacing parameter, so coupling it to coverage needs justification.
  The same question applies to GPIO (#460), where scheduler jitter can
  span many query durations.
- **Window opening on Linux.** `sleepDuration` truncates the opening wait
  to whole milliseconds, causing up to 1 ms of early polling: about 350
  reads per window at a 35 us extent on the UART. Options remain accepting
  the cost, spinning the remainder, or using a direct `nanosleep` system
  call to sleep the fractional-millisecond remainder. Evaluate whether
  this reduces early polling without worsening timing. Measure total
  process CPU, reads per window, wake-up overshoot, timestamp error and
  forwarding gaps before deciding. Timer slack must not be treated as a
  bound on total wake-up delay.
- **Inter-query spacing on Linux.** Enforcing sub-millisecond spacing
  changes measurement resolution and makes the loop timer-paced. Treat
  that as a separate decision from improving the window-opening wait.
  A change to a shared sleep helper would affect both.

## Background and design rationale

This section records the incident and the reasoning behind the initial
controller redesign. References to the old controller and its scalar
uncertainty describe the code before that redesign or the later interface
fix. The local rejection heuristics described here have since been removed;
"Current algorithm" specifies their replacement.

### The incident

Mac mini (M4 Pro), macOS 26.6, ATGM332D-5N at 38400 bps on an FT232R,
PPS on CTS, `pollPreWarm = 0.05`, chrony 4.9 with
`refclock SOCK ... poll 2 precision 5e-5`. State queries take about
200 us, so the loop is query-paced and a normal bracket is 150 to 300 us.
Over the preceding 15 hours chrony received 54475 samples with no gap
longer than 5 s.

At 08:33 the display woke and a Time Machine backup to a NAS was running.
The host stalled the poll thread several times over 4 s. The pulse was
present and was caught every second throughout. What the controller did:

| Time | Event | Window after | Rule responsible |
|---|---|---|---|
| 08:28:35 | one miss | 1.26 ms | hold begins: no shrinking for 300 catches |
| 08:33:26 | catch, bracket 2.4 ms, error 791 us | 6.39 ms | floor = 2 x (791 + 2405) us; catch marked outlier and withheld |
| 08:33:28 | miss, one state read | 9.71 ms | the open query ran about 90 ms late and landed inside the pulse; hold restarts |
| 08:33:29 | catch, bracket 50.3 ms, error 22.9 ms | 146 ms | floor = 2 x (22.9 + 50.3) ms; prediction shifted 11.5 ms |
| 08:33:29 to 08:38:28 | 300 catches | 146 ms | hold: spacing 146 ms / 64 = 2.29 ms, uncertainty about 1.15 ms, not settled, every candidate dropped |
| 08:38:28 to 08:38:52 | shrinking | 146 to 33 ms | uncertainty under 1 ms, but brackets of 0.6 to 1.4 ms exceed 3 x the pre-incident lower quartile (about 190 us): marked outlier, dropped |
| 08:38:52 | first forwarded sample | | |

Five minutes of the gap were the hold, 24 s the outlier reference failing
to follow a spacing-limited bracket, 3 s the stalls themselves. Chrony
kept CTS selected; the system clock drifted from +24 us to -148 us against
gPTP before recovering.

Had the pulse been lost outright, ten misses at the full period would
have returned the loop to acquisition and it would have been forwarding
settled catches again in under 30 s. Catching the pulse every second in
an inflated window was the far more expensive path.

### What went wrong in the design

Each stage of the incident is a rule doing what it was written to do:

- The floor trusts the bracket and prediction error of every catch,
  although the same code marks a stalled catch's midpoint untrustworthy
  and withholds it from chrony. Two stalled reads inflated the window
  116-fold.
- The hold protects "a size that missed" by forbidding any shrink for
  300 catches, but it froze the window at the inflated size, which had
  never missed. The size that missed was 9.7 ms.
- Spacing is the window divided by 64, so a wide window means coarse
  edges. Every catch during the hold had an uncertainty just over the
  1 ms limit and none was settled, since settled requires the window to
  have reached its floor.
- The outlier reference is built from settled brackets only, so during
  shrink-back, when no catch is settled, it cannot follow, and brackets
  set by the coarse spacing rather than by any stall were judged against
  the query-paced brackets from before the incident.

The common thread is that the window does two jobs, bounding where to
look (extent) and setting the measurement resolution (spacing), and
nearly every special case in `track` manages the tension between them.
In the incident the extent job succeeded and the resolution job failed.

The earlier history in the same log shows the steady state the rules
produce: the window shrinks until it misses, roughly once every five
minutes, always within a minute of shrinking resuming; the miss grows the
window by two brackets and blocks shrinking for 300 catches; repeat. The
hold's whole effect is to set the miss rate to one per five minutes. It
also shows catches with a bracket several times normal and a prediction
error of about half the excess, because the edge estimate is the midpoint
of a bracket whose second query returned late.
The floor `2 * (|e| + b)` then asks for about three stalled brackets of
window from one stall.

### What chrony needs

The redesign started from the objective rather than from the failure.
Checked against the chrony 4.9 source:

- Every SOCK datagram goes into a per-refclock median filter and is not
  acted on until the poll timer fires, every 4 s at `poll 2`. One sample
  per poll interval is enough for the source to remain selectable.
- Reachability shifts one bit per poll interval; a source becomes
  unreachable only after eight empty intervals, 32 s, and even then stays
  selected while its newest sample is not older than any reachable
  source's oldest. The cost of a gap is drift, not deselection.
- The SOCK driver passes a fixed quality of 1, the protocol carries no
  per-sample uncertainty, and all samples in a poll interval carry the
  same dispersion, so chrony cannot judge one sample. At the poll it
  sorts the stored samples by offset and keeps the middle 60 percent:
  with four samples it drops the highest and lowest, with three it keeps
  the median, with two it averages both, with one it uses it as is.

So a missed second costs one sample out of a stream that can lose a few
percent, and a bad timestamp forwarded is a wrong correction that chrony
can trim only if at most one of the four seconds in its interval was
lost. The costs are asymmetric: the poller's rejection should be strict,
and a false rejection costs the same as a miss. The controller should
optimise the gap between good samples, not the catch rate. The original
loop optimised the catch rate, and the hold, the two-bracket floor and
the slow shrink all existed to avoid losing a sample.

### Extent and resolution are separate

Tracking polls at a cadence that does not depend on the window:
`MinSpacing` where the platform can sleep that briefly, back to back
where it cannot. Resolution is then whatever the host gives, and the
extent is coverage and CPU only. Consequences:

- Every tracking catch has the finest uncertainty the hardware can give,
  so `Settled` and `atFloor` lose their purpose.
- A wide extent costs queries, not samples. The incident's state, a large
  window frozen while catches continue unforwardable, cannot occur: at any
  extent the catches are at query resolution.
- A normal bracket has a known expected size, one query duration plus
  whatever `MinSpacing` adds, so a disturbed one can be recognised
  locally.

The old coupling bought broad coverage at a bounded query count, which
let the loop track through large phase uncertainty without restarting
acquisition, at the price of resolution. This design trades that for
earlier reacquisition. Given the objective, and given that acquisition
takes a few seconds against the minutes the old path cost, that trade is
right.

`MinSpacing` was considered for removal, and for enforcement by spinning,
and kept as it is. It cannot be enforced below a millisecond on Linux,
and spinning to enforce it would cost the same CPU as polling while
buying nothing: if a driver returns a cached status when polled fast, a
spin in place of the query would return the same stale value. Where the
platform can sleep briefly and the query is fast, the sleep is a real CPU
saving, so the constant stays.

### A catch never widens the extent

`extent = min(extent, max(extent * 31/32, K * bracket))`. The inner max
stops shrinking at `K` brackets; the outer min means no catch, however
wide its bracket, can make the loop work harder. This closes the route by
which the incident's stalls inflated the window. It also covers the one
disturbance the local tests cannot see: two queries slowed alike pass the
symmetry tests and give a wide bracket, and without the outer min that
clean-looking catch would set the extent to `K` times it.

`K` brackets is therefore where shrinking stops, not a floor the extent
is lifted to. If the extent is already below `K` brackets, because the
queries slowed after it shrank or acquisition handed back a small one, a
catch leaves it there and the next miss doubles it.

`K` is expressed in brackets, not as a duration, because the right extent
differs by an order of magnitude between a 10 us Linux UART query and a
200 us USB one, and a duration would be a hardware timing. Only part of
what the margin absorbs scales with the bracket: the half-bracket
residual of the correction does, scheduler jitter at the window open does
not. That other part is found by misses (below) rather than encoded. `K`
was raised from a first proposal of 4 to 8 because the cost is a handful
of queries per second on the key target and the benefit is that the
prediction-error excursions of a few hundred microseconds seen in the Mac
log sit inside the margin instead of being rediscovered by misses.

### Every miss doubles

A miss is the only event that widens the extent, and every miss doubles
it. There is no first-miss special case and no `lastMissed` state. This
is how the loop finds the margin a jittery host needs: it misses, doubles,
and shrinks back over about 22 good catches at 31/32. Where the edge
jitter exceeds `K` brackets the loop probes that boundary and pays about
one isolated miss per 23 s for it, which is within the objective. The
shrink rate was slowed from 15/16 to 31/32 because at a stable coverage
boundary either rate produces the same extent range, from just above the
failing size to twice it, and the slower rate traverses it half as often;
the cost is carrying a doubled extent for 22 s rather than 11 s after a
one-off miss.

Ignoring a first miss and doubling only on the second was considered and
dropped: when misses cluster, waiting for a second one is what chrony's
filter cannot afford, and the state it needs is not worth keeping.

### Local rejection in the initial redesign

These rules are historical. The implemented anomaly flag has replaced them.

A stall can land in three places relative to the two queries around the
edge: inside the first, inside the second, or between them. The two
duration comparisons see the first two: a long query beside a normal one.
Comparing brackets instead of durations would miss a stall inside the
first query, which inflates both the bracket before it and the bracket
after it. The gap test sees the third. The gap before the catching query
is known before that query runs, but the query is still made: if the pin
is still off, the delayed off-read simply starts a new bracket and the
window continues, so the test is applied only when a transition results.

The gap test is gated on `!cur.slept`. Without a sleep there is no
intentional idle time between queries, so a gap large relative to the
query duration is evidence of descheduling. With a sleep the gap contains
timer overshoot, whose ordinary size two query durations cannot reveal.
Ungated, a platform whose overshoot routinely exceeds `R` query durations
would reject every timer-paced catch, fail tracking to acquisition, and
reject acquisition's catches too, so the spacing would never halve and
output would stop for good. The gate removes that without history or a
platform-dependent threshold. What it gives up: on a timer-paced host, a
stall between the two queries is not rejected. Its consequences are a
wider bracket, which is a valid interval for the edge with an honestly
wider uncertainty that `U` bounds, and a half-error correction of a
quarter of the delay, which later misses and good catches undo. A stall
inside a query, where the sampling instant within the call is unknown and
the midpoint is biased, is still rejected everywhere, because timer
overshoot does not change how long a query takes. On the key targets the
loop is query-paced and the gate never applies.

The tests are heuristics: they detect asymmetric disturbance, not two
similarly stalled calls. The uncertainty limit catches the gross cases of
that. The initial redesign deliberately omitted history; the implemented
anomaly classification revisits this choice without coupling history to
extent or to whether the consumer forwards a catch.

A rejected catch changes nothing but the failure count. It is not
evidence about coverage, so it does not widen the extent, and not a
trustworthy measurement, so it does not move the prediction or shrink the
extent. It counts as a failure so that a host stalling continuously
reaches acquisition instead of tracking forever with no output.

### The half-error correction stays

A rule was considered that moves the prediction only as far as the
bracket forces it: leave it if it lies inside the bracket, otherwise pull
it to the nearest end. A stalled bracket contains the prediction and would
move it by nothing, and that rule needs no other rule to be right. It was
dropped because with rejection in place the case it defends against is
already handled, and it introduces a dead zone in which the prediction can
sit half a bracket off. For a good catch, both queries are within `R` of
each other and the second started on time, so the bracket is a few query
durations and its midpoint is a fair estimate; moving halfway toward it
damps the midpoint's quantisation noise. The correction is a damping
policy, not a guarantee of an unbiased estimate: an undetected stall just
under `R`, or two queries slowed alike, perturbs the prediction, and later
good catches halve the perturbation each time.

### U governs forwarding, not tracking

An earlier draft reset `failures` only on a forwardable catch. That was
wrong. In this design tracking already polls at the finest cadence the
host has, so a good catch whose uncertainty exceeds `U` can only come from
slow queries, and acquisition has no finer setting to reach. Returning to
acquisition for that reason would have no corrective effect and, with
slow queries, acquisition is the expensive mode. A transient common
slowdown resolves by itself while tracking keeps its phase; hardware
permanently too slow for `U` deserves a warning from the consumer.

### One budget constant

A polling budget accounted as elapsed query time per period, shared by
both modes and carried across mode switches as a credit balance, was
considered. It is correct accounting and far more machinery than the
problem needs. `MaxExtent` is a single admission bound on tracking
coverage. It does not cover `PreWarm`, which is a configured cost;
acquisition, whose window shrinks as its spacing halves and which on the
target hosts costs a few percent of a core for a second; or a query that
overruns, which the driver bounds. Acquisition's affordability is an
assumption about the supported hardware, not a property the code
enforces.

### Recovery on the incident

With this design the same 4 s of stalls would have gone: two catches
rejected (durations or gap), a miss doubling the extent from about 1.3 to
2.5 ms, one more catch rejected, then good catches forwarded from
08:33:30, with the extent shrinking back over the next 22 s. A gap of
about 4 s instead of 327 s. Which test rejects each catch depends on
where each stall fell, which the per-poll timings in the event log would
show; the bracket widths alone do not.

### Alternatives considered and dropped

- Excluding outlier catches from the floor and the prediction while
  keeping the rest of the controller. Addresses the root cause but keeps
  the coupling, the hold and the reference ring, and needs the outlier
  decision plumbed into `track`.
- A remembered "size that missed" so the hold protects that size rather
  than later inflation. Correct reading of the hold's intent; unnecessary
  once the hold is gone.
- Bounding growth per catch to a doubling. Crude, and repeated stalls
  still ratchet the window up.
- A watchdog on unforwardable catches with the consumer's acceptance
  predicate injected into the controller. Not needed once tracking is at
  fixed resolution, see "U governs forwarding".
- Treating a stalled catch as a miss. The miss path grows the extent,
  which is the wrong response to a host stall, and with the old
  `lastBracket` it would grow by twice the stalled bracket.
- Treating spacing-limited brackets specially in the outlier test. Moot
  once spacing does not follow the window.

## Testing

### Unit tests

The full `make test` suite passed after the anomaly-flag revision. The
incident simulation forwards no sample outside the uncertainty limit and
has a longest forwarding gap of three seconds, with no reacquisition.

The relevant coverage includes:

- `track` via the simulated `attempt`: catches never increase extent,
  shrink stops at `K` brackets, every miss doubles, and both `F` and
  `MaxExtent` hand back to acquisition.
- The four-times-median boundary, empty and short histories, exclusion of
  acquisition widths, and adaptation through anomalous catches.
- Identical polling observations with histories that make all catches
  anomalous or all ordinary: acquisition, prediction, extent and read
  counts must match, including more than `F` consecutive anomalous catches.
- The existing `Poll` tests with the simulated reader and clock, revised
  to the new rules: acquisition still converges, a short outage keeps
  tracking, a long one reacquires, a narrow pulse is still acquired.
- Exact asymmetric endpoints, odd-nanosecond rounding, ordered query
  endpoints and rejection of outer intervals at least one period wide.
- Consumer rejection when either uncertainty component exceeds the limit,
  paired JSON fields, and simulated errors within the reported interval.

### Hardware runs before the interface change

The following procedure and results refer to the initial controller
redesign, before uncertainty became a pair. Scalar uncertainty values and
old JSON field names below belong to those runs, not the current interface.

#### On the Mac

For these runs, satpulsed was stopped and chrony was disciplining the
system clock from the gPTP refclock, so `satpulsetool serial` had the port
and the edge times could be judged against an independent clock. The
receiver is the ATGM332D-5N at 38400 bps on `/dev/cu.usbserial-BG03U08C`
with PPS on CTS. Build with `make`; the binary is
`out/darwin_arm64/satpulsetool`.

1. Baseline with the original controller:

   ```
   satpulsetool -v serial -d /dev/cu.usbserial-BG03U08C -s 38400 -p cts \
       --poll-pre-warm 0.05 -j -t 600 > tmp/poll-old.jsonl 2> tmp/poll-old.log
   ```

   `-v` makes the tool log the track status lines and the `PollStats`
   summary at exit to stderr; `-j` writes one JSON record per edge with
   `t`, `uncertainty`, `settling` and `outlier`.
2. The same run with the new code, to `tmp/poll-new.jsonl`.
3. A run with the new code under induced stalls: start a dozen
   `yes > /dev/null` loops for the middle third of the run, and wake the
   display or start a Time Machine backup if one is due. The incident's
   stalls came from those.

For each run, from the JSON records:

- Lateness: the fractional second of `t`, wrapped to +/-0.5 s, is the
  edge's error against the gPTP-disciplined clock. Report median, p90 and
  max over edges that are not rejected, and separately for rejected ones,
  which should be the ones with large lateness.
- Uncertainty: median and p90 over edges that are not rejected. With
  tracking at query resolution these should be about 100 us throughout,
  with no period of coarse values after a disturbance.
- Gaps: the longest interval between consecutive non-rejected edges, and
  the count of intervals over 4 s. This is the objective. The incident
  was a 327 s gap; the target under induced stalls is a few seconds.
- Rejected and missed counts, and the `PollStats` summary from stderr,
  for the steady-state query count (`K` plus a few per second) and the
  CPU cost.

The old and new baselines should match on lateness and uncertainty; the
new code should show the same or slightly more isolated misses and no
long gaps. Under induced stalls the old code may reproduce a hold; the
new code should not.

#### Linux

Confirm on a Linux host with a fast query that the query-paced loop
behaves the same, and note the steady-state query count. Done on
2026-09-20 on a native UART with 3.6 us queries (PPS on DCD) and an
FT232R with 116 us queries (PPS on CTS), both with `-m poll` against a
chrony disciplined from a GPS PHC: in 1200 s, 1186 of 1189 and 1151 of
1154 edges forwardable, no rejections, isolated misses only, longest gap
2 s, lateness p90 10 us and 114 us. The UART's steady state is about 350
reads per window, set by the early open (see "Review coverage and pacing")
rather than by the extent, which shrinks to 35 us.

#### Long run

A day-long run on the Mac, with `satpulsetool serial` rather than
satpulsed: it needs no privileges or configuration, and its edge times
are judged against the gPTP-disciplined clock, which the serial PPS does
not feed. Compare the longest gap between forwardable edges and the
count of gaps over 4 s against the 15 hours before the incident, which
had no gap over 5 s, and note the rejected and missed counts and the
steady-state read rate. A run was started on 2026-09-20 at 14:05 for
86400 s; its final results are not recorded here.
