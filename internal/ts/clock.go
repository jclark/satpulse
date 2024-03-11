package ts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/jclark/satpulse/internal/ifwait"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
)

type Clock struct {
	phc.Clock
	eraCounter  atomicEra
	pinIndex    uint32
	chanIndex   uint32
	DriverFlags phc.DriverFlags
}

type Event struct {
	Ts        ptime.ClockTime
	TRead     time.Time
	TReadPHC  ptime.ClockTime
	ChanIndex uint32
	Err       error
}

const exttsTimeout = 100 * time.Microsecond // if we hit this timeout, then the next one isn't stale
const existTimeout = 30 * time.Second
const logWaitTimeout = time.Second / 2 // log if we have to wait more than this for an interface

type PinDesc struct {
	PinIndex  uint32
	ChanIndex uint32
}

func OpenClock(ifName string, pinDesc PinDesc, wait bool, lg *slog.Logger) (*Clock, error) {
	var (
		w   *ifwait.IfWaiter
		err error
	)
	if wait {
		w, err = ifwait.NewIfWaiter(ifName)
		if err != nil {
			return nil, err
		}
		defer w.Close()
		err = waitIface(w, lg, nil, "does not exist", existTimeout)
		if err != nil {
			return nil, err
		}
		err = waitIface(w, lg, func(flags net.Flags) bool { return flags&net.FlagUp != 0 }, "is down", 0)
		if err != nil {
			return nil, err
		}
	}
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return nil, err
	}
	if phcIndex < 0 {
		return nil, fmt.Errorf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)
	}
	flags, err := phc.IfDriverFlags(ifName)
	if err != nil {
		return nil, err
	}
	if w != nil && flags&phc.DriverCarrier != 0 {
		err = waitIface(w, lg, func(flags net.Flags) bool { return flags&net.FlagRunning != 0 }, "has no carrier", 0)
		if err != nil {
			return nil, err
		}
	}
	pc, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, err
	}
	clk := &Clock{Clock: *pc, pinIndex: pinDesc.PinIndex, chanIndex: pinDesc.ChanIndex, DriverFlags: flags}
	// We start off with an era that is certain.
	// Zero era represent stale PHC clock readings.
	clk.eraCounter.inc()
	err = clk.validatePinDesc()
	if err != nil {
		clk.Close()
		return nil, err
	}
	return clk, nil
}

func waitIface(w *ifwait.IfWaiter, lg *slog.Logger, f func(net.Flags) bool, status string, timeout time.Duration) error {
	var timeoutTimer <-chan time.Time
	if timeout != 0 {
		timeoutTimer = time.After(timeout)
	}
	start := time.Now()
	logTimer := time.After(logWaitTimeout)
	ch := w.SetCond(f)
loop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return w.Err()
			}
			break loop
		case <-logTimer:
			lg.Info("interface not ready; waiting", "name", w.Name(), "status", status)
			logTimer = nil
		case <-timeoutTimer:
			return fmt.Errorf("interface %s %s: waited for %s: giving up", w.Name(), status, existTimeout.String())
		}
	}
	if logTimer == nil {
		lg.Info("waited for interface", "name", w.Name(), "t", time.Since(start).String())
	}
	return nil
}

func (clk *Clock) validatePinDesc() error {
	var msg string
	if clk.ExttsChanCount() == 0 || clk.PinCount() == 0 {
		msg = fmt.Sprintf("PTP clock %s does not support external timestamping", clk.Path())
	} else if clk.pinIndex >= uint32(clk.PinCount()) {
		msg = fmt.Sprintf("pin index %d is out of range for PTP clock %s: maximum index is %d", clk.pinIndex, clk.Path(), clk.PinCount()-1)
	} else if clk.chanIndex >= uint32(clk.ExttsChanCount()) {
		msg = fmt.Sprintf("channel index %d is out of range for PTP clock %s: maximum index is %d", clk.chanIndex, clk.Path(), clk.ExttsChanCount()-1)
	} else {
		return nil
	}
	return errors.New(msg)
}

func StartWorker(ctx context.Context, clk *Clock, lg *slog.Logger) (<-chan Event, error) {
	err := clk.PinSetFunc(clk.pinIndex, phc.PinFuncExtts, clk.chanIndex)
	if err != nil {
		return nil, err
	}
	edges, err := clk.ExttsEnable(clk.chanIndex, true)
	if err != nil {
		return nil, err
	}
	lg.Info("enabled external timestamping on the PTP hardware clock", "device", clk.Path(), "pin", clk.pinIndex, "channel", clk.chanIndex, "edgesPerPulse", edges)
	clk.DriverFlags = clk.DriverFlags.SetEdges(edges)
	ch := make(chan Event, 1)
	go clk.readWorker(ctx, lg, ch)
	return ch, nil
}

const StaleEra ptime.Era = ptime.Era(0)

func (clk *Clock) readWorker(ctx context.Context, lg *slog.Logger, tsCh chan<- Event) {
	defer func() {
		_, err := clk.ExttsEnable(clk.chanIndex, false)
		if err != nil {
			lg.Warn("error while disabling external timestamping", "path", clk.Path(), "err", err)
		}
	}()
	defer close(tsCh)
	era := StaleEra
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// The idea is that if we poll and there are no pending events, then any step to the clock
		// that we have made with adjtimex will be in effect for the next read.
		if !clk.ExttsAvailable(exttsTimeout) {
			era = clk.eraCounter.load()
			continue
		}
		event := Event{}
		t, err := clk.ReadExtts()
		if err != nil {
			event.Err = err
		} else {
			tClock := ptime.ClockTime{
				T:   t,
				Era: clk.eraCounter.load(),
			}
			if tClock.Era != era && !tClock.Era.Uncertain() {
				if era.Uncertain() {
					tClock.Era = era
				} else {
					// Make the era uncertain.
					// We cannot be sure that the adjtimex is in effect now.
					// We have to wait for a poll that does not return any events.
					tClock.Era = era + 1
				}
			}
			event.TReadPHC, event.TRead, event.Err = clk.sample()
			event.Ts = tClock
		}
		tsCh <- event
	}
}

// sample reads the PHC and system clocks and returns the results.
// This can be called only from ReadWorker.
func (clk *Clock) sample() (tClock ptime.ClockTime, tSys time.Time, err error) {
	eraPre := clk.eraCounter.load()
	ms, err := clk.SysOffsetExtended(6)
	if err != nil {
		return
	}
	eraPost := clk.eraCounter.load()
	if eraPre == eraPost || eraPre.Uncertain() {
		tClock.Era = eraPre
	} else if eraPost.Uncertain() {
		tClock.Era = eraPost
	} else {
		tClock.Era = eraPre + 1
	}
	tClock.T, tSys = ms.Reduce()
	return
}

func (clk *Clock) AdjTime(d time.Duration) (ptime.Era, error) {
	clk.eraCounter.inc()
	err := clk.Clock.AdjTime(d)
	era := clk.eraCounter.inc()
	if err != nil {
		era = 0
	}
	return era, err
}
