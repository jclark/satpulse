//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/clocksim"
)

func main() {
	pulseWidthSec := flag.Float64("w", 0, "pulse width in seconds (0 for single-edge, must be >0 and <1 for dual-edge)")
	flag.Parse()

	if *pulseWidthSec < 0 || *pulseWidthSec >= 1 {
		fmt.Fprintf(os.Stderr, "Error: pulse width must be >= 0 and < 1\n")
		os.Exit(1)
	}

	// Oscillator with 10000ppb drift + small frequency noise
	osc := clocksim.CombineOscillators(
		clocksim.ConstantDrift(10000.0),   // 10000ppb fast
		clocksim.WhiteFreqNoise(10.0, 42), // 10ppb RMS frequency noise
	)

	// PHC starts at ~198510.583 seconds (like in real data)
	baseTimeNs := int64(198510.583865928 * 1e9)
	raw := clocksim.NewRawClock(osc, baseTimeNs)

	// PPS with 10ns jitter
	pps := clocksim.WhiteNoisePPS(10*time.Nanosecond, 123)

	// Prepare dual-edge mode parameters
	var pulseWidth time.Duration
	var trailingEdge clocksim.PPSSimulator
	if *pulseWidthSec > 0 {
		pulseWidth = time.Duration(*pulseWidthSec * 1e9)
		// Trailing edge simulator with small noise (2ns)
		trailingEdge = clocksim.WhiteNoisePPS(10*time.Nanosecond, 456)
		fmt.Printf("Dual-edge mode: pulse width = %.3fs\n", *pulseWidthSec)
	} else {
		fmt.Println("Single-edge mode")
	}

	// Virtual clock
	vclock := clocksim.NewVirtualClock(raw, pps, 0, 500000, pulseWidth, trailingEdge)

	// Generate 30 timestamps
	fmt.Println("Simulated PHC timestamps:")
	simTime := 0.0
	for i := 0; i < 30; i++ {
		simTime += 1.0
		vclock.AdvanceTo(simTime)

		for vclock.TimestampAvailable() {
			ts, _, _ := vclock.ReadTimestamp()
			fmt.Printf("%.9f\n", ts.Seconds())
		}
	}
}
