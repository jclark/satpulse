package syncsim

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phcsync"
)

// modeTracker counts not-in-sync samples after achieving first sync
type modeTracker struct {
	obs.DefaultObserver
	sawInSync         bool
	notInSyncCount    int
}

func (m *modeTracker) Sample(s phcsync.SampleData) {
	isInSync := s.SyncState == phcsync.InSync
	if isInSync {
		m.sawInSync = true
	} else if m.sawInSync {
		m.notInSyncCount++
	}
}

func TestPHCSync(t *testing.T) {
	tests := []struct {
		name                   string
		pulseWidth             float64       // 0 for single-edge mode
		duration               float64       // simulation duration in seconds
		maxTrackingStdDev      time.Duration // maximum acceptable tracking stddev
		toggleTimes            []float64     // absolute times to toggle pulse delivery
		expectNotInSync        int           // expected number of not-in-sync samples after first sync
		expectTrackingSamples  int           // expected number of samples in tracking mode
		expectResetSamples     int           // expected number of samples in reset mode (includes initial sync and recovery)
		expectConvergingSamples int          // expected number of samples in converging mode (0 = don't check)
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
			name:                  "enters reset mode on permanent outage",
			pulseWidth:            0,
			duration:              80.0,
			toggleTimes:           []float64{60.0}, // stop at t=60s, never restart
			expectNotInSync:       5,               // 5 missing samples in tracking mode before transition to reset
			expectTrackingSamples: 40,              // 35 before outage + 5 missing samples in tracking before transition
			expectResetSamples:    1,               // 1 initial reset sample only (reset mode doesn't generate missing samples)
		},
		{
			name:              "recovers from temporary outage",
			pulseWidth:        0,
			duration:          120.0,
			toggleTimes:       []float64{60.0, 70.0}, // stop at t=60s, restart at t=70s
			maxTrackingStdDev: 30 * time.Nanosecond,  // slightly higher tolerance due to recovery transient
			expectNotInSync:   21,                    // 5 missing in tracking + ~16 in converging/reset during recovery
			// After recovery: reset→converging→tracking for remaining ~50 seconds
		},
		{
			name:                   "signal loss during converging mode",
			pulseWidth:             0,
			duration:               40.0,
			toggleTimes:            []float64{13.0, 23.0}, // stop at t=13s during converging, restart at t=23s
			expectTrackingSamples:  0,                     // never reaches tracking
			expectResetSamples:     2,                     // 1 initial + 1 after recovery from converging loss
			expectConvergingSamples: 22,                    // 7 good + 3 missing + 12 good in second phase
			// Expected flow:
			// - Reset #1 at t=6s (1 sample)
			// - Converging from t=6s to t=12s (7 good samples)
			// - Missing samples at t=13.5s, 14.5s, 15.5s (3 missing samples in converging)
			// - Transition to reset at t=15.5s after 3rd missing sample
			// - Signal restarts at t=23s
			// - Reset #2 at t=27s (1 sample)
			// - Converging from t=28s to t=40s (12 good samples)
			// Total converging: 7 + 3 + 12 = 22 samples
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with default configs
			phcCfg := phcsync.DefaultConfig()
			simCfg := DefaultConfig()

			// Override for tighter test conditions
			simCfg.Duration = tt.duration
			simCfg.PHCFreqOffset = 0.0  // no offset
			simCfg.PHCFreqDrift = 0.0   // no drift
			simCfg.PHCNoise = 0.1       // 0.1 ppb frequency noise
			simCfg.MsgDelay = 0.08      // 80ms message delay
			simCfg.PulseWidth = tt.pulseWidth
			simCfg.ToggleTimes = tt.toggleTimes

			curTime := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)

			// Discard logs during test
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))

			// Create mode tracker observer if we expect not-in-sync samples
			var tracker *modeTracker
			var observers []obs.Observer
			if tt.expectNotInSync > 0 {
				tracker = &modeTracker{}
				observers = []obs.Observer{tracker}
			}

			stats, err := Simulate(observers, phcCfg, simCfg, &curTime, lg)
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
			if tt.expectResetSamples > 0 {
				if stats.InitSamples != tt.expectResetSamples {
					t.Errorf("InitSamples (reset mode) = %d, want %d", stats.InitSamples, tt.expectResetSamples)
				}
			}
			if tt.expectConvergingSamples > 0 {
				if stats.ConvergingSamples != tt.expectConvergingSamples {
					t.Errorf("ConvergingSamples = %d, want %d", stats.ConvergingSamples, tt.expectConvergingSamples)
				}
			}

			t.Logf("Simulation completed: %d samples (reset=%d, converging=%d, tracking=%d), tracking stddev = %v",
				stats.SampleCount, stats.InitSamples, stats.ConvergingSamples, stats.TrackingSamples, stats.TrackingStdDev)
		})
	}
}
