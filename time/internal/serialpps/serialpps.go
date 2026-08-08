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
	pollsPerWindow = 8
	safetyFactor   = 4
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
// window to safetyFactor times the measured bracket gap, and a miss doubles
// it up to half the pulse period, where the window covers the whole period
// and polling is uniform (the cold-start state). Everything hardware- and
// load-dependent comes from measured poll timestamps: the shrink bottoms out
// wherever the state-query time, the spacing floor, pulse jitter, or clock
// drift binds, without the loop knowing which.
//
// Edges are sent only once settled: while the window is still shrinking
// toward its measured floor, caught edges are too coarse to publish. The
// settled state latches at the first catch that does not shrink the window
// (excluding catches that leave it at the cap, where settling has not
// begun), and clears only when the window walks back up to the cap, i.e. on
// signal loss. After the latch every caught edge is sent, however coarse: a
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
	for {
		spacing := max(margin/pollsPerWindow, minPollSpacing)
		cur, err := readState(ctx, r, predicted.Add(-margin))
		if err != nil {
			return err
		}
		prev := cur
		missed := inPulse(cur.state, w)
		deadline := predicted.Add(margin)
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
			newMargin := min(safetyFactor*gap, maxMargin)
			if !settled && newMargin >= margin && newMargin < maxMargin {
				settled = true
			}
			margin = newMargin
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
		}
		predicted = predicted.Add(pulsePeriod)
	}
}

// classifyReading gives a detected transition precedence over the deadline.
// The deadline says when to stop looking, not whether a measured edge is valid.
func classifyReading(prev, cur reading, w Wiring, deadline time.Time) (time.Time, bool) {
	if !inPulse(prev.state, w) && inPulse(cur.state, w) {
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
// until a time message is available or when the newest message is over three
// seconds old.
func (g *Generator) Edge(edge Edge) (Sample, bool) {
	if g.tRead.IsZero() || edge.T.Sub(g.tRead) > maxMessageAge {
		return Sample{}, false
	}
	// Advancing utc by the monotonic elapsed time since tRead puts the edge
	// within half a second of the UTC second it marks, and is immune to any
	// wall-clock step between the message and the edge.
	reference := g.utc.Add(edge.T.Sub(g.tRead)).Round(time.Second)
	leap := g.leap
	// The message's leap flag announces a leap at the end of the message's
	// UTC day. If the edge falls in a different day, that announcement does
	// not apply to it; a retained pre-midnight flag must not re-announce a
	// leap that has already happened.
	if !reference.Truncate(24 * time.Hour).Equal(g.utc.Truncate(24 * time.Hour)) {
		leap = ptime.LeapSecondNone
	}
	return Sample{
		Reference: reference,
		System:    edge.T,
		Leap:      leap,
	}, true
}
