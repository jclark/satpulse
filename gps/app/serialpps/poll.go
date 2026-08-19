package serialpps

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

type poller struct {
	ctx         context.Context
	lg          *slog.Logger
	r           StateReader
	w           Wiring
	ceCh        chan<- CandidateEdge
	stats       *PollStats
	nextEdge    time.Time
	lastBracket time.Duration
	slept       bool
	stateReads  int
}

// Poll adaptively polls for the pulse described by w and sends a candidate for
// every leading edge it catches. It repeatedly runs acquisition followed by
// tracking. Acquisition ends when polling resolution is acquired, or restarts
// from cold after losing the partly acquired signal. Tracking first reduces
// the window geometrically, then adjusts it additively until signal loss
// restarts the cycle from acquisition.
//
// Candidates caught during acquisition have Settled false; candidates from
// tracking have Settled true. Consumers decide whether an edge is usable from
// its Uncertainty and Settled state. Every caught edge is logged to lg at
// debug level. Tracking starts, window changes, misses, and loss are logged at
// info level with actual state-read counts. If stats is non-nil, Poll records
// timing and outcome statistics in it.
func Poll(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, ceCh chan<- CandidateEdge, stats *PollStats) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	stats.begin()
	p := poller{ctx: ctx, lg: lg, r: r, w: w, ceCh: ceCh, stats: stats}
	if err := p.init(); err != nil {
		return err
	}
	for {
		window, acquired, err := p.acquire()
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		if err := p.track(window); err != nil {
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
// load-dependent is measured by the polling loop itself.
const (
	// initialPolls is the number of polls across the cold-start window. It
	// determines the initial spacing, and with it the narrowest pulse acquired
	// promptly. The spacing scales with the window, so once it falls below the
	// state-query time or minSpacing, those pace the loop instead.
	initialPolls = 64
	// missLimit consecutive misses declare the pulse gone, restarting
	// acquisition from the full-period window.
	missLimit = 10
	// minSpacing bounds the CPU spent when the state query is very fast.
	minSpacing = 50 * time.Microsecond
)

// acquire searches for the pulse while reducing an independent poll spacing.
// It starts with initialPolls intervals across the full-period window. Every
// catch halves the spacing down to minSpacing and sets the next window to
// initialPolls times that spacing; a miss leaves the spacing unchanged. The
// bracket measures candidate uncertainty but does not constrain this descent.
//
// A catch at minSpacing acquires immediately. Two consecutive caught windows
// with no scheduled sleep also acquire, confirming at successively smaller
// spacings that the state queries pace the loop. A slept catch or miss resets
// that confirmation. Misses at the full-period window sweep the poll-grid
// phase; missLimit misses after the window narrows abandon this attempt. The
// returned duration is the window with which tracking should begin.
func (p *poller) acquire() (time.Duration, bool, error) {
	spacing := maxWindow / initialPolls
	misses, queryPaced := 0, 0
	for {
		window := initialPolls * spacing
		caught, err := p.pollWindow(window, spacing, false)
		if err != nil {
			return 0, false, err
		}
		if caught {
			misses = 0
			acquired := spacing == minSpacing
			if p.slept {
				queryPaced = 0
			} else {
				queryPaced++
				acquired = acquired || queryPaced >= 2
			}
			if acquired {
				p.lg.Debug("serial PPS settled", "window", window, "bracket", p.lastBracket)
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
		if window == maxWindow {
			// At full size, consecutive windows tile the period exactly, so a
			// locked poll grid would revisit the same phases every period and
			// could straddle a pulse narrower than the spacing indefinitely;
			// advancing the grid by an irregular fraction of the spacing
			// sweeps the phase instead.
			p.nextEdge = p.nextEdge.Add(spacing * 618 / 1000)
			continue
		}
		misses++
		if misses >= missLimit {
			p.lg.Debug("serial PPS pulse lost, restarting acquisition", "window", maxWindow, "misses", misses)
			return 0, false, nil
		}
	}
}

type trackObservation struct {
	caught     bool
	bracket    time.Duration
	stateReads int
}

type trackAttempt func(time.Duration) (trackObservation, error)

type trackEventKind uint8

const (
	trackStarted trackEventKind = iota
	trackInitialChanged
	trackReduced
	trackMissed
	trackLost
)

type trackEvent struct {
	kind               trackEventKind
	window, nextWindow time.Duration
	observation        trackObservation
	catches, misses    int
}

// track adapts the real polling window and logging to the shared tracking
// control. Tests call the same track function with a simulated attempt.
func (p *poller) track(window time.Duration) error {
	attempt := func(window time.Duration) (trackObservation, error) {
		spacing := max(window/initialPolls, minSpacing)
		caught, err := p.pollWindow(window, spacing, true)
		return trackObservation{caught: caught, bracket: p.lastBracket, stateReads: p.stateReads}, err
	}
	return track(window, attempt, p.logTrackEvent)
}

func (p *poller) logTrackEvent(e trackEvent) {
	switch e.kind {
	case trackStarted:
		p.lg.Info("serial PPS track status", "reason", "start", "window", e.window)
	case trackInitialChanged:
		p.lg.Info("serial PPS track status", "reason", "initial",
			"window", e.window, "nextWindow", e.nextWindow,
			"stateReads", e.observation.stateReads, "bracket", e.observation.bracket)
	case trackReduced:
		p.lg.Info("serial PPS track status", "reason", "shrink",
			"window", e.window, "nextWindow", e.nextWindow,
			"stateReads", e.observation.stateReads, "bracket", e.observation.bracket,
			"catches", e.catches)
	case trackMissed:
		p.lg.Info("serial PPS track status", "reason", "miss",
			"window", e.window, "nextWindow", e.nextWindow,
			"stateReads", e.observation.stateReads, "bracket", e.observation.bracket,
			"misses", e.misses)
	case trackLost:
		p.lg.Info("serial PPS track status", "reason", "lost",
			"window", e.window, "stateReads", e.observation.stateReads,
			"bracket", e.observation.bracket, "misses", e.misses)
	}
}

// track first reduces the acquired window geometrically and then maintains it
// additively. Keeping the polling operation behind attempt lets deterministic
// tests execute this exact control flow.
func track(window time.Duration, attempt trackAttempt, report func(trackEvent)) error {
	report(trackEvent{kind: trackStarted, window: window})
	window, initial, err := trackInitial(window, attempt, report)
	if err != nil {
		return err
	}
	return trackContinue(window, initial, attempt, report)
}

// trackInitial geometrically reduces an acquired window. Each catch replaces
// the window with the greater of half its size and two bracket widths. It ends
// when that calculation leaves the window unchanged or at the first miss.
func trackInitial(window time.Duration, attempt trackAttempt, report func(trackEvent)) (time.Duration, trackObservation, error) {
	for {
		obs, err := attempt(window)
		if err != nil {
			return 0, trackObservation{}, err
		}
		if !obs.caught {
			return window, obs, nil
		}
		nextWindow := max(window/2, 2*obs.bracket)
		if nextWindow == window {
			return window, obs, nil
		}
		report(trackEvent{kind: trackInitialChanged, window: window,
			nextWindow: nextWindow, observation: obs})
		window = nextWindow
	}
}

// shrinkAfter is the target miss rate: the continuing tracking window widens
// by a bracket width at each end on a miss and narrows by the same after this
// many consecutive catches.
const shrinkAfter = 60

// trackContinue maintains the steady-state window additively. A miss grows
// the window by the last bracket width at each end. Every shrinkAfter-th
// consecutive catch removes the current bracket width from each end while the
// window is at least four bracket widths. initial is the observation that
// ended trackInitial, and may represent a miss already consumed there.
func trackContinue(window time.Duration, initial trackObservation, attempt trackAttempt, report func(trackEvent)) error {
	catches, misses := 0, 0
	if !initial.caught {
		misses = 1
		var lost bool
		window, lost = growTrackWindow(window, initial, misses, report)
		if lost {
			return nil
		}
	}
	for {
		obs, err := attempt(window)
		if err != nil {
			return err
		}
		if obs.caught {
			misses = 0
			catches++
			if catches >= shrinkAfter && window >= 4*obs.bracket {
				nextWindow := window - 2*obs.bracket
				report(trackEvent{kind: trackReduced, window: window,
					nextWindow: nextWindow, observation: obs, catches: catches})
				window = nextWindow
				catches = 0
			}
			continue
		}
		misses++
		nextWindow, lost := growTrackWindow(window, obs, misses, report)
		if lost {
			return nil
		}
		window = nextWindow
		catches = 0
	}
}

func growTrackWindow(window time.Duration, obs trackObservation, misses int, report func(trackEvent)) (time.Duration, bool) {
	if misses >= missLimit || window+2*obs.bracket >= maxWindow {
		report(trackEvent{kind: trackLost, window: window, observation: obs, misses: misses})
		return window, true
	}
	nextWindow := window + 2*obs.bracket
	report(trackEvent{kind: trackMissed, window: window, nextWindow: nextWindow,
		observation: obs, misses: misses})
	return nextWindow, false
}

// clockReading keeps adjacent readings of the clocks used by serial PPS
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
	state gpsio.ModemControlPinState
	poll  poll
	start time.Time
	sched time.Time // when this poll was scheduled to run
	slept bool      // whether the schedule was still in the future
}

func (p *poller) init() error {
	first, err := readState(p.ctx, p.r, time.Time{})
	if err != nil {
		return err
	}
	p.stats.addPoll(first.poll, nil)
	p.nextEdge = first.poll.midpoint().mono.Add(maxWindow / 2)
	return nil
}

// pollWindow waits for one window to open, polls through a pulse already in
// progress, hunts for the next leading edge, classifies the outcome, records
// statistics, and sends a caught candidate. It returns whether it caught an
// edge. The wait for the window open is excluded from slept.
func (p *poller) pollWindow(window, spacing time.Duration, acquired bool) (bool, error) {
	nextEdge := p.nextEdge
	deadline := nextEdge.Add(window / 2)
	cur, err := readState(p.ctx, p.r, nextEdge.Add(-window/2))
	if err != nil {
		return false, err
	}
	p.stats.addPoll(cur.poll, nil)
	p.slept = false
	p.stateReads = 1
	// The windows advance in lockstep with the pulses, so treating an
	// in-progress pulse at the open as a miss would reopen at the same phase
	// every period and never acquire; poll through it instead. slept accumulates
	// over the window's scheduled polls (the wait for the window open is
	// excluded: it always sleeps).
	for inPulse(cur.state, p.w) && cur.poll.midpoint().mono.Before(deadline) {
		prev := cur
		cur, err = readState(p.ctx, p.r, cur.start.Add(spacing))
		if err != nil {
			return false, err
		}
		p.stats.addPoll(cur.poll, &prev.poll)
		p.stateReads++
		p.slept = p.slept || cur.slept
	}
	prev := cur
	missed := inPulse(cur.state, p.w)
	var edge clockReading
	for !missed && edge.stamp.IsZero() {
		cur, err = readState(p.ctx, p.r, prev.start.Add(spacing))
		if err != nil {
			return false, err
		}
		p.stats.addPoll(cur.poll, &prev.poll)
		p.stateReads++
		p.slept = p.slept || cur.slept
		edge, missed = classify(prev, cur, p.w, deadline)
		if !edge.stamp.IsZero() {
			p.lastBracket = cur.poll.midpoint().elapsedSince(prev.poll.midpoint())
		}
		prev = cur
	}
	if edge.stamp.IsZero() {
		p.stats.addWindow(false, false)
		p.nextEdge = nextEdge.Add(period)
		return false, nil
	}
	// "late" is how far past its scheduled time the catching poll started:
	// sleep overshoot when the loop is sleep-paced, queue debt when the queries
	// pace it.
	p.lg.Debug("serial PPS caught edge", "window", window, "bracket", p.lastBracket,
		"offset", edge.mono.Sub(nextEdge), "late", cur.start.Sub(cur.sched), "stateReads", p.stateReads)
	p.stats.addWindow(true, acquired)
	p.nextEdge = edge.mono.Add(period)
	ce := CandidateEdge{
		Edge: Edge{
			Timestamp: edge.stamp,
			TRead:     cur.poll.end.mono,
		},
		Uncertainty: halfCeil(p.lastBracket),
		Settled:     acquired,
	}
	select {
	case p.ceCh <- ce:
		return true, nil
	case <-p.ctx.Done():
		return false, p.ctx.Err()
	}
}

func readState(ctx context.Context, r StateReader, sched time.Time) (reading, error) {
	slept, err := waitUntil(ctx, sched)
	if err != nil {
		return reading{}, err
	}
	start := now()
	state, err := r.ModemControlPinState()
	end := now()
	if err != nil {
		return reading{}, err
	}
	return reading{state: state, poll: poll{start: start, end: end}, start: start.mono,
		sched: sched, slept: slept}, nil
}

// waitUntil reports whether it actually had to wait: false means the
// scheduled time was already past, i.e. the previous state query outlasted
// the poll spacing.
func waitUntil(ctx context.Context, t time.Time) (bool, error) {
	d := time.Until(t)
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
// leading edges, so its midpoint identifies none of them: it is a miss.
func classify(prev, cur reading, w Wiring, deadline time.Time) (clockReading, bool) {
	if !inPulse(prev.state, w) && inPulse(cur.state, w) {
		if cur.poll.midpoint().elapsedSince(prev.poll.midpoint()) >= period {
			return clockReading{}, true
		}
		return prev.poll.midpoint().midpoint(cur.poll.midpoint()), false
	}
	return clockReading{}, !cur.poll.midpoint().mono.Before(deadline)
}

// inPulse reports whether the state was observed during a pulse. The pulse's
// electrically rising leading edge reaches the host inverted through the TTL
// driver chain, so the flag reads deasserted during the pulse.
func inPulse(s gpsio.ModemControlPinState, w Wiring) bool {
	return !s.Asserted(w.Pin)
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
