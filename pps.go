package main

import (
	"context"
	"time"

	"github.com/jclark/gps2phc/phc"
)

const timeout = 100 * time.Microsecond

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
