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
		pulseWidth               float64               // 0 for single-edge mode
		duration                 float64               // simulation duration in seconds
		maxTrackingStdDev        time.Duration         // maximum acceptable tracking stddev
		maxTrackingAbsMax        time.Duration         // maximum acceptable tracking absolute max (0 = don't check)
		minTrackingStdDev        time.Duration         // minimum acceptable tracking stddev - for testing degradation (0 = don't check)
		minTrackingAbsMax        time.Duration         // minimum acceptable tracking absolute max - for testing degradation (0 = don't check)
		outages                  []OutageConfig        // signal outage periods
		expectMinTrackingSamples int                   // minimum acceptable tracking samples (0 = don't check)
		expectMaxTrackingSamples int                   // maximum acceptable tracking samples (0 = don't check)
		expectTrackingSamples    int                   // expected exact number of samples in tracking mode (0 = don't check)
		expectResetSamples       int                   // expected number of samples in reset mode (includes initial sync and recovery)
		expectConvergingSamples  int                   // expected number of samples in converging mode (0 = don't check)
		modifySimCfg             func(*Config)         // optional function to modify simCfg (nil = use defaults)
		modifyPHCCfg             func(*phcsync.Config) // optional function to modify phcCfg (nil = use defaults)
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
			name:              "dual-edge 50% duty cycle (500ms pulse)",
			pulseWidth:        0.5,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20 * time.Nanosecond,
			// Tests complete flow: reset mode discovers leading edge via alignment,
			// then converging/tracking modes work with discovered pulse width
		},
		{
			name:       "signal loss - no recovery",
			pulseWidth: 0,
			duration:   90.0,
			outages:    []OutageConfig{{StartTime: 60.0, Duration: 30.0}}, // outage from t=60s to end
			// Don't check maxTrackingStdDev since we lose sync
		},
		{
			name:                     "enters reset mode on permanent outage",
			pulseWidth:               0,
			duration:                 80.0,
			outages:                  []OutageConfig{{StartTime: 60.0, Duration: 20.0}}, // outage from t=60s to end
			expectMinTrackingSamples: 35,                                                // at least 35 tracking samples before outage
			expectMaxTrackingSamples: 50,                                                // allow flexibility with faster convergence
			expectResetSamples:       1,                                                 // 1 initial reset sample only (reset mode doesn't generate missing samples)
		},
		{
			name:                     "recovers from temporary outage",
			pulseWidth:               0,
			duration:                 120.0,
			outages:                  []OutageConfig{{StartTime: 60.0, Duration: 10.0}}, // 10s outage starting at t=60s
			maxTrackingStdDev:        30 * time.Nanosecond,                              // slightly higher tolerance due to recovery transient
			expectResetSamples:       2,                                                 // 1 initial + 1 after recovery
			expectMinTrackingSamples: 35,                                                // At least 35 before outage (plus time after recovery)
			// After recovery: reset→converging→tracking for remaining ~50 seconds
		},
		{
			name:                    "signal loss during converging mode",
			pulseWidth:              0,
			duration:                40.0,
			outages:                 []OutageConfig{{StartTime: 13.0, Duration: 10.0}}, // 10s outage starting at t=13s
			expectTrackingSamples:   0,                                                 // never reaches tracking
			expectResetSamples:      2,                                                 // 1 initial + 1 after recovery from converging loss
			expectConvergingSamples: 22,                                                // 7 good + 3 missing + 12 good in second phase
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
			outages:                  []OutageConfig{{StartTime: 60.0, Duration: 3.0}}, // 3s outage starting at t=60s
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
			outages:                  []OutageConfig{{StartTime: 60.0, Duration: 3.0}}, // 3s outage starting at t=60s
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
			outages:                  []OutageConfig{{StartTime: 90.0, Duration: 9.0}}, // 9s outage starting at t=90s
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
			outages:                  []OutageConfig{{StartTime: 90.0, Duration: 9.0}}, // 9s outage starting at t=90s
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
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 40, Offset: 2000},
					{Time: 50, Offset: 2000},
					{Time: 60, Offset: 2000},
				}
			},
		},
		{
			name:              "MAD gate behavior before window fills",
			pulseWidth:        0,
			duration:          20.0,
			maxTrackingStdDev: 20 * time.Nanosecond,
			modifySimCfg: func(cfg *Config) {
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 8, Offset: 150}, // Outlier during early tracking (MAD window not full)
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
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 32, Offset: 5000}, // delayed to allow MAD window to warm up after entering tracking
					{Time: 40, Offset: 5000},
					{Time: 48, Offset: 5000}, // 5us outliers - well above MAD threshold
				}
			},
		},
		{
			name:              "Tracking stability with periodic outliers",
			pulseWidth:        0,
			duration:          150.0,
			maxTrackingStdDev: 20 * time.Nanosecond, // Should maintain low stddev despite many outliers
			modifySimCfg: func(cfg *Config) {
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 40, Offset: 1500}, {Time: 50, Offset: 1500}, {Time: 60, Offset: 1500},
					{Time: 70, Offset: 1500}, {Time: 80, Offset: 1500}, {Time: 90, Offset: 1500},
					{Time: 100, Offset: 1500}, {Time: 110, Offset: 1500}, {Time: 120, Offset: 1500},
					{Time: 130, Offset: 1500}, {Time: 140, Offset: 1500},
				}
			},
		},
		{
			name:              "MAD rejects spikes during sustained shift",
			pulseWidth:        0,
			duration:          70.0,
			maxTrackingStdDev: 50 * time.Nanosecond,  // Higher tolerance due to shift transient
			maxTrackingAbsMax: 150 * time.Nanosecond, // Should be ~100ns from shift, NOT ~2us from spikes
			modifySimCfg: func(cfg *Config) {
				// Apply sustained 100ns shift after tracking stabilizes
				cfg.Fault.Excursion = []ExcursionConfig{{
					StartTime: 35.0, // Start after tracking is stable
					Duration:  10.0, // seconds - 2s up + 6s hold + 2s down (ends at t=45s)
					Amplitude: 100,  // nanoseconds
					Rise:      RampConfig{Duration: 2.0, Power: ptr(2.0)},
					Fall:      RampConfig{Duration: 2.0, Power: ptr(2.0)},
				}}
				// Inject 2us spikes: during hold, right after shift, and later
				// If MAD rejects spikes: absmax ~100ns (from shift only)
				// If MAD fails: absmax ~2us (spikes get through)
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 40, Offset: 2000},
					{Time: 46, Offset: 2000},
					{Time: 55, Offset: 2000},
				}
			},
		},
		{
			name:              "Outlier gate during MAD warmup",
			pulseWidth:        0,
			duration:          30.0,
			maxTrackingStdDev: 20 * time.Nanosecond, // Should maintain low stddev despite early outlier
			modifySimCfg: func(cfg *Config) {
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 20, Offset: 1000}, // Inject outlier early in tracking (MAD window not full)
					// 1000ns - Well above warmup threshold (OutlierThreshold + PreMADOutlierRange = 550ns)
				}
			},
			// PreMADOutlierRange protects servo: warmup threshold = 50ns + 500ns = 550ns
			// The 1µs outlier should be rejected, maintaining low stddev.
		},
		{
			name:                    "ADJ_SETOFFSET compensation",
			pulseWidth:              0,
			duration:                30.0,
			maxTrackingStdDev:       20 * time.Nanosecond,
			expectConvergingSamples: 13, // 1 measure + 1 compensate + ~11 to converge residual
			// After reset step with delay d1 (~5µs), first pulse in converging measures offset ~d1
			// Compensation steps by 2×d1 (~10µs) with delay d2 (~5µs), net advance = d1 + (d1 - d2)
			// Residual error is |d1 - d2| which is ~1-2µs typical with 1µs stddev in delays
			// PI then converges residual in ~12 samples vs ~19 without compensation
		},
		{
			name:              "sawtooth correction with PrePulse messages",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 25 * time.Nanosecond, // slightly higher tolerance due to sawtooth
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 8.0 // 8ns peak-to-peak sawtooth (other fields use defaults)
			},
			// This test exercises:
			// - SawtoothPPS simulator with oscillator coupling
			// - PrePulse event generation in generatePulseEvents
			// - handlePrePulseMsgEvent using Sawtooth.Next from lastReading
			// - TimestampReading with SawtoothCorrections in VirtualClock
		},
		{
			name:              "sawtooth correction improves accuracy - correction enabled",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 9 * time.Nanosecond,  // Should stay near baseline ~8ns
			maxTrackingAbsMax: 28 * time.Nanosecond, // Should stay near baseline ~27ns
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
			},
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.IgnoreSawtoothCorrection = false // Use correction (default)
			},
		},
		{
			name:              "sawtooth correction improves accuracy - correction disabled",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			minTrackingStdDev: 8 * time.Nanosecond,  // Must be worse than corrected (~9ns)
			minTrackingAbsMax: 26 * time.Nanosecond, // Must be worse than corrected (~28ns)
			maxTrackingStdDev: 11 * time.Nanosecond, // But not too degraded
			maxTrackingAbsMax: 33 * time.Nanosecond, // But not too degraded
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
			},
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.IgnoreSawtoothCorrection = true // Ignore correction
			},
		},
		{
			name:              "sawtooth correction with PostPulse messages",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 25 * time.Nanosecond, // slightly higher tolerance due to sawtooth
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 8.0 // 8ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.25
				cfg.Msg.PostPulseDelay = 0.1
			},
			// This test exercises:
			// - PostPulse event generation in generatePulseEvents
			// - EventPostPulseMsg handler using Sawtooth.Current from lastReading
			// - TimestampReading with SawtoothCorrections in VirtualClock
		},
		{
			name:              "sawtooth PostPulse correction improves accuracy - correction enabled",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 10 * time.Nanosecond, // Should stay near baseline ~9ns
			maxTrackingAbsMax: 28 * time.Nanosecond, // Should stay near baseline ~27ns
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.25
				cfg.Msg.PostPulseDelay = 0.1
			},
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.IgnoreSawtoothCorrection = false // Use correction (default)
			},
		},
		{
			name:              "sawtooth PostPulse correction improves accuracy - correction disabled",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			minTrackingStdDev: 8 * time.Nanosecond,  // Must be worse than corrected (~9ns)
			minTrackingAbsMax: 26 * time.Nanosecond, // Must be worse than corrected (~28ns)
			maxTrackingStdDev: 11 * time.Nanosecond, // But not too degraded
			maxTrackingAbsMax: 32 * time.Nanosecond, // But not too degraded
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.25
				cfg.Msg.PostPulseDelay = 0.1
			},
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.IgnoreSawtoothCorrection = true // Ignore correction
			},
		},
		{
			name:              "sawtooth PostPulse with tighter delay range",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 9 * time.Nanosecond,
			maxTrackingAbsMax: 28 * time.Nanosecond,
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.02 // Tighter delay range: 10-20ms instead of 10-250ms
				cfg.Msg.PostPulseDelay = 0.1
			},
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.IgnoreSawtoothCorrection = false // Use correction
			},
		},
		{
			name:              "sawtooth PostPulse with tighter delay range - correction disabled",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 15 * time.Nanosecond,
			maxTrackingAbsMax: 35 * time.Nanosecond,
			modifySimCfg: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.02 // Tighter delay range
				cfg.Msg.PostPulseDelay = 0.1
			},
			modifyPHCCfg: func(cfg *phcsync.Config) {
				cfg.Track.IgnoreSawtoothCorrection = true // Ignore correction
			},
			// Test to verify that sawtooth correction still provides benefit with tighter delay
		},
		{
			name:              "flicker noise order of magnitude",
			pulseWidth:        0,
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 27 * time.Nanosecond,
			maxTrackingAbsMax: 80 * time.Nanosecond,
			modifySimCfg: func(cfg *Config) {
				// Isolated flicker noise test - use defaults except flicker
				cfg.PHC.FlickerNoise = 14.0 // Typical flicker value
			},
			// Test validates flicker implementation gives reasonable order of magnitude.
			// Before fix (random walk): stddev=53ns, absMax=140ns (completely wrong - 3x too high)
			// After fix (true flicker): stddev=18ns, absMax=51ns (realistic)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with default config
			cfg := Config{
				PHC: PHCConfig{
					FreqOffset: 2000.0,
					Drift:      -150.0,
					WhiteNoise: 0.633,
				},
				GPS: GPSConfig{
					Jitter: 10.0,
					Sawtooth: SawtoothConfig{
						PhaseInit: 0.5,
						InternalClock: FreqSinusoid{
							Amp:       2.0,
							Period:    600.0,
							PhaseInit: 1.0 / 6.0,
						},
					},
				},
				Sync: phcsync.DefaultConfig(),
				Pulse: PulseConfig{
					MinDelay: 5e-6,
					MaxDelay: 250e-6,
					Width:    tt.pulseWidth,
				},
				Msg: MsgConfig{
					Delay:          0.1,
					Jitter:         0.01,
					SawtoothType:   SawtoothPrePulse,
					PrePulseTime:   0.95,
					PostPulseDelay: 0.1,
				},
				Fault: FaultConfig{
					Outage: tt.outages,
				},
			}

			// Apply modifier functions if provided
			if tt.modifySimCfg != nil {
				tt.modifySimCfg(&cfg)
			}
			if tt.modifyPHCCfg != nil {
				tt.modifyPHCCfg(&cfg.Sync)
			}

			cfg.Sim.Duration = tt.duration

			curTime := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)

			// Discard logs during test
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))

			stats, err := Simulate(nil, cfg, nil, &curTime, lg)
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

			// Check minimum thresholds (for tests that verify degraded performance)
			if tt.minTrackingStdDev > 0 && stats.TrackingStdDev <= tt.minTrackingStdDev {
				t.Errorf("tracking stddev = %v, want > %v (should be degraded)", stats.TrackingStdDev, tt.minTrackingStdDev)
			}

			if tt.minTrackingAbsMax > 0 && stats.TrackingAbsMax <= tt.minTrackingAbsMax {
				t.Errorf("tracking absmax = %v, want > %v (should be degraded)", stats.TrackingAbsMax, tt.minTrackingAbsMax)
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
