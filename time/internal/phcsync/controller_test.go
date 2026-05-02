package phcsync

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/phctime"
)

// TestDefaultConfigValidates verifies that DefaultConfig returns a valid configuration.
func TestDefaultConfigValidates(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate()
	if err != nil {
		t.Errorf("DefaultConfig().Validate() failed: %v", err)
	}
}

// TestConfigValidation tests various validation scenarios.
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		modifyFn    func(*Config)
		expectError string // expected substring in error message (empty means no error expected)
	}{
		{
			name:        "ValidDefaultConfig",
			modifyFn:    func(c *Config) {},
			expectError: "",
		},
		{
			name: "ResetPulseWindowTooSmall",
			modifyFn: func(c *Config) {
				c.Reset.PulseWindow = 2
			},
			expectError: "reset.pulseWindow",
		},
		{
			name: "ResetPulseWindowTooLarge",
			modifyFn: func(c *Config) {
				c.Reset.PulseWindow = 100
			},
			expectError: "reset.pulseWindow",
		},
		{
			name: "ResetStepThresholdNegative",
			modifyFn: func(c *Config) {
				c.Reset.StepThreshold = -1
			},
			expectError: "reset.stepThreshold",
		},
		{
			name: "ResetStepThresholdTooLarge",
			modifyFn: func(c *Config) {
				c.Reset.StepThreshold = 1_000_000
			},
			expectError: "reset.stepThreshold",
		},
		{
			name: "ResetPulseVariationTooSmall",
			modifyFn: func(c *Config) {
				c.Reset.PulseVariation = 4
			},
			expectError: "reset.pulseVariation",
		},
		{
			name: "ResetDelayConfidenceWindowZero",
			modifyFn: func(c *Config) {
				c.Reset.DelayConfidenceWindow = 0
			},
			expectError: "reset.delayConfidenceWindow",
		},
		{
			name: "ResetDelayConfidenceWindowTooLarge",
			modifyFn: func(c *Config) {
				c.Reset.DelayConfidenceWindow = 1.1
			},
			expectError: "reset.delayConfidenceWindow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modifyFn(&cfg)
			err := cfg.Validate()

			if tt.expectError == "" {
				if err != nil {
					t.Errorf("Config.Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Config.Validate() expected error containing %q, got nil", tt.expectError)
				} else if !strings.Contains(err.Error(), tt.expectError) {
					t.Errorf("Config.Validate() error = %v, expected to contain %q", err, tt.expectError)
				}
			}
		})
	}
}

// TestSharedConfigValidation pins down the boundary behaviour of
// the new [sync.share] fields beyond the defaults check above.
func TestSharedConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		modifyFn    func(*Config)
		expectError string
	}{
		{
			name:        "DetectInvalidLeapFalseAccepted",
			modifyFn:    func(c *Config) { c.Share.DetectInvalidLeap = false },
			expectError: "",
		},
		{
			name:        "MinInvalidRepeatIntervalZeroRejected",
			modifyFn:    func(c *Config) { c.Share.MinInvalidRepeatInterval = 0 },
			expectError: "share.minInvalidRepeatInterval",
		},
		{
			name:        "MinInvalidRepeatIntervalAtUpperBoundRejected",
			modifyFn:    func(c *Config) { c.Share.MinInvalidRepeatInterval = 1.0 },
			expectError: "share.minInvalidRepeatInterval",
		},
		{
			name:        "MinInvalidRepeatIntervalNearUpperBoundAccepted",
			modifyFn:    func(c *Config) { c.Share.MinInvalidRepeatInterval = 0.999 },
			expectError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modifyFn(&cfg)
			err := cfg.Validate()
			if tt.expectError == "" {
				if err != nil {
					t.Errorf("Config.Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("Config.Validate() expected error containing %q, got nil", tt.expectError)
			} else if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("Config.Validate() error = %v, expected to contain %q", err, tt.expectError)
			}
		})
	}
}

// fakeBuffer satisfies the TimeMsgBuffer interface for whitebox tests
// of controller wiring around the invalid-leap detector. The values
// returned by GetPostTimeMessages are test-controlled; the pulse-
// correction methods are no-ops because the detector does not consult
// them. A zero-value fakeBuffer behaves as "no time messages
// available" (lastSec is zero, tRead is nil, detectLeap is nil).
type fakeBuffer struct {
	lastSec    ptime.Time
	tRead      []time.Time
	detectLeap func(time.Duration) bool
}

func (b *fakeBuffer) GetPostTimeMessages(n int) (ptime.Time, []time.Time, func(time.Duration) bool) {
	return b.lastSec, b.tRead, b.detectLeap
}

func (b *fakeBuffer) GetPulseCorrection(refTime ptime.Time) (time.Duration, bool) {
	return 0, false
}

func (b *fakeBuffer) WaitForPulseCorrection(refTime ptime.Time) bool {
	return false
}

// recordingSampler captures Sample calls for inspection.
type recordingSampler struct {
	samples []Sample
}

func (s *recordingSampler) Sample(data Sample) { s.samples = append(s.samples, data) }

// fakeClock is a minimal phcsync.Clock implementation for tests.
type fakeClock struct {
	freq float64
}

func (c *fakeClock) SetFreqOffset(f float64) error          { c.freq = f; return nil }
func (c *fakeClock) FreqOffset() (float64, error)           { return c.freq, nil }
func (c *fakeClock) MaxFreqOffset() float64                 { return 1e8 }
func (c *fakeClock) AdjTime(d time.Duration) (phctime.Era, error) {
	return phctime.Era(1), nil
}

// newTestController builds a Controller wired to the fake helpers, in
// ModeReset (after SetTimeMsgBuffer).
func newTestController(t *testing.T, buf TimeMsgBuffer) *Controller {
	t.Helper()
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	c, err := NewController(&fakeClock{}, &recordingSampler{}, nil, DefaultConfig(), ptime.LeapSecond{UTCOffAfter: 37}, 1, lg)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	c.SetTimeMsgBuffer(buf)
	return c
}

// forceConverging puts a freshly-constructed controller into
// ModeConverging with a known detectLeap closure (or nil) installed on
// the resetSampleGenerator before transitioning. Tests that target
// the post-reset behaviour can then optionally overwrite c.detectLeap
// to the scenario's closure.
func forceConverging(t *testing.T, c *Controller, rsgDetectLeap func(time.Duration) bool) {
	t.Helper()
	rsg, ok := c.sampleGen.(*resetSampleGenerator)
	if !ok {
		t.Fatalf("expected resetSampleGenerator, got %T", c.sampleGen)
	}
	rsg.detectLeap = rsgDetectLeap
	c.lastSample = &Sample{Kind: SampleOK}
	c.changeMode(ModeConverging)
}

// TestTimeMessageDetectsLeap exercises the Controller.TimeMessage
// gating around the captured detectLeap closure. Kill-switch and
// capture-vs-not-capture wiring is covered separately by
// TestChangeModeWiringDetectLeap.
func TestTimeMessageDetectsLeap(t *testing.T) {
	tests := []struct {
		name              string
		closureReturns    *bool // nil => clear c.detectLeap before the call
		expectMode        Mode
		expectClosureKept bool // true => detectLeap must remain installed after the call
		expectInvoked     bool
	}{
		{
			name:              "TriggersResetWhenClosureReturnsTrue",
			closureReturns:    boolPtr(true),
			expectMode:        ModeReset,
			expectClosureKept: false,
			expectInvoked:     true,
		},
		{
			name:              "NoResetWhenClosureReturnsFalse",
			closureReturns:    boolPtr(false),
			expectMode:        ModeConverging,
			expectClosureKept: true,
			expectInvoked:     true,
		},
		{
			name:              "NilClosureSkipsCheck",
			closureReturns:    nil,
			expectMode:        ModeConverging,
			expectClosureKept: false,
			expectInvoked:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestController(t, &fakeBuffer{})
			forceConverging(t, c, func(time.Duration) bool { return false })
			var called bool
			if tt.closureReturns != nil {
				ret := *tt.closureReturns
				c.detectLeap = func() bool {
					called = true
					return ret
				}
			} else {
				c.detectLeap = nil
			}
			c.TimeMessage()
			if c.Mode() != tt.expectMode {
				t.Errorf("Mode() = %v, want %v", c.Mode(), tt.expectMode)
			}
			if called != tt.expectInvoked {
				t.Errorf("detectLeap invoked = %v, want %v", called, tt.expectInvoked)
			}
			if tt.expectClosureKept && c.detectLeap == nil {
				t.Errorf("expected detectLeap to remain installed; got nil")
			}
			if !tt.expectClosureKept && c.detectLeap != nil {
				t.Errorf("expected detectLeap to be cleared; got non-nil")
			}
		})
	}
}

// TestChangeModeWiringDetectLeap covers the closure-capture and
// closure-clear seams in changeMode, including the kill-switch and
// the threshold being baked into the captured closure.
func TestChangeModeWiringDetectLeap(t *testing.T) {
	t.Run("LeavingResetCapturesClosureWithThreshold", func(t *testing.T) {
		c := newTestController(t, &fakeBuffer{})
		// The marker records the minDelta it receives so we can verify
		// the controller baked the configured threshold into the
		// captured closure.
		var got time.Duration
		marker := func(d time.Duration) bool { got = d; return false }
		c.cfg.Share.MinInvalidRepeatInterval = 0.5
		forceConverging(t, c, marker)
		if c.detectLeap == nil {
			t.Fatalf("expected controller to capture closure on leaving reset")
		}
		c.detectLeap()
		if want := 500 * time.Millisecond; got != want {
			t.Errorf("captured closure invoked inner with %v, want %v", got, want)
		}
	})

	t.Run("EnteringResetClearsClosure", func(t *testing.T) {
		c := newTestController(t, &fakeBuffer{})
		forceConverging(t, c, func(time.Duration) bool { return false })
		if c.detectLeap == nil {
			t.Fatalf("preconditions: closure should be set after leaving reset")
		}
		c.changeMode(ModeReset)
		if c.detectLeap != nil {
			t.Errorf("expected detectLeap to be cleared on entering reset, got non-nil")
		}
	})

	t.Run("LeavingResetWithNilClosureLeavesNil", func(t *testing.T) {
		c := newTestController(t, &fakeBuffer{})
		forceConverging(t, c, nil)
		if c.detectLeap != nil {
			t.Errorf("expected nil detectLeap when rsg captured none; got non-nil")
		}
	})

	t.Run("KillSwitchPreventsCapture", func(t *testing.T) {
		c := newTestController(t, &fakeBuffer{})
		c.cfg.Share.DetectInvalidLeap = false
		forceConverging(t, c, func(time.Duration) bool { return true })
		if c.detectLeap != nil {
			t.Errorf("expected nil detectLeap when DetectInvalidLeap is false, got non-nil")
		}
	})
}

func boolPtr(b bool) *bool { return &b }