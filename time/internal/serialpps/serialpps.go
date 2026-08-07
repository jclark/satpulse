// Package serialpps detects PPS edges on serial modem-control lines and turns
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
	slowPollSpacing = 100 * time.Millisecond
	windowMargin    = 5 * time.Millisecond
	maxWindowMargin = 100 * time.Millisecond
	minPollSpacing  = 100 * time.Microsecond
	pulsePeriod     = time.Second
	maxMessageAge   = 3 * time.Second
)

// Edge is a precisely polled leading edge. T is the midpoint of the two modem
// state observations that straddled the transition.
type Edge struct {
	T time.Time
}

// StateReader is implemented by a TTY-backed gpsio.SerialConn.
type StateReader interface {
	ModemControlLineState() (gpsio.ModemControlLineState, error)
}

// Wiring describes how the PPS pulse is represented on the serial port's
// modem-control inputs.
type Wiring struct {
	Line gpsio.ModemControlLine
}

type reading struct {
	state gpsio.ModemControlLineState
	at    time.Time
	start time.Time
}

// Poll adaptively polls for the pulse described by w and sends precisely
// detected leading edges to edges. Edges found during slow phase acquisition
// are used only to predict the next pulse and are not sent.
func Poll(ctx context.Context, r StateReader, w Wiring, edges chan<- Edge) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	prev, err := readState(ctx, r, time.Time{})
	if err != nil {
		return err
	}
	for {
		// Slow polling acquires the approximate pulse phase. The acquisition
		// edge itself is too coarsely known to publish.
		var coarseEdge time.Time
		for coarseEdge.IsZero() {
			cur, err := readState(ctx, r, prev.start.Add(slowPollSpacing))
			if err != nil {
				return err
			}
			if !inPulse(prev.state, w) && inPulse(cur.state, w) {
				coarseEdge = midpoint(prev.at, cur.at)
			}
			prev = cur
		}

		predicted := coarseEdge.Add(pulsePeriod)
		margin := windowMargin
		firstWindow := true
		for {
			earlyMargin := margin
			lateMargin := margin
			if firstWindow {
				earlyMargin += slowPollSpacing / 2
				lateMargin += slowPollSpacing / 2
			}
			cur, err := readState(ctx, r, predicted.Add(-earlyMargin))
			if err != nil {
				return err
			}
			prev = cur
			missed := inPulse(cur.state, w)
			deadline := predicted.Add(lateMargin)
			var edge time.Time
			for !missed && edge.IsZero() {
				cur, err = readState(ctx, r, prev.start.Add(minPollSpacing))
				if err != nil {
					return err
				}
				edge, missed = classifyReading(prev, cur, w, deadline)
				prev = cur
			}
			if !edge.IsZero() {
				select {
				case edges <- Edge{T: edge}:
				case <-ctx.Done():
					return ctx.Err()
				}
				predicted = edge.Add(pulsePeriod)
				margin = windowMargin
				firstWindow = false
				continue
			}

			// A missed pulse widens both sides of the next window. Once the
			// margin would exceed the slow-poll interval, reacquire phase.
			margin *= 2
			if margin > maxWindowMargin {
				break
			}
			predicted = predicted.Add(pulsePeriod)
			firstWindow = false
		}
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
func inPulse(s gpsio.ModemControlLineState, w Wiring) bool {
	return !s.Asserted(w.Line)
}

func readState(ctx context.Context, r StateReader, notBefore time.Time) (reading, error) {
	if err := waitUntil(ctx, notBefore); err != nil {
		return reading{}, err
	}
	start := time.Now()
	state, err := r.ModemControlLineState()
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
	Offset    float64
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
	// utc-tRead estimates the system clock error. Applying it at the edge
	// puts the edge within half a second of the UTC second that it marks.
	reference := edge.T.Add(g.utc.Sub(g.tRead)).Round(time.Second)
	return Sample{
		Reference: reference,
		System:    edge.T,
		Offset:    reference.Sub(edge.T).Seconds(),
		Leap:      g.leap,
	}, true
}
