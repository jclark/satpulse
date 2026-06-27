package phcsample

import (
	"strings"
	"testing"
)

// TestDefaultConfigValidates confirms the shipped defaults pass
// validation.
func TestDefaultConfigValidates(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v", err)
	}
}

// TestDiscontinuityThresholdBounds confirms the validation bounds on
// DiscontinuityThreshold. The lower bound exists because the threshold
// is converted to time.Duration (ns resolution) inside phcWindow;
// anything below 1 ns would truncate to zero and make the discontinuity
// detector fire on any non-zero interval deviation.
func TestDiscontinuityThresholdBounds(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		wantOK bool
	}{
		{name: "default (1 ms)", value: 0.001, wantOK: true},
		{name: "one nanosecond", value: 1e-9, wantOK: true},
		{name: "sub-nanosecond rejected", value: 5e-10, wantOK: false},
		{name: "zero rejected", value: 0, wantOK: false},
		{name: "one second rejected", value: 1.0, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.DiscontinuityThreshold = tc.value
			err := cfg.Validate()
			if tc.wantOK {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error mentioning discontinuityThreshold")
			}
			if !strings.Contains(err.Error(), "discontinuityThreshold") {
				t.Errorf("error %q does not mention discontinuityThreshold", err.Error())
			}
		})
	}
}

// TestConfigRejectsMsgWindowNotGreaterThanMinSpan confirms the
// inter-field invariant that msgWindow must strictly exceed
// minMsgSpan; otherwise the wallClock prunes history faster than the
// fit needs and SecondAt is permanently stuck in warm-up.
func TestConfigRejectsMsgWindowNotGreaterThanMinSpan(t *testing.T) {
	tests := []struct {
		name      string
		msgWindow int
		minSpan   float64
		wantOK    bool
	}{
		{name: "defaults", msgWindow: 30, minSpan: 3.0, wantOK: true},
		{name: "window equals span", msgWindow: 10, minSpan: 10.0, wantOK: false},
		{name: "window just above span", msgWindow: 10, minSpan: 9.5, wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MsgWindow = tc.msgWindow
			cfg.MinMsgSpan = tc.minSpan
			err := cfg.Validate()
			if tc.wantOK {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error mentioning msgWindow/minMsgSpan")
			}
			if !strings.Contains(err.Error(), "msgWindow") || !strings.Contains(err.Error(), "minMsgSpan") {
				t.Errorf("error missing field names: %v", err)
			}
		})
	}
}
