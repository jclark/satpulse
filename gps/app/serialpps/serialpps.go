// Package serialpps detects PPS edges on serial modem-control pins and turns
// them into refclock samples using UTC time messages from the receiver.
package serialpps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

// maxMsgAge is how old the newest receiver time message may be for an edge to
// still be identified with a UTC second.
const maxMsgAge = 3 * time.Second

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

// CandidateEdge is an edge reported by a detection backend. Poll reports
// every edge it catches so that diagnostic consumers can follow acquisition:
// Uncertainty is half the width of the polling bracket, and Settled says no
// improvement in accuracy is to be expected, because the polling schedule did
// not limit this measurement (its spacing was at the floor or the state
// queries paced the window) or the window has stopped shrinking. Candidates
// are unsettled during acquisition, including the catch that completes it
// (Settled records the state in which the edge was captured, and acquisition
// succeeds as a consequence of that catch), and unsettled again during
// tracking while a window grown by misses is still shrinking back. Timing
// consumers can accept unsettled candidates with sufficiently small
// Uncertainty, and use Settled to accept the resolution the hardware can
// achieve. A wait or kernel candidate carries the backend timestamp directly,
// has no polling uncertainty, and is always settled.
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

// Detect sends candidate edges for the pulse described by w. An unspecified
// method automatically tries kernel, then wait, then poll, moving on when a
// method is unsupported or unavailable for the device. Other failures are
// returned. An explicitly requested method never falls back. If stats is
// non-nil, it records timings only when polling is selected.
func Detect(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, method gpsio.PPSMethod, ceCh chan<- CandidateEdge, stats *PollStats) error {
	if method != 0 {
		return detect(ctx, lg, r, w, method, ceCh, stats)
	}
	if _, ok := r.(ChangeWaiter); ok {
		for _, m := range []gpsio.PPSMethod{gpsio.PPSMethodKernel, gpsio.PPSMethodWait} {
			err := detect(ctx, lg, r, w, m, ceCh, stats)
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
	return detect(ctx, lg, r, w, gpsio.PPSMethodPoll, ceCh, stats)
}

func detect(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, method gpsio.PPSMethod, ceCh chan<- CandidateEdge, stats *PollStats) error {
	switch method {
	case gpsio.PPSMethodPoll, gpsio.PPSMethodWait, gpsio.PPSMethodKernel:
	default:
		panic("serialpps: invalid PPS method")
	}
	lg.Info("serial PPS method selected", "method", method)
	if method == gpsio.PPSMethodPoll {
		return Poll(ctx, lg, r, w, ceCh, stats)
	}
	cw, ok := r.(ChangeWaiter)
	if !ok {
		return fmt.Errorf("%v PPS method: %w", method, errors.ErrUnsupported)
	}
	return Wait(ctx, lg, cw, w, method, ceCh)
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
