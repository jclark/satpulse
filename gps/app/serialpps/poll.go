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
	polls       int
}

// Poll adaptively polls for the pulse described by w and sends a candidate for
// every leading edge it catches. It repeatedly runs three phases. Acquisition
// ends when polling resolution is acquired, or restarts from cold after losing
// the partly acquired signal. Narrowing ends when geometric window reduction
// reaches its floor or observes its first miss. Tracking then adjusts the
// window additively until signal loss restarts the cycle from acquisition.
//
// Candidates caught during acquisition have Settled false; candidates from
// narrowing and tracking have Settled true. Consumers that require usable
// edges must filter on Settled and take the embedded Edge. Every caught edge
// and every window size change is logged to lg at debug level. If stats is
// non-nil, Poll records timing and outcome statistics in it.
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
		window, firstMiss, err := p.narrow(window)
		if err != nil {
			return err
		}
		if err := p.track(window, firstMiss); err != nil {
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
// returned duration is the window with which narrowing should begin.
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

// narrow geometrically reduces an acquired window. Each catch replaces the
// window with the greater of half its size and two bracket widths. Narrowing
// ends when that calculation no longer changes the window or at the first
// miss. The returned bool reports whether narrow consumed that first miss, so
// track can apply it without polling the same period again.
func (p *poller) narrow(window time.Duration) (time.Duration, bool, error) {
	for {
		spacing := max(window/initialPolls, minSpacing)
		caught, err := p.pollWindow(window, spacing, true)
		if err != nil {
			return 0, false, err
		}
		if !caught {
			return window, true, nil
		}
		narrowed := max(window/2, 2*p.lastBracket)
		if narrowed == window {
			return window, false, nil
		}
		window = narrowed
		p.lg.Debug("serial PPS poll window halved", "window", window)
	}
}

// shrinkAfter is the target miss rate: the tracking window widens by a
// bracket width at each end on a miss and narrows by the same after this many
// consecutive catches, so it hovers just above the observed edge scatter,
// missing about one pulse in shrinkAfter.
const shrinkAfter = 60

// track maintains the steady-state window additively. A miss grows the window
// by the last bracket width at each end. Every shrinkAfter-th consecutive
// catch removes the current bracket width from each end while the window is at
// least four bracket widths. firstMiss reports a miss already consumed by
// narrow. missLimit consecutive misses, or growth reaching maxWindow, return
// to Poll so acquisition can restart from cold.
func (p *poller) track(window time.Duration, firstMiss bool) error {
	catches, misses := 0, 0
	if firstMiss {
		// narrow already polled this missed window; apply it before polling
		// the next one.
		misses = 1
		var lost bool
		window, lost = p.growWindow(window, misses)
		if lost {
			return nil
		}
	}
	for {
		spacing := max(window/initialPolls, minSpacing)
		caught, err := p.pollWindow(window, spacing, true)
		if err != nil {
			return err
		}
		if caught {
			misses = 0
			catches++
			if catches >= shrinkAfter && window >= 4*p.lastBracket {
				catches = 0
				window -= 2 * p.lastBracket
				p.lg.Debug("serial PPS poll window shrank", "window", window)
			}
			continue
		}
		misses++
		var lost bool
		window, lost = p.growWindow(window, misses)
		if lost {
			return nil
		}
		catches = 0
	}
}

// growWindow applies one missed tracking window and reports signal loss.
func (p *poller) growWindow(window time.Duration, misses int) (time.Duration, bool) {
	if misses >= missLimit || window+2*p.lastBracket >= maxWindow {
		p.lg.Debug("serial PPS pulse lost, restarting acquisition", "window", maxWindow, "misses", misses)
		return window, true
	}
	window += 2 * p.lastBracket
	p.lg.Debug("serial PPS poll window grew", "window", window, "misses", misses, "polls", p.polls)
	return window, false
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
	p.polls = 1
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
		p.polls++
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
		p.polls++
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
		"offset", edge.mono.Sub(nextEdge), "late", cur.start.Sub(cur.sched), "polls", p.polls)
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
