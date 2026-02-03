package phcsync

import (
	"strings"
	"testing"
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