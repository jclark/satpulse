//go:build ignore
// +build ignore

// This is for experimenting with PTP_SYS_OFFSET_EXTENDED.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jclark/satpulse/time/phc"
)

func main() {
	dev := "/dev/ptp0"
	if len(os.Args) > 1 {
		dev = os.Args[1]
	}
	err := run(dev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

const nSamples = 6
const utcOffset = 37

func run(dev string) error {
	clk, err := phc.Open(dev)
	if err != nil {
		return err
	}
	defer clk.Close()
	samples, err := clk.SysOffsetExtended(6)
	if err != nil {
		return err
	}
	i := samples.Select()
	fmt.Printf("Sample interval: %v\n", samples.SysInterval(i))
	p, sys, _ := samples.Extract(i)
	fmt.Printf("System time: %v\n", sys)
	phcTime := time.Unix(0, int64(p))
	fmt.Printf("TAI time from PHC: %v\n", phcTime)
	off := phcTime.Sub(sys)
	off -= time.Duration(utcOffset) * time.Second
	fmt.Printf("PHC offset (assuming %d leap seconds): %v\n", utcOffset, off)
	return nil
}
