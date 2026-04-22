package sim

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/time/internal/syncsim"
)

// TestValidateRejectsBadConfig covers the fail-fast behaviour
// Simulate inherits from Config.Validate. Each case tweaks one
// field to an out-of-range value and expects a specific fragment
// in the error.
func TestValidateRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(*Config)
		expectMatch string
	}{
		{
			name:        "defaults pass",
			modify:      nil,
			expectMatch: "",
		},
		{
			name: "PHC white noise out of range",
			modify: func(c *Config) {
				c.PHC.WhiteNoise = 20000 // limit is <=10000
			},
			expectMatch: "whiteNoise",
		},
		{
			name: "GPS jitter out of range",
			modify: func(c *Config) {
				c.GPS.Jitter = 2000 // limit is <=1000
			},
			expectMatch: "jitter",
		},
		{
			name: "pulse MaxDelay out of range",
			modify: func(c *Config) {
				c.Pulse.MaxDelay = 2 // limit is <1
			},
			expectMatch: "maxDelay",
		},
		{
			name: "phcsample.Config.msgWindow <= minMsgSpan (cross-field only)",
			modify: func(c *Config) {
				// Both per-field range tags pass (MsgWindow >= 5,
				// MinMsgSpan in [1, 60]); only the relationship
				// MsgWindow > MinMsgSpan is violated.
				c.Sample.MsgWindow = 10
				c.Sample.MinMsgSpan = float64(c.Sample.MsgWindow)
			},
			expectMatch: "msgWindow",
		},
		{
			name: "outlier inside Fault.Outlier slice",
			modify: func(c *Config) {
				// Outlier has no check tags today; exercise the
				// slice-descent path via an Excursion entry instead,
				// whose Rise.Duration has check:">=0".
				c.Fault.Excursion = append(c.Fault.Excursion, syncsim.ExcursionConfig{
					StartTime: 10,
					Duration:  5,
					Amplitude: 100,
					Rise:      syncsim.RampConfig{Duration: -1},
				})
			},
			expectMatch: "fault.excursion",
		},
		{
			name: "bad entry inside MsgOutage slice",
			modify: func(c *Config) {
				c.MsgOutage = []syncsim.OutageConfig{{StartTime: -1, Duration: 1}}
			},
			expectMatch: "msgOutage",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Sim.Duration = 10
			if tc.modify != nil {
				tc.modify(&cfg)
			}
			err := cfg.Validate()
			if tc.expectMatch == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error mentioning %q", tc.expectMatch)
			}
			if !strings.Contains(err.Error(), tc.expectMatch) {
				t.Errorf("Validate() error %q does not mention %q", err.Error(), tc.expectMatch)
			}
		})
	}
}

// TestPHCSample exercises the Generator through the sim rig across
// the representative scenarios called out in plan/phc-sample.md step
// 5: single-edge, dual-edge, missing messages, missing edges, gross
// outliers, and startup. Threshold-style assertions cover residual
// stddev, absolute max, and minimum admitted sample count; each test
// case is tunable independently.
//
// Several of these cases will fail until phcWindow.TrueTimeOffset is
// filled in at step 6: while the stub returns ErrNotReady the
// Generator never emits a sample, so stats.Samples stays at zero and
// the residual fields stay at zero too. The skeleton is written
// against the eventual working behaviour per the plan.
func TestPHCSample(t *testing.T) {
	tests := []struct {
		name            string
		duration        float64 // simulation seconds
		maxStdDev       float64 // max tolerated residual stddev in ns; 0 = no check
		maxAbsMax       time.Duration
		minSamples      int // 0 = no check
		minNotReady     int // 0 = no check
		maxErrors       int // when > 0, stats.Errors must stay <= this
		enforceMaxError bool
		modifyConfig    func(*Config)
	}{
		{
			name:            "single-edge steady-state",
			duration:        60,
			maxStdDev:       50,
			maxAbsMax:       200 * time.Nanosecond,
			minSamples:      40,
			enforceMaxError: true,
		},
		{
			name:            "dual-edge 100ms pulse",
			duration:        60,
			maxStdDev:       50,
			maxAbsMax:       200 * time.Nanosecond,
			minSamples:      80,
			enforceMaxError: true,
			modifyConfig: func(cfg *Config) {
				cfg.Pulse.Width = 0.1
			},
		},
		{
			name:            "startup warm-up returns ErrNotReady",
			duration:        4, // shorter than MinMsgSpan + PulseWindow slack
			minNotReady:     1,
			enforceMaxError: true,
		},
		{
			name:            "missing messages mid-run",
			duration:        90,
			minSamples:      40,
			enforceMaxError: true,
			modifyConfig: func(cfg *Config) {
				cfg.MsgOutage = []syncsim.OutageConfig{{StartTime: 30, Duration: 15}}
			},
		},
		{
			name:            "missing edges mid-run",
			duration:        90,
			minSamples:      40,
			enforceMaxError: true,
			modifyConfig: func(cfg *Config) {
				cfg.EdgeOutage = []syncsim.OutageConfig{{StartTime: 30, Duration: 15}}
			},
		},
		{
			name:            "5 Hz messages",
			duration:        60,
			maxStdDev:       50,
			maxAbsMax:       200 * time.Nanosecond,
			minSamples:      40,
			enforceMaxError: true,
			modifyConfig: func(cfg *Config) {
				cfg.MsgRate = 5
			},
		},
		{
			name:            "gross outliers rejected by pre-admission filter",
			duration:        90,
			maxStdDev:       80, // noisier than clean; outliers should not dominate
			maxAbsMax:       500 * time.Nanosecond,
			minSamples:      60,
			enforceMaxError: true,
			modifyConfig: func(cfg *Config) {
				cfg.Fault.Outlier = []syncsim.OutlierConfig{
					{Time: 40, Offset: 10000},
					{Time: 55, Offset: 10000},
					{Time: 70, Offset: 10000},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Sim.Duration = tc.duration
			cfg.PHC.FreqOffset = 2000
			cfg.PHC.Drift = -150
			cfg.PHC.WhiteNoise = 0.633
			cfg.GPS.Jitter = 10
			if tc.modifyConfig != nil {
				tc.modifyConfig(&cfg)
			}
			curTime := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))
			stats, err := Simulate(cfg, &curTime, lg)
			if err != nil {
				t.Fatalf("Simulate: %v", err)
			}
			if tc.maxStdDev > 0 && stats.StdDev >= tc.maxStdDev {
				t.Errorf("StdDev = %.2f ns, want < %.0f ns", stats.StdDev, tc.maxStdDev)
			}
			if tc.maxAbsMax > 0 && stats.AbsMax >= tc.maxAbsMax {
				t.Errorf("AbsMax = %v, want < %v", stats.AbsMax, tc.maxAbsMax)
			}
			if tc.minSamples > 0 && stats.Samples < tc.minSamples {
				t.Errorf("Samples = %d, want >= %d", stats.Samples, tc.minSamples)
			}
			if tc.minNotReady > 0 && stats.NotReady < tc.minNotReady {
				t.Errorf("NotReady = %d, want >= %d", stats.NotReady, tc.minNotReady)
			}
			if tc.enforceMaxError && stats.Errors > tc.maxErrors {
				t.Errorf("Errors = %d, want <= %d", stats.Errors, tc.maxErrors)
			}
			t.Logf("stats:\n%s", stats)
		})
	}
}

type warmUpSweepSummary struct {
	meanPulse      float64
	maxPulse       int
	maxPulseSeed   int64
	pulseHistogram map[int]int
}

func sweepWarmUpOverMsgSeeds(t *testing.T, msgSeeds int) warmUpSweepSummary {
	t.Helper()

	var totalPulse int
	summary := warmUpSweepSummary{
		pulseHistogram: make(map[int]int),
	}

	for seed := range msgSeeds {
		cfg := DefaultConfig()
		cfg.Sim.Duration = 10
		cfg.PHC.FreqOffset = 2000
		cfg.PHC.Drift = -150
		cfg.PHC.WhiteNoise = 0.633
		cfg.GPS.Jitter = 10
		cfg.PulseSeed = 999
		cfg.MsgSeed = int64(seed)

		curTime := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
		lg := slog.New(slog.NewTextHandler(io.Discard, nil))
		stats, err := Simulate(cfg, &curTime, lg)
		if err != nil {
			t.Fatalf("seed %d: Simulate: %v", seed, err)
		}
		if stats.Samples == 0 {
			t.Fatalf("seed %d: no samples generated\n%s", seed, stats)
		}
		if stats.ReadyPulse != 5 {
			t.Fatalf("seed %d: ReadyPulse = %d, want 5\n%s", seed, stats.ReadyPulse, stats)
		}

		totalPulse += stats.ReadyPulse
		summary.pulseHistogram[stats.ReadyPulse]++
		if stats.ReadyPulse > summary.maxPulse {
			summary.maxPulse = stats.ReadyPulse
			summary.maxPulseSeed = int64(seed)
		}
	}

	summary.meanPulse = float64(totalPulse) / float64(msgSeeds)
	return summary
}

func TestPHCSampleWarmUpMsgSeedSweep(t *testing.T) {
	const msgSeeds = 100

	summary := sweepWarmUpOverMsgSeeds(t, msgSeeds)
	t.Logf("defaults over %d msgSeeds: meanPulse=%.2f maxPulse=%d maxPulseSeed=%d histogram=%v",
		msgSeeds, summary.meanPulse, summary.maxPulse, summary.maxPulseSeed, summary.pulseHistogram)
}

// TestPHCSampleSawtoothCorrection is the step-10 acceptance test.
// Residual per sample is (fitted UTC at cross-sample PHC) - (true UTC
// at cross-sample PHC); it is the delta we want to keep small. With
// a 15 ns peak-to-peak PrePulse sawtooth injected by the sim:
//
//   - IgnoreSawtoothCorrection=true must leave the delta at the
//     uncorrected-sawtooth level;
//   - corrections applied must shrink the delta meaningfully toward
//     the zero-sawtooth baseline.
//
// A wrong-sign or wrong-timescale correction fails the "corrected
// beats ignored" assertions because it doubles the sawtooth in X
// rather than cancelling it.
//
// Two delay regimes are exercised:
//   - "fast delivery": Pulse.MaxDelay = 250 µs (the MinDelay..MaxDelay
//     defaults), matching fast PHC-timestamper paths. xQuery is on top
//     of the last admitted edge.
//   - "CM4/5 delivery": Pulse.MaxDelay = 0.25 s, matching the Raspberry
//     Pi CM4/CM5 kernel worker-thread cadence that can delay edge
//     timestamp delivery by up to a quarter second. xQuery is a full
//     extrapolation horizon past the last admitted edge, so any slope
//     error in the OLS fit is amplified by 0.25 s before surfacing as
//     residual. This is where the PHC regression is really stressed.
//
// The sawtooth cycles fast enough (driven by a 100 ppb internal clock)
// to look non-linear over the PulseWindow; otherwise OLS absorbs it
// perfectly as a slope shift.
func TestPHCSampleSawtoothCorrection(t *testing.T) {
	const (
		duration   = 3000.0 // a few thousand samples
		sawAmp     = 15.0   // ns peak-to-peak
		phcFreqOff = 2000
		phcDrift   = -150.0
	)
	cases := []struct {
		name     string
		minDelay float64
		maxDelay float64
	}{
		{name: "fast delivery (MaxDelay=250us)", minDelay: 5e-6, maxDelay: 250e-6},
		{name: "CM4/5 delivery (MaxDelay=0.25s)", minDelay: 0.01, maxDelay: 0.25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := func(name string, modify func(*Config)) Stats {
				t.Helper()
				cfg := DefaultConfig()
				cfg.Sim.Duration = duration
				cfg.PHC.FreqOffset = phcFreqOff
				cfg.PHC.Drift = phcDrift
				cfg.Pulse.MinDelay = tc.minDelay
				cfg.Pulse.MaxDelay = tc.maxDelay
				if modify != nil {
					modify(&cfg)
				}
				curTime := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
				lg := slog.New(slog.NewTextHandler(io.Discard, nil))
				stats, err := Simulate(cfg, &curTime, lg)
				if err != nil {
					t.Fatalf("%s: Simulate: %v", name, err)
				}
				if stats.Samples < 2000 {
					t.Errorf("%s: Samples = %d, want >= 2000", name, stats.Samples)
				}
				if stats.Errors > 0 {
					t.Errorf("%s: Errors = %d, want 0", name, stats.Errors)
				}
				t.Logf("%-10s samples=%d  mean=%+.3f ns  stddev=%.3f ns  absMax=%d ns",
					name, stats.Samples, stats.Mean, stats.StdDev, stats.AbsMax.Nanoseconds())
				return stats
			}

			baseline := run("baseline", nil)

			withSawtooth := func(cfg *Config) {
				cfg.GPS.Sawtooth.Amp = sawAmp
				cfg.GPS.Sawtooth.InternalClock.Amp = 100.0
				cfg.Msg.SawtoothType = syncsim.SawtoothPrePulse
				cfg.Msg.PrePulseTime = 0.95
			}
			ignored := run("ignored", func(cfg *Config) {
				withSawtooth(cfg)
				cfg.Sample.IgnoreSawtoothCorrection = true
			})
			corrected := run("corrected", withSawtooth)

			// Sanity: uncorrected sawtooth must visibly widen the
			// delta versus baseline, so that "corrected beats ignored"
			// is a real assertion.
			if ignored.StdDev < 2.0*baseline.StdDev {
				t.Errorf("ignored stddev %.2f ns is not visibly worse than baseline %.2f ns; sawtooth injection too weak", ignored.StdDev, baseline.StdDev)
			}

			// Corrections must meaningfully shrink the delta relative
			// to the ignored case. A wrong-sign or unscaled
			// implementation fails here because it adds the sawtooth
			// back in rather than cancelling it.
			if corrected.StdDev >= 0.5*ignored.StdDev {
				t.Errorf("corrected stddev %.2f ns does not improve on ignored %.2f ns (< 0.5x)", corrected.StdDev, ignored.StdDev)
			}
			if corrected.AbsMax*2 > ignored.AbsMax*3/2+time.Nanosecond {
				// Require absMax to drop by at least ~33 percent.
				t.Errorf("corrected absMax %v does not improve enough on ignored %v", corrected.AbsMax, ignored.AbsMax)
			}

			// Corrections should recover close to the zero-sawtooth
			// baseline. The slack is wider for the CM4/5 case because
			// extrapolation amplifies residual sub-ns noise in X.
			absSlack := 3 * time.Nanosecond
			if tc.maxDelay > 0.01 {
				absSlack = 5 * time.Nanosecond
			}
			if corrected.AbsMax > baseline.AbsMax+absSlack {
				t.Errorf("corrected absMax %v does not recover close to baseline %v (slack %v)", corrected.AbsMax, baseline.AbsMax, absSlack)
			}
		})
	}
}
