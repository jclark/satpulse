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

	// Load hardware config without defaults (only what's in TOML file)
	var hw syncsim.HWConfig
	if err := syncsim.LoadHWConfig(hwPath, &hw); err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Check which channels to generate
	hasPHC := !hw.PHC.IsZero()
	hasGPS := !hw.GPS.IsZero()

	// Create PHC oscillator and RawClock if needed
	var phcClock *clocksim.RawClock
	if hasPHC {
		phcOsc := hw.PHC.CreateSimulator()
		startPhaseNs := int64(500_000_000) // 0.5 seconds
		phcClock = clocksim.NewRawClock(phcOsc, startPhaseNs)
	}

	// Create GPS simulators if needed
	var nonSawtoothGPS, sawtoothGPS clocksim.GPSSimulator
	if hasGPS {
		nonSawtoothGPS = hw.GPS.CreateSimulator()

		// Create sawtooth simulator (following VirtualClock pattern)
		if hw.GPS.Sawtooth.Amp > 0 {
			// If internal clock not specified in TOML, use Python's default
			internalClock := hw.GPS.Sawtooth.InternalClock
			if internalClock.Amp == 0 {
				internalClock.Amp = 2.0
				internalClock.Period = 600.0
				internalClock.PhaseInit = 1.0 / 6.0
			}
			// Create oscillator for GPS internal clock (for sawtooth coupling)
			gpsOsc := clocksim.SinusoidOsc(
				internalClock.Amp,
				internalClock.Period,
				internalClock.PhaseInit,
			)
			ampSec := hw.GPS.Sawtooth.Amp * 1e-9
			// Python hardcodes phase_init=0.5 for sawtooth (simulate.py:180)
			phaseInit := hw.GPS.Sawtooth.PhaseInit
			if phaseInit == 0 {
				phaseInit = 0.5
			}
			sawtoothGPS = clocksim.SawtoothGPS(gpsOsc, ampSec, phaseInit)
		} else {
			sawtoothGPS = clocksim.PerfectGPS()
		}
	}

	// Generate timestamps
	enc := json.NewEncoder(os.Stdout)
	for t := 1.0; t <= duration; t += 1.0 {
		// Chan B (GPS) - only if GPS parameters configured
		if hasGPS {
			nonSawtoothErr := nonSawtoothGPS(t)
			sawtoothErr := sawtoothGPS(t)
			gpsTimestamp := t + nonSawtoothErr + sawtoothErr
			qErrNs := math.Round(sawtoothErr*1e9*1000) / 1000

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
		}

		// Chan A (PHC) - only if PHC parameters configured
		if hasPHC {
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
	}

	return nil
}
