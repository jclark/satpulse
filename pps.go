package main

import (
	"context"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/unix2"
)

const chanIndex = 0
const pinIndex = 0

const timeout = 100 * time.Microsecond

func StartPPS(ctx context.Context, clk *phc.Clock) (<-chan phc.TsEvent, error) {
	err := clk.PinSetfunc(pinIndex, chanIndex, unix2.PTP_PF_EXTTS)
	if err != nil {
		return nil, err
	}
	err = clk.ExttsEnable(chanIndex, true)
	if err != nil {
		return nil, err
	}
	c := make(chan phc.TsEvent, 1)
	go func() {
		clk.ReadWorker(ctx.Done(), c, timeout)
	}()
	return c, nil
}
