package serialpps

import (
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		errText string
	}{
		{name: "defaults", cfg: DefaultConfig()},
		{name: "zero uncertainty", cfg: Config{MaxDelay: 0.8}},
		{name: "negative uncertainty", cfg: Config{DelayUncertainty: -0.001, MaxDelay: 0.8}, errText: "delayUncertainty"},
		{name: "zero maximum", cfg: Config{DelayUncertainty: 0.005}, errText: "maxDelay"},
		{name: "one-second interval", cfg: Config{DelayUncertainty: 0.2, MaxDelay: 0.8}, errText: "delayUncertainty + maxDelay"},
		{name: "interval over one second", cfg: Config{DelayUncertainty: 0.3, MaxDelay: 0.8}, errText: "delayUncertainty + maxDelay"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.errText == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.errText) {
				t.Fatalf("Validate error = %v, want text %q", err, tc.errText)
			}
		})
	}
}
