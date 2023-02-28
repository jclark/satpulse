package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jclark/gps2phc/phc"
)

type TimePulseConfig struct {
	Interface string
	Pin       uint8
	Channel   uint8
}

const timeout = 100 * time.Microsecond

func openExttsClock(cfg TimePulseConfig) (*phc.Clock, error) {
	ifName := cfg.Interface
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return nil, err
	}
	if phcIndex < 0 {
		return nil, fmt.Errorf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)
	}
	clk, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, err
	}
	err = validateTimePulseConfig(clk, cfg)
	if err != nil {
		clk.Close()
		return nil, err
	}
	return clk, nil
}

func validateTimePulseConfig(clk *phc.Clock, cfg TimePulseConfig) error {
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

func StartPPS(ctx context.Context, clk *phc.Clock, cfg TimePulseConfig) (<-chan phc.TsEvent, error) {
	err := clk.PinSetFunc(uint32(cfg.Pin), phc.PinFuncExtts, uint32(cfg.Channel))
	if err != nil {
		return nil, err
	}
	err = clk.ExttsEnable(uint32(cfg.Pin), true)
	if err != nil {
		return nil, err
	}
	c := make(chan phc.TsEvent, 1)
	go func() {
		clk.ReadWorker(ctx.Done(), c, timeout)
	}()
	return c, nil
}
