package syncsim

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phcsync"
)

// modeTracker counts not-in-sync samples after achieving first sync
type modeTracker struct {
	obs.DefaultObserver
	sawInSync         bool
	notInSyncCount    int
}

func (m *modeTracker) Sample(s mon.SampleData) {
	isInSync := s.SyncState == mon.InSync
	if isInSync {
		m.sawInSync = true
	} else if m.sawInSync {
		m.notInSyncCount++
	}
}

func TestPHCSync(t *testing.T) {
	tests := []struct {
		name                 string
		pulseWidth           float64       // 0 for single-edge mode
		duration             float64       // simulation duration in seconds
		maxTrackingStdDev    time.Duration // maximum acceptable tracking stddev
		toggleTimes          []float64     // absolute times to toggle pulse delivery
		expectNotInSync      int           // expected number of not-in-sync samples after first sync
		expectTrackingSamples int          // expected number of samples in tracking mode
		expectLostSamples    int           // expected number of samples in lost mode
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
		{
			name:        "signal loss - no recovery",
			pulseWidth:  0,
			duration:    90.0,
			toggleTimes: []float64{60.0}, // stop at t=60s, never restart
			// Don't check maxTrackingStdDev since we lose sync
		},
		{
			name:                  "enters lost mode on permanent outage",
			pulseWidth:            0,
			duration:              80.0,
			toggleTimes:           []float64{60.0}, // stop at t=60s, never restart
			expectNotInSync:       20,              // 20s of outage = 20 missing samples that should be NoSync
			expectTrackingSamples: 46,              // 41 before outage + 5 missing samples in tracking before transition
			expectLostSamples:     15,              // remaining 15 missing samples in lost mode
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
			simCfg.ToggleTimes = tt.toggleTimes

			// Discard logs during test
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))

			// Create mode tracker observer if we expect not-in-sync samples
			var tracker *modeTracker
			var observers []obs.Observer
			if tt.expectNotInSync > 0 {
				tracker = &modeTracker{}
				observers = []obs.Observer{tracker}
			}

			stats, err := Simulate(observers, phcCfg, simCfg, lg)
			if err != nil {
				t.Fatalf("Simulate failed: %v", err)
			}

			// Check that tracking standard deviation is acceptable (skip if toggleTimes is set)
			if tt.maxTrackingStdDev > 0 && stats.TrackingStdDev >= tt.maxTrackingStdDev {
				t.Errorf("tracking stddev = %v, want < %v", stats.TrackingStdDev, tt.maxTrackingStdDev)
			}

			// Check not-in-sync samples after first sync
			if tt.expectNotInSync > 0 {
				if tracker == nil {
					t.Fatal("tracker is nil but expectNotInSync > 0")
				}
				if tracker.notInSyncCount != tt.expectNotInSync {
					t.Errorf("notInSyncCount = %d, want %d", tracker.notInSyncCount, tt.expectNotInSync)
				} else {
					t.Logf("Not-in-sync sample check passed: notInSyncCount = %d", tracker.notInSyncCount)
				}
			}

			// Check mode-based sample counts
			if tt.expectTrackingSamples > 0 {
				if stats.TrackingSamples != tt.expectTrackingSamples {
					t.Errorf("TrackingSamples = %d, want %d", stats.TrackingSamples, tt.expectTrackingSamples)
				}
			}
			if tt.expectLostSamples > 0 {
				if stats.LostSamples != tt.expectLostSamples {
					t.Errorf("LostSamples = %d, want %d", stats.LostSamples, tt.expectLostSamples)
				}
			}

			t.Logf("Simulation completed: %d samples (init=%d, converging=%d, tracking=%d, lost=%d), tracking stddev = %v",
				stats.SampleCount, stats.InitSamples, stats.ConvergingSamples, stats.TrackingSamples, stats.LostSamples, stats.TrackingStdDev)
		})
	}
}
