// Package serialpps detects PPS edges on serial modem-control pins and turns
// them into refclock samples using UTC time messages from the receiver.
package serialpps

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

// None of these constants encodes hardware timing; everything hardware- and
// load-dependent is measured by the polling loop itself.
const (
	// initialPollsPerSecond is the polling rate from cold, when the
	// window is the whole period; it determines the cold-start spacing,
	// and with it the narrowest pulse acquired promptly. The spacing
	// scales with the window (window divided by this), so once it falls
	// below the state-query time or minPollSpacing, those pace the loop
	// instead and the constant has no further effect.
	initialPollsPerSecond = 64
	// shrinkAfter is the target miss rate: the settled window widens by a
	// bracket width at each end on a miss and narrows by the same after
	// this many consecutive catches, so it hovers just above the observed
	// edge scatter, missing about one pulse in shrinkAfter.
	shrinkAfter = 60
	// missLimit consecutive misses declare the pulse gone, restarting
	// acquisition from the full-period window.
	missLimit = 10
	// minPollSpacing bounds the CPU spent when the state query is very
	// fast.
	minPollSpacing = 50 * time.Microsecond
	pulsePeriod    = time.Second
	// maxPollWindow is the whole period: the cold-start window, within
	// which polling is uniform.
	maxPollWindow = pulsePeriod
	// maxMessageAge is how old the newest receiver time message may be
	// for an edge to still be identified with a UTC second.
	maxMessageAge = 3 * time.Second
)

// Edge is a precisely polled leading edge. T is the midpoint of the two modem
// state observations that straddled the transition.
type Edge struct {
	T time.Time
}

// StateReader is implemented by a TTY-backed gpsio.SerialConn.
type StateReader interface {
	ModemControlPinState() (gpsio.ModemControlPinState, error)
}

// Wiring describes how the PPS pulse is represented on the serial port's
// modem-control inputs.
type Wiring struct {
	Pin gpsio.ModemControlPin
}

type reading struct {
	state gpsio.ModemControlPinState
	at    time.Time
	start time.Time
	sched time.Time // when this poll was scheduled to run
	slept bool      // whether the schedule was still in the future
}

// Poll adaptively polls for the pulse described by w and sends detected
// leading edges to edges. Each period it polls a window centered on the
// predicted next edge time. A caught edge is located by its bracket: the
// pair of consecutive polls with one poll on each side of the transition.
// The bracket's width -- the time between those two polls -- is how
// precisely the edge is located, and is the unit the window is sized in.
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
// Edges are sent only once the settled latch is set. The latch means the
// resolution has reached its floor, and the loop observes that directly
// from its pacing rather than inferring it from bracket measurements,
// which sleep overshoot contaminates while the loop is sleep-paced: a
// catch settles the loop when its window was polled without a single
// sleep firing (the queries pace the loop, so halving cannot improve
// resolution further) or when the spacing target sits at minPollSpacing,
// which halving no longer changes. The latch clears only on the cold
// restart, i.e. on signal loss. Before the latch, caught edges are too
// coarse to publish; after it every caught edge is sent, however coarse:
// a stretched sample is characteristic of what the system delivers, and
// outliers are the NTP daemon's filtering's job. Every caught edge and
// every window size change is logged to lg at debug level.
func Poll(ctx context.Context, r StateReader, w Wiring, edges chan<- Edge, lg *slog.Logger) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	first, err := readState(ctx, r, time.Time{})
	if err != nil {
		return err
	}
	predicted := first.at.Add(maxPollWindow / 2)
	window := maxPollWindow
	settled := false  // publishing gate: resolution at its floor
	tracking := false // halving is over; the window moves additively
	catches, misses := 0, 0
	var prevBracketWidth time.Duration
	for {
		spacing := max(window/initialPollsPerSecond, minPollSpacing)
		deadline := predicted.Add(window / 2)
		cur, err := readState(ctx, r, predicted.Add(-window/2))
		if err != nil {
			return err
		}
		// The windows advance in lockstep with the pulses, so treating an
		// in-progress pulse at the open as a miss would reopen at the same
		// phase every period and never acquire; poll through it instead.
		// slept accumulates over the window's scheduled polls (the wait for
		// the window open is excluded: it always sleeps).
		slept := false
		for inPulse(cur.state, w) && cur.at.Before(deadline) {
			cur, err = readState(ctx, r, cur.start.Add(spacing))
			if err != nil {
				return err
			}
			slept = slept || cur.slept
		}
		prev := cur
		missed := inPulse(cur.state, w)
		var edge time.Time
		var bracketWidth time.Duration
		for !missed && edge.IsZero() {
			cur, err = readState(ctx, r, prev.start.Add(spacing))
			if err != nil {
				return err
			}
			slept = slept || cur.slept
			edge, missed = classifyReading(prev, cur, w, deadline)
			if !edge.IsZero() {
				bracketWidth = cur.at.Sub(prev.at)
			}
			prev = cur
		}
		if !edge.IsZero() {
			// "late" is how far past its scheduled time the catching poll
			// started: sleep overshoot when the loop is sleep-paced, queue
			// debt when the queries pace it.
			lg.Debug("serial PPS caught edge", "window", window, "bracket", bracketWidth,
				"offset", edge.Sub(predicted), "late", cur.start.Sub(cur.sched))
			predicted = edge.Add(pulsePeriod)
			misses = 0
			if !settled && (!slept || spacing == minPollSpacing) {
				settled = true
				lg.Debug("serial PPS settled", "window", window, "bracket", bracketWidth)
			}
			if !tracking {
				if halved := max(window/2, 2*bracketWidth); halved != window {
					window = halved
					lg.Debug("serial PPS poll window halved", "window", window)
				} else {
					tracking = true
				}
			} else if catches++; catches >= shrinkAfter && window >= 4*bracketWidth {
				catches = 0
				window -= 2 * bracketWidth
				lg.Debug("serial PPS poll window shrank", "window", window)
			}
			prevBracketWidth = bracketWidth
			if settled {
				select {
				case edges <- Edge{T: edge}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			continue
		}
		predicted = predicted.Add(pulsePeriod)
		if window == maxPollWindow {
			// At full size, consecutive windows tile the period exactly, so a
			// locked poll grid would revisit the same phases every period and
			// could straddle a pulse narrower than the spacing indefinitely;
			// advancing the grid by an irregular fraction of the spacing
			// sweeps the phase instead.
			predicted = predicted.Add(spacing * 618 / 1000)
			continue
		}
		if misses++; misses >= missLimit || ((settled || tracking) && window+2*prevBracketWidth >= maxPollWindow) {
			window = maxPollWindow
			lg.Debug("serial PPS pulse lost, restarting acquisition", "window", window, "misses", misses)
			settled = false
			tracking = false
			prevBracketWidth = 0
			catches, misses = 0, 0
		} else if settled || tracking {
			tracking = true
			window += 2 * prevBracketWidth
			catches = 0
			lg.Debug("serial PPS poll window grew", "window", window, "misses", misses)
		}
	}
}

// classifyReading gives a detected transition precedence over the deadline.
// The deadline says when to stop looking, not whether a measured edge is
// valid. A bracket spanning a full period or more may contain several
// leading edges, so its midpoint identifies none of them: it is a miss.
func classifyReading(prev, cur reading, w Wiring, deadline time.Time) (time.Time, bool) {
	if !inPulse(prev.state, w) && inPulse(cur.state, w) {
		if cur.at.Sub(prev.at) >= pulsePeriod {
			return time.Time{}, true
		}
		return midpoint(prev.at, cur.at), false
	}
	return time.Time{}, !cur.at.Before(deadline)
}

// inPulse reports whether the state was observed during a pulse. The pulse's
// electrically rising leading edge reaches the host inverted through the TTL
// driver chain, so the flag reads deasserted during the pulse.
func inPulse(s gpsio.ModemControlPinState, w Wiring) bool {
	return !s.Asserted(w.Pin)
}

func readState(ctx context.Context, r StateReader, notBefore time.Time) (reading, error) {
	slept, err := waitUntil(ctx, notBefore)
	if err != nil {
		return reading{}, err
	}
	start := time.Now()
	state, err := r.ModemControlPinState()
	end := time.Now()
	if err != nil {
		return reading{}, err
	}
	return reading{state: state, at: midpoint(start, end), start: start, sched: notBefore, slept: slept}, nil
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
	Reference time.Time
	System    time.Time
	Leap      ptime.LeapSecondKind
}

// Generator associates PPS edges with UTC seconds using the latest receiver
// time message. It is intended to be called from one dispatcher goroutine.
type Generator struct {
	utc   time.Time
	tRead time.Time // zero until the first message arrives
	leap  ptime.LeapSecondKind
}

// NewGenerator creates an empty sample generator.
func NewGenerator() *Generator {
	return new(Generator)
}

// MsgUTCTime records the newest UTC/system-time pair from a receiver message.
func (g *Generator) MsgUTCTime(utc, tRead time.Time, leap ptime.LeapSecondKind) {
	if tRead.Before(g.tRead) {
		return
	}
	g.utc = utc
	g.tRead = tRead
	g.leap = leap
}

// Edge turns a precisely detected PPS edge into a sample. It returns false
// until a time message is available, when the newest message is over three
// seconds old, or for the pulse marking an inserted leap second.
func (g *Generator) Edge(edge Edge) (Sample, bool) {
	if g.tRead.IsZero() || edge.T.Sub(g.tRead) > maxMessageAge {
		return Sample{}, false
	}
	// Advancing utc by the monotonic elapsed time since tRead puts the edge
	// within half a second of the UTC second it marks, and is immune to any
	// wall-clock step between the message and the edge.
	reference := g.utc.Add(edge.T.Sub(g.tRead)).Round(time.Second)
	// The message's leap flag announces a leap at the end of the message's
	// UTC day, which the monotonic extrapolation cannot see: past the
	// boundary it runs one second ahead of UTC after a positive leap and
	// one second behind after a negative one. The inserted second itself
	// has no value on the leap-free timescale of time.Time and the refclock
	// samples (midnight is the next pulse's label, 23:59:59 the previous
	// one's), so its pulse yields no sample.
	midnight := g.utc.Truncate(24 * time.Hour).Add(24 * time.Hour)
	if g.leap == ptime.LeapSecondPositive && !reference.Before(midnight) {
		if reference.Equal(midnight) {
			return Sample{}, false
		}
		reference = reference.Add(-time.Second)
	} else if g.leap == ptime.LeapSecondNegative && !reference.Before(midnight.Add(-time.Second)) {
		reference = reference.Add(time.Second)
	}
	leap := g.leap
	// If the edge falls in a different day than the message, the
	// announcement does not apply to it; a retained pre-midnight flag must
	// not re-announce a leap that has already happened.
	if !reference.Truncate(24 * time.Hour).Equal(g.utc.Truncate(24 * time.Hour)) {
		leap = ptime.LeapSecondNone
	}
	return Sample{
		Reference: reference,
		System:    edge.T,
		Leap:      leap,
	}, true
}
