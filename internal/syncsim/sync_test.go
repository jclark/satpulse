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
		duration                 float64       // simulation duration in seconds
		maxTrackingStdDev        float64       // maximum acceptable tracking stddev in nanoseconds
		maxTrackingAbsMax        time.Duration // maximum acceptable tracking absolute max (0 = don't check)
		minTrackingStdDev        float64       // minimum acceptable tracking stddev in nanoseconds - for testing degradation (0 = don't check)
		minTrackingAbsMax        time.Duration // minimum acceptable tracking absolute max - for testing degradation (0 = don't check)
		expectMinTrackingSamples int           // minimum acceptable tracking samples (0 = don't check)
		expectMaxTrackingSamples int           // maximum acceptable tracking samples (0 = don't check)
		expectTrackingSamples    int           // expected exact number of samples in tracking mode (0 = don't check)
		expectResetSamples       int           // expected number of samples in reset mode (includes initial sync and recovery)
		expectConvergingSamples  int           // expected number of samples in converging mode (0 = don't check)
		expectResetEntered       *int          // expected times reset mode entered (nil = don't check)
		expectConvergingEntered  *int          // expected times converging mode entered (nil = don't check)
		expectTrackingEntered    *int          // expected times tracking mode entered (nil = don't check)
		modifyConfig             func(*Config) // optional function to modify config (nil = use defaults)
	}{
		{
			name:              "single-edge mode",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20,
		},
		{
			name:              "dual-edge 100ms pulse",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20,
			modifyConfig:      func(cfg *Config) { cfg.Pulse.Width = 0.1 },
		},
		{
			name:              "dual-edge 200ms pulse",
			duration:          600.0, // 10 minutes (needs longer to converge)
			maxTrackingStdDev: 20,
			modifyConfig:      func(cfg *Config) { cfg.Pulse.Width = 0.2 },
		},
		{
			name:              "dual-edge 10ms pulse",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20,
			modifyConfig:      func(cfg *Config) { cfg.Pulse.Width = 0.01 },
		},
		{
			name:              "dual-edge 50% duty cycle (500ms pulse)",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 20,
			// Tests complete flow: reset mode discovers leading edge via alignment,
			// then converging/tracking modes work with discovered pulse width
			modifyConfig: func(cfg *Config) { cfg.Pulse.Width = 0.5 },
		},
		{
			name:     "signal loss - no recovery",
			duration: 90.0,
			// Don't check maxTrackingStdDev since we lose sync
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 30.0}} // outage from t=60s to end
			},
		},
		{
			name:                     "enters reset mode on permanent outage",
			duration:                 80.0,
			expectMinTrackingSamples: 35, // at least 35 tracking samples before outage
			expectMaxTrackingSamples: 50, // allow flexibility with faster convergence
			expectResetSamples:       1,  // 1 initial reset sample only (reset mode doesn't generate missing samples)
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 20.0}} // outage from t=60s to end
			},
		},
		{
			name:                     "recovers from temporary outage",
			duration:                 120.0,
			maxTrackingStdDev:        30,     // slightly higher tolerance due to recovery transient
			expectResetSamples:       2,      // 1 initial + 1 after recovery
			expectMinTrackingSamples: 35,     // At least 35 before outage (plus time after recovery)
			expectResetEntered:       ptr(2), // entered reset twice (initial + after outage)
			expectTrackingEntered:    ptr(2), // entered tracking twice (initial + after recovery)
			// After recovery: reset→converging→tracking for remaining ~50 seconds
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 10.0}} // 10s outage starting at t=60s
			},
		},
		{
			name:                    "signal loss during converging mode",
			duration:                40.0,
			expectTrackingSamples:   0,      // never reaches tracking
			expectResetSamples:      2,      // 1 initial + 1 after recovery from converging loss
			expectConvergingSamples: 22,     // 7 good + 3 missing + 12 good in second phase
			expectTrackingEntered:   ptr(0), // tracking never entered
			// Expected flow:
			// - Reset #1 at t=6s (1 sample)
			// - Converging from t=6s to t=12s (7 good samples)
			// - Missing samples at t=13.5s, 14.5s, 15.5s (3 missing samples in converging)
			// - Transition to reset at t=15.5s after 3rd missing sample
			// - Signal restarts at t=23s
			// - Reset #2 at t=27s (1 sample)
			// - Converging from t=28s to t=40s (12 good samples)
			// Total converging: 7 + 3 + 12 = 22 samples
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 13.0, Duration: 10.0}} // 10s outage starting at t=13s
			},
		},
		{
			name:                     "EMA feature reduces drift during brief outages",
			duration:                 120.0,
			maxTrackingStdDev:        15,
			maxTrackingAbsMax:        40 * time.Nanosecond, // With EMA enabled, absmax ~24ns (vs 58ns without)
			expectMinTrackingSamples: 90,                   // At least 90 tracking samples despite outage
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 3.0}} // 3s outage starting at t=60s
				cfg.Sync.Track.Kp = 0.5                                             // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.AvgFreqTimeConstant = 30 // Enable EMA with 30s time constant
			},
		},
		{
			name:                     "Without EMA drift is worse during brief outages",
			duration:                 120.0,
			maxTrackingStdDev:        18,
			maxTrackingAbsMax:        80 * time.Nanosecond, // Without EMA, absmax ~58ns (2x worse than with EMA)
			expectMinTrackingSamples: 90,                   // At least 90 tracking samples despite outage
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 3.0}} // 3s outage starting at t=60s
				cfg.Sync.Track.Kp = 0.5                                             // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.AvgFreqTimeConstant = 0 // Disable EMA feature
			},
		},
		{
			name:                     "EMA feature with longer outage (increased badSampleLimit)",
			duration:                 180.0,
			maxTrackingStdDev:        12,
			maxTrackingAbsMax:        35 * time.Nanosecond, // With EMA, absmax ~23ns even with 9s outage
			expectMinTrackingSamples: 150,                  // At least 150 tracking samples despite longer outage
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 90.0, Duration: 9.0}} // 9s outage starting at t=90s
				cfg.Sync.Track.Kp = 0.5                                             // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.AvgFreqTimeConstant = 30 // Enable EMA with 30s time constant
				cfg.Sync.Track.BadSampleRunLimit = 15   // Increase limit to allow longer outage
			},
		},
		{
			name:                     "Without EMA longer outage shows severe drift (increased badSampleLimit)",
			duration:                 180.0,
			maxTrackingStdDev:        30,
			maxTrackingAbsMax:        180 * time.Nanosecond, // Without EMA, absmax ~139ns (5-6x worse than with EMA)
			expectMinTrackingSamples: 150,                   // At least 150 tracking samples despite longer outage
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outage = []OutageConfig{{StartTime: 90.0, Duration: 9.0}} // 9s outage starting at t=90s
				cfg.Sync.Track.Kp = 0.5                                             // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.AvgFreqTimeConstant = 0 // Disable EMA feature
				cfg.Sync.Track.BadSampleRunLimit = 15  // Increase limit to allow longer outage
			},
		},
		{
			name:              "MAD detects scheduled outliers",
			duration:          90.0, // Longer duration to ensure we're in tracking mode
			maxTrackingStdDev: 20,   // Should maintain low stddev despite outliers
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 40, Offset: 2000},
					{Time: 50, Offset: 2000},
					{Time: 60, Offset: 2000},
				}
			},
		},
		{
			name:              "MAD gate behavior before window fills",
			duration:          20.0,
			maxTrackingStdDev: 20,
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 8, Offset: 150}, // Outlier during early tracking (MAD window not full)
				}
				cfg.Sync.Track.MADThreshold = 200 // 200ns gate - 150ns outlier should pass through
			},
		},
		{
			name:              "Large outliers rejected despite MAD window",
			duration:          60.0,
			maxTrackingStdDev: 20,
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outlier = []OutlierConfig{
					{Time: 32, Offset: 5000}, // delayed to allow MAD window to warm up after entering tracking
					{Time: 40, Offset: 5000},
					{Time: 48, Offset: 5000}, // 5us outliers - well above MAD threshold
				}
			},
		},
		{
			name:              "Tracking stability with periodic outliers",
			duration:          150.0,
			maxTrackingStdDev: 20, // Should maintain low stddev despite many outliers
			modifyConfig: func(cfg *Config) {
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
			duration:          70.0,
			maxTrackingStdDev: 50,                    // Higher tolerance due to shift transient
			maxTrackingAbsMax: 150 * time.Nanosecond, // Should be ~100ns from shift, NOT ~2us from spikes
			modifyConfig: func(cfg *Config) {
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
			duration:          30.0,
			maxTrackingStdDev: 20, // Should maintain low stddev despite early outlier
			modifyConfig: func(cfg *Config) {
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
			duration:                30.0,
			maxTrackingStdDev:       20,
			expectConvergingSamples: 13, // 1 measure + 1 compensate + ~11 to converge residual
			// After reset step with delay d1 (~5µs), first pulse in converging measures offset ~d1
			// Compensation steps by 2×d1 (~10µs) with delay d2 (~5µs), net advance = d1 + (d1 - d2)
			// Residual error is |d1 - d2| which is ~1-2µs typical with 1µs stddev in delays
			// PI then converges residual in ~12 samples vs ~19 without compensation
		},
		{
			name:              "sawtooth correction with PrePulse messages",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 25,    // slightly higher tolerance due to sawtooth
			modifyConfig: func(cfg *Config) {
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
			duration:          300.0,                // 5 minutes
			maxTrackingStdDev: 9,                    // Should stay near baseline ~8ns
			maxTrackingAbsMax: 28 * time.Nanosecond, // Should stay near baseline ~27ns
			modifyConfig: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Sync.Track.Kp = 0.5     // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.IgnoreSawtoothCorrection = false // Use correction (default)
			},
		},
		{
			name:              "sawtooth correction improves accuracy - correction disabled",
			duration:          300.0,                // 5 minutes
			minTrackingStdDev: 8,                    // Must be worse than corrected (~9ns)
			minTrackingAbsMax: 26 * time.Nanosecond, // Must be worse than corrected (~28ns)
			maxTrackingStdDev: 11,                   // But not too degraded
			maxTrackingAbsMax: 33 * time.Nanosecond, // But not too degraded
			modifyConfig: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Sync.Track.Kp = 0.5     // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.IgnoreSawtoothCorrection = true // Ignore correction
			},
		},
		{
			name:              "sawtooth correction with PostPulse messages",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 25,    // slightly higher tolerance due to sawtooth
			modifyConfig: func(cfg *Config) {
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
			duration:          300.0,                // 5 minutes
			maxTrackingStdDev: 10,                   // Should stay near baseline ~9ns
			maxTrackingAbsMax: 28 * time.Nanosecond, // Should stay near baseline ~27ns
			modifyConfig: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.25
				cfg.Msg.PostPulseDelay = 0.1
				cfg.Sync.Track.Kp = 0.5 // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.IgnoreSawtoothCorrection = false // Use correction (default)
			},
		},
		{
			name:              "sawtooth PostPulse correction improves accuracy - correction disabled",
			duration:          300.0,                // 5 minutes
			minTrackingStdDev: 8,                    // Must be worse than corrected (~9ns)
			minTrackingAbsMax: 26 * time.Nanosecond, // Must be worse than corrected (~28ns)
			maxTrackingStdDev: 11,                   // But not too degraded
			maxTrackingAbsMax: 32 * time.Nanosecond, // But not too degraded
			modifyConfig: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.25
				cfg.Msg.PostPulseDelay = 0.1
				cfg.Sync.Track.Kp = 0.5 // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.IgnoreSawtoothCorrection = true // Ignore correction
			},
		},
		{
			name:              "sawtooth PostPulse with tighter delay range",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 9,
			maxTrackingAbsMax: 28 * time.Nanosecond,
			modifyConfig: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.02 // Tighter delay range: 10-20ms instead of 10-250ms
				cfg.Msg.PostPulseDelay = 0.1
				cfg.Sync.Track.Kp = 0.5 // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.IgnoreSawtoothCorrection = false // Use correction
			},
		},
		{
			name:              "sawtooth PostPulse with tighter delay range - correction disabled",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 15,
			maxTrackingAbsMax: 35 * time.Nanosecond,
			modifyConfig: func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = 20.0 // 20ns peak-to-peak sawtooth
				cfg.Msg.SawtoothType = SawtoothPostPulse
				cfg.Pulse.MinDelay = 0.01
				cfg.Pulse.MaxDelay = 0.02 // Tighter delay range
				cfg.Msg.PostPulseDelay = 0.1
				cfg.Sync.Track.Kp = 0.5 // Explicit Kp/Ki for high-jitter test config
				cfg.Sync.Track.Ki = 0.1
				cfg.Sync.Track.IgnoreSawtoothCorrection = true // Ignore correction
			},
			// Test to verify that sawtooth correction still provides benefit with tighter delay
		},
		{
			name:              "flicker noise order of magnitude",
			duration:          300.0, // 5 minutes
			maxTrackingStdDev: 27,
			maxTrackingAbsMax: 80 * time.Nanosecond,
			modifyConfig: func(cfg *Config) {
				// Isolated flicker noise test - use defaults except flicker
				cfg.PHC.FlickerNoise = 14.0 // Typical flicker value
			},
			// Test validates flicker implementation gives reasonable order of magnitude.
			// Before fix (random walk): stddev=53ns, absMax=140ns (completely wrong - 3x too high)
			// After fix (true flicker): stddev=18ns, absMax=51ns (realistic)
		},
		{
			name:              "phase step absorbed with MADWindow=10 (outlierRatioLimit disabled)",
			duration:          1000.0,
			minTrackingAbsMax: 600 * time.Nanosecond, // Step is absorbed, high tracking error
			modifyConfig: func(cfg *Config) {
				cfg.Sync.Track.MADWindow = 10
				cfg.Sync.Track.OutlierThreshold = 600   // Above step amplitude so MAD window behavior is tested
				cfg.Sync.Track.OutlierRatioLimit = 1.0  // Disable outlier ratio limit to test old behavior
				cfg.Fault.Excursion = []ExcursionConfig{{
					StartTime: 500.0,
					Duration:  10.0,
					Amplitude: 500,
					Rise:      RampConfig{Duration: 0.01},
					Fall:      RampConfig{Duration: 0.01},
				}}
			},
			// Issue #174: With MADWindow=10 and outlierRatioLimit disabled, median recenters
			// after ~4 outliers, so BadSampleRunLimit=5 is not reached and the step is absorbed.
		},
		{
			name:              "phase step triggers reset with MADWindow=20",
			duration:          1000.0,
			maxTrackingAbsMax: 200 * time.Nanosecond, // Reset triggered, low tracking error
			modifyConfig: func(cfg *Config) {
				cfg.Sync.Track.MADWindow = 20
				cfg.Sync.Track.OutlierThreshold = 600 // Above step amplitude so MAD window behavior is tested
				cfg.Fault.Excursion = []ExcursionConfig{{
					StartTime: 500.0,
					Duration:  10.0,
					Amplitude: 500,
					Rise:      RampConfig{Duration: 0.01},
					Fall:      RampConfig{Duration: 0.01},
				}}
			},
			// Issue #174: With MADWindow=20, enough consecutive outliers to hit
			// BadSampleRunLimit=5, triggering reset and proper resync.
		},
		{
			name:               "upper gate rejects extreme step",
			duration:           1000.0,
			maxTrackingAbsMax:  200 * time.Nanosecond, // Outliers rejected, low tracking error
			expectResetEntered: ptr(1),                // Only initial reset, no reset from excursion
			modifyConfig: func(cfg *Config) {
				// 600ns step exceeds default OutlierThreshold (500ns)
				// Upper gate rejects samples unconditionally, excursion < BadSampleRunLimit
				cfg.Sync.Track.BadSampleRunLimit = 15
				cfg.Fault.Excursion = []ExcursionConfig{{
					StartTime: 500.0,
					Duration:  12.0, // < BadSampleRunLimit, no reset
					Amplitude: 600,
					Rise:      RampConfig{Duration: 0.01},
					Fall:      RampConfig{Duration: 0.01},
				}}
			},
			// Issue #194: Upper gate (OutlierThreshold) rejects extreme samples without
			// adding them to MAD window. Outliers discarded, tracking stays accurate.
		},
		{
			name:               "without upper gate step is absorbed (outlierRatioLimit disabled)",
			duration:           1000.0,
			minTrackingAbsMax:  600 * time.Nanosecond, // Step absorbed, high tracking error
			expectResetEntered: ptr(2),                // Initial + reset when step ends
			modifyConfig: func(cfg *Config) {
				// Same 600ns step, but OutlierThreshold raised above it
				// Samples pass to MAD, get added to window, median shifts, step absorbed
				cfg.Sync.Track.OutlierThreshold = 650
				cfg.Sync.Track.BadSampleRunLimit = 15
				cfg.Sync.Track.OutlierRatioLimit = 1.0 // Disable outlier ratio limit to test old behavior
				cfg.Fault.Excursion = []ExcursionConfig{{
					StartTime: 500.0,
					Duration:  12.0,
					Amplitude: 600,
					Rise:      RampConfig{Duration: 0.01},
					Fall:      RampConfig{Duration: 0.01},
				}}
			},
			// Issue #194: Without upper gate and outlierRatioLimit disabled, step passes to MAD,
			// median shifts to ~600ns. When step ends, normal samples are outliers vs shifted median.
		},
		{
			name:               "outlierRatioLimit prevents step absorption with MADWindow=10",
			duration:           1000.0,
			maxTrackingAbsMax:  200 * time.Nanosecond, // Reset triggered early, low tracking error
			expectResetEntered: ptr(2),                // Initial + reset from outlier ratio
			modifyConfig: func(cfg *Config) {
				cfg.Sync.Track.MADWindow = 10
				cfg.Sync.Track.OutlierThreshold = 600  // Above step amplitude so MAD window behavior is tested
				cfg.Sync.Track.OutlierRatioLimit = 0.3 // Default: triggers reset before median recenters
				cfg.Fault.Excursion = []ExcursionConfig{{
					StartTime: 500.0,
					Duration:  10.0,
					Amplitude: 500,
					Rise:      RampConfig{Duration: 0.01},
					Fall:      RampConfig{Duration: 0.01},
				}}
			},
			// Fix for #174: OutlierRatioLimit (0.3) triggers reset after >3 outliers in window of 10,
			// before median can recenter. Step is not absorbed.
		},
		{
			name:               "outlierRatioLimit prevents step absorption without upper gate",
			duration:           1000.0,
			maxTrackingAbsMax:  200 * time.Nanosecond, // Reset triggered early, low tracking error
			expectResetEntered: ptr(2),                // Initial + reset from outlier ratio
			modifyConfig: func(cfg *Config) {
				cfg.Sync.Track.OutlierThreshold = 650  // Above step amplitude
				cfg.Sync.Track.OutlierRatioLimit = 0.3 // Default: triggers reset before median shifts
				cfg.Fault.Excursion = []ExcursionConfig{{
					StartTime: 500.0,
					Duration:  12.0,
					Amplitude: 600,
					Rise:      RampConfig{Duration: 0.01},
					Fall:      RampConfig{Duration: 0.01},
				}}
			},
			// Fix for #187/#194: OutlierRatioLimit triggers reset before median can shift,
			// preventing step absorption even without upper gate protection.
		},
		{
			name:               "badSampleRatioLimit detects intermittent failures",
			duration:           120.0,
			expectResetEntered: ptr(2), // Initial + reset from bad sample ratio
			modifyConfig: func(cfg *Config) {
				// Use small window for faster test (10 seconds)
				cfg.Sync.Track.BadSampleWindow = 10
				cfg.Sync.Track.BadSampleRatioLimit = 0.5 // >50% bad triggers reset
				cfg.Sync.Track.BadSampleRunLimit = 100   // Disable consecutive limit
				cfg.Sync.Track.OutlierRatioLimit = 1.0   // Disable outlier ratio limit
				// Pattern: 2 outliers, 1 good (repeating) = 66% bad
				// Starts at t=30 after tracking is established and window can fill
				var outliers []OutlierConfig
				for t := 30.0; t < 100.0; t += 3 {
					outliers = append(outliers, OutlierConfig{Time: t, Offset: 2000})
					outliers = append(outliers, OutlierConfig{Time: t + 1, Offset: 2000})
					// t+2 is a good sample
				}
				cfg.Fault.Outlier = outliers
			},
			// Issue #178: Detects intermittent failures even when not consecutive.
			// With 66% bad samples (2 bad, 1 good pattern) and 50% limit, reset triggers
			// once the 10-sample window fills with >50% bad samples.
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
				},
				Msg: MsgConfig{
					Delay:          0.1,
					Jitter:         0.01,
					SawtoothType:   SawtoothPrePulse,
					PrePulseTime:   0.95,
					PostPulseDelay: 0.1,
				},
			}
			if tt.modifyConfig != nil {
				tt.modifyConfig(&cfg)
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
				t.Errorf("tracking stddev = %.2f, want < %.0f", stats.TrackingStdDev, tt.maxTrackingStdDev)
			}

			// Check that tracking absolute max is acceptable
			if tt.maxTrackingAbsMax > 0 && stats.TrackingAbsMax >= tt.maxTrackingAbsMax {
				t.Errorf("tracking absmax = %v, want < %v", stats.TrackingAbsMax, tt.maxTrackingAbsMax)
			}

			// Check minimum thresholds (for tests that verify degraded performance)
			if tt.minTrackingStdDev > 0 && stats.TrackingStdDev <= tt.minTrackingStdDev {
				t.Errorf("tracking stddev = %.2f, want > %.0f (should be degraded)", stats.TrackingStdDev, tt.minTrackingStdDev)
			}

			if tt.minTrackingAbsMax > 0 && stats.TrackingAbsMax <= tt.minTrackingAbsMax {
				t.Errorf("tracking absmax = %v, want > %v (should be degraded)", stats.TrackingAbsMax, tt.minTrackingAbsMax)
			}

			// Check mode-based sample counts
			trackingSamples := stats.ModeSamples[phcsync.ModeTracking]
			resetSamples := stats.ModeSamples[phcsync.ModeReset]
			convergingSamples := stats.ModeSamples[phcsync.ModeConverging]
			if tt.expectMinTrackingSamples > 0 {
				if trackingSamples < tt.expectMinTrackingSamples {
					t.Errorf("TrackingSamples = %d, want >= %d", trackingSamples, tt.expectMinTrackingSamples)
				}
			}
			if tt.expectMaxTrackingSamples > 0 {
				if trackingSamples > tt.expectMaxTrackingSamples {
					t.Errorf("TrackingSamples = %d, want <= %d", trackingSamples, tt.expectMaxTrackingSamples)
				}
			}
			if tt.expectTrackingSamples > 0 {
				if trackingSamples != tt.expectTrackingSamples {
					t.Errorf("TrackingSamples = %d, want %d", trackingSamples, tt.expectTrackingSamples)
				}
			}
			if tt.expectResetSamples > 0 {
				if resetSamples != tt.expectResetSamples {
					t.Errorf("ResetSamples = %d, want %d", resetSamples, tt.expectResetSamples)
				}
			}
			if tt.expectConvergingSamples > 0 {
				if convergingSamples != tt.expectConvergingSamples {
					t.Errorf("ConvergingSamples = %d, want %d", convergingSamples, tt.expectConvergingSamples)
				}
			}

			// Check mode entry counts
			resetEntered := stats.ModeTransitions[phcsync.ModeReset]
			convergingEntered := stats.ModeTransitions[phcsync.ModeConverging]
			trackingEntered := stats.ModeTransitions[phcsync.ModeTracking]
			if tt.expectResetEntered != nil && resetEntered != *tt.expectResetEntered {
				t.Errorf("ResetEntered = %d, want %d", resetEntered, *tt.expectResetEntered)
			}
			if tt.expectConvergingEntered != nil && convergingEntered != *tt.expectConvergingEntered {
				t.Errorf("ConvergingEntered = %d, want %d", convergingEntered, *tt.expectConvergingEntered)
			}
			if tt.expectTrackingEntered != nil && trackingEntered != *tt.expectTrackingEntered {
				t.Errorf("TrackingEntered = %d, want %d", trackingEntered, *tt.expectTrackingEntered)
			}

			t.Logf("Simulation completed: %d samples (reset=%d, converging=%d, tracking=%d), entered (reset=%d, converging=%d, tracking=%d), tracking stddev = %v, tracking absmax = %v",
				stats.SampleCount, resetSamples, convergingSamples, trackingSamples, resetEntered, convergingEntered, trackingEntered, stats.TrackingStdDev, stats.TrackingAbsMax)
		})
	}
}
