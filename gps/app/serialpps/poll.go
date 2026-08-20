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
// from cold after losing the partly acquired signal. Tracking shrinks the
// polling window onto the measured need and holds it just above the size
// where misses occur; consecutive misses double the window, and sustained
// loss restarts the cycle from acquisition.
//
// Candidates caught during acquisition have Acquired false, including the
// catch that completes acquisition, whose transition the "serial PPS acquired"
// log line marks; candidates from tracking have Acquired true. Consumers
// decide whether an edge is usable from its Uncertainty and Acquired state. Every caught edge is logged to lg at
// debug level. Tracking starts, significant window changes, misses, and loss
// are logged at info level with actual state-read counts. If stats is non-nil,
// Poll records timing and outcome statistics in it.
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
	// missLimit consecutive misses declare the pulse gone: acquisition
	// abandons its attempt, and tracking, whose window doubles toward the
	// full period as misses accumulate, returns to acquisition.
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
		caught, _, err := p.pollWindow(window, spacing, false)
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
	caught          bool
	predictionError time.Duration
	bracket         time.Duration
	stateReads      int
}

type trackEventKind uint8

const (
	trackStarted trackEventKind = iota
	trackChanged
	trackMissed
	trackRecovered
	trackLost
)

type trackEvent struct {
	kind               trackEventKind
	window, nextWindow time.Duration
	observation        trackObservation
	misses             int
}

// track adapts the real polling window and logging to the shared tracking
// control. Tests call the same track function with a simulated attempt.
func (p *poller) track(window time.Duration) error {
	attempt := func(window time.Duration) (trackObservation, error) {
		spacing := max(window/initialPolls, minSpacing)
		caught, predictionError, err := p.pollWindow(window, spacing, true)
		return trackObservation{caught: caught, predictionError: predictionError,
			bracket: p.lastBracket, stateReads: p.stateReads}, err
	}
	advance := func(d time.Duration) { p.nextEdge = p.nextEdge.Add(d) }
	return track(window, attempt, advance, p.logTrackEvent)
}

// trackRelease sets the shrink rate: each catch shrinks the window by
// 1/trackRelease until a measured limit stops it. It controls dynamics only;
// every time scale comes from observed prediction errors and brackets.
const trackRelease = 16

// shrinkAfter paces the re-testing of a window size that missed: after
// shrinkAfter consecutive catches without a miss, minWindow steps down by a
// bracket width at each end, so tracking tries a smaller window every five
// minutes or so while the pulse is being caught reliably.
const shrinkAfter = 300

// track maintains the polling window with one feedback loop. A catch shrinks
// the window by 1/trackRelease, but never below twice the sum of its
// prediction error and bracket (the error the prediction just showed, half a
// bracket of edge quantization, and half a bracket keeping an equal offset
// inside the window edge), and never below minWindow, the size remembered
// from the last first miss. Half of each prediction error corrects the next
// prediction, so one noisy edge cannot displace the window by its full error.
// A first miss grows the window by a bracket width at each end and sets
// minWindow; each further consecutive miss doubles the window, since phase
// uncertainty compounds while no edges are observed. Growth is capped at the
// full period; missLimit consecutive misses at the full period declare the
// pulse gone. Misses and recovery are reported as they happen; other window
// changes only once per doubling or halving, and minWindow steps when they
// move the window, to keep the log quiet.
func track(window time.Duration, attempt func(time.Duration) (trackObservation, error),
	advance func(time.Duration), report func(trackEvent)) error {
	report(trackEvent{kind: trackStarted, window: window})
	logged := window
	minWindow := time.Duration(0)
	catches, misses, fullMisses := 0, 0, 0
	for {
		obs, err := attempt(window)
		if err != nil {
			return err
		}
		if !obs.caught {
			advance(period)
			misses++
			if window >= maxWindow {
				fullMisses++
				if fullMisses >= missLimit {
					report(trackEvent{kind: trackLost, window: window, observation: obs, misses: misses})
					return nil
				}
			}
			nextWindow := window + 2*obs.bracket
			if misses >= 2 {
				nextWindow = 2 * window
			}
			nextWindow = min(nextWindow, maxWindow)
			if misses == 1 {
				minWindow = nextWindow
			}
			report(trackEvent{kind: trackMissed, window: window, nextWindow: nextWindow,
				observation: obs, misses: misses})
			logged = nextWindow
			window = nextWindow
			catches = 0
			continue
		}
		advance(period + obs.predictionError/2)
		if misses >= 2 {
			report(trackEvent{kind: trackRecovered, window: window, observation: obs, misses: misses})
		}
		misses, fullMisses = 0, 0
		catches++
		stepped := false
		if catches >= shrinkAfter {
			catches = 0
			if minWindow > 0 {
				minWindow = max(minWindow-2*obs.bracket, 0)
				stepped = true
			}
		}
		nextWindow := min(max(window-window/trackRelease,
			2*(obs.predictionError.Abs()+obs.bracket), minWindow), maxWindow)
		if nextWindow >= 2*logged || 2*nextWindow <= logged || (stepped && nextWindow < window) {
			report(trackEvent{kind: trackChanged, window: window,
				nextWindow: nextWindow, observation: obs})
			logged = nextWindow
		}
		window = nextWindow
	}
}

func (p *poller) logTrackEvent(e trackEvent) {
	switch e.kind {
	case trackStarted:
		p.lg.Info("serial PPS track status", "reason", "start", "window", e.window)
	case trackChanged:
		reason := "shrink"
		if e.nextWindow > e.window {
			reason = "expand"
		}
		p.lg.Info("serial PPS track status", "reason", reason,
			"window", e.window, "nextWindow", e.nextWindow,
			"stateReads", e.observation.stateReads, "bracket", e.observation.bracket,
			"predictionError", e.observation.predictionError)
	case trackMissed:
		p.lg.Info("serial PPS track status", "reason", "miss",
			"window", e.window, "nextWindow", e.nextWindow,
			"stateReads", e.observation.stateReads, "bracket", e.observation.bracket,
			"misses", e.misses)
	case trackRecovered:
		p.lg.Info("serial PPS track status", "reason", "recovered",
			"window", e.window, "stateReads", e.observation.stateReads,
			"bracket", e.observation.bracket,
			"predictionError", e.observation.predictionError, "misses", e.misses)
	case trackLost:
		p.lg.Info("serial PPS track status", "reason", "lost",
			"window", e.window, "stateReads", e.observation.stateReads,
			"bracket", e.observation.bracket, "misses", e.misses)
	}
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
// edge and its error from the predicted edge. The wait for the window open is
// excluded from slept.
func (p *poller) pollWindow(window, spacing time.Duration, acquired bool) (bool, time.Duration, error) {
	nextEdge := p.nextEdge
	deadline := nextEdge.Add(window / 2)
	cur, err := readState(p.ctx, p.r, nextEdge.Add(-window/2))
	if err != nil {
		return false, 0, err
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
			return false, 0, err
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
			return false, 0, err
		}
		p.stats.addPoll(cur.poll, &prev.poll)
		p.stateReads++
		p.slept = p.slept || cur.slept
		edge, missed = classify(prev, cur, p.w, deadline)
		if !edge.stamp.IsZero() {
			// On Windows the stamps carry no monotonic reading, so a backward
			// clock step inside the bracket could make it negative; clamp so
			// bad timing cannot corrupt the window arithmetic.
			p.lastBracket = max(cur.poll.midpoint().elapsedSince(prev.poll.midpoint()), 0)
		}
		prev = cur
	}
	if edge.stamp.IsZero() {
		p.stats.addWindow(false, acquired)
		if !acquired {
			p.nextEdge = nextEdge.Add(period)
		}
		return false, 0, nil
	}
	// "late" is how far past its scheduled time the catching poll started:
	// sleep overshoot when the loop is sleep-paced, queue debt when the queries
	// pace it.
	predictionError := edge.mono.Sub(nextEdge)
	p.lg.Debug("serial PPS caught edge", "window", window, "bracket", p.lastBracket,
		"predictionError", predictionError, "late", cur.start.Sub(cur.sched), "stateReads", p.stateReads)
	p.stats.addWindow(true, acquired)
	if !acquired {
		p.nextEdge = edge.mono.Add(period)
	}
	ce := CandidateEdge{
		Edge: Edge{
			Timestamp: edge.stamp,
			TRead:     cur.poll.end.mono,
		},
		Uncertainty: halfCeil(p.lastBracket),
		Acquired:    acquired,
	}
	select {
	case p.ceCh <- ce:
		return true, predictionError, nil
	case <-p.ctx.Done():
		return false, 0, p.ctx.Err()
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
