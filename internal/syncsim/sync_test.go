package syncsim

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/phcsync"
)

func TestPHCSync(t *testing.T) {
	tests := []struct {
		name             string
		pulseWidth       float64       // 0 for single-edge mode
		duration         float64       // simulation duration in seconds
		maxTrackingStdDev time.Duration // maximum acceptable tracking stddev
	}{
		{
			name:              "single-edge mode",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20 * time.Nanosecond,
		},
		{
			name:              "dual-edge 100ms pulse",
			pulseWidth:        0.1,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20 * time.Nanosecond,
		},
		{
			name:              "dual-edge 200ms pulse",
			pulseWidth:        0.2,
			duration:          600.0, // 10 minutes (needs longer to converge)
			maxTrackingStdDev: 20 * time.Nanosecond,
		},
		{
			name:              "dual-edge 10ms pulse",
			pulseWidth:        0.01,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20 * time.Nanosecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with default configs
			phcCfg := phcsync.DefaultConfig()
			simCfg := DefaultConfig()

			// Override for tighter test conditions
			simCfg.Duration = tt.duration
			simCfg.OscDrift = 0.0       // no drift
			simCfg.OscNoise = 0.1       // 0.1 ppb frequency noise
			simCfg.MsgDelay = 0.08      // 80ms message delay
			simCfg.GPSStartTime = 1.3e9 // ~2017
			simCfg.PulseWidth = tt.pulseWidth

			// Discard logs during test
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))

			stats, err := Simulate(nil, phcCfg, simCfg, lg)
			if err != nil {
				t.Fatalf("Simulate failed: %v", err)
			}

			// Check that tracking standard deviation is acceptable
			if stats.TrackingStdDev >= tt.maxTrackingStdDev {
				t.Errorf("tracking stddev = %v, want < %v", stats.TrackingStdDev, tt.maxTrackingStdDev)
			}

			t.Logf("Simulation completed: %d samples, tracking stddev = %v", stats.SampleCount, stats.TrackingStdDev)
		})
	}
}
