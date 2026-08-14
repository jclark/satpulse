// Package serialpps detects PPS edges on serial modem-control pins and turns
// them into refclock samples using UTC time messages from the receiver.
package serialpps

import (
	"context"
	"errors"
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

// Edge is the detected time of a leading edge. Wall and Mono are readings of
// that one instant on the two clocks used here: Wall is the most precise
// available system-time reading and is what published samples carry; Mono is
// an ordinary time.Now reading, the only one valid for elapsed-time arithmetic
// against other time.Now values such as message read times. Polling locates the
// edge at the midpoint of the modem-state observations straddling it; a wait
// backend uses the wait primitive's paired wakeup readings. Where time.Now is
// the best clock available both fields contain the same reading.
type Edge struct {
	Wall time.Time
	Mono time.Time
}

// Observation is a leading edge together with the quality of its detection.
// For polling, Edge is the midpoint of the modem-state observations bracketing
// the transition, Uncertainty is half the elapsed time between them, and
// Settled reports whether the loop has adapted far enough that scheduled sleeps
// no longer control its normal resolution. A wait observation carries the
// wait timestamp directly, has no polling-bracket uncertainty, and is settled.
type Observation struct {
	Edge
	Uncertainty time.Duration
	Settled     bool
}

// StateReader is implemented by a TTY-backed gpsio.SerialConn.
type StateReader interface {
	ModemControlPinState() (gpsio.ModemControlPinState, error)
}

// ChangeWaiter is a StateReader that may be able to block until a modem
// control input changes. CanWaitModemControlPinChange reports whether it
// actually can. Implemented by a TTY-backed gpsio.SerialConn.
type ChangeWaiter interface {
	StateReader
	CanWaitModemControlPinChange() bool
	WaitModemControlPinChange(context.Context, gpsio.ModemControlPin) (gpsio.ModemControlPinChange, int, error)
}

// Wiring describes how the PPS pulse is represented on the serial port's
// modem-control inputs.
type Wiring struct {
	Pin gpsio.ModemControlPin
}

// clockReading keeps adjacent readings of the clocks used by serial PPS
// together. wall locates an edge in system time; mono relates it to receiver
// message read times and paces the polling loop, which must not be disturbed
// by a step in the system clock. A platform can choose the better pair for
// measuring short elapsed intervals in elapsedSince.
type clockReading struct {
	wall time.Time
	mono time.Time
}

func (r clockReading) midpoint(other clockReading) clockReading {
	return clockReading{
		wall: midpoint(r.wall, other.wall),
		mono: midpoint(r.mono, other.mono),
	}
}

func (r clockReading) edge() Edge {
	return Edge{Wall: r.wall, Mono: r.mono}
}

type reading struct {
	state gpsio.ModemControlPinState
	poll  poll
	start time.Time
	sched time.Time // when this poll was scheduled to run
	slept bool      // whether the schedule was still in the future
}

// poll retains both clock readings around one modem-state query. Its methods
// choose the platform-appropriate clock for elapsed-time measurements.
type poll struct {
	start clockReading
	end   clockReading
}

func (p poll) observationTime() clockReading {
	return p.start.midpoint(p.end)
}

func (p poll) duration() time.Duration {
	return p.end.elapsedSince(p.start)
}

func (p poll) gapAfter(prev poll) time.Duration {
	return p.start.elapsedSince(prev.end)
}

// Detect sends observations of the pulse described by w, blocking on the
// wait primitive when r provides it and polling adaptively otherwise. A tty
// driver without the wait shows up only as the first wait failing with
// errors.ErrUnsupported; Detect then falls back to polling. If stats is
// non-nil, it records timings only when the polling backend is selected.
func Detect(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, observations chan<- Observation, stats *PollStats) error {
	if cw, ok := r.(ChangeWaiter); ok && cw.CanWaitModemControlPinChange() {
		lg.Debug("serial PPS wait backend selected")
		err := Wait(ctx, lg, cw, w, observations)
		if !errors.Is(err, errors.ErrUnsupported) {
			return err
		}
		lg.Info("serial driver cannot wait for modem control pin changes; polling instead")
	}
	lg.Debug("serial PPS polling backend selected")
	return Poll(ctx, lg, r, w, observations, stats)
}

// Poll adaptively polls for the pulse described by w and sends every detected
// leading edge to observations. Each period it polls a window centered on the
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
// The Settled field is set once the settled latch is set. The loop observes
// that the polling schedule no longer controls resolution from its pacing
// rather than inferring it from bracket measurements, which sleep overshoot
// contaminates while the loop is sleep-paced. A catch settles immediately
// when the spacing target sits at minPollSpacing, which halving no longer
// changes. Otherwise two consecutive caught windows must be polled without
// a single sleep firing; since each catch halves the window, the second
// confirms at a smaller spacing that the queries pace the loop. A
// sleep-paced catch or a miss clears the confirmation, so a transient run
// of slow queries cannot settle the detector. The latch clears only on
// the cold restart, i.e. on signal loss. Every caught edge is sent; consumers
// that require settled edges must filter on Settled. Every caught edge and
// every window size change is logged to lg at debug level. If stats is
// non-nil, Poll records timing and outcome statistics in it.
func Poll(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, observations chan<- Observation, stats *PollStats) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	stats.begin()

	first, err := readState(ctx, r, time.Time{})
	if err != nil {
		return err
	}
	stats.addPoll(first.poll, nil)
	predicted := first.poll.observationTime().mono.Add(maxPollWindow / 2)
	window := maxPollWindow
	settled := false  // polling schedule no longer binds
	tracking := false // halving is over; the window moves additively
	catches, misses := 0, 0
	queryPacedCatches := 0
	var prevBracketWidth time.Duration
	for {
		spacing := max(window/initialPollsPerSecond, minPollSpacing)
		deadline := predicted.Add(window / 2)
		cur, err := readState(ctx, r, predicted.Add(-window/2))
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
		for inPulse(cur.state, w) && cur.poll.observationTime().mono.Before(deadline) {
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
		var edge Edge
		var bracketWidth time.Duration
		for !missed && edge.Wall.IsZero() {
			cur, err = readState(ctx, r, prev.start.Add(spacing))
			if err != nil {
				return err
			}
			stats.addPoll(cur.poll, &prev.poll)
			polls++
			slept = slept || cur.slept
			edge, missed = classifyReading(prev, cur, w, deadline)
			if !edge.Wall.IsZero() {
				bracketWidth = cur.poll.observationTime().elapsedSince(prev.poll.observationTime())
			}
			prev = cur
		}
		if !edge.Wall.IsZero() {
			// "late" is how far past its scheduled time the catching poll
			// started: sleep overshoot when the loop is sleep-paced, queue
			// debt when the queries pace it.
			lg.Debug("serial PPS caught edge", "window", window, "bracket", bracketWidth,
				"offset", edge.Mono.Sub(predicted), "late", cur.start.Sub(cur.sched), "polls", polls)
			predicted = edge.Mono.Add(pulsePeriod)
			misses = 0
			if !settled {
				if spacing == minPollSpacing {
					settled = true
				} else if slept {
					queryPacedCatches = 0
				} else {
					queryPacedCatches++
					if queryPacedCatches >= 2 {
						settled = true
					}
				}
				if settled {
					lg.Debug("serial PPS settled", "window", window, "bracket", bracketWidth)
				}
			}
			observation := Observation{
				Edge:        edge,
				Uncertainty: halfDurationCeil(bracketWidth),
				Settled:     settled,
			}
			stats.addWindow(true, settled)
			select {
			case observations <- observation:
			case <-ctx.Done():
				return ctx.Err()
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
			continue
		}
		stats.addWindow(false, false)
		predicted = predicted.Add(pulsePeriod)
		queryPacedCatches = 0
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
			catches, misses, queryPacedCatches = 0, 0, 0
		} else if settled || tracking {
			tracking = true
			window += 2 * prevBracketWidth
			catches = 0
			lg.Debug("serial PPS poll window grew", "window", window, "misses", misses, "polls", polls)
		}
	}
}

// Wait sends observations of leading edges detected with modem-control change
// notifications. The wait primitive timestamps each unambiguous transition
// and reports the pin sense derived by the backend. The pulse's electrically
// rising leading edge reaches the host inverted through the TTL driver chain,
// so the pin reads deasserted during the pulse.
func Wait(ctx context.Context, lg *slog.Logger, r ChangeWaiter, w Wiring, observations chan<- Observation) error {
	for {
		change, missed, err := r.WaitModemControlPinChange(ctx, w.Pin)
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
			observation := Observation{
				Edge:    Edge{Wall: change.Wall, Mono: change.Mono},
				Settled: true,
			}
			select {
			case observations <- observation:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func halfDurationCeil(d time.Duration) time.Duration {
	return d/2 + d%2
}

// classifyReading gives a detected transition precedence over the deadline.
// The deadline says when to stop looking, not whether a measured edge is
// valid. A bracket spanning a full period or more may contain several
// leading edges, so its midpoint identifies none of them: it is a miss.
func classifyReading(prev, cur reading, w Wiring, deadline time.Time) (Edge, bool) {
	if !inPulse(prev.state, w) && inPulse(cur.state, w) {
		if cur.poll.observationTime().elapsedSince(prev.poll.observationTime()) >= pulsePeriod {
			return Edge{}, true
		}
		return prev.poll.observationTime().midpoint(cur.poll.observationTime()).edge(), false
	}
	return Edge{}, !cur.poll.observationTime().mono.Before(deadline)
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
	start := now()
	state, err := r.ModemControlPinState()
	end := now()
	if err != nil {
		return reading{}, err
	}
	return reading{state: state, poll: poll{start: start, end: end}, start: start.mono,
		sched: notBefore, slept: slept}, nil
}

// now reads the clocks behind Edge: wall locates edges, mono paces the loop
// and serves elapsed-time arithmetic against other time.Now values. Here
// they are one time.Now reading; a platform whose precise wall-clock read
// carries no monotonic reading separates them.
func now() clockReading {
	t := time.Now()
	return clockReading{wall: t, mono: t}
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
	utc              time.Time
	tRead            time.Time // zero until the first message arrives
	leap             ptime.LeapSecondKind
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
	if tRead.Before(g.tRead) {
		return
	}
	g.utc = utc
	g.tRead = tRead
	g.leap = leap
}

// Edge turns a precisely detected PPS edge into a sample. It returns false
// until a time message is available, when the newest message is over three
// seconds old, when no unique UTC label satisfies the configured
// pulse-to-message delay bounds, or for the pulse marking an inserted leap
// second.
func (g *Generator) Edge(edge Edge) (Sample, bool) {
	if g.tRead.IsZero() || edge.Mono.Sub(g.tRead) > maxMessageAge {
		return Sample{}, false
	}
	// Advancing utc by the monotonic elapsed time since tRead gives the
	// edge's position on the message's UTC timescale and is immune to any
	// wall-clock step between the message and the edge. Select the integral
	// second whose inferred pulse-to-message delay is in the configured
	// causal interval. The validated interval is narrower than a second, so
	// the label is unique when it exists.
	candidate := g.utc.Add(edge.Mono.Sub(g.tRead))
	reference := candidate.Truncate(time.Second)
	delay := reference.Sub(candidate)
	if delay < -g.delayUncertainty {
		reference = reference.Add(time.Second)
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
		System:    edge.Wall,
		Leap:      leap,
	}, true
}
