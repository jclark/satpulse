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
extent; misses grow it by 25% while the polling budget allows; catches
shrink it at 31/32, stopping at eight midpoint-to-midpoint brackets.
The first redesign replaced settled-bracket history with local rejection
tests and removed `Settled`. The subsequent
anomaly-flag revision below has removed those local tests and `Rejected`.

### Poll-count budget replaces maximum extent

Implemented: a missed window increases the next extent by one quarter
unless both the read count is at least 50 and the extent is at least
`50 * MinSpacing`. The count is feedback from a completed search, not a
reason to stop polling early. Catches keep the existing shrink rule.

`MaxExtent` and the acquisition handoff cap are removed. Only ten
consecutive misses return tracking to acquisition; a catch resets that
count. This resolves review point 9: reaching the work budget no longer
causes premature reacquisition.

The product `MaxPolls * MinSpacing` is 2.5 ms with the defaults: roughly
1.25 ms either side of the prediction. Skipping sleeps increases the
number of polls within a chosen extent; it does not shorten that extent.
The extent condition prevents those extra reads from stopping expansion
below this coverage scale. It is not a minimum extent: catches can still
shrink below it. Slower queries can permit growth beyond it.

The budget limits permission to grow rather than imposing a hard read
limit. Skipped waits, an extent inherited from acquisition, a change in
query pace, or one expansion can produce more than 50 reads. Growth by
1.25 reduces the overshoot compared with doubling. Linux
fractional-millisecond sleeping is now done in acquisition only; see
"Acquisition sleeps to its deadline".

### Prediction correction requires a sufficiently narrow interval

Implemented: tracking applies half the prediction error only when the
outer interval width `W` is at most half the current extent, before any
shrinking. A wider catch advances prediction by exactly one period.
Every catch still resets failures, follows the ordinary shrink rule and
is sent to the consumer. The controller does not read `Anomalous`.
Acquisition continues to adopt its coarse catches normally.

This protects a narrow search window from an imprecise phase estimate.
On the loaded Mac, a 13 ms opening poll corrected prediction by 5 ms and
cost four misses. In the synthetic three-stall scenario, a 50 ms query
shifted prediction by about 6 ms. With 1.25 growth but no correction
guard, that produced nine recovery misses and a 12-second forwarding gap.
With the half-extent guard, the scenario has only the two misses directly
caused by stalls, a three-second forwarding gap and no reacquisition.

The outer width controls correction eligibility; midpoint separation
remains the shrink rule's cadence measure, so `K` stays at eight.

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
`bracket`, with the same shrink constant. Eligible prediction corrections
still use half the prediction error.

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

### Windows open early by the measured first-read lead

Without prewarm, the Mac's timer woke about 1 ms late and the first query
after the idle wait took about 400 us, so the first query of a window
nominally opening 1.3 ms before the prediction completed after the pulse
had begun. Every one of the 18 misses in a three-minute debug run had its
first reading in the pulse. The controller shrank the extent until that
delayed first sample sat on the edge and bounced off it: the 12% miss
rate was the shrink/grow ratio at that boundary, not host jitter.

The poller now keeps `lead`, an exponentially weighted moving average of
how long after its scheduled time a window's first query completes (timer
overshoot plus the slowed first query), and schedules each window's first
query that much before the nominal open, so it completes, on average, at
the open. The lead moves only the first query: the deadline, the
prediction and the extent controller are unchanged. It is clamped at zero
so an early wakeup, as Linux's truncated sleeps give, never schedules the
query later than the open. With prewarm the spin ends at the early time
and the lead settles to about one warm query. A stall's excess is repaid
once, spread over later windows, since the average's decay sums to it.

On the same Mac setup a repeat three-minute run without prewarm had 2
misses in 171 tracking attempts against 18 before, both at timer
overshoots several times the typical 1.05 ms, with the extent settling
near 1.4 ms instead of 2.6 ms, a median of 5 reads per catch instead of
3, and CPU of 0.205% of one core against 0.218%.

### Queries target the window's grid

Each query was scheduled one spacing after the previous query's start,
so every query's timing error, a truncated sleep, a wake overshoot or a
stall, shifted all the later queries of the window. In a timer-paced
window the errors accumulated: on Linux the truncated sleeps shortened a
7.8 ms spacing to about 7.2 ms, putting the grid 20 ms off by the
prediction, and even exact sleeps with a 50 us overshoot drifted 1.6 ms
over the 32 queries before it. Each halving window relies on its grid
landing again on the sample that was inside the pulse, so with a pulse
narrower than that drift each halving was a chance of roughly width over
spacing, a miss swept the phase away, and ten misses restarted from the
full period. A 1 ms pulse on an FT232R restarted repeatedly on real
hardware, and the simulator reproduced it.

Each query now targets the first point after the previous query's start
of a grid anchored at the window open, so a query's error is its own.
Fresh starts with the same 1 ms pulse then caught every halving window
first time with no restart, leaving the coarse sweep as the only
variable part of the time to the first catch. Tracking is unchanged on
every measured host: its grid points are always past when the next
query is due, so the sequence is the same back-to-back one as before.

### Acquisition sleeps to its deadline

Scheduling each query at a grid point assumes the wait reaches it. On
Linux it does not: `sleepDuration` truncates to whole milliseconds, so a
15.625 ms wait sleeps 15 and the query starts 0.625 ms short of the point
it aimed at. That point is still ahead, so the loop targets it again, and
every wait to it is now sub-millisecond, which on Linux is no wait at
all. The loop reads back to back until it crosses the point, spending the
truncation remainder on polling instead of sleeping through it. Measured
by polling a pin nothing drives on the Linux host's native UART, so that
acquisition runs indefinitely: a one-second window took 67 state reads
before grid scheduling and 4192 to 5452 after, and the process went from
2.53% of one core to 4.79%.

Acquisition now waits precisely. The runtime timer takes the whole
milliseconds, so the wait stays cancellable, and `clock_nanosleep`
against an absolute `CLOCK_MONOTONIC` deadline takes the remainder,
retried when the runtime's preemption signal interrupts it. `Poll` also
narrows its thread's timer slack, 50 us by default, which the kernel
would otherwise add to every sleep it makes; that is three quarters of
the improvement, taking a window open from 62 us after its deadline to
19 us. The remainder is the whole wait where the wait is shorter than the
runtime timer's resolution, so acquisition honours its sub-millisecond
spacings too: every window now takes about 34 state reads whatever its
spacing, and the no-pulse window is back to 65 reads and 2.75% of one
core.

Acquisition therefore reaches `MinSpacing` on Linux as it does elsewhere,
rather than stopping early because the loop appeared query-paced when it
was only unable to sleep. The query-paced confirmation still applies
where a query genuinely outlasts the spacing, which reports no sleep as
before. With the halving rule used at that stage, the Linux UART handed
tracking a 3.2 ms extent instead of 15.62 ms, shortening the shrink that
followed, but took four further halvings, around 10 s against 6 s. The
faster refinement below replaces that halving rule.

Tracking keeps the truncated wait. Its spacing never sleeps, so only its
window open would change, and opening early is the only thing covering
the extent before the prediction; see "Review coverage and pacing".

### Acquisition preserves the catching query's phase

Anchoring the next grid on the midpoint prediction moved it half a query
later on each refinement, which could lose a narrow pulse. Each catch now
anchors the next acquisition grid on the catching query's actual start
one period later, independently of the midpoint prediction. A regression
with a 0.3 ms pulse and 130 us queries catches every refinement, including
when the first catching sample is near the pulse's trailing edge.

### Acquisition refines faster and confirms at a fixed extent

Ordinary refinement now divides spacing and extent by eight, while
query-paced confirmation holds the window fixed. The nominal poll count
per window remains fixed at `InitialPolls`, defaulting to 64. Reaching
`MinSpacing` or confirming query pacing determines the handoff. A
query-paced catch means that no inter-query wait slept, excluding the
wait to open the window.

1. **The first query initializes the lead.** Its duration supplies the
   initial `lead`, replacing zero. The existing per-window updates apply
   thereafter, with no separate initialization for the first scheduled
   opening.
2. **Ordinary refinement divides by eight.** A catch that is not
   query-paced and is not a confirmation attempt divides the spacing by
   eight, floored at `MinSpacing`, and reduces the extent accordingly.
3. **Query-paced confirmation holds the window.** The first query-paced
   catch holds both spacing and extent for the next window. If that
   window also catches and is query-paced, acquisition exits with the
   current extent. Neither catch reduces the window.
4. **A caught but unconfirmed window halves the spacing.** A confirmation
   catch that is not query-paced halves the spacing, floored at
   `MinSpacing`, reduces the extent accordingly, clears confirmation,
   and resumes ordinary refinement. This retains the conservative
   reduction when query pacing has not proved consistent.
5. **Reaching the spacing floor completes acquisition.** Either reduction
   reaching `MinSpacing` hands tracking an extent of
   `InitialPolls * MinSpacing`, 3.2 ms with the defaults. No further
   acquisition catch at the floor or reduction on handoff is required.
6. **A narrowed-window miss restarts immediately.** Any miss after the
   window has narrowed, including a confirmation miss, restarts the
   full-period sweep. Acquisition's consecutive-miss counter is removed.
   The phase shifts by 0.618 of a spacing only after a full-period sweep
   miss; the sweep continues indefinitely until it catches.

Preserving the catching query's phase supplies the alignment for
refinement. An isolated miss can still occur from timing variation;
restarting the sweep accepts that cost during the short acquisition
process and removes the separate narrowed-window recovery path.

These changes speed refinement after the first catch. They do not change
the initial coarse sweep or shorten discovery of a narrow pulse. In the
steady-query case with the defaults, fast native queries reach handoff
two pulse periods after the first catch instead of nine; typical USB
queries take two instead of eight. First usable samples arrive about two
periods earlier. These are consequences of the spacing and exit rules,
not measured before/after differences: the
[hardware validation](#acquisition-refinement-validation) used only the new
algorithm and the receivers' existing settings.

## Current algorithm

### Objective

Deliver good edge timestamps to the time consumer with short gaps between
them. An occasional missed pulse is acceptable; a bad timestamp forwarded
is not. Long-term CPU must stay small.

Prewarm trades CPU usage for timing performance; the user chooses that
tradeoff. Evaluate both settings. Simplicity means behaviour that can be
understood across operating conditions, with each rule earning its cost.

Measure against that objective. What matters at startup is the time to
the first forwarded sample, not the time to the "acquired" log line:
every catch goes to the consumer, acquisition's included, and it forwards
any non-anomalous catch whose uncertainty components are within the limit,
even before acquisition ends. After that, what matters is the gap between
forwarded samples. Reads per
window and per second measure work; CPU is process time from
`/usr/bin/time` on the host. The simulator's `blocked` statistic is the
fraction of the run spent outside queries and clock reads; it is not a
CPU figure, since a USB query is mostly the process blocked in the
kernel.

### State

- `prediction`: monotonic time of the next expected leading edge.
- `extent`: width of the window polled around the prediction.
- `failures`: count of consecutive tracking misses.
- `lead`: exponentially weighted moving average of the interval from a
  window's scheduled first query to that query's completion, initialized
  from the very first query's duration.
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

Each query is scheduled at the first point after the previous query's
start of a fixed grid, spaced by `MinSpacing` in tracking and by the
current acquisition spacing in acquisition, so one query's timing error
does not shift the rest of the window. Tracking and the initial sweep
anchor the grid at the window open. Each acquisition catch anchors the
next grid on the catching query's start one period later. The wait
sleeps where the platform can and returns at once where it cannot: on
Linux, sub-millisecond waits are truncated to zero, because the runtime
would otherwise round them up to a millisecond. Acquisition sleeps out
whatever the runtime timer cannot, the whole wait included, so it arrives
at a grid point instead of polling its way to it. Tracking is therefore
query-paced wherever `MinSpacing` is below the timer's resolution, and on
macOS with a USB serial adapter whose query already exceeds `MinSpacing`.
There is no spinning.
`MinSpacing` is a CPU saving where it can be honoured and nothing where
it cannot.

### One attempt, once per period

1. `open = prediction - extent/2`, `close = prediction + extent/2`. Wait
   until `open - lead`, with `PreWarm` as today, so that the first query
   completes at about `open`; then update `lead` from that query's
   completion.
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
   catch:     prediction += period
              if W <= extent / 2:
                  prediction += predictionError / 2
              extent      = min(extent, max(extent * 31/32, K * bracket))
              failures    = 0
   miss:      prediction += period
              failures++
              if stateReads < MaxPolls || extent < MaxPolls * MinSpacing:
                  extent += extent / 4
   ```

5. Give up and return to acquisition if `failures >= F`. No extent growth
   is applied when returning to acquisition. Reaching the poll budget
   can prevent growth once the coverage scale is reached; it does not
   cause reacquisition. A miss searches the full extent, so its read count
   measures the cost of that search.
   Catches stop early and their read counts do not control growth.
6. Send every catch to the consumer with the midpoint of the two poll
   midpoints as its timestamp `T`, `Uncertainty = [T - prev.start,
   cur.end - T]`, `PollWidths = [duration(prev), duration(cur)]`, and a
   computed `Anomalous` flag. The consumer forwards a catch to the time
   daemon only if it is not anomalous and
   `max(Uncertainty[0], Uncertainty[1]) <= U`.
   The reported interval bounds the physical edge if each query samples
   fresh pin state during its call; cached status can add unmeasured delay.

### Acquisition

Start with a full-period sweep at `period/64` spacing. The window is 64
spacings. Every catch adopts the caught midpoint as the prediction and
preserves the catching query's start as the next grid's phase. Coarse or
anomalous catches advance acquisition normally.

An ordinary sleep-paced catch divides the spacing by eight, floored at
`MinSpacing`, and sets the next extent to 64 times that spacing. Reaching
`MinSpacing` completes acquisition immediately, handing tracking
`64 * MinSpacing` without another acquisition window at the floor.

A query-paced catch, with no inter-query sleep, holds spacing and extent
for confirmation at the next pulse. A second query-paced catch completes
acquisition with that same extent. If confirmation catches but is not
query-paced, halve the spacing, clear confirmation and resume refinement;
that reduction also exits immediately if it reaches `MinSpacing`. Neither
the first query-paced catch nor its successful confirmation reduces the
window.

A miss in the full-period sweep shifts the phase by 0.618 of a spacing
and continues sweeping. Any miss after the window has narrowed, including
a confirmation miss, restarts the full-period sweep immediately, without
an extra phase shift. Acquisition has no consecutive-miss counter. An
in-progress pulse at the opening is polled through.

### Constants

None encodes a hardware timing.

| Constant | Role | Current value |
|---|---|---|
| `MinSpacing` | sleep between queries where the platform can | 50 us, as today |
| `K` | brackets at which shrinking stops | 8 |
| shrink | fraction of the extent kept per catch | 31/32 |
| growth | extent multiplier after a miss when growth is allowed | 1.25 |
| `MaxPolls` | state-read threshold for allowing further growth | 50 |
| correction width fraction | largest outer width relative to the pre-catch extent for phase correction | 1/2 |
| anomaly ratio | outer width relative to the recent median | 4 |
| history length | previous valid tracking widths | 31 |
| `F` | consecutive misses before giving up | 10 |
| `leadWeight` | reciprocal weight of the newest observation in the lead average | 8 |
| `U` | consumer's uncertainty limit | 1 ms, as today |

Measured quantities: state reads per attempt, the bracket, the durations
of the two queries around the edge, and the gap before the catching query.

### Consumer

`sysPulseCandidateEdge` in `time/internal/gpsevent/dispatcher.go`
forwards on
`!Anomalous && max(Uncertainty[0], Uncertainty[1]) <= sysPulseMaxUncertainty`.
The consumer allows 30 seconds from startup for the first usable edge,
then warns once if none has arrived. If no candidates arrived it reports
no edges; otherwise it reports no usable edges with counts of anomalous
and non-anomalous over-limit candidates. Anomalous takes precedence so
each withheld candidate is counted once. A usable candidate cancels the
timeout even if it cannot yet be matched to a receiver time message.
There are no repeat warnings, recovery messages or later outage checks.
When the best achievable resolution is coarser than `U`, the current
interface does not provide the information needed to handle that case;
see "Restore support for coarse achievable resolution" below.
Reacquisition cannot make queries faster.

### Removed from the original controller

- Spacing that scales with the window (`window/InitialPolls`) in tracking.
- The floor `2 * (|predictionError| + bracket)`.
- The 300-catch hold after a short run of misses (`shrinkAfter`,
  `catches`, `absentRun`).
- First-miss growth by two brackets, distinct from later doubling.
- Loss declared by ten misses at the full period (replaced by `F`
  consecutive misses at any extent).
- `atFloor` and `Settled`.
- The ring of recent settled brackets and `OutlierRatio`. The new width
  history includes all valid tracking catches, independently of forwarding.

## Remaining fixes and validation

- **Regression comparison.** Compare the previous and current algorithms
  under matching hardware, prewarm and load conditions. The completed runs
  establish current behaviour, but do not by themselves establish whether
  accuracy, forwarding gaps or CPU use have regressed.
  The September 20-21 Mac tests had roughly 12% tracking misses without
  prewarm, compared with 3.55% in an earlier run before the growth and
  correction changes. The cause was the slow first query after the idle
  wait, now compensated by the lead (see "Revisions made"). The same
  boundary caused the Mac's slow no-prewarm acquisition, which fresh
  starts with the lead no longer show. Repeat the 15-minute prewarm-off
  and prewarm-on runs with the lead, and a Linux run on USB serial and a
  native UART to confirm the lead stays near zero there.
- **macOS measurements.** Replace the article's measurements with results
  from the implemented algorithm, covering prewarm on, prewarm off and
  load. The separate raw-versus-chrony-filtered comparison still needs a
  daemon run; serial-tool captures alone do not supply it.

### Restore support for coarse achievable resolution

Removing `Settled` regressed support for readers whose best achievable
uncertainty is worse than 1 ms. Previously, the poller indicated that no
further improvement in bracket width was expected, allowing the consumer
to accept non-outlier samples at that resolution. The current interface
leaves the consumer withholding every catch when even the best
measurements exceed the limit. This requires a fix.

Whether any supported reader is affected is doubtful. Uncertainty over
1 ms needs queries slower than about 450 us, since the outer interval
is two queries plus a spacing, and the recorded query times are 3.6 us
on a native UART, about 116 us on a Linux FT232R, about 200 us on the
Mac and on Windows, and microseconds for a GPIO read. The old bypass was
needed because window size set resolution; that coupling is gone. The
remaining choices are to drop the requirement and rely on the consumer's
startup warning about unusable edges, or to define the interface below.

Determine what information the poller should expose to distinguish
achievable resolution from measurements that can still improve. An
`Acquiring` flag would identify the mode and could be omitted from JSON
during tracking, but simply using `!Acquiring` to bypass the uncertainty
limit would disable that check throughout tracking. Recover the useful
information previously conveyed by `Settled` without restoring the old
coupling between tracking extent and polling resolution. The interface
and its semantics remain open.

## Possible follow-up work

These are optional investigations, not prerequisites for the implemented
algorithm. Evaluate the current implementation before adding mechanisms;
each must justify its complexity with a concrete problem and measurable
improvement.
The four-times-median anomaly flag and removal of rejected catches are
complete, as recorded under [Revisions made](#anomaly-flag-replaces-rejected-catches).

### Improve first-catch latency for narrow pulses

The initial full-period sweep still uses 64 spacings by default and shifts
its phase by 0.618 of a spacing after each miss. Faster refinement begins
only after the first catch; reducing the discovery time for narrow pulses
remains separate work. Measure it by time to the first usable sample
across start phases, reads and process CPU, with and without a pulse.

Controlled narrow pulses, no-pulse CPU and acquisition under induced load
also remain to be measured on hardware.

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

- **Polling budget and growth.** Evaluate the 50-read growth threshold
  together with the `MaxPolls * MinSpacing` coverage scale and 1.25 growth
  factor against recorded disturbances. Slower expansion reduces budget
  overshoot but can delay recovery from a large prediction error. Compare
  forwarding gaps and total CPU, including reacquisition. Check that the
  default 2.5 ms coverage scale accommodates the observed phase error and
  window-opening delay; increasing `MinSpacing` also changes resolution
  where that spacing is honoured.
- **Minimum coverage.** With 3.6 us UART queries, midpoint separation is
  about 4.5 us and extent shrinks to about 35 us. Scheduler jitter above
  roughly 17 us then costs a miss: 11 isolated misses in 20 minutes
  unloaded. Consider an explicit minimum extent as well as the previously
  suggested `K * max(bracket, MinSpacing)` floor, which would give 400 us
  and cost about 90 extra reads per second on the UART. `MinSpacing` is
  a pacing parameter, so coupling it to coverage needs justification.
  The same question applies to GPIO (#460), where scheduler jitter can
  span many query durations.
- **Tracking's window open on Linux.** Tracking keeps the truncated wait,
  so its window opens up to a millisecond early and polls from there. That
  polling is nearly free, about 100 reads per second at 4.7 us on the
  native UART, but it is also the only thing covering the extent before
  the prediction. Opening at the deadline instead, measured at
  `pollPreWarm = 0`, raised tracking error's standard deviation from
  1.51 us to 3.58 us and its largest magnitude from 7.6 us to 20.4 us,
  with two misses in 285 s against none. The extent shrinks to about ten
  reads, so the edge is then often caught by the first read after the
  sleep, whose p90 duration rose from 4.5 us to 20.7 us while warm reads
  stayed at 3.7 us. Whether prewarm recovers that is untested. macOS
  sleeps exactly and has no such margin, so this is the minimum coverage
  question above rather than a Linux one; settle that first.
- **Inter-query spacing on Linux.** Enforcing sub-millisecond spacing
  changes measurement resolution and makes the loop timer-paced: honouring
  the 50 us `MinSpacing` would replace the UART's 4.5 us bracket with one
  of about 60 us. Acquisition's precise wait leaves it alone because
  tracking's waits are not precise; any future change to the shared wait
  must keep that distinction.

## Background and design rationale

This section records the incident and the reasoning behind the initial
controller redesign. References to the old controller and its scalar
uncertainty describe the code before that redesign or the later interface
fix. The local rejection heuristics, miss doubling, unconditional phase
correction and maximum-extent exit described here have since been
replaced; "Current algorithm" specifies the implemented rules.

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

### Miss doubling in the initial redesign

This rationale is historical. Miss growth is now 1.25 and conditional on
the observed poll count, as recorded under "Revisions made".

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

Tests of the former rejection rules produced gaps of 14, 24 and 44 s for
ten, twenty and forty stalled catches, including reacquisition. The
longest rejection run seen on hardware was seven before the absolute
floor was added and two since. These results motivate checking consecutive
forwarding gaps, not just anomaly rates, when evaluating the controller
and forwarding gate together.

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

### The maximum-extent budget in the initial redesign

This rationale is historical. The implemented budget now uses observed
state-read counts rather than an extent limit.

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

The full `make test` suite passed after the poll-budget and prediction-guard
revision. The synthetic three-stall scenario forwards no sample outside
the uncertainty limit and has two tracking misses and a longest
forwarding gap of three seconds, with no reacquisition.

The relevant coverage includes:

- `track` via the simulated `attempt`: catches never increase extent,
  shrink stops at `K` brackets, a miss grows by a quarter below 50 reads
  or below `50 * MinSpacing`, and holds when both thresholds are reached.
  Only `F` consecutive misses hand back to acquisition, including at the
  former extent limit; a catch resets the failure count even when it is
  too wide to correct prediction.
- The half-extent correction boundary, unchanged ordinary catch handling
  when correction is skipped, and continued growth despite extra reads
  from skipped waits, including a nondefault minimum spacing.
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

### Acquisition refinement validation

The acquisition tests cover initial lead measurement, both exits, a caught
but unconfirmed window followed by either exit, immediate restart without
a phase shift after a refinement or confirmation miss, and continued
phase sweeping without a pulse. The no-pulse test also covers 1024 initial
polls, where integer rounding makes the sweep extent slightly shorter
than a second. Sweep detection uses the initial spacing rather than exact
equality of the extent with one second. The 0.3 ms phase-preservation
regression and the polling simulator tests pass.

Hardware validation on September 21 used twenty 12-second starts per
device, ten without prewarm and ten with 50 ms prewarm. The Mac and the
three Linux devices ran concurrently, alternating prewarm settings and
varying the pause between starts. All 80 starts caught every acquisition
window, with no restart. The table measures receipt of the first usable
serial-tool candidate: non-anomalous, with both uncertainty components
at most 1 ms. These runs did not feed chrony.

| Device | Prewarm | First usable, min / median / max (s) | Acquisition reads, min-max | CPU, one core |
|---|---|---|---|---|
| Mac FT232R | off | 2.08 / 2.74 / 3.35 | 62-118 | 0.456% |
| Mac FT232R | 50 ms | 1.45 / 2.33 / 2.90 | 75-129 | 4.699% |
| Linux native UART | off | 1.44 / 1.74 / 1.96 | 95-129 | 1.265% |
| Linux native UART | 50 ms | 1.02 / 1.27 / 1.82 | 70-121 | 5.784% |
| Linux FT232R | off | 1.43 / 2.62 / 2.88 | 87-129 | 0.581% |
| Linux FT232R | 50 ms | 2.01 / 2.35 / 2.98 | 73-131 | 5.049% |
| Linux FT232H | off | 1.52 / 2.40 / 2.98 | 82-137 | 0.598% |
| Linux FT232H | 50 ms | 1.30 / 1.75 / 2.97 | 78-132 | 5.107% |

CPU is total process user plus system time divided by total elapsed time,
from `/usr/bin/time -p`, across the ten starts in each row. It includes
startup and acquisition and is not a steady-state measurement.

One Mac start without prewarm exercised the failed-confirmation path:
it caught at the held 15.625 ms window without confirming query pacing,
halved to 7.812 ms, and confirmed at that fixed extent. All other starts
handed tracking 3.2 ms. The main batch had nine isolated tracking misses,
all on the Mac without prewarm; the longest gap between usable candidates
was about two seconds. Both hosts retained independent selected clock
references throughout.

The final binary, including the sweep-rounding check above, was verified
with another prewarm-off/on pair on each device. All eight starts caught
every acquisition window, with no restart; there was one further isolated
Mac tracking miss without prewarm. The 80-start batch used the same
algorithm at the default initial poll count, before that rounding check.

### Mac validation after the poll-budget and correction changes

Tests on September 20-21 used commit
`00a38dbf7420081c64b85d20503ac3eca11eb3c0`: four 900-second runs and
one overnight run. The binary was pinned for the whole sequence. The
receiver was an ATGM332D-5N at 38400 baud on FT232R BG03U08C, PPS on CTS,
on the Mac mini M4 Pro. Chrony disciplined the host clock from an
independent GPTP source throughout.

Prewarm on means 50 ms. Quiet means no induced load; normal applications
remained running. Each loaded test used twelve CPU workers during seconds
300-600, with five minutes quiet before and five minutes recovery.
The overnight run had no prewarm and no induced load. It was stopped
gracefully at the user's request after 7 h 39 min 29 s, rather than the
planned eight hours. All five processes exited successfully.

These are serial-tool measurements. Forwardable means non-anomalous with
both uncertainty components at most 1 ms; the tool did not feed chrony.
Error is relative to integral seconds of the independently disciplined
host clock and includes receiver, cable and reference offsets. CPU is
process user plus system time divided by elapsed time, including acquisition.

| Run | Duration | Initial acquisition | Tracking misses | Tracking anomalies | Forwardable tracking catches | Longest forwarding gap | Absolute error p99 | CPU, one core |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Prewarm off, quiet | 900 s | 53.925 s | 107 | 0 | 739 | 4 s | 181.062 us | 0.305% |
| Prewarm on, quiet | 900 s | 8.247 s | 0 | 1 | 890 | 2 s | 131.094 us | 4.986% |
| Prewarm off, load | 900 s | 35.986 s | 108 | 8 | 748 | 3 s | 175.365 us | 0.248% |
| Prewarm on, load | 900 s | 8.212 s | 1 | 7 | 883 | 2 s | 151.603 us | 4.959% |
| Overnight, prewarm off, quiet | 27569.334 s | 26.555 s | 3297 | 54 | 24011 | 4 s | 302.729 us | 0.286% |

Every forwardable interval contained the independently referenced edge.
All five runs had zero tracking losses. This means none reached ten
consecutive misses; it does not mean they caught every pulse. Miss rates
in the two 900-second no-prewarm runs were 107/846 (12.65%) and 108/864
(12.50%), with at most three and two consecutive misses respectively.

The overnight run completed 27,542 tracking attempts: 24,245 catches and
3,297 misses (11.97%), at most two consecutive. Of the catches, 234 exceeded
the uncertainty ceiling, including all 54 anomalies. The 24,011
forwardable catches had mean error -17.793 us and maximum absolute error
653.281 us. Extent ranged from 2.486 to 8.794 ms; 52 catches skipped
phase correction. There was one restart during initial acquisition and
none after tracking began. All 919 reference checks selected GPTP and
completed successfully.

The first 60-second no-prewarm smoke had 27 acquisition catches, 33 misses
and two acquisition restarts, without reaching tracking. A second smoke
with prewarm acquired in 8.158 s and caught all 51 tracking pulses with
no anomalies; maximum absolute error was 111.865 us.

Raw candidates, per-catch debug logs, query counts, prediction errors,
exact load events, process CPU accounting and reference snapshots were
retained. Reference checks ran every 30 seconds. Final query-time
distributions use at most the last 10,000 reads; per-catch query widths
cover the whole run. Gap statistics exclude initial acquisition and the
trailing interval after the final candidate.

#### Comparison with the preceding controller

Earlier 600-second Mac runs used
`90c8cae81190d903f50d1235718bc8348d07c7a4`, which already had asymmetric
uncertainty and the anomaly flag, but doubled extent on misses and lacked
the phase-correction width guard:

| Run | Tracking misses | Longest forwarding gap | Absolute error p99 | CPU, one core |
| --- | ---: | ---: | ---: | ---: |
| Prewarm on, quiet | 0 | 1 s | 112.989 us | 5.132% |
| Prewarm off, quiet | 21 | 3 s | 356.989 us | 0.314% |
| Prewarm on, load | 5 | 3 s | 215.302 us | 5.037% |

The earlier no-prewarm miss rate was 21/591 (3.55%). Durations, host
conditions and debug verbosity were not identical between the two sets;
neither the new growth factor nor host timing has been isolated as the
cause of the higher miss rate. The completed tests describe current
behaviour but do not establish an old-versus-new regression result.

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
86400 s, then stopped on September 20 at 18:35 before the interface-change
tests. Its logs were preserved. It is no longer running; the later
overnight results above describe the current controller.
