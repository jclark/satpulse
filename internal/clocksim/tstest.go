//go:build ignore

package main

import (
	"fmt"

	"github.com/jclark/satpulse/internal/clocksim"
)

func main() {
	// Oscillator with 10000ppb drift + small frequency noise
	osc := clocksim.CombineOscillators(
		clocksim.ConstantDrift(10000.0), // 10000ppb fast
		clocksim.WhiteFreqNoise(10.0, 42), // 10ppb RMS frequency noise
	)

	// PHC starts at ~198510.583 seconds (like in real data)
	baseTimeNs := int64(198510.583865928 * 1e9)
	raw := clocksim.NewRawClock(osc, baseTimeNs)

	// PPS with 10ns jitter
	pps := clocksim.WhiteNoisePPS(10e-9, 123)

	// Virtual clock (not disciplined yet, just raw)
	vclock := clocksim.NewVirtualClock(raw, pps, 0, 500000)

	// Generate 30 timestamps
	fmt.Println("Simulated PHC timestamps:")
	simTime := 0.0
	for i := 0; i < 30; i++ {
		simTime += 1.0
		vclock.AdvanceTo(simTime)

		if vclock.TimestampAvailable() {
			ts, _ := vclock.ReadTimestamp()
			fmt.Printf("%.9f\n", ts.Seconds())
		}
	}
}
