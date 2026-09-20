package pps

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// PulseReader reads whether a pin is currently in a pulse. Its query time
// bounds the resolution of polling, so it should be as fast as the platform
// allows.
type PulseReader interface {
	InPulse() (bool, error)
}

// PollParams tunes Poll to the cost of the reader's queries. The zero value
// suits serial modem-status queries, which take from tens of microseconds to
// milliseconds, and sleeps on the runtime's timers.
type PollParams struct {
	// InitialPolls is the number of polls across the cold-start window. It
	// determines the initial acquisition spacing, and with it the narrowest
	// pulse acquired promptly. Zero means 64.
	InitialPolls int
	// MinSpacing bounds the CPU spent when the query is very fast: it is the
	// sleep between queries where the platform can sleep that briefly, and
	// nothing where it cannot. Zero means 50 microseconds.
	MinSpacing time.Duration
	// PreWarm ends the sleep to each window open that much early and
	// busy-waits the remainder. It is for hosts whose queries slow down
	// severalfold while the machine idles, where only continuous work
	// ending at the open restores full query speed; it costs that fraction
	// of a core. Zero disables it.
	PreWarm time.Duration
	// Wait, if non-nil, replaces the runtime timer that sleeps until a
	// scheduled poll. It reports whether it actually waited: false means
	// the scheduled time was already past or nearer than it can sleep to.
	Wait func(ctx context.Context, t time.Time) (bool, error)
	// Now, if non-nil, replaces the clock the loop reads, so that a
	// simulation can drive it in virtual time together with Wait. The
	// reading serves as both the measurement stamp and the pacing
	// coordinate.
	Now func() time.Time
}

type poller struct {
	ctx         context.Context
	lg          *slog.Logger
	r           PulseReader
	params      PollParams
	ceCh        chan<- CandidateEdge
	stats       *PollStats
	nextEdge    time.Time
	lastBracket time.Duration
	slept       bool
	stateReads  int
}

// Poll adaptively polls for the pulse read by r and sends a candidate for
// every leading edge it catches. It repeatedly runs acquisition followed by
// tracking. Acquisition ends when polling resolution is acquired, or restarts
// from cold after losing the partly acquired signal. Tracking polls at the
// finest cadence the host has and adapts only the extent of the window
// around the predicted edge: a good catch shrinks it toward a few brackets,
// a miss doubles it, and sustained failure restarts the cycle from
// acquisition.
//
// Every catch is sent, with its bracket midpoint as timestamp, half the
// bracket as Uncertainty, and Rejected set when the timing of the two
// queries around the edge looks disturbed. Consumers forward a candidate
// that is not rejected and whose Uncertainty is within their limit. Every
// caught edge is logged to lg at debug level. Tracking starts, halvings of
// the extent, misses, rejections, and loss are logged at info level with
// actual state-read counts. If stats is non-nil, Poll records timing and
// outcome statistics in it.
func Poll(ctx context.Context, lg *slog.Logger, r PulseReader, params PollParams, ceCh chan<- CandidateEdge, stats *PollStats) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	stats.begin()
	if params.InitialPolls == 0 {
		params.InitialPolls = initialPolls
	}
	if params.MinSpacing == 0 {
		params.MinSpacing = minSpacing
	}
	p := poller{ctx: ctx, lg: lg, r: r, params: params, ceCh: ceCh, stats: stats}
	if err := p.init(); err != nil {
		return err
	}
	for {
		extent, acquired, err := p.acquire()
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		if err := p.track(extent); err != nil {
			return err
		}
	}
}

const (
	period = time.Second
	// maxWindow is the whole period: the cold-start window, within which
	// polling is uniform.
	maxWindow = period
)

// None of these constants encodes hardware timing; everything hardware- and
// load-dependent is measured by the polling loop itself. initialPolls and
// minSpacing are the PollParams defaults, suited to serial modem-status
// queries.
const (
	initialPolls = 64
	// missLimit consecutive misses after acquisition's window has narrowed
	// declare the pulse gone, and acquisition restarts from cold.
	missLimit  = 10
	minSpacing = 50 * time.Microsecond
)

// outcome classifies one polling window: no transition seen, a transition
// whose surrounding queries look disturbed, or a good catch.
type outcome uint8

const (
	miss outcome = iota
	rejectedCatch
	goodCatch
)

// acquire searches for the pulse while reducing an independent poll spacing.
// It starts with initialPolls intervals across the full-period window. Every
// good catch halves the spacing down to minSpacing and sets the next window to
// initialPolls times that spacing; a miss leaves the spacing unchanged. The
// bracket measures candidate uncertainty but does not constrain this descent.
//
// A good catch at minSpacing acquires immediately. Two consecutive caught
// windows with no scheduled sleep also acquire, confirming at successively
// smaller spacings that the state queries pace the loop. A slept catch or
// miss resets that confirmation. A rejected catch shows the pulse is present
// but is not trusted: it resets the miss count and the confirmation and
// leaves the spacing unchanged. Misses at the full-period window sweep the
// poll-grid phase; missLimit misses after the window narrows abandon this
// attempt. The returned duration is the extent with which tracking should
// begin, at most maxExtent.
func (p *poller) acquire() (time.Duration, bool, error) {
	initialPolls, minSpacing := time.Duration(p.params.InitialPolls), p.params.MinSpacing
	spacing := maxWindow / initialPolls
	misses, queryPaced := 0, 0
	for {
		window := initialPolls * spacing
		o, _, err := p.pollWindow(window, spacing, false)
		if err != nil {
			return 0, false, err
		}
		if o == rejectedCatch {
			misses, queryPaced = 0, 0
			continue
		}
		if o == goodCatch {
			misses = 0
			acquired := spacing == minSpacing
			if p.slept {
				queryPaced = 0
			} else {
				queryPaced++
				acquired = acquired || queryPaced >= 2
			}
			if acquired {
				p.lg.Debug("serial PPS acquired", "window", window, "bracket", p.lastBracket)
			}
			if spacing > minSpacing {
				spacing /= 2
				if spacing < minSpacing {
					spacing = minSpacing
					p.lg.Debug("serial PPS poll window reached spacing floor", "window", initialPolls*spacing)
				} else {
					p.lg.Debug("serial PPS poll window halved", "window", initialPolls*spacing)
				}
			}
			if acquired {
				return min(initialPolls*spacing, maxExtent), true, nil
			}
			continue
		}
		queryPaced = 0
		// A miss advances the prediction by exactly one period, matching the
		// pulse, so a locked poll grid would revisit the same phases every
		// period and could straddle a pulse narrower than the spacing
		// indefinitely; advancing the grid by an irregular fraction of the
		// spacing sweeps the phase instead.
		p.nextEdge = p.nextEdge.Add(spacing * 618 / 1000)
		if window == maxWindow {
			continue
		}
		misses++
		if misses >= missLimit {
			p.lg.Debug("serial PPS pulse lost, restarting acquisition", "window", window, "misses", misses)
			return 0, false, nil
		}
	}
}

type trackObservation struct {
	outcome         outcome
	predictionError time.Duration
	// bracket is the most recent caught bracket: this catch's own, or on a
	// miss the previous catch's, since a missed window measures none.
	bracket    time.Duration
	stateReads int
}

type trackEventKind uint8

const (
	trackStarted trackEventKind = iota
	trackChanged
	trackMissed
	trackRejected
	trackLost
)

type trackEvent struct {
	kind               trackEventKind
	extent, nextExtent time.Duration
	observation        trackObservation
	failures           int
}

// track adapts the real polling window and logging to the shared tracking
// control. Tests call the same track function with a simulated attempt.
func (p *poller) track(extent time.Duration) error {
	attempt := func(extent time.Duration) (trackObservation, error) {
		o, predictionError, err := p.pollWindow(extent, p.params.MinSpacing, true)
		return trackObservation{outcome: o, predictionError: predictionError,
			bracket: p.lastBracket, stateReads: p.stateReads}, err
	}
	advance := func(d time.Duration) { p.nextEdge = p.nextEdge.Add(d) }
	return track(extent, attempt, advance, p.logTrackEvent)
}

// The tracking constants control dynamics only; every time scale comes from
// observed brackets.
const (
	// shrinkStop is the extent, in brackets, at which good catches stop
	// shrinking it. It is in brackets rather than a duration because the
	// right extent differs by an order of magnitude between a fast UART
	// query and a USB one.
	shrinkStop = 8
	// shrinkDivisor sets the shrink rate: each good catch drops the extent
	// by 1/shrinkDivisor until shrinkStop brackets stop it.
	shrinkDivisor = 32
	// rejectRatio is the ratio between the durations of the two queries
	// around an edge, or between the gap before the catching query and the
	// preceding query's duration, beyond which the catch is rejected.
	rejectRatio = 3
	// failureLimit consecutive attempts without a good catch return tracking
	// to acquisition.
	failureLimit = 10
	// maxExtent bounds the extent tracking will poll; a miss whose doubling
	// would exceed it returns to acquisition instead.
	maxExtent = period / 8
)

// track maintains the extent of the polling window with one feedback loop.
// Polling resolution does not depend on the extent, so the extent is
// coverage and CPU only. A good catch advances the prediction by a period
// plus half its prediction error, damping the midpoint's quantisation noise,
// and shrinks the extent by 1/shrinkDivisor down to shrinkStop brackets; it
// never widens it, so no catch, however wide its bracket, can make the loop
// work harder. A miss doubles the extent, which is how the loop finds the
// margin a jittery host needs. A rejected catch is neither evidence about
// coverage nor a trustworthy measurement: it advances the prediction by a
// period and changes nothing else. Misses and rejections count as failures;
// failureLimit consecutive failures, or a doubling that would exceed
// maxExtent, hand back to acquisition, since catching the pulse in an
// inflated extent is far more expensive than reacquiring it. Misses and
// rejections are reported as they happen; shrinking only once per halving,
// to keep the log quiet.
func track(extent time.Duration, attempt func(time.Duration) (trackObservation, error),
	advance func(time.Duration), report func(trackEvent)) error {
	report(trackEvent{kind: trackStarted, extent: extent})
	logged := extent
	failures := 0
	for {
		obs, err := attempt(extent)
		if err != nil {
			return err
		}
		if obs.outcome == goodCatch {
			advance(period + obs.predictionError/2)
			failures = 0
			next := min(extent, max(extent-extent/shrinkDivisor, shrinkStop*obs.bracket))
			if 2*next <= logged {
				report(trackEvent{kind: trackChanged, extent: extent, nextExtent: next, observation: obs})
				logged = next
			}
			extent = next
			continue
		}
		advance(period)
		failures++
		next, kind := extent, trackRejected
		if obs.outcome == miss {
			next, kind = 2*extent, trackMissed
		}
		if failures >= failureLimit || next > maxExtent {
			report(trackEvent{kind: trackLost, extent: extent, nextExtent: next, observation: obs, failures: failures})
			return nil
		}
		report(trackEvent{kind: kind, extent: extent, nextExtent: next, observation: obs, failures: failures})
		logged, extent = next, next
	}
}

func (p *poller) logTrackEvent(e trackEvent) {
	switch e.kind {
	case trackStarted:
		p.lg.Info("serial PPS track status", "reason", "start", "extent", e.extent)
	case trackChanged:
		p.lg.Info("serial PPS track status", "reason", "shrink",
			"extent", e.extent, "nextExtent", e.nextExtent,
			"stateReads", e.observation.stateReads, "bracket", e.observation.bracket,
			"predictionError", e.observation.predictionError)
	case trackMissed:
		p.lg.Info("serial PPS track status", "reason", "miss",
			"extent", e.extent, "nextExtent", e.nextExtent,
			"stateReads", e.observation.stateReads, "bracket", e.observation.bracket,
			"failures", e.failures)
	case trackRejected:
		p.lg.Info("serial PPS track status", "reason", "rejected",
			"extent", e.extent, "stateReads", e.observation.stateReads,
			"bracket", e.observation.bracket,
			"predictionError", e.observation.predictionError, "failures", e.failures)
	case trackLost:
		cause := "failures"
		if e.failures < failureLimit {
			cause = "extent"
		}
		p.lg.Info("serial PPS track status", "reason", "lost", "cause", cause,
			"extent", e.extent, "nextExtent", e.nextExtent, "stateReads", e.observation.stateReads,
			"bracket", e.observation.bracket, "failures", e.failures)
	}
}

// clockReading keeps adjacent readings of the clocks used by the poller
// together. stamp is the measurement reading used for short intervals and
// published edge timestamps; mono paces the polling loop, which must not be
// disturbed by a step in the system clock.
type clockReading struct {
	stamp time.Time
	mono  time.Time
}

// poll retains both clock readings around one modem-state query.
type poll struct {
	start clockReading
	end   clockReading
}

type reading struct {
	inPulse bool
	poll    poll
	start   time.Time
	sched   time.Time // when this poll was scheduled to run
	slept   bool      // whether waiting for the schedule used a timer
}

func (p *poller) init() error {
	first, err := p.readState(time.Time{})
	if err != nil {
		return err
	}
	p.stats.addPoll(first.poll, nil)
	p.nextEdge = first.poll.midpoint().mono.Add(maxWindow / 2)
	return nil
}

// pollWindow waits for one window to open, polls through a pulse already in
// progress, hunts for the next leading edge, classifies the outcome, records
// statistics, and sends a caught candidate. It returns the outcome and, for
// a catch, its error from the predicted edge. The wait for the window open
// is excluded from slept. During acquisition it also advances the
// prediction: to the caught edge for a good catch, by one period otherwise.
func (p *poller) pollWindow(window, spacing time.Duration, acquired bool) (outcome, time.Duration, error) {
	nextEdge := p.nextEdge
	deadline := nextEdge.Add(window / 2)
	open := nextEdge.Add(-window / 2)
	if p.params.PreWarm > 0 {
		if _, err := p.wait(open.Add(-p.params.PreWarm)); err != nil {
			return miss, 0, err
		}
		for p.now().mono.Before(open) {
			if p.ctx.Err() != nil {
				return miss, 0, p.ctx.Err()
			}
		}
	}
	cur, err := p.readState(open)
	if err != nil {
		return miss, 0, err
	}
	p.stats.addPoll(cur.poll, nil)
	p.slept = false
	p.stateReads = 1
	// The windows advance in lockstep with the pulses, so treating an
	// in-progress pulse at the open as a miss would reopen at the same phase
	// every period and never acquire; poll through it instead. slept accumulates
	// over the window's scheduled polls (the wait for the window open is
	// excluded: it always sleeps).
	for cur.inPulse && cur.poll.midpoint().mono.Before(deadline) {
		prev := cur
		cur, err = p.readState(cur.start.Add(spacing))
		if err != nil {
			return miss, 0, err
		}
		p.stats.addPoll(cur.poll, &prev.poll)
		p.stateReads++
		p.slept = p.slept || cur.slept
	}
	prev := cur
	missed := cur.inPulse
	var edge clockReading
	rejected := false
	for !missed && edge.stamp.IsZero() {
		cur, err = p.readState(prev.start.Add(spacing))
		if err != nil {
			return miss, 0, err
		}
		p.stats.addPoll(cur.poll, &prev.poll)
		p.stateReads++
		p.slept = p.slept || cur.slept
		edge, missed = classify(prev, cur, deadline)
		if !edge.stamp.IsZero() {
			p.lastBracket = cur.poll.midpoint().elapsedSince(prev.poll.midpoint())
			rejected = disturbed(prev, cur)
		}
		prev = cur
	}
	if edge.stamp.IsZero() {
		p.stats.addWindow(miss, acquired)
		if !acquired {
			p.nextEdge = nextEdge.Add(period)
		}
		return miss, 0, nil
	}
	predictionError := edge.mono.Sub(nextEdge)
	o := goodCatch
	if rejected {
		o = rejectedCatch
	}
	// "late" is how far past its scheduled time the catching poll started:
	// sleep overshoot when the loop is sleep-paced, queue debt when the queries
	// pace it.
	p.lg.Debug("serial PPS caught edge", "window", window, "bracket", p.lastBracket,
		"predictionError", predictionError, "late", cur.start.Sub(cur.sched), "stateReads", p.stateReads,
		"rejected", rejected)
	p.stats.addWindow(o, acquired)
	if !acquired {
		if rejected {
			p.nextEdge = nextEdge.Add(period)
		} else {
			p.nextEdge = edge.mono.Add(period)
		}
	}
	ce := CandidateEdge{
		Edge: Edge{
			Timestamp: edge.stamp,
			TRead:     cur.poll.end.mono,
		},
		Uncertainty: halfCeil(p.lastBracket),
		Rejected:    rejected,
	}
	select {
	case p.ceCh <- ce:
		return o, predictionError, nil
	case <-p.ctx.Done():
		return miss, 0, p.ctx.Err()
	}
}

// disturbed reports whether the timing of the two queries around an edge
// shows a stall. A stall can land inside either query, where the sampling
// instant within the call is unknown and the midpoint is biased, or between
// them. The duration comparisons see the first two; the gap test sees the
// third, and applies only when no sleep was scheduled before the catching
// query, since a sleep's timer overshoot is not a stall and its ordinary
// size is not something two query durations can reveal. The tests use the
// measurement stamps, like the bracket: on Windows the monotonic reading is
// quantised far more coarsely than a query lasts (see now there), and a
// step of the system clock inside a bracket is already a miss in classify.
func disturbed(prev, cur reading) bool {
	dp, dc := prev.poll.duration(), cur.poll.duration()
	return !cur.slept && cur.poll.gapAfter(prev.poll) > rejectRatio*dp ||
		dc > rejectRatio*dp || dp > rejectRatio*dc
}

func (p *poller) readState(sched time.Time) (reading, error) {
	slept, err := p.wait(sched)
	if err != nil {
		return reading{}, err
	}
	start := p.now()
	inPulse, err := p.r.InPulse()
	end := p.now()
	if err != nil {
		return reading{}, err
	}
	return reading{inPulse: inPulse, poll: poll{start: start, end: end}, start: start.mono,
		sched: sched, slept: slept}, nil
}

func (p *poller) now() clockReading {
	if p.params.Now != nil {
		t := p.params.Now()
		return clockReading{stamp: t, mono: t}
	}
	return now()
}

func (p *poller) wait(t time.Time) (bool, error) {
	if p.params.Wait != nil {
		return p.params.Wait(p.ctx, t)
	}
	return waitUntil(p.ctx, t)
}

// waitUntil reports whether it actually had to wait: false means the
// scheduled time was already past, i.e. the previous state query outlasted
// the poll spacing, or was nearer than the platform can sleep to.
func waitUntil(ctx context.Context, t time.Time) (bool, error) {
	d := sleepDuration(time.Until(t))
	if d <= 0 {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
			return false, nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// classify gives a detected transition precedence over the deadline.
// The deadline says when to stop looking, not whether a measured edge is
// valid. A bracket spanning a full period or more may contain several
// leading edges, so its midpoint identifies none of them, and a nonpositive
// bracket means the measurement clock stepped backward between the reads
// (its stamps carry no monotonic reading on Windows), so its midpoint is
// equally meaningless: both are a miss.
func classify(prev, cur reading, deadline time.Time) (clockReading, bool) {
	if !prev.inPulse && cur.inPulse {
		if d := cur.poll.midpoint().elapsedSince(prev.poll.midpoint()); d >= period || d <= 0 {
			return clockReading{}, true
		}
		return prev.poll.midpoint().midpoint(cur.poll.midpoint()), false
	}
	return clockReading{}, !cur.poll.midpoint().mono.Before(deadline)
}

func halfCeil(d time.Duration) time.Duration {
	return d/2 + d%2
}

func (p poll) midpoint() clockReading {
	return p.start.midpoint(p.end)
}

func (p poll) duration() time.Duration {
	return p.end.elapsedSince(p.start)
}

func (p poll) gapAfter(prev poll) time.Duration {
	return p.start.elapsedSince(prev.end)
}

func (r clockReading) midpoint(other clockReading) clockReading {
	return clockReading{
		stamp: midpoint(r.stamp, other.stamp),
		mono:  midpoint(r.mono, other.mono),
	}
}

func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.stamp.Sub(start.stamp)
}

func midpoint(a, b time.Time) time.Time {
	return a.Add(b.Sub(a) / 2)
}
