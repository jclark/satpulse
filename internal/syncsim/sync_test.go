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
		name                     string
		pulseWidth               float64                  // 0 for single-edge mode
		duration                 float64                  // simulation duration in seconds
		maxTrackingStdDev        time.Duration            // maximum acceptable tracking stddev
		maxTrackingAbsMax        time.Duration            // maximum acceptable tracking absolute max (0 = don't check)
		toggleTimes              []float64                // absolute times to toggle pulse delivery
		expectMinTrackingSamples int                      // minimum acceptable tracking samples (0 = don't check)
		expectMaxTrackingSamples int                      // maximum acceptable tracking samples (0 = don't check)
		expectTrackingSamples    int                      // expected exact number of samples in tracking mode (0 = don't check)
		expectResetSamples       int                      // expected number of samples in reset mode (includes initial sync and recovery)
		expectConvergingSamples  int                      // expected number of samples in converging mode (0 = don't check)
		modifySimCfg             func(*Config)            // optional function to modify simCfg (nil = use defaults)
		modifyPHCCfg             func(*phcsync.Config)    // optional function to modify phcCfg (nil = use defaults)
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
			name:                     "enters reset mode on permanent outage",
			pulseWidth:               0,
			duration:                 80.0,
			toggleTimes:              []float64{60.0}, // stop at t=60s, never restart
			expectMaxTrackingSamples: 40,              // 35 before outage + up to 5 missing samples in tracking before transition
			expectResetSamples:       1,               // 1 initial reset sample only (reset mode doesn't generate missing samples)
		},
		{
			name:                     "recovers from temporary outage",
			pulseWidth:               0,
			duration:                 120.0,
			toggleTimes:              []float64{60.0, 70.0}, // stop at t=60s, restart at t=70s
			maxTrackingStdDev:        30 * time.Nanosecond,  // slightly higher tolerance due to recovery transient
			expectResetSamples:       2,                     // 1 initial + 1 after recovery
			expectMinTrackingSamples: 35,                    // At least 35 before outage (plus time after recovery)
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
		{
			name:                     "EMA feature reduces drift during brief outages",
			pulseWidth:               0,
			duration:                 120.0,
			toggleTimes:              []float64{60.0, 63.0}, // 3 second outage (< badSampleLimit of 5)
			maxTrackingStdDev:        15 * time.Nanosecond,
			maxTrackingAbsMax:        40 * time.Nanosecond, // With EMA enabled, absmax ~24ns (vs 58ns without)
			expectMinTrackingSamples: 90,                   // At least 90 tracking samples despite outage
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.AvgFreqTimeConstant = 30 // Enable EMA with 30s time constant
			},
		},
		{
			name:                     "Without EMA drift is worse during brief outages",
			pulseWidth:               0,
			duration:                 120.0,
			toggleTimes:              []float64{60.0, 63.0}, // 3 second outage (< badSampleLimit of 5)
			maxTrackingStdDev:        18 * time.Nanosecond,
			maxTrackingAbsMax:        80 * time.Nanosecond, // Without EMA, absmax ~58ns (2x worse than with EMA)
			expectMinTrackingSamples: 90,                   // At least 90 tracking samples despite outage
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.AvgFreqTimeConstant = 0 // Disable EMA feature
			},
		},
		{
			name:                     "EMA feature with longer outage (increased badSampleLimit)",
			pulseWidth:               0,
			duration:                 180.0,
			toggleTimes:              []float64{90.0, 99.0}, // 9 second outage (< badSampleLimit of 15)
			maxTrackingStdDev:        12 * time.Nanosecond,
			maxTrackingAbsMax:        35 * time.Nanosecond, // With EMA, absmax ~23ns even with 9s outage
			expectMinTrackingSamples: 150,                  // At least 150 tracking samples despite longer outage
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.AvgFreqTimeConstant = 30 // Enable EMA with 30s time constant
				cfg.Track.BadSampleLimit = 15      // Increase limit to allow longer outage
			},
		},
		{
			name:                     "Without EMA longer outage shows severe drift (increased badSampleLimit)",
			pulseWidth:               0,
			duration:                 180.0,
			toggleTimes:              []float64{90.0, 99.0}, // 9 second outage (< badSampleLimit of 15)
			maxTrackingStdDev:        30 * time.Nanosecond,
			maxTrackingAbsMax:        180 * time.Nanosecond, // Without EMA, absmax ~139ns (5-6x worse than with EMA)
			expectMinTrackingSamples: 150,                   // At least 150 tracking samples despite longer outage
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.AvgFreqTimeConstant = 0 // Disable EMA feature
				cfg.Track.BadSampleLimit = 15     // Increase limit to allow longer outage
			},
		},
		{
			name:              "MAD detects scheduled outliers",
			pulseWidth:        0,
			duration:          90.0, // Longer duration to ensure we're in tracking mode
			maxTrackingStdDev: 20 * time.Nanosecond, // Should maintain low stddev despite outliers
			modifySimCfg: func(cfg *Config) {
				cfg.Outlier = OutlierConfig{
					Times:  []float64{40, 50, 60},
					Offset: 2000 * time.Nanosecond,
				}
			},
		},
		{
			name:              "MAD gate behavior before window fills",
			pulseWidth:        0,
			duration:          20.0,
			maxTrackingStdDev: 20 * time.Nanosecond,
			modifySimCfg: func(cfg *Config) {
				cfg.Outlier = OutlierConfig{
					Times:  []float64{8}, // Outlier during early tracking (MAD window not full)
					Offset: 150 * time.Nanosecond,
				}
			},
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.OutlierThreshold = 200 // 200ns gate - 150ns outlier should pass through
			},
		},
		{
			name:              "Large outliers rejected despite MAD window",
			pulseWidth:        0,
			duration:          60.0,
			maxTrackingStdDev: 20 * time.Nanosecond,
			modifySimCfg: func(cfg *Config) {
				cfg.Outlier = OutlierConfig{
					Times:  []float64{20, 30, 40},
					Offset: 5000 * time.Nanosecond, // 5us outliers - well above MAD threshold
				}
			},
		},
		{
			name:              "Tracking stability with periodic outliers",
			pulseWidth:        0,
			duration:          150.0,
			maxTrackingStdDev: 20 * time.Nanosecond, // Should maintain low stddev despite many outliers
			modifySimCfg: func(cfg *Config) {
				cfg.Outlier = OutlierConfig{
					Times:  []float64{40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140},
					Offset: 1500 * time.Nanosecond,
				}
			},
		},
		{
			name:              "MAD rejects spikes during sustained shift",
			pulseWidth:        0,
			duration:          70.0,
			maxTrackingStdDev: 50 * time.Nanosecond, // Higher tolerance due to shift transient
			maxTrackingAbsMax: 150 * time.Nanosecond, // Should be ~100ns from shift, NOT ~2us from spikes
			modifySimCfg: func(cfg *Config) {
				// Apply sustained 100ns shift after tracking stabilizes
				cfg.Shift = ShiftConfig{
					StartTime: 35.0, // Start after tracking is stable
					Ramp:      2 * time.Second,
					Duration:  10 * time.Second, // 2s up + 6s hold + 2s down (ends at t=45s)
					Shift:     100 * time.Nanosecond,
				}
				// Inject 2us spikes: during hold, right after shift, and later
				// If MAD rejects spikes: absmax ~100ns (from shift only)
				// If MAD fails: absmax ~2us (spikes get through)
				cfg.Outlier = OutlierConfig{
					Times:  []float64{40, 46, 55},
					Offset: 2000 * time.Nanosecond,
				}
			},
		},
		{
			name:              "Outlier gate during MAD warmup",
			pulseWidth:        0,
			duration:          30.0,
			maxTrackingStdDev: 20 * time.Nanosecond, // Should maintain low stddev despite early outlier
			modifySimCfg: func(cfg *Config) {
				cfg.Outlier = OutlierConfig{
					Times:  []float64{20}, // Inject outlier early in tracking (MAD window not full)
					Offset: 1000 * time.Nanosecond, // Well above warmup threshold (OutlierThreshold + PreMADOutlierRange = 550ns)
				}
			},
			// PreMADOutlierRange protects servo: warmup threshold = 50ns + 500ns = 550ns
			// The 1µs outlier should be rejected, maintaining low stddev.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with default configs
			phcCfg := phcsync.DefaultConfig()
			simCfg := DefaultConfig()

			// Apply test-specific overrides
			simCfg.Duration = tt.duration
			simCfg.PulseWidth = tt.pulseWidth
			simCfg.ToggleTimes = tt.toggleTimes

			// Apply modifier functions if provided
			if tt.modifySimCfg != nil {
				tt.modifySimCfg(&simCfg)
			}
			if tt.modifyPHCCfg != nil {
				tt.modifyPHCCfg(&phcCfg)
			}

			curTime := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)

			// Discard logs during test
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))

			stats, err := Simulate(nil, phcCfg, simCfg, &curTime, lg)
			if err != nil {
				t.Fatalf("Simulate failed: %v", err)
			}

			// Check that tracking standard deviation is acceptable
			if tt.maxTrackingStdDev > 0 && stats.TrackingStdDev >= tt.maxTrackingStdDev {
				t.Errorf("tracking stddev = %v, want < %v", stats.TrackingStdDev, tt.maxTrackingStdDev)
			}

			// Check that tracking absolute max is acceptable
			if tt.maxTrackingAbsMax > 0 && stats.TrackingAbsMax >= tt.maxTrackingAbsMax {
				t.Errorf("tracking absmax = %v, want < %v", stats.TrackingAbsMax, tt.maxTrackingAbsMax)
			}

			// Check mode-based sample counts
			if tt.expectMinTrackingSamples > 0 {
				if stats.TrackingSamples < tt.expectMinTrackingSamples {
					t.Errorf("TrackingSamples = %d, want >= %d", stats.TrackingSamples, tt.expectMinTrackingSamples)
				}
			}
			if tt.expectMaxTrackingSamples > 0 {
				if stats.TrackingSamples > tt.expectMaxTrackingSamples {
					t.Errorf("TrackingSamples = %d, want <= %d", stats.TrackingSamples, tt.expectMaxTrackingSamples)
				}
			}
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

			t.Logf("Simulation completed: %d samples (reset=%d, converging=%d, tracking=%d), tracking stddev = %v, tracking absmax = %v",
				stats.SampleCount, stats.InitSamples, stats.ConvergingSamples, stats.TrackingSamples, stats.TrackingStdDev, stats.TrackingAbsMax)
		})
	}
}
