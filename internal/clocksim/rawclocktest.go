//go:build ignore

package main

import (
	"fmt"

	"github.com/jclark/satpulse/internal/clocksim"
)

func main() {
	// Create oscillator running 10000ppb fast with white frequency noise
	osc := clocksim.CombineOscillators(
		clocksim.ConstantDrift(10000.0), // 10000ppb drift
		clocksim.WhiteFreqNoise(10.0, 42), // 10ppb RMS frequency noise
	)
	raw := clocksim.NewRawClock(osc, 0) // Start at epoch = 0 nanoseconds

	fmt.Println("Time(s)\tRawClock(s)\tDrift(s)\tJitter(ns)")
	fmt.Println("-------\t-----------\t--------\t----------")

	for t := 0.0; t <= 10.0; t += 1.0 {
		phaseNs := raw.ReadAt(t)
		phaseSec := float64(phaseNs) / 1e9
		// Expected with just drift: t * (1 + 10e-6)
		expectedDrift := t * (1 + 10e-6)
		// Jitter is the difference from the pure drift
		jitterNs := (phaseSec - expectedDrift) * 1e9

		fmt.Printf("%.1f\t%.9f\t%.9f\t%+.1f\n", t, phaseSec, expectedDrift, jitterNs)
	}
}
