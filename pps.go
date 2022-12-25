package main

import (
	"fmt"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/unix2"
)

const chanIndex = 0
const pinIndex = 0

func StartPPS(clk *phc.Clock) (<-chan phc.TsEvent, error) {
	err := clk.PinSetfunc(pinIndex, chanIndex, unix2.PTP_PF_EXTTS)
	if err != nil {
		return nil, err
	}
	err = clk.ExttsEnable(chanIndex, false)
	if err != nil {
		return nil, err
	}
	clk.ReadTsEvents(true)
	c := clk.TsChan()
	limit := time.After(time.Millisecond * 50)
	timedOut := false
	nStale := 0
	for !timedOut {
		select {
		case <-limit:
			timedOut = true
		case <-c:
			nStale++
		}
	}
	fmt.Printf("Skipped %d stale events\n", nStale)
	err = clk.ExttsEnable(chanIndex, true)
	if err != nil {
		return nil, err
	}
	return c, nil
}
