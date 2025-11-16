package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/syncsim"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: tsgen <hw-config.toml> <duration-seconds>")
	}
	hwPath := args[0]
	duration, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return fmt.Errorf("invalid duration: %v", err)
	}
	if duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	// Load hardware config
	hw, err := syncsim.LoadHWConfig(hwPath)
	if err != nil {
		return fmt.Errorf("failed to load hardware config: %v", err)
	}

	// Create PHC oscillator and RawClock
	phcOsc := hw.PHC.CreateSimulator()
	startPhaseNs := int64(500_000_000) // 0.5 seconds
	phcClock := clocksim.NewRawClock(phcOsc, startPhaseNs)

	// Create GPS simulators
	nonSawtoothGPS := hw.GPS.CreateSimulator()

	// Create sawtooth simulator (following VirtualClock pattern)
	var sawtoothGPS clocksim.GPSSimulator
	if hw.GPS.Sawtooth.Amp > 0 {
		// Create oscillator for GPS internal clock (for sawtooth coupling)
		gpsOsc := clocksim.SinusoidOsc(
			hw.GPS.Sawtooth.InternalClock.Amp,
			hw.GPS.Sawtooth.InternalClock.Period,
			hw.GPS.Sawtooth.InternalClock.PhaseInit,
		)
		ampSec := hw.GPS.Sawtooth.Amp * 1e-9
		sawtoothGPS = clocksim.SawtoothGPS(gpsOsc, ampSec, hw.GPS.Sawtooth.PhaseInit)
	} else {
		sawtoothGPS = clocksim.PerfectGPS()
	}

	// Generate timestamps
	enc := json.NewEncoder(os.Stdout)
	for t := 1.0; t <= duration; t += 1.0 {
		// Chan B (GPS)
		nonSawtoothErr := nonSawtoothGPS(t)
		sawtoothErr := sawtoothGPS(t)
		gpsTimestamp := t + nonSawtoothErr + sawtoothErr
		qErrNs := math.Round(sawtoothErr*1e9*1000) / 1000

		// Use struct to control JSON field order
		if err := enc.Encode(struct {
			Chan      string  `json:"chan"`
			Timestamp float64 `json:"timestamp"`
			QErr      float64 `json:"qErr"`
		}{
			Chan:      "B",
			Timestamp: gpsTimestamp,
			QErr:      qErrNs,
		}); err != nil {
			return err
		}

		// Chan A (PHC)
		phcPhaseNs := phcClock.ReadAt(t)
		phcTimestamp := float64(phcPhaseNs) / 1e9

		if err := enc.Encode(struct {
			Chan      string  `json:"chan"`
			Timestamp float64 `json:"timestamp"`
		}{
			Chan:      "A",
			Timestamp: phcTimestamp,
		}); err != nil {
			return err
		}
	}

	return nil
}
