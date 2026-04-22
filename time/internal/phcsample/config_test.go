package phcsample

import (
	"strings"
	"testing"
)

// TestDefaultConfigValidates confirms the shipped defaults pass
// validation.
func TestDefaultConfigValidates(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.SmoothPhase {
		t.Errorf("DefaultConfig().SmoothPhase = false, want true")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v", err)
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
