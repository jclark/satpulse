package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/jclark/satpulse/internal/ifwait"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ts"
)

type PHCConfig struct {
	Interface string `toml:"interface"`
	Pin       uint8  `toml:"pin"`
	Channel   uint8  `toml:"channel"`
	Wait      bool   `toml:"wait"`
}

const exttsTimeout = 100 * time.Microsecond // if we hit this timeout, then the next one isn't stale
const existTimeout = 30 * time.Second
const logWaitTimeout = time.Second / 2 // log if we have to wait more than this for an interface

func openExttsClock(lg *slog.Logger, cfg PHCConfig) (*ts.Clock, phc.DriverFlags, error) {
	ifName := cfg.Interface
	var (
		w   *ifwait.IfWaiter
		err error
	)
	if cfg.Wait {
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
	clk, err := ts.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, 0, err
	}
	err = validateTimePulseConfig(clk, cfg)
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

func validateTimePulseConfig(clk *ts.Clock, cfg PHCConfig) error {
	var msg string
	if clk.ExttsChanCount() == 0 || clk.PinCount() == 0 {
		msg = fmt.Sprintf("PTP clock %s does not support external timestamping", clk.Path())
	} else if int(cfg.Pin) >= clk.PinCount() {
		msg = fmt.Sprintf("pin index %d is out of range for PTP clock %s: maximum index is %d", cfg.Pin, clk.Path(), clk.PinCount()-1)
	} else if int(cfg.Channel) >= clk.ExttsChanCount() {
		msg = fmt.Sprintf("channel index %d is out of range for PTP clock %s: maximum index is %d", cfg.Channel, clk.Path(), clk.ExttsChanCount()-1)
	} else {
		return nil
	}
	return errors.New(msg)
}

func StartPPS(ctx context.Context, clk *ts.Clock, cfg PHCConfig, lg *slog.Logger) (<-chan ts.Event, int, error) {
	err := clk.PinSetFunc(uint32(cfg.Pin), phc.PinFuncExtts, uint32(cfg.Channel))
	if err != nil {
		return nil, 0, err
	}
	edges, err := clk.ExttsEnable(uint32(cfg.Channel), true)
	if err != nil {
		return nil, 0, err
	}
	lg.Info("enabled external timestamping on the PTP hardware clock", "device", clk.Path(), "pin", cfg.Pin, "channel", cfg.Channel, "edgesPerPulse", edges)
	c := make(chan ts.Event, 1)
	go func() {
		clk.ReadWorker(ctx.Done(), c, exttsTimeout)
		_, err := clk.ExttsEnable(uint32(cfg.Channel), false)
		if err != nil {
			lg.Warn("error while disabling external timestamping", "path", clk.Path(), "err", err)
		}
	}()
	return c, edges, nil
}
