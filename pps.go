package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/unix2"
)

const chanIndex = 0
const pinIndex = 0

const timeout = 100 * time.Microsecond

func StartPPS(cx context.Context, clk *phc.Clock) (<-chan phc.TsEvent, error) {
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
		clk.ReadWorker(cx.Done(), c, timeout)
	}()
	return c, nil
}

func SkipStale(clk *phc.Clock, c <-chan phc.TsEvent) error {
	limit := time.After(time.Millisecond * 50)
	nStale := 0
Loop:
	for {
		select {
		case <-limit:
			break Loop
		case <-c:
			nStale++
		}
	}
	fmt.Printf("Skipped %d stale events\n", nStale)
	return clk.ExttsEnable(chanIndex, true)
}
