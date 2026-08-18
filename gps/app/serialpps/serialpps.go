// Package serialpps detects PPS edges on serial modem-control pins and turns
// them into refclock samples using UTC time messages from the receiver.
package serialpps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

// None of these constants encodes hardware timing; everything hardware- and
// load-dependent is measured by the polling loop itself.
const (
	// initialPolls is the number of polls across the cold-start window. It
	// determines the initial spacing, and with it the narrowest pulse acquired
	// promptly. The spacing scales with the window, so once it falls below the
	// state-query time or minSpacing, those pace the loop instead.
	initialPolls = 64
	// shrinkAfter is the target miss rate: the settled window widens by a
	// bracket width at each end on a miss and narrows by the same after
	// this many consecutive catches, so it hovers just above the observed
	// edge scatter, missing about one pulse in shrinkAfter.
	shrinkAfter = 60
	// missLimit consecutive misses declare the pulse gone, restarting
	// acquisition from the full-period window.
	missLimit = 10
	// minSpacing bounds the CPU spent when the state query is very fast.
	minSpacing = 50 * time.Microsecond
	period     = time.Second
	// maxWindow is the whole period: the cold-start window, within which
	// polling is uniform.
	maxWindow = period
	// maxMsgAge is how old the newest receiver time message may be
	// for an edge to still be identified with a UTC second.
	maxMsgAge = 3 * time.Second
)

// Edge is a detected leading edge and the time at which the backend read it.
// Timestamp is the time assigned to the edge: a kernel timestamp, a polling
// bracket midpoint, or a wait wakeup used as an edge proxy. Its wall reading
// is always meaningful, and it carries a monotonic reading when the backend
// can preserve one. TRead is an ordinary time.Now reading captured when the
// wait or closing poll completed, before subsequent validation.
type Edge struct {
	Timestamp time.Time
	TRead     time.Time
}

// CandidateEdge is an edge reported by a detection backend. Poll reports every
// edge it catches so that diagnostic consumers can follow acquisition:
// Uncertainty is half the width of the polling bracket, and Settled says that
// scheduled sleeps no longer limit the normal resolution. Timing consumers
// must ignore polling candidates until they settle. A wait or kernel candidate
// carries the backend timestamp directly, has no polling uncertainty, and is
// always settled.
type CandidateEdge struct {
	Edge
	Uncertainty time.Duration
	Settled     bool
}

// StateReader is implemented by a TTY-backed gpsio.SerialConn.
type StateReader interface {
	ModemControlPinState() (gpsio.ModemControlPinState, error)
}

// ChangeWaiter is a StateReader that may be able to block until a modem
// control input changes using the wait or kernel method. A backend that
// cannot support the method at all fails with an error wrapping
// errors.ErrUnsupported; a method that exists but is unavailable for the
// particular device or driver fails with an error wrapping
// gpsio.ErrUnavailable. Implemented by gpsio.SerialConn.
type ChangeWaiter interface {
	StateReader
	WaitModemControlPinChange(context.Context, gpsio.ModemControlPin, gpsio.PPSMethod) (gpsio.ModemControlPinChange, int, error)
}

// Wiring describes how the PPS pulse is represented on the serial port's
// modem-control inputs.
type Wiring struct {
	Pin gpsio.ModemControlPin
}

// clockReading keeps adjacent readings of the clocks used by serial PPS
// together. stamp is the measurement reading used for short intervals and
// published edge timestamps; mono paces the polling loop, which must not be
// disturbed by a step in the system clock.
type clockReading struct {
	stamp time.Time
	mono  time.Time
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

type reading struct {
	state gpsio.ModemControlPinState
	poll  poll
	start time.Time
	sched time.Time // when this poll was scheduled to run
	slept bool      // whether the schedule was still in the future
}

// poll retains both clock readings around one modem-state query.
type poll struct {
	start clockReading
	end   clockReading
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

// Detect sends candidate edges for the pulse described by w. An unspecified
// method automatically tries kernel, then wait, then poll, moving on when a
// method is unsupported or unavailable for the device. Other failures are
// returned. An explicitly requested method never falls back. cfg.PollPreWarm
// applies only to polling, the one method whose resolution the host's own
// speed sets. If stats is non-nil, it records timings only when polling is
// selected.
func Detect(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, cfg Config, ceCh chan<- CandidateEdge, stats *PollStats) error {
	prewarm := seconds(cfg.PollPreWarm)
	if cfg.Method != 0 {
		return detect(ctx, lg, r, w, cfg.Method, prewarm, ceCh, stats)
	}
	if _, ok := r.(ChangeWaiter); ok {
		for _, m := range []gpsio.PPSMethod{gpsio.PPSMethodKernel, gpsio.PPSMethodWait} {
			err := detect(ctx, lg, r, w, m, prewarm, ceCh, stats)
			if ctx.Err() != nil {
				return err
			}
			switch {
			case errors.Is(err, errors.ErrUnsupported):
				lg.Debug("serial PPS method unavailable", "method", m, "error", err)
			case errors.Is(err, gpsio.ErrUnavailable):
				lg.Warn("serial PPS method unavailable; falling back", "method", m, "error", err)
			default:
				return err
			}
		}
	}
	return detect(ctx, lg, r, w, gpsio.PPSMethodPoll, prewarm, ceCh, stats)
}

func detect(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, method gpsio.PPSMethod, prewarm time.Duration, ceCh chan<- CandidateEdge, stats *PollStats) error {
	switch method {
	case gpsio.PPSMethodPoll, gpsio.PPSMethodWait, gpsio.PPSMethodKernel:
	default:
		panic("serialpps: invalid PPS method")
	}
	lg.Info("serial PPS method selected", "method", method)
	if method == gpsio.PPSMethodPoll {
		return Poll(ctx, lg, r, w, prewarm, ceCh, stats)
	}
	cw, ok := r.(ChangeWaiter)
	if !ok {
		return fmt.Errorf("%v PPS method: %w", method, errors.ErrUnsupported)
	}
	return Wait(ctx, lg, cw, w, method, ceCh)
}

// Poll adaptively polls for the pulse described by w and sends a candidate for
// every leading edge it catches, including those caught before acquisition has
// settled. Each period it polls a window centered on the predicted next edge.
// A candidate is located by its bracket: the pair of consecutive polls with
// one poll on each side of the transition. The bracket width determines the
// candidate's uncertainty and is the unit in which the window is sized.
//
// From cold the window is the whole pulse period, polled uniformly, and
// each catch halves it; a miss leaves it alone. Halving ends at its floor
// of two bracket widths, or at the first settled miss (the halving
// overshot the edge scatter); the window is then tracked additively: a
// miss widens it by a bracket width at each end and every shrinkAfter-th
// consecutive catch narrows it by the same, so it hovers just above the
// observed edge scatter, missing about one pulse in shrinkAfter. missLimit
// consecutive misses mean the pulse is gone: the window returns to the
// whole period in one step. At that size the poll grid is advanced by a
// fraction of the spacing each period, sweeping the phase so that a pulse
// narrower than the spacing is still found. A pulse already in progress
// when a window opens is polled through, not declared a miss: the search
// resumes on its far side.
//
// A candidate becomes settled once the polling schedule no longer controls
// resolution. The loop recognizes this from its pacing
// rather than inferring it from bracket measurements, which sleep overshoot
// contaminates while the loop is sleep-paced. A catch settles immediately
// when the spacing target sits at minSpacing, which halving no longer
// changes. Otherwise two consecutive caught windows must be polled without
// a single sleep firing; since each catch halves the window, the second
// confirms at a smaller spacing that the queries pace the loop. A
// sleep-paced catch or a miss clears the confirmation, so a transient run
// of slow queries cannot settle the detector. The latch clears only on
// the cold restart, i.e. on signal loss. Consumers that require usable edges
// must filter on Settled and take the embedded Edge. Every caught edge and
// every window size change is logged to lg at debug level. If stats is
// non-nil, Poll records timing and outcome statistics in it.
//
// A nonzero prewarm ends the sleep to each window open that much early and
// busy-waits the remainder. It is for hosts whose state queries slow down
// severalfold while the machine idles, where only continuous work ending at
// the open restores full query speed; it costs that fraction of a core.
func Poll(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, prewarm time.Duration, ceCh chan<- CandidateEdge, stats *PollStats) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	stats.begin()

	first, err := readState(ctx, r, time.Time{})
	if err != nil {
		return err
	}
	stats.addPoll(first.poll, nil)
	nextEdge := first.poll.midpoint().mono.Add(maxWindow / 2)
	window := maxWindow
	settled := false  // polling schedule no longer binds
	tracking := false // halving is over; the window moves additively
	catches, misses := 0, 0
	queryPaced := 0
	var prevBracket time.Duration
	for {
		spacing := max(window/initialPolls, minSpacing)
		deadline := nextEdge.Add(window / 2)
		open := nextEdge.Add(-window / 2)
		if prewarm > 0 {
			if _, err := waitUntil(ctx, open.Add(-prewarm)); err != nil {
				return err
			}
			for time.Now().Before(open) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
			}
		}
		cur, err := readState(ctx, r, open)
		if err != nil {
			return err
		}
		stats.addPoll(cur.poll, nil)
		polls := 1
		// The windows advance in lockstep with the pulses, so treating an
		// in-progress pulse at the open as a miss would reopen at the same
		// phase every period and never acquire; poll through it instead.
		// slept accumulates over the window's scheduled polls (the wait for
		// the window open is excluded: it always sleeps).
		slept := false
		for inPulse(cur.state, w) && cur.poll.midpoint().mono.Before(deadline) {
			prev := cur
			cur, err = readState(ctx, r, cur.start.Add(spacing))
			if err != nil {
				return err
			}
			stats.addPoll(cur.poll, &prev.poll)
			polls++
			slept = slept || cur.slept
		}
		prev := cur
		missed := inPulse(cur.state, w)
		var edge clockReading
		var bracket time.Duration
		for !missed && edge.stamp.IsZero() {
			cur, err = readState(ctx, r, prev.start.Add(spacing))
			if err != nil {
				return err
			}
			stats.addPoll(cur.poll, &prev.poll)
			polls++
			slept = slept || cur.slept
			edge, missed = classify(prev, cur, w, deadline)
			if !edge.stamp.IsZero() {
				bracket = cur.poll.midpoint().elapsedSince(prev.poll.midpoint())
			}
			prev = cur
		}
		if !edge.stamp.IsZero() {
			// "late" is how far past its scheduled time the catching poll
			// started: sleep overshoot when the loop is sleep-paced, queue
			// debt when the queries pace it.
			lg.Debug("serial PPS caught edge", "window", window, "bracket", bracket,
				"offset", edge.mono.Sub(nextEdge), "late", cur.start.Sub(cur.sched), "polls", polls)
			nextEdge = edge.mono.Add(period)
			misses = 0
			if !settled {
				if spacing == minSpacing {
					settled = true
				} else if slept {
					queryPaced = 0
				} else {
					queryPaced++
					if queryPaced >= 2 {
						settled = true
					}
				}
				if settled {
					lg.Debug("serial PPS settled", "window", window, "bracket", bracket)
				}
			}
			ce := CandidateEdge{
				Edge: Edge{
					Timestamp: edge.stamp,
					TRead:     cur.poll.end.mono,
				},
				Uncertainty: halfCeil(bracket),
				Settled:     settled,
			}
			stats.addWindow(true, settled)
			select {
			case ceCh <- ce:
			case <-ctx.Done():
				return ctx.Err()
			}
			if !tracking {
				if halved := max(window/2, 2*bracket); halved != window {
					window = halved
					lg.Debug("serial PPS poll window halved", "window", window)
				} else {
					tracking = true
				}
			} else if catches++; catches >= shrinkAfter && window >= 4*bracket {
				catches = 0
				window -= 2 * bracket
				lg.Debug("serial PPS poll window shrank", "window", window)
			}
			prevBracket = bracket
			continue
		}
		stats.addWindow(false, false)
		nextEdge = nextEdge.Add(period)
		queryPaced = 0
		if window == maxWindow {
			// At full size, consecutive windows tile the period exactly, so a
			// locked poll grid would revisit the same phases every period and
			// could straddle a pulse narrower than the spacing indefinitely;
			// advancing the grid by an irregular fraction of the spacing
			// sweeps the phase instead.
			nextEdge = nextEdge.Add(spacing * 618 / 1000)
			continue
		}
		if misses++; misses >= missLimit || ((settled || tracking) && window+2*prevBracket >= maxWindow) {
			window = maxWindow
			lg.Debug("serial PPS pulse lost, restarting acquisition", "window", window, "misses", misses)
			settled = false
			tracking = false
			prevBracket = 0
			catches, misses, queryPaced = 0, 0, 0
		} else if settled || tracking {
			tracking = true
			window += 2 * prevBracket
			catches = 0
			lg.Debug("serial PPS poll window grew", "window", window, "misses", misses, "polls", polls)
		}
	}
}

// Wait sends settled candidate edges from modem-control change notifications,
// using the wait or kernel method. The backend timestamps each unambiguous
// transition, so these candidates have no polling uncertainty. The pulse's
// electrically rising leading edge reaches the host inverted through the TTL
// driver chain, so the pin reads deasserted during the pulse.
func Wait(ctx context.Context, lg *slog.Logger, r ChangeWaiter, w Wiring, method gpsio.PPSMethod, ceCh chan<- CandidateEdge) error {
	for {
		change, missed, err := r.WaitModemControlPinChange(ctx, w.Pin, method)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if missed > 0 {
			lg.Debug("serial PPS transitions not observed", "atLeast", missed)
		}
		if !change.Asserted {
			ce := CandidateEdge{
				Edge:    Edge{Timestamp: change.Timestamp, TRead: change.TRead},
				Settled: true,
			}
			select {
			case ceCh <- ce:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func halfCeil(d time.Duration) time.Duration {
	return d/2 + d%2
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

func midpoint(a, b time.Time) time.Time {
	return a.Add(b.Sub(a) / 2)
}

// Sample is a serial-PPS refclock sample.
type Sample struct {
	Ref  time.Time
	Sys  time.Time
	Leap ptime.LeapSecondKind
}

// Generator associates PPS edges with UTC seconds using the latest receiver
// time message. It is intended to be called from one dispatcher goroutine.
type Generator struct {
	msgUTC           time.Time
	msgRead          time.Time // zero until the first message arrives
	msgLeap          ptime.LeapSecondKind
	delayUncertainty time.Duration
	maxDelay         time.Duration
}

// NewGenerator creates an empty sample generator using cfg.
func NewGenerator(cfg Config) *Generator {
	return &Generator{
		delayUncertainty: seconds(cfg.DelayUncertainty),
		maxDelay:         seconds(cfg.MaxDelay),
	}
}

// MsgUTCTime records the newest UTC/system-time pair from a receiver message.
func (g *Generator) MsgUTCTime(utc, tRead time.Time, leap ptime.LeapSecondKind) {
	if tRead.Before(g.msgRead) {
		return
	}
	g.msgUTC = utc
	g.msgRead = tRead
	g.msgLeap = leap
}

// Sample turns a precisely detected PPS edge into a sample. It returns false
// until a time message is available, when the newest message is over three
// seconds old, when no unique UTC label satisfies the configured
// pulse-to-message delay bounds, or for the pulse marking an inserted leap
// second.
func (g *Generator) Sample(edge Edge) (Sample, bool) {
	if g.msgRead.IsZero() {
		return Sample{}, false
	}
	// Transfer the timestamp onto the message-read timeline through TRead.
	// The long message-to-read interval is monotonic; the short correction
	// back to the edge uses Timestamp's monotonic reading when it has one and
	// otherwise its wall reading. A wall-clock step during that correction can
	// still corrupt it, but a step anywhere else in the message-to-edge span
	// cannot. Use the transferred interval for both age and UTC extrapolation.
	edgeSinceMsg := edge.TRead.Sub(g.msgRead) - edge.TRead.Sub(edge.Timestamp)
	if edgeSinceMsg > maxMsgAge {
		return Sample{}, false
	}
	// Select the integral second whose inferred pulse-to-message delay is in
	// the configured causal interval. The validated interval is narrower than
	// a second, so the label is unique when it exists.
	edgeUTC := g.msgUTC.Add(edgeSinceMsg)
	ref := edgeUTC.Truncate(time.Second)
	delay := ref.Sub(edgeUTC)
	if delay < -g.delayUncertainty {
		ref = ref.Add(time.Second)
		delay += time.Second
	}
	if delay < -g.delayUncertainty || delay >= g.maxDelay {
		return Sample{}, false
	}
	// The message's leap flag announces a leap at the end of the message's
	// UTC day, which the monotonic extrapolation cannot see: past the
	// boundary it runs one second ahead of UTC after a positive leap and
	// one second behind after a negative one. The inserted second itself
	// has no value on the leap-free timescale of time.Time and the refclock
	// samples (midnight is the next pulse's label, 23:59:59 the previous
	// one's), so its pulse yields no sample.
	midnight := g.msgUTC.Truncate(24 * time.Hour).Add(24 * time.Hour)
	if g.msgLeap == ptime.LeapSecondPositive && !ref.Before(midnight) {
		if ref.Equal(midnight) {
			return Sample{}, false
		}
		ref = ref.Add(-time.Second)
	} else if g.msgLeap == ptime.LeapSecondNegative && !ref.Before(midnight.Add(-time.Second)) {
		ref = ref.Add(time.Second)
	}
	leap := g.msgLeap
	// If the edge falls in a different day than the message, the
	// announcement does not apply to it; a retained pre-midnight flag must
	// not re-announce a leap that has already happened.
	if !ref.Truncate(24 * time.Hour).Equal(g.msgUTC.Truncate(24 * time.Hour)) {
		leap = ptime.LeapSecondNone
	}
	return Sample{
		Ref:  ref,
		Sys:  edge.Timestamp,
		Leap: leap,
	}, true
}
