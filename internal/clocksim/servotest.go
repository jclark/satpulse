//go:build ignore

package main

import (
	"log/slog"
	"math/rand"
	"os"

	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/servo"
)

func main() {
	// Use INFO level logging to see key servo behavior
	lg := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Simulation parameters
	const simDuration = 60.0                // 60 seconds
	const minDeliveryDelay = 5e-6           // 5µs minimum delivery delay
	const maxDeliveryDelay = 0.25           // 250ms maximum delivery delay
	rng := rand.New(rand.NewSource(999))

	// GPS time starts near present (2024-10-08 is roughly GPS time ~1.4e9 seconds)
	gpsStartTime := 1.4e9

	// Create oscillator running 10000ppb fast with some frequency noise
	osc := clocksim.CombineOscillators(
		clocksim.ConstantDrift(10000.0),   // 10000ppb fast
		clocksim.WhiteFreqNoise(1.0, 42),  // 1ppb RMS frequency noise
	)

	// PHC starts near epoch (1970-01-01T00:00:00 TAI) - way off from GPS
	// 0 nanoseconds = epoch
	raw := clocksim.NewRawClock(osc, 0)

	// PPS with realistic 10ns jitter
	pps := clocksim.WhiteNoisePPS(10e-9, 123)

	// Virtual clock starts at t=0, max ±500ppm (like i210)
	vclock := clocksim.NewVirtualClock(raw, pps, 0, 500000)

	// Test clock with era tracking
	testClock := clocksim.NewTestClock(vclock)

	// Create servo
	s, err := servo.New(testClock, lg)
	if err != nil {
		lg.Error("failed to create servo", "err", err)
		os.Exit(1)
	}

	lg.Info("starting servo simulation",
		"duration", simDuration,
		"deliveryDelay", "5µs-250ms",
		"oscDrift", "10000ppb",
		"ppsJitter", "10ns",
		"gpsStartTime", gpsStartTime,
		"phcStartTime", 0.0)

	sampleCount := 0
	nextPPS := 1.0 // First PPS at t=1.0

	for nextPPS < simDuration {
		// Generate random delivery delay (uniform between min and max)
		deliveryDelay := minDeliveryDelay + rng.Float64()*(maxDeliveryDelay-minDeliveryDelay)

		// Advance simulation time to when timestamp is delivered to userspace
		// This is when the servo actually gets called
		simTime := nextPPS + deliveryDelay
		vclock.AdvanceTo(simTime)

		// Read the timestamp that was just delivered
		if !vclock.TimestampAvailable() {
			lg.Error("expected timestamp not available", "nextPPS", nextPPS, "simTime", simTime)
			break
		}

		ts, ok := testClock.ReadTimestampWithEra()
		if !ok {
			lg.Error("failed to read timestamp")
			break
		}

		// GPS time (already converted to TAI) for this PPS
		// PPS occurred at simulation second nextPPS (1.0, 2.0, 3.0, ...)
		// GPS receiver outputs TAI time = gpsStartTime + simulation_second
		gpsTime := ptime.Time((gpsStartTime + nextPPS) * 1e9)

		// Feed sample to servo (happens at simTime = PPS time + delivery delay)
		s.Sample(gpsTime, ts, false)
		sampleCount++

		// Log every sample to see the whole convergence
		lg.Info("sample",
			"num", sampleCount,
			"simTime", simTime,
			"deliveryDelay", deliveryDelay,
			"offset", ts.T.Sub(gpsTime),
			"freq", s.FreqOffset())

		nextPPS += 1.0
	}

	lg.Info("simulation complete",
		"samples", sampleCount,
		"finalFreq", s.FreqOffset())
}
