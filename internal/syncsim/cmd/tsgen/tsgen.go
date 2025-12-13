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
		return fmt.Errorf("usage: tsgen <config.toml> <duration-seconds>")
	}
	cfgPath := args[0]
	duration, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return fmt.Errorf("invalid duration: %v", err)
	}
	if duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	// Load config with defaults, then override from TOML
	cfg := syncsim.DefaultConfig()
	if err := syncsim.LoadConfig(cfgPath, &cfg); err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Check which channels to generate
	hasPHC := !cfg.PHC.IsZero()
	hasGPS := !cfg.GPS.IsZero()

	// Create PHC oscillator and RawClock if needed
	var phcClock *clocksim.RawClock
	if hasPHC {
		phcOsc := cfg.PHC.CreateSimulator()
		startPhaseNs := int64(500_000_000) // 0.5 seconds
		phcClock = clocksim.NewRawClock(phcOsc, startPhaseNs)
	}

	// Create GPS simulators if needed
	var nonSawtoothGPS, sawtoothGPS clocksim.GPSSimulator
	if hasGPS {
		nonSawtoothGPS = cfg.GPS.CreateSimulator()

		// Create sawtooth simulator (following VirtualClock pattern)
		if cfg.GPS.Sawtooth.Amp > 0 {
			// Create oscillator for GPS internal clock (for sawtooth coupling)
			// InternalClock defaults come from DefaultSawtoothConfig()
			ic := cfg.GPS.Sawtooth.InternalClock
			gpsOsc := clocksim.SinusoidOsc(
				clocksim.PPB(ic.Amp),
				ic.Period,
				ic.PhaseInit,
			)
			ampSec := cfg.GPS.Sawtooth.Amp * 1e-9
			// PhaseInit defaults to 0.5 from DefaultSawtoothConfig()
			sawtoothGPS = clocksim.SawtoothGPS(gpsOsc, ampSec, cfg.GPS.Sawtooth.PhaseInit)
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
			// qErr is the correction to ADD to remove sawtooth, so negate it
			qErrNs := math.Round(-sawtoothErr*1e9*1000) / 1000

			if err := enc.Encode(struct {
				Chan      string  `json:"chan"`
				Timestamp string  `json:"timestamp"`
				QErr      float64 `json:"qErr"`
			}{
				Chan:      "B",
				Timestamp: formatTimestamp(int64(t), nonSawtoothErr+sawtoothErr),
				QErr:      qErrNs,
			}); err != nil {
				return err
			}
		}

		// Chan A (PHC) - only if PHC parameters configured
		if hasPHC {
			phcPhaseNs := phcClock.ReadAt(t)

			if err := enc.Encode(struct {
				Chan      string `json:"chan"`
				Timestamp string `json:"timestamp"`
			}{
				Chan:      "A",
				Timestamp: formatTimestamp(phcPhaseNs/1e9, float64(phcPhaseNs%1e9)/1e9),
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

// formatTimestamp formats sec + frac as a string with 12 decimal digits.
// sec and frac are kept separate to preserve precision
// (float64 can't reliably represent integers >1000 plus 12 fractional digits).
func formatTimestamp(sec int64, frac float64) string {
	// Normalize frac to [0, 1)
	fracFloor := math.Floor(frac)
	sec += int64(fracFloor)
	frac -= fracFloor

	sign := ""
	var fracInt int64
	if sec < 0 {
		sign = "-"
		sec = -sec
		if frac > 0 {
			sec--
			// Round (1-frac) directly instead of complementing after rounding
			fracInt = int64(math.Round((1 - frac) * 1e12))
		}
	} else {
		fracInt = int64(math.Round(frac * 1e12))
	}
	if fracInt == 1e12 {
		fracInt = 0
		sec++
	}
	return fmt.Sprintf("%s%d.%012d", sign, sec, fracInt)
}
