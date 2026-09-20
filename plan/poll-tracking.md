# Simplify the serial PPS poll tracking loop (#TBD)

Related: #460 (GPIO polling reuses this loop).

## Overview

The poll method in `gps/app/pps/poll.go` finds PPS leading edges by
repeatedly reading a pin around the time the next pulse is expected. Its
tracking phase adapts a polling window to the measured need. That
adaptation has grown a set of interacting rules whose combined behaviour
is hard to predict, and on 2026-09-20 they combined to withhold every
sample from chrony for 5 min 27 s while the pulse was caught every second.

This plan replaces the tracking controller with a simpler one. The window
no longer sets the polling resolution; it is only coverage. A catch can
never widen it, only misses can, and a catch whose timing looks disturbed
is rejected locally without any history. The consumer forwards a catch on
two conditions only: it was not rejected and its uncertainty is under a
fixed limit.

The first part of this document specifies the algorithm. The second part
explains the problem with the current design and how this one was
arrived at.

## The algorithm

### Objective

Deliver good edge timestamps to the time consumer with short gaps between
them. An occasional missed pulse is acceptable; a bad timestamp forwarded
is not. Long-term CPU must stay small.

### State

- `prediction`: monotonic time of the next expected leading edge.
- `extent`: width of the window polled around the prediction.
- `failures`: count of consecutive attempts that produced no good catch.

### Recorded per query

These are recorded today, in `reading` and `poll`:

- `sched`: when the query was scheduled to start.
- `start`, `end`: when it actually started and returned.
- `slept`: whether the wait for `sched` used a timer. False when `sched`
  was already past, or nearer than the platform can sleep to.
- `value`: pin on or off.

The timing tests below measure short intervals through the existing
`clockReading` abstraction, `poll.duration` and `poll.gapAfter`, as the
bracket does, so each platform uses the reading appropriate to it: the
one `time.Now` reading where it carries both wall and monotonic time,
and on Windows the precise measurement stamp rather than the coarse
monotonic reading.

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
   the query midpoint passes `close`. Transition precedence over the
   deadline and the `0 < bracket < period` check in `classify` are
   unchanged.
3. Classify the attempt as one of three outcomes.

   **Miss**: no transition seen.

   **Catch**: a transition seen between query `prev` (off) and query `cur`
   (on). `bracket` is the interval between their midpoints, as today. The
   catch is **rejected** if any of these holds, otherwise it is **good**:

   ```
   !cur.slept && gap > R * duration(prev)        && gap > MinSpacing
   duration(cur)  > R * duration(prev)           && duration(cur) - duration(prev) > MinSpacing
   duration(prev) > R * duration(cur)            && duration(prev) - duration(cur) > MinSpacing
                                                    unless prev is the read at the open
   ```

   where `gap = cur.start - prev.end`. `MinSpacing` is the floor of the
   excess each test judges: the loop itself idles that long between
   queries where it can sleep, so a shorter disturbance is within its own
   pacing and biases the midpoint by at most half of it. Without the
   floor a microsecond UART query is failed by any preemption: on a
   native UART with 3.6 us queries, runs of seven consecutive good
   catches (brackets of 13 us, prediction errors of 1 to 2 us) were
   rejected, opening a 14 s gap.

   The third test is not applied when `prev` is the read at the window
   open. That read follows the idle wait between windows and, on hosts
   whose queries slow down while idle, is routinely severalfold longer
   than the reads after it, so its length says nothing about the read
   that follows; on the Mac, good edges caught in the first bracket
   (lateness 10 to 130 us) were rejected by it. A stall inside the open
   read still widens the bracket, which the uncertainty reports.

4. Update the state:

   ```
   good:      prediction += period + predictionError/2      (as today)
              extent      = min(extent, max(extent * 31/32, K * bracket))
              failures    = 0
   rejected:  prediction += period
              failures++
   miss:      prediction += period
              extent      = 2 * extent
              failures++
   ```

5. Give up and return to acquisition if `failures >= F`, or if the
   doubling in step 4 would make `extent` exceed `MaxExtent`. In the
   second case the doubling is not applied. The candidate from a rejected
   catch is sent (step 6) before giving up.
6. Send every catch to the consumer with the bracket midpoint as its
   timestamp, half the bracket as its `Uncertainty`, and a `Rejected`
   flag. The consumer forwards a catch to the time daemon only if it is
   not rejected and `Uncertainty <= U`.

### Acquisition

Structure unchanged: the spacing halves from `period/64` toward
`MinSpacing` on each catch, the window is 64 spacings, misses sweep the
poll-grid phase, an in-progress pulse is polled through. Three changes:

- A catch that fails the step 3 tests is rejected in acquisition too. It
  advances the prediction by one period, resets the consecutive-miss
  count (a transition was seen, so the pulse is present), resets the
  query-paced confirmation, and neither halves the spacing nor completes
  acquisition. A good acquisition catch behaves as today, including a
  coarse one: acquisition must be able to progress from coarse
  resolution.
- The prediction update on a good catch is unchanged (adopt the caught
  midpoint).
- The extent handed to tracking is capped at `MaxExtent`.

### Constants

None encodes a hardware timing.

| Constant | Role | Proposed |
|---|---|---|
| `MinSpacing` | sleep between queries where the platform can | 50 us, as today |
| `K` | brackets at which shrinking stops | 8 |
| shrink | fraction of the extent kept per good catch | 31/32 |
| `R` | ratio for the gap and duration tests | 3 |
| floor of the excess the tests judge | `MinSpacing` | 50 us |
| `F` | consecutive failures before giving up | 10 |
| `MaxExtent` | largest tracking extent, a fraction of the period | 1/8 |
| `U` | consumer's uncertainty limit | 1 ms, as today |

Measured quantities: the bracket, the durations of the two queries around
the edge, and the gap before the catching query.

### Consumer

`sysPulseCandidateEdge` in `time/internal/gpsevent/dispatcher.go`
forwards on `!Rejected && Uncertainty <= sysPulseMaxUncertainty`. The
`Settled` field of `CandidateEdge` goes away, from the event log record
and from the `satpulsetool serial` JSON output as well, and `Outlier` is
renamed `Rejected` in all three places. The consumer logs a rate-limited warning when good catches are
consistently above `U`, since hardware too slow for the limit is a
configuration problem, not a tracking failure.

### Removed from the current code

- Spacing that scales with the window (`window/InitialPolls`) in tracking.
- The floor `2 * (|predictionError| + bracket)`.
- The 300-catch hold after a short run of misses (`shrinkAfter`,
  `catches`, `absentRun`).
- First-miss growth by two brackets, distinct from later doubling.
- Loss declared by ten misses at the full period (replaced by `F` and
  `MaxExtent`).
- `atFloor` and `Settled`.
- The ring of recent settled brackets and `OutlierRatio` (replaced by the
  local tests).

## Design justification

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
optimise the gap between good samples, not the catch rate. The current
loop optimises the catch rate, and the hold, the two-bracket floor and
the slow shrink all exist to avoid losing a sample.

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

### Rejected catches are recognised locally

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
that. A relative history would discriminate further and is deliberately
not kept; the design accepts that loss.

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

## Implementation notes

Everything is in `gps/app/pps/poll.go` except the consumer and the
surfaces that expose the removed fields.

- `track` and its `trackObservation`, `trackEvent` types: the observation
  gains a rejected outcome; `shrinkAfter`, `absentRun`, `trackRelease`
  and the `catches`, `misses`, `fullMisses`, `atFloor` state go; the
  event kinds reduce to started, changed, missed, rejected, lost. The
  simulated-`attempt` test seam stays.
- `pollWindow`: the `spacing` argument becomes `MinSpacing` in tracking;
  the `atFloor` argument and `settled` go; the rejection tests replace
  `outlierLimit`; `recordBracket`, `settledBrackets`, `nextBracket`,
  `outlierHistory`, `outlierMinHistory` go. The tests need the monotonic
  `start`/`end` of `prev` and `cur` and `cur.slept`, all of which
  `reading` already carries.
- `acquire`: rejected catches as specified above; cap the extent it
  returns.
- `PollParams.OutlierRatio` goes. `CandidateEdge.Settled` goes and
  `Outlier` becomes `Rejected`, with the same rename in `SysPulseEdge`
  (`rejected` in the event log) and the tool's JSON edge record.
- `PollStats`: the outliers count becomes the rejected count; the
  settled/unsettled distinction in its summary goes.
- `gps/app/serialpps/config.go`, `serialpps.go`: the `pollOutlierRatio`
  key and its validation go. `docs/man/satpulse.toml.5.md` loses the key.
- `time/app/serialcmd/serialflags.go`: `--poll-outlier-ratio` goes.
  `serialpps.go`'s JSON edge record loses `settling`.
- `time/internal/gpsevent/dispatcher.go`: the forwarding rule, the
  `SysPulseEdge` record loses `settled`, and the rate-limited warning for
  good catches consistently above the limit.
- Existing tests to revise: `TestTrackSimulation`, `TestTrackFeedback`,
  `TestTrackLoss`, `TestTrackAbsenceShrinksAtOnce` (its premise is gone),
  `TestPollOutlier`, `TestPollTrackingConverges`,
  `TestPollShortOutageKeepsTracking`, `TestPollOutageReacquires`, and
  the dispatcher tests that set `Settled`.
- Release notes: serial PPS is new in the unreleased 0.3 and its entry
  (#402) covers the feature as a whole, so removing `pollOutlierRatio`
  and `settled` before release needs no entry.

## Testing

### Unit tests

- `track` via the simulated `attempt`: a rejected catch changes only
  `failures`; a good catch never increases the extent and stops shrinking
  at `K` brackets; every miss doubles; `F` and `MaxExtent` each hand back
  to acquisition; the candidate of the `F`th rejected catch is still sent.
- The rejection tests on synthetic `reading` pairs: a long `cur`, a long
  `prev`, a gap with `slept` false (rejected) and the same gap with
  `slept` true (not rejected), and a normal pair.
- The existing `Poll` tests with the simulated reader and clock, revised
  to the new rules: acquisition still converges, a short outage keeps
  tracking, a long one reacquires, a narrow pulse is still acquired.

### On the Mac

satpulsed is stopped and chrony is disciplining the system clock from
the gPTP refclock, so `satpulsetool serial` has the port and the edge
times can be judged against a clock that does not depend on them. The
receiver is the ATGM332D-5N at 38400 bps on `/dev/cu.usbserial-BG03U08C`
with PPS on CTS. Build with `make`; the binary is
`out/darwin_arm64/satpulsetool`.

1. Baseline with the current code, before changing anything:

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

### Later

- Run satpulsed with the new code on the Mac for a day and compare, from
  chrony's refclock log, the longest gap between CTS samples and the count
  of gaps over 4 s against the 15 hours before the incident.
- Confirm on a Linux host with a fast query that the query-paced loop
  behaves the same, and note the steady-state query count.

## Open questions

- Whether `MaxExtent` at 1/8 of the period, 125 ms, is too generous: it
  is about 12 percent of a core on the Mac for the few seconds before
  acquisition takes over. Revisit from the Mac runs.
- Whether the same `K` serves the GPIO reader (#460), whose bracket is
  tens of microseconds and whose scheduler jitter is therefore many
  brackets; the miss-double-shrink cycle may be more visible there.

The following were raised by review of the implementation and by the
first day's hardware runs (Mac mini with an FT232R on CTS, a native UART
on DCD and an FT232R on CTS on a Linux desktop) and are undecided.

- **The bracket interval.** The pin is sampled at an unknown instant
  inside each query, so the edge is only known to lie between the start
  of the off query and the end of the on query. The midpoint-to-midpoint
  bracket understates that interval, by up to 2.5 times at `R = 3`, and
  so does the `Uncertainty` built from it. This is the "two queries
  slowed alike" case: both stretched to 2 ms gives `Uncertainty` 1 ms,
  which the consumer forwards, with an error of up to 2 ms; one such edge
  (1.33 ms late, `Uncertainty` 746 us) was forwarded in 50 minutes on
  the Mac. The alternative is timestamp and `Uncertainty` from
  `[prev.start, cur.end]`: the same midpoint in the symmetric case,
  double the `Uncertainty`, so the consumer's limit withholds the
  symmetric slowdown by itself, at the cost of `Uncertainty` about
  200 us instead of 100 us on the Mac. In simulation of the real loop the
  x8 symmetric slowdown is then withheld and the quiet-case maximum
  error falls from 375 to 230 us. `K` would go from 8 to 4 to keep the
  query count.
- **Acquisition on rejected catches.** The rule that a rejected catch
  neither halves the spacing nor completes acquisition is unbounded: a
  device whose catching query is always more than `R` times longer than
  its neighbours never acquires. Recorded as implausible: a stall hits
  one catch and the next is clean, and no known driver does extra work
  only on the query that first sees a transition; none of the three
  devices tested shows any such asymmetry. The consumer warns after 30
  consecutive rejected edges, so the state would at least be visible.
- **Rejected catches as failures.** The plan counts them, so ten
  consecutive rejected catches hand back to acquisition, which rejects
  the same catches and so has no corrective effect; ten, twenty and
  forty stalled catches cost gaps of 14, 24 and 44 s including the
  reacquisition, the last past chrony's 32 s reachability. The
  alternative is to count only misses, on the same reasoning the plan
  applies to the uncertainty limit, leaving a rejected catch a pure
  observer. The longest run seen on hardware was 7, before the absolute
  floor was added to the tests, and 2 since.
- **The basis of the rejection floor.** The tests need a floor, since
  on the UART pure ratios rejected runs of good catches over
  disturbances of a few microseconds. The implementation uses
  `MinSpacing`, 50 us, but that is the loop's CPU budget, not the thing
  the floor guards. What the floor stands for is the host's ordinary
  timing noise, a scheduler tick or an interrupt, tens of microseconds,
  below which two readings are not distinguishable. The candidates are a
  constant with that justification, which is a host timing of the kind
  this plan avoids, or a per-host estimate from the query durations the
  loop already sees, which reintroduces history into the rejection
  decision. Neither is attractive; undecided.
- **The extent floor on a fast UART.** With 3.6 us queries the bracket
  is about 4.5 us and the extent shrinks to `K` brackets, 35 us, so
  scheduler jitter above about 17 us costs a miss: 11 isolated misses in
  20 minutes unloaded. Within the objective, but one pulse in a hundred
  for a few microseconds of coverage that cost nothing. The alternative
  is to stop shrinking at `K` times the larger of the bracket and
  `MinSpacing`, never below 400 us: no change on the Mac or the FT232R,
  about 90 more reads per second on the UART. `MinSpacing` is already
  the floor of the rejection tests, the scale below which the loop does
  not distinguish timing.
- **The early open on Linux.** `sleepDuration` truncates the wait for
  the window open to whole milliseconds, so the window opens up to 1 ms
  early and the loop reads through that millisecond: on the UART about
  350 reads per window at a 35 us extent, 0.1 percent of a core at
  3.6 us per read. The old code did the same. It matters only for the
  GPIO reader (#460), where the read is cheaper still and the count
  correspondingly higher. Options: accept it; spin the remainder, which
  the plan rejected for the inter-query spacing on the grounds that
  spinning costs the same as reading; or sleep the remainder with a
  direct `nanosleep` from `sleep_linux.go`, the runtime timer taking the
  whole milliseconds and the syscall the rest. The poller already holds
  `LockOSThread`, so blocking its thread costs nothing extra. That would
  also honour `MinSpacing` on Linux, making the UART loop timer-paced at
  about 10 reads per window instead of query-paced at 350, and flipping
  the gap test's `slept` gate on as on the Mac. Overshoot is bounded by
  the thread's timer slack, 50 us by default and reducible with
  `PR_SET_TIMERSLACK`; the remainder is under a millisecond, so its not
  being interruptible by the context does not affect shutdown. FreeBSD
  may want the same. To be measured on the UART for reads per window and
  overshoot before deciding.
