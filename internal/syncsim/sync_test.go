package syncsim

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jclark/satpulse/internal/phcsync"
)

func TestPHCSync(t *testing.T) {
	// Start with default configs
	phcCfg := phcsync.DefaultConfig()
	simCfg := DefaultConfig()

	// Override for tighter test conditions
	simCfg.Duration = 300.0     // 5 minutes
	simCfg.OscDrift = 0.0       // no drift
	simCfg.OscNoise = 0.1       // 0.1 ppb frequency noise
	simCfg.MsgDelay = 0.08      // 80ms message delay
	simCfg.GPSStartTime = 1.3e9 // ~2017

	// Discard logs during test
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))

	stats, err := Simulate(nil, phcCfg, simCfg, lg)
	if err != nil {
		t.Fatalf("Simulate failed: %v", err)
	}

	// Check that tracking standard deviation is < 20ns
	if stats.TrackingStdDev >= 20 {
		t.Errorf("tracking stddev = %v, want < 20ns", stats.TrackingStdDev)
	}

	t.Logf("Simulation completed: %d samples, tracking stddev = %v", stats.SampleCount, stats.TrackingStdDev)
}
