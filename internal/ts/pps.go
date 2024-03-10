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
)

const exttsTimeout = 100 * time.Microsecond // if we hit this timeout, then the next one isn't stale
const existTimeout = 30 * time.Second
const logWaitTimeout = time.Second / 2 // log if we have to wait more than this for an interface

type PinDesc struct {
	PinIndex  uint32
	ChanIndex uint32
}

func OpenClock(ifName string, pinDesc PinDesc, wait bool, lg *slog.Logger) (*Clock, phc.DriverFlags, error) {
	var (
		w   *ifwait.IfWaiter
		err error
	)
	if wait {
		w, err = ifwait.NewIfWaiter(ifName)
		if err != nil {
			return nil, 0, err
		}
		defer w.Close()
		err = waitIface(w, lg, nil, "does not exist", existTimeout)
		if err != nil {
			return nil, 0, err
		}
		err = waitIface(w, lg, func(flags net.Flags) bool { return flags&net.FlagUp != 0 }, "is down", 0)
		if err != nil {
			return nil, 0, err
		}
	}
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return nil, 0, err
	}
	if phcIndex < 0 {
		return nil, 0, fmt.Errorf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)
	}
	flags, err := phc.IfDriverFlags(ifName)
	if err != nil {
		return nil, 0, err
	}
	if w != nil && flags&phc.DriverCarrier != 0 {
		err = waitIface(w, lg, func(flags net.Flags) bool { return flags&net.FlagRunning != 0 }, "has no carrier", 0)
		if err != nil {
			return nil, 0, err
		}
	}
	pc, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, 0, err
	}
	clk := &Clock{Clock: *pc, pinIndex: pinDesc.PinIndex, chanIndex: pinDesc.ChanIndex}
	// We start off with an era that is certain.
	// Zero era represent stale PHC clock readings.
	clk.eraCounter.inc()
	err = clk.validatePinDesc()
	if err != nil {
		clk.Close()
		return nil, 0, err
	}
	return clk, flags, nil
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

func StartWorker(ctx context.Context, clk *Clock, lg *slog.Logger) (<-chan Event, int, error) {
	err := clk.PinSetFunc(clk.pinIndex, phc.PinFuncExtts, clk.chanIndex)
	if err != nil {
		return nil, 0, err
	}
	edges, err := clk.ExttsEnable(clk.chanIndex, true)
	if err != nil {
		return nil, 0, err
	}
	lg.Info("enabled external timestamping on the PTP hardware clock", "device", clk.Path(), "pin", clk.pinIndex, "channel", clk.chanIndex, "edgesPerPulse", edges)
	c := make(chan Event, 1)
	go func() {
		clk.ReadWorker(ctx.Done(), c, exttsTimeout)
		_, err := clk.ExttsEnable(clk.chanIndex, false)
		if err != nil {
			lg.Warn("error while disabling external timestamping", "path", clk.Path(), "err", err)
		}
	}()
	return c, edges, nil
}
