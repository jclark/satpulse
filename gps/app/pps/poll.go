package pps

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/jclark/satpulse/time/lib/median"
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
	// A precise wait must end at the scheduled time rather than before it,
	// however coarse the host's timer; acquisition asks for one because it
	// polls its way to any deadline it wakes short of.
	Wait func(ctx context.Context, t time.Time, precise bool) (bool, error)
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
	lastWidth   time.Duration
	widths      *median.Window[time.Duration]
	// lead is an exponentially weighted moving average of how long after
	// its scheduled time a window's first query completes: the timer's
	// overshoot plus a query slowed by the idle wait before it. Each window
	// opens that much early, so the first query completes, on average, at
	// the nominal open.
	lead       time.Duration
	slept      bool
	stateReads int
}

// Poll adaptively polls for the pulse read by r and sends a candidate for
// every leading edge it catches. It repeatedly runs acquisition followed by
// tracking. Acquisition ends when polling resolution is acquired, or restarts
// from cold after losing the partly acquired signal. Tracking polls at the
// finest cadence the host has and adapts only the extent of the window
// around the predicted edge: a catch shrinks it toward a few brackets,
// a miss grows it by a quarter while the polling budget allows, and
// sustained failure restarts the cycle from acquisition. Every window also
// opens early by the measured lead of its first query, so that the extent
// before the prediction is actually covered.
//
// Every catch is sent, with the midpoint of the two query midpoints as its
// timestamp, Uncertainty reaching the outer endpoints of those queries, and
// Anomalous set when the outer width exceeds four times its recent median.
// Consumers forward a candidate that is not anomalous and whose larger
// uncertainty is within their limit. Every caught edge is logged to lg at
// debug level. Tracking starts, halvings of the extent, misses, and loss are
// logged at info level with actual state-read counts. If stats is non-nil,
// Poll records timing and outcome statistics in it.
func Poll(ctx context.Context, lg *slog.Logger, r PulseReader, params PollParams, ceCh chan<- CandidateEdge, stats *PollStats) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := tightenTimerSlack(); err != nil {
		lg.Debug("serial PPS could not tighten timer slack", "err", err)
	}
	stats.begin()
	if params.InitialPolls == 0 {
		params.InitialPolls = initialPolls
	}
	if params.MinSpacing == 0 {
		params.MinSpacing = minSpacing
	}
	p := poller{ctx: ctx, lg: lg, r: r, params: params, ceCh: ceCh, stats: stats,
		widths: median.New[time.Duration](widthHistory)}
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

// outcome classifies one polling window: no valid transition seen, or a catch.
type outcome uint8

const (
	miss outcome = iota
	caught
)

// acquire searches for the pulse while reducing an independent poll spacing.
// It starts with initialPolls intervals across the full-period window. Every
// catch halves the spacing down to minSpacing and sets the next window to
// initialPolls times that spacing; a miss leaves the spacing unchanged. The
// caught interval determines candidate uncertainty but does not constrain
// this descent.
//
// A catch at minSpacing acquires immediately. Two consecutive caught
// windows with no scheduled sleep also acquire, confirming at successively
// smaller spacings that the state queries pace the loop. A slept catch or
// miss resets that confirmation. Misses at the full-period window sweep the
// poll-grid phase; missLimit misses after the window narrows abandon this
// attempt. The returned duration is the extent with which tracking should
// begin.
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
		if o == caught {
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
				return initialPolls * spacing, true, nil
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
	width           time.Duration // outer interval width; used only on catches
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
			width: p.lastWidth, bracket: p.lastBracket, stateReads: p.stateReads}, err
	}
	advance := func(d time.Duration) { p.nextEdge = p.nextEdge.Add(d) }
	return track(extent, p.params.MinSpacing, attempt, advance, p.logTrackEvent)
}

// The tracking constants control dynamics and polling work, without a
// hardware-dependent time limit on coverage.
const (
	// shrinkStop is the extent, in brackets, at which catches stop
	// shrinking it. It is in brackets rather than a duration because the
	// right extent differs by an order of magnitude between a fast UART
	// query and a USB one.
	shrinkStop = 8
	// shrinkDivisor sets the shrink rate: each catch drops the extent
	// by 1/shrinkDivisor until shrinkStop brackets stop it.
	shrinkDivisor = 32
	// growthDivisor sets miss growth to a quarter of the extent, limiting
	// how far one expansion can overshoot the polling budget at a steady pace.
	growthDivisor = 4
	// maxPolls is the threshold for further growth, not a limit on an
	// attempt: polling continues through the full extent or until a catch.
	maxPolls = 50
	// failureLimit consecutive misses return tracking to acquisition.
	failureLimit = 10
)

// track maintains the extent of the polling window with one feedback loop.
// Polling resolution does not depend on the extent, so the extent is
// coverage and CPU only. A catch advances the prediction by a period,
// adding half its prediction error only when its outer interval is no wider
// than half the extent: a coarse measurement must not displace a narrow
// search window. Every catch resets failures and shrinks the extent by
// 1/shrinkDivisor down to shrinkStop brackets; it never widens it, so no
// catch, however wide its bracket, can add coverage. A miss
// grows the extent by a quarter unless both maxPolls reads and an extent of
// maxPolls*minSpacing have been reached. Ignoring short sleeps can cost more
// polls, but must not prevent growth to that coverage.
// This finds the margin a jittery host needs while using observed polling
// work to decide whether more coverage is affordable. Only failureLimit
// consecutive misses hand back to acquisition.
// Misses are reported as they happen; shrinking only once per halving, to
// keep the log quiet.
func track(extent, minSpacing time.Duration, attempt func(time.Duration) (trackObservation, error),
	advance func(time.Duration), report func(trackEvent)) error {
	report(trackEvent{kind: trackStarted, extent: extent})
	logged := extent
	failures := 0
	for {
		obs, err := attempt(extent)
		if err != nil {
			return err
		}
		if obs.outcome == caught {
			d := period
			if obs.width <= extent/2 {
				d += obs.predictionError / 2
			}
			advance(d)
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
		next := extent
		if obs.stateReads < maxPolls || extent < maxPolls*minSpacing {
			next += extent / growthDivisor
		}
		if failures >= failureLimit {
			report(trackEvent{kind: trackLost, extent: extent, nextExtent: next, observation: obs, failures: failures})
			return nil
		}
		report(trackEvent{kind: trackMissed, extent: extent, nextExtent: next, observation: obs, failures: failures})
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
	case trackLost:
		p.lg.Info("serial PPS track status", "reason", "lost",
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
	first, err := p.readState(time.Time{}, false)
	if err != nil {
		return err
	}
	p.stats.addPoll(first.poll, nil)
	p.nextEdge = first.poll.midpoint().mono.Add(maxWindow / 2)
	return nil
}

// leadWeight is the reciprocal weight of the newest observation in the
// exponentially weighted moving average that is the lead: a sustained
// change is two thirds adopted after that many windows, and a single stall
// moves the lead by only that fraction of its excess.
const leadWeight = 8

// pollWindow waits for one window to open, polls through a pulse already in
// progress, hunts for the next leading edge, classifies the outcome, records
// statistics, and sends a caught candidate. It returns the outcome and, for
// a catch, its error from the predicted edge. The wait for the window open
// is excluded from slept. Acquisition waits precisely, because its spacings
// are long enough to sleep and a wait ending short of a grid point is spent
// reading the rest of the way to it; tracking's spacing is too short to
// sleep at all, so only its window open would be affected, and opening early
// is the coverage its extent does not guarantee. During acquisition it also
// advances the prediction: to the caught edge for a catch, by one period
// otherwise.
func (p *poller) pollWindow(window, spacing time.Duration, acquired bool) (outcome, time.Duration, error) {
	precise := !acquired
	nextEdge := p.nextEdge
	deadline := nextEdge.Add(window / 2)
	open := nextEdge.Add(-window / 2)
	// The first query is scheduled a lead before the open so that it
	// completes at the open; the lead moves only this query, never the
	// deadline or the prediction. It is clamped at zero so that an early
	// wakeup (a truncated sleep) never schedules the query later than the
	// open.
	lead := p.lead
	start := open.Add(-lead)
	if p.params.PreWarm > 0 {
		if _, err := p.wait(start.Add(-p.params.PreWarm), precise); err != nil {
			return miss, 0, err
		}
		for p.now().mono.Before(start) {
			if p.ctx.Err() != nil {
				return miss, 0, p.ctx.Err()
			}
		}
	}
	cur, err := p.readState(start, precise)
	if err != nil {
		return miss, 0, err
	}
	first := cur
	p.lead = max(0, p.lead+(first.poll.end.mono.Sub(start)-p.lead)/leadWeight)
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
		cur, err = p.readState(nextGridPoint(open, spacing, cur.start), precise)
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
	var stamp time.Time
	var uncertainty, pollWidths [2]time.Duration
	for !missed && edge.stamp.IsZero() {
		cur, err = p.readState(nextGridPoint(open, spacing, prev.start), precise)
		if err != nil {
			return miss, 0, err
		}
		p.stats.addPoll(cur.poll, &prev.poll)
		p.stateReads++
		p.slept = p.slept || cur.slept
		edge, missed = classify(prev, cur, deadline)
		if !edge.stamp.IsZero() {
			p.lastBracket = cur.poll.midpoint().elapsedSince(prev.poll.midpoint())
			uncertainty = [2]time.Duration{edge.elapsedSince(prev.poll.start), cur.poll.end.elapsedSince(edge)}
			p.lastWidth = uncertainty[0] + uncertainty[1]
			pollWidths = [2]time.Duration{prev.poll.duration(), cur.poll.duration()}
			stamp = reconciledStamp(prev.poll, cur.poll)
		}
		prev = cur
	}
	p.lg.Debug("serial PPS poll window", "tracking", acquired, "caught", !edge.stamp.IsZero(),
		"window", window, "spacing", spacing, "stateReads", p.stateReads,
		"lead", lead, "wakeLate", first.start.Sub(start), "firstEndFromOpen", first.poll.end.mono.Sub(open),
		"firstPollWidth", first.poll.duration(), "firstInPulse", first.inPulse,
		"lastStartFromPrediction", cur.start.Sub(nextEdge), "lastPollWidth", cur.poll.duration(), "lastInPulse", cur.inPulse)
	if edge.stamp.IsZero() {
		p.stats.addWindow(miss, acquired, false)
		if !acquired {
			p.nextEdge = nextEdge.Add(period)
		}
		return miss, 0, nil
	}
	predictionError := edge.mono.Sub(nextEdge)
	anomalous := p.anomalous(p.lastWidth, acquired)
	// "late" is how far past its scheduled time the catching poll started:
	// sleep overshoot when the loop is sleep-paced, queue debt when the queries
	// pace it.
	p.lg.Debug("serial PPS caught edge", "window", window, "bracket", p.lastBracket,
		"uncertainty", uncertainty, "pollWidths", pollWidths,
		"predictionError", predictionError, "late", cur.start.Sub(cur.sched), "stateReads", p.stateReads,
		"anomalous", anomalous)
	p.stats.addWindow(caught, acquired, anomalous)
	if !acquired {
		p.nextEdge = edge.mono.Add(period)
	}
	ce := CandidateEdge{
		Edge: Edge{
			Timestamp: stamp,
			TRead:     cur.poll.end.mono,
		},
		Uncertainty: uncertainty,
		PollWidths:  pollWidths,
		Anomalous:   anomalous,
	}
	select {
	case p.ceCh <- ce:
		return caught, predictionError, nil
	case <-p.ctx.Done():
		return miss, 0, p.ctx.Err()
	}
}

const (
	widthHistory = 31
	anomalyRatio = 4
)

// anomalous compares the outer width with previous tracking catches before
// recording it. Anomalous widths still enter the history so a sustained
// change can become ordinary; coarse acquisition widths never enter it.
func (p *poller) anomalous(width time.Duration, acquired bool) bool {
	anomalous := p.widths.Len() > 0 && width > anomalyRatio*p.widths.Median()
	if acquired {
		p.widths.Add(width)
	}
	return anomalous
}

// nextGridPoint is the first point of the poll grid anchored at open strictly
// after t. Queries target grid points rather than an interval after the
// previous query, so that a late or early query, or a stall, does not shift
// the rest of the window's grid.
func nextGridPoint(open time.Time, spacing time.Duration, t time.Time) time.Time {
	d := t.Sub(open)
	n := d / spacing
	if d%spacing < 0 {
		n--
	}
	return open.Add(spacing * (n + 1))
}

func (p *poller) readState(sched time.Time, precise bool) (reading, error) {
	slept, err := p.wait(sched, precise)
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

func (p *poller) wait(t time.Time, precise bool) (bool, error) {
	if p.params.Wait != nil {
		return p.params.Wait(p.ctx, t, precise)
	}
	return waitUntil(p.ctx, t, precise)
}

// waitUntil reports whether it actually had to wait: false means the
// scheduled time was already past, or, for a wait that is not precise, nearer
// than the runtime timer can sleep to. A precise wait always ends at the
// deadline: the runtime timer takes the whole milliseconds, so the wait stays
// cancellable, and sleepRemainder takes the rest, which is all of it when the
// wait is shorter than the timer's resolution.
func waitUntil(ctx context.Context, t time.Time, precise bool) (bool, error) {
	slept := false
	if d := sleepDuration(time.Until(t)); d > 0 {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			slept = true
		case <-ctx.Done():
			return false, ctx.Err()
		}
	} else if err := ctx.Err(); err != nil {
		return false, err
	}
	if precise && sleepRemainder(t) {
		slept = true
	}
	return slept, nil
}

// classify gives a detected transition precedence over the deadline.
// The deadline says when to stop looking, not whether a measured edge is
// valid. The outer interval must be shorter than a period to identify one
// leading edge. The query endpoints must also be in order: a backward step
// of the measurement clock (whose stamps carry no monotonic reading on
// Windows) can otherwise produce an invalid interval or poll width.
func classify(prev, cur reading, deadline time.Time) (clockReading, bool) {
	if !prev.inPulse && cur.inPulse {
		if d := cur.poll.end.elapsedSince(prev.poll.start); d >= period || d <= 0 {
			return clockReading{}, true
		}
		if prev.poll.duration() < 0 || cur.poll.gapAfter(prev.poll) < 0 || cur.poll.duration() < 0 {
			return clockReading{}, true
		}
		return prev.poll.midpoint().midpoint(cur.poll.midpoint()), false
	}
	return clockReading{}, !cur.poll.midpoint().mono.Before(deadline)
}

// reconciledStamp interpolates the edge's wall-clock time from the four clock
// samples bracketing it. It repeats the interpolation classify performs, but
// over wall readings reconciled against the monotonic ones. The edge's wall
// time is otherwise anchored on the single sample the interpolation starts
// from, so a delay between that sample's two clock reads slides the reported
// timestamp off the edge while leaving its uncertainty the usual width.
func reconciledStamp(prev, cur poll) time.Time {
	w := ReconcileTimes(
		[]time.Time{prev.start.stamp, prev.end.stamp, cur.start.stamp, cur.end.stamp},
		[]time.Time{prev.start.mono, prev.end.mono, cur.start.mono, cur.end.mono})
	return midpoint(midpoint(w[0], w[1]), midpoint(w[2], w[3]))
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
