//go:build ignore

// Demonstrates ReconcileTimes against the real clock. It reads time.Now four
// times in a row, reconciles the readings, and reports every group where the
// wall clock had to be corrected by more than the threshold. A group is only
// corrected when one of its four calls was interrupted between reading
// CLOCK_REALTIME and reading CLOCK_MONOTONIC, leaving that wall reading early;
// the correction is how far it was out, and the other three readings stay put.
// The rate depends on the clocksource and on how much the host is interrupted,
// so it is worth running on a new machine before trusting microsecond
// timestamps from it.
//
// Run from the repository root with:
//
//	go run ./gps/app/pps/reconciledemo.go [groups [threshold]]
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jclark/satpulse/gps/app/pps"
)

func main() {
	groups, threshold := 1000000, time.Microsecond
	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil || n < 1 {
			usage()
		}
		groups = n
	}
	if len(os.Args) > 2 {
		d, err := time.ParseDuration(os.Args[2])
		if err != nil || d <= 0 {
			usage()
		}
		threshold = d
	}
	ts := make([]time.Time, 4)
	corrections := make([]time.Duration, 4)
	corrected := 0
	for g := range groups {
		for i := range ts {
			ts[i] = time.Now()
		}
		var worst time.Duration
		for i, v := range pps.ReconcileTimes(ts, ts) {
			corrections[i] = v.Sub(ts[i].Round(0))
			if corrections[i] > worst {
				worst = corrections[i]
			}
		}
		if worst > threshold {
			corrected++
			fmt.Printf("group %d: wall clock corrected by %v, correction per reading %v\n", g, worst, corrections)
		}
	}
	fmt.Printf("%d of %d groups corrected by more than %v (%.6f%%)\n",
		corrected, groups, threshold, 100*float64(corrected)/float64(groups))
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: reconciledemo [groups [threshold]]")
	os.Exit(2)
}
