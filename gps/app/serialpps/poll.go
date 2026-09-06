package serialpps

import (
	"context"
	"log/slog"
	"math"
	"runtime"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

type poller struct {
	ctx         context.Context
	lg          *slog.Logger
	r           StateReader
	w           Wiring
	prewarm     time.Duration
	ceCh        chan<- CandidateEdge
	stats       *PollStats
	nextEdge    time.Time
	lastBracket time.Duration
	slept       bool
	stateReads  int
	// outlierRatio is the multiple of the settled-bracket lower quartile
	// beyond which a tracking catch is an outlier (zero disables the check);
	// settledBrackets is a ring of the most recent settled tracking brackets,
	// the reference, and nextBracket the slot the next one overwrites once
	// the ring is full.
	outlierRatio    float64
	settledBrackets []time.Duration
	nextBracket     int
}

// Poll adaptively polls for the pulse described by w and sends a candidate for
// every leading edge it catches. It repeatedly runs acquisition followed by
// tracking. Acquisition ends when polling resolution is acquired, or restarts
// from cold after losing the partly acquired signal. Tracking shrinks the
// polling window onto the measured need and holds it just above the size
// where misses occur; consecutive misses double the window, and sustained
// loss restarts the cycle from acquisition.
//
// A nonzero prewarm ends the sleep to each window open that much early and
// busy-waits the remainder. It is for hosts whose state queries slow down
// severalfold while the machine idles, where only continuous work ending at
// the open restores full query speed; it costs that fraction of a core.
//
// A positive outlierRatio marks a tracking catch whose bracket exceeds that
// multiple of the lower quartile of the recent settled brackets as an
// outlier. A state read stalled by host load widens its bracket severalfold
// while the reads around it stay normal, so the mark identifies edges whose
// midpoint no longer locates the pulse; tracking still uses them, and
// consumers decide whether to.
//
// Candidates caught during acquisition are unsettled, including the catch
// that completes acquisition, whose transition the "serial PPS acquired" log
// line marks; tracking candidates are settled once the polling schedule stops
// limiting the measurement or the window has stopped shrinking. Consumers
// decide whether an edge is usable from its Uncertainty and Settled state.
// Every caught edge is logged to lg at
// debug level. Tracking starts, significant window changes, misses, and loss
// are logged at info level with actual state-read counts. If stats is non-nil,
// Poll records timing and outcome statistics in it.
func Poll(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, prewarm time.Duration, outlierRatio float64, ceCh chan<- CandidateEdge, stats *PollStats) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	stats.begin()
	p := poller{ctx: ctx, lg: lg, r: r, w: w, prewarm: prewarm, outlierRatio: outlierRatio, ceCh: ceCh, stats: stats}
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
		caught, _, err := p.pollWindow(window, spacing, false, false)
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
	// lastBracket is the most recent caught bracket: this catch's own, or on
	// a miss the previous catch's, since a missed window measures none.
	lastBracket time.Duration
	stateReads  int
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
	attempt := func(window time.Duration, atFloor bool) (trackObservation, error) {
		spacing := max(window/initialPolls, minSpacing)
		caught, predictionError, err := p.pollWindow(window, spacing, true, atFloor)
		return trackObservation{caught: caught, predictionError: predictionError,
			lastBracket: p.lastBracket, stateReads: p.stateReads}, err
	}
	advance := func(d time.Duration) { p.nextEdge = p.nextEdge.Add(d) }
	return track(window, attempt, advance, p.logTrackEvent)
}

// trackRelease sets the shrink rate: each catch shrinks the window by
// 1/trackRelease until a measured limit stops it. It controls dynamics only;
// every time scale comes from observed prediction errors and brackets.
const trackRelease = 16

// shrinkAfter is how many consecutive catches must follow a short run of
// misses before the window may shrink again, so a size that missed is
// re-tested only after five minutes or so of reliable catching.
const shrinkAfter = 300

// A miss can mean the window was too small or that no pulse arrived, and one
// observation cannot tell the causes apart. A short run looks like the first:
// on hardware with late wakeups the boundary is hit singly or in pairs, and
// one doubling puts the window far above it. A run of absentRun or more can
// only be an absent pulse, which says nothing about window size, so holding
// the window after it would only delay recovery.
const absentRun = 3

// track maintains the polling window with one feedback loop. A catch shrinks
// the window by 1/trackRelease, but never below twice the sum of its
// prediction error and bracket (the error the prediction just showed, half a
// bracket of edge quantization, and half a bracket keeping an equal offset
// inside the window edge). Half of each prediction error corrects the next
// prediction, so one noisy edge cannot displace the window by its full error.
// A first miss grows the window by a bracket width at each end; each further
// consecutive miss doubles the window, since phase uncertainty compounds
// while no edges are observed. After a run of misses shorter than absentRun,
// the window holds -- expanding on demand but not shrinking -- until
// shrinkAfter consecutive catches confirm it, since the misses may mean that
// size was too small; after a longer run the pulse was simply absent and
// shrinking resumes at once. Growth is capped at the full period; missLimit
// consecutive misses at the full period declare the pulse gone. Each attempt
// is told whether the window has stopped shrinking at its measured floor
// (the limit, not the 1/trackRelease term, bounded the last shrink), which
// polling uses to mark candidates settled; holding is not that. Misses and
// recovery are reported as they happen; other window changes only once per
// doubling or halving, to keep the log quiet.
func track(window time.Duration, attempt func(time.Duration, bool) (trackObservation, error),
	advance func(time.Duration), report func(trackEvent)) error {
	report(trackEvent{kind: trackStarted, window: window})
	logged := window
	catches, misses, fullMisses := shrinkAfter, 0, 0
	atFloor := false
	for {
		obs, err := attempt(window, atFloor)
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
			nextWindow := window + 2*obs.lastBracket
			if misses >= 2 {
				nextWindow = 2 * window
			}
			nextWindow = min(nextWindow, maxWindow)
			report(trackEvent{kind: trackMissed, window: window, nextWindow: nextWindow,
				observation: obs, misses: misses})
			logged = nextWindow
			window = nextWindow
			catches = 0
			atFloor = false
			continue
		}
		advance(period + obs.predictionError/2)
		if misses >= 2 {
			report(trackEvent{kind: trackRecovered, window: window, observation: obs, misses: misses})
		}
		if misses >= absentRun {
			catches = shrinkAfter
		}
		misses, fullMisses = 0, 0
		if catches < shrinkAfter {
			catches++
		}
		floor := 2 * (obs.predictionError.Abs() + obs.lastBracket)
		var nextWindow time.Duration
		if catches < shrinkAfter {
			nextWindow = min(max(window, floor), maxWindow)
			atFloor = false
		} else {
			nextWindow = min(max(window-window/trackRelease, floor), maxWindow)
			// atFloor deliberately includes upward corrections: the measured
			// requirement controlled the next window either way, and calling
			// expansions unsettled would flicker the settled state on hardware
			// whose brackets are irreducibly coarse, costing it samples. The
			// price is that a transient expansion marks the next candidate
			// settled, which affects dispatch only if that candidate is itself
			// coarser than the dispatcher's uncertainty limit.
			atFloor = floor >= window-window/trackRelease
		}
		if nextWindow >= 2*logged || 2*nextWindow <= logged {
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
			"stateReads", e.observation.stateReads, "bracket", e.observation.lastBracket,
			"predictionError", e.observation.predictionError)
	case trackMissed:
		p.lg.Info("serial PPS track status", "reason", "miss",
			"window", e.window, "nextWindow", e.nextWindow,
			"stateReads", e.observation.stateReads, "bracket", e.observation.lastBracket,
			"misses", e.misses)
	case trackRecovered:
		p.lg.Info("serial PPS track status", "reason", "recovered",
			"window", e.window, "stateReads", e.observation.stateReads,
			"bracket", e.observation.lastBracket,
			"predictionError", e.observation.predictionError, "misses", e.misses)
	case trackLost:
		p.lg.Info("serial PPS track status", "reason", "lost",
			"window", e.window, "stateReads", e.observation.stateReads,
			"bracket", e.observation.lastBracket, "misses", e.misses)
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
	state gpsio.SerialPinState
	poll  poll
	start time.Time
	sched time.Time // when this poll was scheduled to run
	slept bool      // whether waiting for the schedule used a timer
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
// excluded from slept. A candidate is settled when tracking (acquired) says
// the window has stopped shrinking (atFloor), or when this window's own
// polling could not have been finer: its spacing was at the floor, or the
// state queries paced it without a scheduled sleep.
func (p *poller) pollWindow(window, spacing time.Duration, acquired, atFloor bool) (bool, time.Duration, error) {
	nextEdge := p.nextEdge
	deadline := nextEdge.Add(window / 2)
	open := nextEdge.Add(-window / 2)
	if p.prewarm > 0 {
		if _, err := waitUntil(p.ctx, open.Add(-p.prewarm)); err != nil {
			return false, 0, err
		}
		for time.Now().Before(open) {
			if p.ctx.Err() != nil {
				return false, 0, p.ctx.Err()
			}
		}
	}
	cur, err := readState(p.ctx, p.r, open)
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
			p.lastBracket = cur.poll.midpoint().elapsedSince(prev.poll.midpoint())
		}
		prev = cur
	}
	if edge.stamp.IsZero() {
		p.stats.addWindow(false, acquired, false)
		if !acquired {
			p.nextEdge = nextEdge.Add(period)
		}
		return false, 0, nil
	}
	predictionError := edge.mono.Sub(nextEdge)
	settled := acquired && (atFloor || spacing == minSpacing || !p.slept)
	// The reference excludes this catch, so a stalled bracket is judged
	// against the brackets before it; it then joins the ring regardless, so
	// the reference follows hardware that has genuinely become slower.
	limit := p.outlierLimit()
	outlier := acquired && limit > 0 && p.lastBracket > limit
	// "late" is how far past its scheduled time the catching poll started:
	// sleep overshoot when the loop is sleep-paced, queue debt when the queries
	// pace it.
	p.lg.Debug("serial PPS caught edge", "window", window, "bracket", p.lastBracket,
		"predictionError", predictionError, "late", cur.start.Sub(cur.sched), "stateReads", p.stateReads,
		"outlierLimit", limit, "outlier", outlier)
	p.stats.addWindow(true, acquired, outlier)
	if settled {
		p.recordBracket(p.lastBracket)
	}
	if !acquired {
		p.nextEdge = edge.mono.Add(period)
	}
	ce := CandidateEdge{
		Edge: Edge{
			Timestamp: edge.stamp,
			TRead:     cur.poll.end.mono,
		},
		Uncertainty: halfCeil(p.lastBracket),
		Settled:     settled,
		Outlier:     outlier,
	}
	select {
	case p.ceCh <- ce:
		return true, predictionError, nil
	case <-p.ctx.Done():
		return false, 0, p.ctx.Err()
	}
}

// outlierHistory is how many settled brackets the outlier reference is drawn
// from: about two minutes of catches, so a hardware change moves the
// reference within that time. outlierMinHistory is how many it needs before
// any catch is marked.
const (
	outlierHistory    = 128
	outlierMinHistory = 16
)

// outlierLimit is the bracket beyond which a tracking catch is an outlier,
// or zero while the check is disabled or the history is too short. The
// reference is the lower quartile rather than the median because stalls only
// ever widen a bracket: the quartile stays inside the normal population until
// three quarters of the recent catches are stalled, where the median gives
// way at half, which is the load level the check exists for.
func (p *poller) outlierLimit() time.Duration {
	if p.outlierRatio == 0 || len(p.settledBrackets) < outlierMinHistory {
		return 0
	}
	sorted := slices.Clone(p.settledBrackets)
	slices.Sort(sorted)
	// A ratio large enough to overflow means never: saturate rather than
	// let the conversion wrap.
	if limit := float64(sorted[len(sorted)/4]) * p.outlierRatio; limit < math.MaxInt64 {
		return time.Duration(limit)
	}
	return math.MaxInt64
}

func (p *poller) recordBracket(d time.Duration) {
	if len(p.settledBrackets) < outlierHistory {
		p.settledBrackets = append(p.settledBrackets, d)
		return
	}
	p.settledBrackets[p.nextBracket] = d
	p.nextBracket = (p.nextBracket + 1) % outlierHistory
}

func readState(ctx context.Context, r StateReader, sched time.Time) (reading, error) {
	slept, err := waitUntil(ctx, sched)
	if err != nil {
		return reading{}, err
	}
	start := now()
	state, err := r.SerialPinState()
	end := now()
	if err != nil {
		return reading{}, err
	}
	return reading{state: state, poll: poll{start: start, end: end}, start: start.mono,
		sched: sched, slept: slept}, nil
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
func classify(prev, cur reading, w Wiring, deadline time.Time) (clockReading, bool) {
	if !inPulse(prev.state, w) && inPulse(cur.state, w) {
		if d := cur.poll.midpoint().elapsedSince(prev.poll.midpoint()); d >= period || d <= 0 {
			return clockReading{}, true
		}
		return prev.poll.midpoint().midpoint(cur.poll.midpoint()), false
	}
	return clockReading{}, !cur.poll.midpoint().mono.Before(deadline)
}

// inPulse reports whether the state was observed during a pulse, as
// selected by the wiring's polarity.
func inPulse(s gpsio.SerialPinState, w Wiring) bool {
	return s.Asserted(w.Pin) == w.Polarity.Asserted()
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
