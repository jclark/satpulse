// Package serialpps detects PPS edges on serial modem-control pins and turns
// them into refclock samples using UTC time messages from the receiver.
package serialpps

import (
	"context"
	"runtime"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

const (
	// The chosen constants are the safety factor (the window half-width in
	// units of the measured bracket gap, absorbing that much prediction
	// error) and the shrink rate (what each catch multiplies the window by
	// during acquisition). The polls per window follow from them: spacing
	// is window/pollsPerWindow and a catch's gap is one spacing, so the
	// shrink per catch is safetyFactor/pollsPerWindow. Deriving rather
	// than choosing pollsPerWindow keeps the pair convergent; a shrink
	// rate of 1 or more would leave the window stuck at the cap.
	safetyFactor   = 16
	shrinkRate     = 0.5
	pollsPerWindow = safetyFactor / shrinkRate
	minPollSpacing = 50 * time.Microsecond
	pulsePeriod    = time.Second
	maxMargin      = pulsePeriod / 2
	maxMessageAge  = 3 * time.Second
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
}

// Poll adaptively polls for the pulse described by w and sends detected
// leading edges to edges. One loop with two rules: a caught edge shrinks the
// window to safetyFactor times the measured bracket gap (capped at half the
// pulse period), and a miss doubles it up to that cap, where the window
// covers the whole period and polling is uniform (the cold-start state). A
// pulse already in progress when a window opens is polled through, not
// declared a miss: the search resumes on its far side. Everything hardware-
// and load-dependent comes from measured poll timestamps: the shrink
// bottoms out wherever the state-query time or the spacing floor binds,
// without the loop knowing which.
//
// Edges are sent only once settled: while each catch still improves on the
// previous catch's bracket gap, caught edges are too coarse to publish. The
// settled state latches at the first catch whose gap does not improve on
// the previous one (misses in between do not affect the comparison), and
// clears only when the window walks back up to the cap, i.e. on signal
// loss. After the latch every caught edge is sent, however coarse: a
// stretched sample is characteristic of what the system delivers, and
// outliers are the NTP daemon's filtering's job.
func Poll(ctx context.Context, r StateReader, w Wiring, edges chan<- Edge) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	first, err := readState(ctx, r, time.Time{})
	if err != nil {
		return err
	}
	predicted := first.at.Add(maxMargin)
	margin := maxMargin
	settled := false
	var prevGap time.Duration
	for {
		spacing := max(margin/pollsPerWindow, minPollSpacing)
		deadline := predicted.Add(margin)
		cur, err := readState(ctx, r, predicted.Add(-margin))
		if err != nil {
			return err
		}
		// The windows advance in lockstep with the pulses, so treating an
		// in-progress pulse at the open as a miss would reopen at the same
		// phase every period and never acquire; poll through it instead.
		for inPulse(cur.state, w) && cur.at.Before(deadline) {
			cur, err = readState(ctx, r, cur.start.Add(spacing))
			if err != nil {
				return err
			}
		}
		prev := cur
		missed := inPulse(cur.state, w)
		var edge time.Time
		var gap time.Duration
		for !missed && edge.IsZero() {
			cur, err = readState(ctx, r, prev.start.Add(spacing))
			if err != nil {
				return err
			}
			edge, missed = classifyReading(prev, cur, w, deadline)
			if !edge.IsZero() {
				gap = cur.at.Sub(prev.at)
			}
			prev = cur
		}
		if !edge.IsZero() {
			predicted = edge.Add(pulsePeriod)
			if !settled && prevGap > 0 && gap >= prevGap {
				settled = true
			}
			prevGap = gap
			margin = min(safetyFactor*gap, maxMargin)
			if settled {
				select {
				case edges <- Edge{T: edge}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			continue
		}
		margin = min(2*margin, maxMargin)
		if margin == maxMargin {
			settled = false
			prevGap = 0
		}
		predicted = predicted.Add(pulsePeriod)
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
	if err := waitUntil(ctx, notBefore); err != nil {
		return reading{}, err
	}
	start := time.Now()
	state, err := r.ModemControlPinState()
	end := time.Now()
	if err != nil {
		return reading{}, err
	}
	return reading{state: state, at: midpoint(start, end), start: start}, nil
}

func waitUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
