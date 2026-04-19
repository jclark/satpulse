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
