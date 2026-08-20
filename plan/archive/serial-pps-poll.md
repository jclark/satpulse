# Serial PPS polling design

This records the design of `gps/app/serialpps/poll.go` as of the
serial-pps-poll branch (PR #419): how the poller finds a once-per-second
pulse on a modem-control pin, how it tracks the pulse cheaply, and why
each rule is shaped the way it is.

## The problem

A receiver pulses a modem-control pin (DCD or CTS) once a second. The
only way to see the pulse without kernel support is to read the pin
state repeatedly: an edge is visible only as a difference between two
adjacent reads. Each read costs a system call (100-200 us of CPU on
typical hardware), so reading all second long is out of the question.
The poller therefore predicts when the next leading edge will arrive,
polls only inside a window around that prediction, and adapts the
window from what it observes.

Two host regimes must both work:

- query-paced: the state read itself is slower than the requested poll
  spacing, so reads run back to back and pace the loop;
- sleep-paced: reads are fast, and the loop sleeps between reads.

Nothing in the design encodes hardware timing. Every time scale is
measured: the bracket (interval between the two reads around an edge),
the prediction error, and misses. The constants are dimensionless
rates and counts.

## One attempt: pollWindow

Each second, `pollWindow` runs one attempt against the prediction
`nextEdge`:

1. Sleep until the window opens at `nextEdge - window/2`.
2. If the pin is already in a pulse, poll through it. Windows advance
   in lockstep with the pulses, so treating an in-progress pulse as a
   miss would reopen at the same phase every period and never resolve
   it.
3. Hunt for the leading edge, reading at the requested spacing (or
   back to back when query-paced), until the edge is found or the
   deadline `nextEdge + window/2` passes.

The edge is located at the midpoint of the two reads that bracket it,
so the published uncertainty is half the bracket. A bracket of a full
period or more may contain several edges and identifies none of them:
it is a miss. `predictionError` is the located edge minus `nextEdge`.

Two clocks are read around every state query: the measurement clock
stamps published edges and measures short intervals; the monotonic
clock paces the loop, so a step in the system clock cannot disturb the
schedule. Whether any scheduled read actually had to sleep is
recorded; a window whose reads never slept is query-paced.

Candidates are published with their uncertainty and a Settled flag,
which says no improvement in accuracy is to be expected: the polling
schedule did not limit the measurement (its spacing was at the floor,
or the state queries paced the window), or the window has stopped
shrinking. Candidates are unsettled during acquisition, including the
catch that completes it (the flag records the state in which the edge
was captured, and acquisition succeeds as a consequence of that
catch), and unsettled again during tracking while a window grown by
misses is still shrinking back, since the uncertainty of a sleep-paced
window falls as the window does. The same capture-time rule attributes
the acquisition-ending window to the acquire phase in the statistics.
Consumers decide what to accept; the poller only reports what it
measured.

## Acquisition: acquire

Acquisition finds the pulse from nothing and establishes the usable
polling resolution. It controls an independent poll spacing, not the
window: the window is always `initialPolls` (64) spacings, starting at
the full period with 15.6 ms spacing.

- Every catch halves the spacing, down to the floor `minSpacing`
  (50 us), which bounds CPU when the state query is very fast.
- A miss leaves the spacing unchanged.
- A miss advances the prediction by exactly one period, matching the
  pulse, so a locked poll grid could straddle a pulse narrower than
  the spacing forever; each miss therefore advances the grid by an
  irregular 0.618 of the spacing, sweeping the phase.
- After the window has narrowed, missLimit (10) consecutive misses
  abandon the attempt and restart cold.

Acquisition ends when a catch happens at the
spacing floor, or when two consecutive caught windows ran with no
scheduled sleep. The second condition confirms at successively smaller
spacings that the state queries themselves pace the loop: on such a
host the spacing cannot be met anyway, and waiting for the floor would
stall. A slept catch or a miss resets that confirmation.

The bracket deliberately does not steer this descent. Brackets are
noisy (a single slow read stretches one), and earlier designs that
latched on bracket comparisons declared acquisition over early on noise, publishing
millisecond-class samples from a still-wide window.

## Tracking: track

Tracking runs one feedback loop over `window`, the count of
consecutive misses, and the count of consecutive catches. There is no
remembered floor: what a miss buys is a hold, not a value.

On a catch:

- Advance the prediction by one period plus half of
  `predictionError`. The half gain means one noisy edge estimate
  cannot displace the next window by its full error; the cost is a
  small standing lag under clock drift, which the window covers
  because the lag appears inside the measured error.
- If the last run of misses was short (fewer than absentRun) and
  fewer than shrinkAfter (300) consecutive catches have followed it,
  the window is frozen: it holds, expanding immediately if this
  catch's measured requirement `2*(|predictionError| + bracket)`
  exceeds it, but never shrinking.
- Otherwise shrink the window by 1/trackRelease (1/16), but never
  below `2*(|predictionError| + bracket)`.

On a miss:

- Advance the prediction by exactly one period.
- First miss of a run: grow the window by a bracket at each end.
- Each further consecutive miss: double the window, capped at the
  full period.
- With the window at the full period, missLimit consecutive misses
  declare the pulse gone and return to acquisition.
- A run of absentRun (3) or more misses does not freeze the window:
  shrinking resumes on the recovery catch.

### Why these rules

Shrink by 1/16. Proportional shrinking is fast when the window is far
too big (3.2 ms to 1 ms in about 20 catches) and gentle near the
right size, so one rate serves both initial convergence and recovery
after growth. The previous design needed two separate laws here:
blind geometric halving that overshot, and an additive crawl that
took hundreds of pulses.

The shrink limit `2*(|predictionError| + bracket)`. This is what the
catch itself proved necessary: the prediction was off by
`|predictionError|`; that measurement is only good to half a bracket;
and an equal offset next second must land at least one read inside
the window edge, not on it. The old halving ignored the first term,
which is why it overshot on hardware with real prediction error.

The freeze. Some causes of misses never appear on a caught pulse.
The motivating case: the sleep to the window open can occasionally
wake about 0.9 ms late (measured on the development host), so with a
small window the edge has passed before polling starts. Every caught
pulse still measures a tiny prediction error, so shrinking looks safe
right up until it is not. The miss is the only evidence, and the
freeze is how it is respected: after the miss, the grown window holds
for shrinkAfter catches instead of shrinking straight back into the
size that failed. Nothing is remembered beyond the hold, so a
mistaken hold costs at most five minutes; an earlier design kept a
persistent floor learned from misses, and a miss whose real cause was
an absent pulse (a dropout during the descent from an outage) could
plant that floor at fifty times the right size for hours.

The freeze expiring is also the re-test. Whether a smaller window
works can change (load, kernel configuration, hardware), and the loop
cannot find out without trying: on a caught pulse everything looks
fine at any window size. When the hold ends, shrinking resumes and
probes below the size that missed; if it still misses, the price is a
pulse or two and another hold. Requiring consecutive catches matters:
the probe happens only from a window size that is demonstrably
reliable.

The absentRun exception. A single miss, or a pair (on late-wake
hardware the boundary is hit singly or in pairs, one doubling putting
the window far above it), is the shape of a too-small window and
deserves the cautious hold. Three or more consecutive misses can only
mean the pulse was absent, which says nothing about window size;
holding the outage-grown window would keep candidates unsettled and
the refclock without samples for the whole hold, so shrinking resumes
immediately on recovery.

Doubling on consecutive misses. While pulses are being missed there
are no observations and the prediction only coasts, so its
uncertainty grows with every missed second. Additive growth chased
that at two brackets per second and could take four misses to
reconnect. Doubling reconnects in one or two, and makes short outages
cheap: after an eight-second outage the window has grown faster than
the clock can have drifted, so the first pulse after the outage is
caught, and the 1/16 shrink then walks straight back down. The
prediction is kept through short outages; longer ones are rare enough
that the cost of a full reacquisition does not matter.

### Reporting

Misses and the recovery from a run of misses are reported at INFO as
they happen; other window changes only once per doubling or halving.
A quiet minute in the
log is a minute in which every pulse was caught and nothing changed
by more than half.

## Steady state

On hardware whose only disturbance is edge jitter, the window rides
`2*(|predictionError| + bracket)`: about 1 ms and three to five reads
per pulse on the bench FT232R receiver. On hardware with late wakes,
the window holds just above the size that misses (about 2 ms on the
development host) and loses a pulse or two only when each five-minute
hold expires and the probe re-finds the boundary. Timestamp quality
is the same either way:
published uncertainty comes from the bracket, not the window.

## Testing

The simulation in `poll_test.go` executes the production `track`
function; polling and prediction advancement are behind function
parameters so the tests run the exact production control flow. The
simulation models absolute edge times (so the prediction law is
exercised, not assumed), per-attempt query pacing, sporadically late
window opens, dropouts, and outages, across five scenarios for
twelve simulated minutes, which covers a full shrinkAfter hold.
`TestTrackFeedback` pins the update rules step by step with exact
expected windows, prediction advances, and reported events. The
remaining tests drive the real `Poll` against a fake pulse, covering
acquisition (spacing floor, query-paced confirmation, sleep jitter,
coarse state refresh), missed-pulse handling, outage reacquisition, and
steady-state read cost.

## History

Three earlier designs inform the current shape:

- Geometric halving to `2*bracket` plus additive steady-state
  tracking: halving ignored prediction error and overshot; additive
  recovery needed four missed pulses on hardware; additive reduction
  of an oversized window took ~900 pulses.
- A nominal poll-count controller: converted a poll count to a window
  with a fixed 50 us spacing that had no relation to the actual query
  cadence on query-paced hosts, so the controlled quantity was
  fictional. Lesson: control the time window; state reads are an
  observed consequence.
- A pure shrink-with-evidence-limit loop with no response to misses
  beyond growth: kept shrinking back into the late-wake zone it could
  not observe, missing every ~20 s. Lesson: a miss must suspend
  shrinking for a long time, not just grow the window.
- A persistent floor (`minWindow`) set by the first miss of every run
  and stepped down every shrinkAfter catches: held the late-wake
  hardware perfectly, but charged every miss to window size, so a
  dropout during a post-outage descent could plant the floor at the
  transiently huge window for hours. Lesson: remembered values
  outlive their evidence; a bounded hold buys the same stability
  without the permanence.
