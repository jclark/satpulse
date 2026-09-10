// Package serialpps detects PPS edges on serial modem-control pins and
// reports them as pps candidate edges.
package serialpps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/pps"
	"github.com/jclark/satpulse/gps/lib/wakeup"
	"github.com/jclark/satpulse/gps/ptime"
)

// StateReader is implemented by a TTY-backed gpsio.SerialConn.
type StateReader interface {
	SerialPinState() (gpsio.SerialPinState, error)
}

// ChangeWaiter is a StateReader that may be able to block until a modem
// control input changes using the wait or kernel method. A backend that
// cannot support the method at all fails with an error wrapping
// errors.ErrUnsupported; a method that exists but is unavailable for the
// particular device or driver fails with an error wrapping
// gpsio.ErrUnavailable. Implemented by gpsio.SerialConn.
type ChangeWaiter interface {
	StateReader
	WaitSerialPinChange(context.Context, gpsio.SerialPin, gpsio.PPSMethod) (gpsio.SerialPinChange, int, error)
}

// Polarity identifies which flag transition is the on-time PPS edge.
//
// In RS-232 signalling, the voltage level that represents logic 0 on a
// data line is the same level that asserts a control line, and drivers
// apply the logic-to-voltage mapping uniformly to data and control
// lines. A control line driven from a logic-level signal is therefore
// asserted at logic 0 and deasserted at logic 1. A PPS pulse is
// active-high, so the flag reads deasserted while the pulse is active,
// and the on-time leading edge is the flag deasserting. This holds both
// for native RS-232 ports and for TTL-to-USB adapters, whose flag
// inputs are likewise active-low. PolarityDeassert names this normal
// convention and is the zero value.
//
// PolarityAssert is for wiring where the pulse instead asserts the
// flag, which shows up as detected edges trailing the start of the
// second by the pulse width (typically 0.1 s).
type Polarity int

const (
	PolarityDeassert Polarity = iota
	PolarityAssert
)

// Asserted reports whether the flag is asserted while the pulse is active.
func (p Polarity) Asserted() bool {
	return p == PolarityAssert
}

// Wiring describes how the PPS pulse is represented on the serial port's
// modem-control inputs.
type Wiring struct {
	Pin      gpsio.SerialPin
	Polarity Polarity
}

// Detect sends candidate edges for the pulse described by w. An unspecified
// method automatically tries kernel, then wait, then poll, moving on when a
// method is unsupported or unavailable for the device. Other failures are
// returned. An explicitly requested method never falls back. cfg.PollPreWarm
// applies only to polling, the one method whose resolution the host's own
// speed sets. If stats is non-nil, it records timings only when polling is
// selected. cfg.MaxWakeupLatency, if set, limits CPU wakeup latency for as
// long as detection runs.
func Detect(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, cfg Config, ceCh chan<- pps.CandidateEdge, stats *pps.PollStats) error {
	if cfg.MaxWakeupLatency != nil {
		max := ptime.Seconds(*cfg.MaxWakeupLatency)
		if req, err := wakeup.RequestLatencyLimit(max); err != nil {
			lg.Warn("cannot limit CPU wakeup latency", "max", max, "err", err)
		} else {
			lg.Info("limited CPU wakeup latency", "max", max.Truncate(wakeup.LatencyResolution))
			defer func() {
				if err := req.Close(); err != nil {
					lg.Warn("cannot release CPU wakeup latency limit", "err", err)
				}
			}()
		}
	}
	prewarm := ptime.Seconds(cfg.PollPreWarm)
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

// pinReader reads a modem-control input as a pulse state for pps.Poll.
type pinReader struct {
	r StateReader
	w Wiring
}

// InPulse reports whether the pin was observed during a pulse, as selected
// by the wiring's polarity.
func (p pinReader) InPulse() (bool, error) {
	s, err := p.r.SerialPinState()
	return s.Asserted(p.w.Pin) == p.w.Polarity.Asserted(), err
}

func detect(ctx context.Context, lg *slog.Logger, r StateReader, w Wiring, method gpsio.PPSMethod, prewarm time.Duration, ceCh chan<- pps.CandidateEdge, stats *pps.PollStats) error {
	switch method {
	case gpsio.PPSMethodPoll, gpsio.PPSMethodWait, gpsio.PPSMethodKernel:
	default:
		panic("serialpps: invalid PPS method")
	}
	lg.Info("serial PPS method selected", "method", method)
	if method == gpsio.PPSMethodPoll {
		return pps.Poll(ctx, lg, pinReader{r, w}, pps.PollParams{PreWarm: prewarm}, ceCh, stats)
	}
	cw, ok := r.(ChangeWaiter)
	if !ok {
		return fmt.Errorf("%v PPS method: %w", method, errors.ErrUnsupported)
	}
	return Wait(ctx, lg, cw, w, method, ceCh)
}

// Wait sends settled candidate edges from modem-control change notifications,
// using the wait or kernel method. The backend timestamps each unambiguous
// transition, so these candidates have no polling uncertainty. The leading
// edge is the transition into the pulse, as selected by w.Polarity.
func Wait(ctx context.Context, lg *slog.Logger, r ChangeWaiter, w Wiring, method gpsio.PPSMethod, ceCh chan<- pps.CandidateEdge) error {
	for {
		change, missed, err := r.WaitSerialPinChange(ctx, w.Pin, method)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if missed > 0 {
			lg.Debug("serial PPS transitions not observed", "atLeast", missed)
		}
		if change.Asserted == w.Polarity.Asserted() {
			ce := pps.CandidateEdge{
				Edge:    pps.Edge{Timestamp: change.Timestamp, TRead: change.TRead},
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
