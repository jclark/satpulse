package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jclark/satpulse/internal/phc"
)

type PHCConfig struct {
	Interface string `toml:"interface"`
	Pin       uint8  `toml:"pin"`
	Channel   uint8  `toml:"channel"`
}

const timeout = 100 * time.Microsecond

func openExttsClock(cfg PHCConfig) (*phc.Clock, phc.DriverFlags, error) {
	ifName := cfg.Interface
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
	clk, err := phc.Open(phc.ClockPath(phcIndex))
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

func validateTimePulseConfig(clk *phc.Clock, cfg PHCConfig) error {
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

func StartPPS(ctx context.Context, clk *phc.Clock, cfg PHCConfig) (<-chan phc.TsEvent, int, error) {
	err := clk.PinSetFunc(uint32(cfg.Pin), phc.PinFuncExtts, uint32(cfg.Channel))
	if err != nil {
		return nil, 0, err
	}
	edges, err := clk.ExttsEnable(uint32(cfg.Channel), true)
	if err != nil {
		return nil, 0, err
	}
	c := make(chan phc.TsEvent, 1)
	go func() {
		clk.ReadWorker(ctx.Done(), c, timeout)
	}()
	return c, edges, nil
}
