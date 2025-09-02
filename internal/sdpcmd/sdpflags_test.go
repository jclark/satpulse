package sdpcmd

import (
	"testing"
	"time"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expected    *FlagConfig
		expectError bool
	}{
		{
			name: "no args - show all mode",
			args: []string{},
			expected: &FlagConfig{
				Mode:    ModeShowIfaces,
				Timeout: 2 * time.Second,
				Pin:     "",
				Chan:    0,
			},
		},
		{
			name: "interface only - show mode",
			args: []string{"eth0"},
			expected: &FlagConfig{
				Mode:      ModeShowPins,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "",
				Chan:      0,
			},
		},
		{
			name: "extts mode basic",
			args: []string{"-i", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "0",
				Chan:      0,
			},
		},
		{
			name: "extts mode with custom timeout",
			args: []string{"-i", "-t", "5.5", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   5500 * time.Millisecond,
				Pin:       "0",
				Chan:      0,
			},
		},
		{
			name: "extts mode with timeout 30 using long flag",
			args: []string{"-i", "--timeout", "30", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   30 * time.Second,
				Pin:       "0",
				Chan:      0,
			},
		},
		{
			name: "extts mode with zero timeout (unlimited)",
			args: []string{"-i", "-t", "0", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   0,
				Pin:       "0",
				Chan:      0,
			},
		},
		{
			name: "extts mode with show-stale",
			args: []string{"-i", "--show-stale", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				ShowStale: true,
				Pin:       "0",
				Chan:      0,
			},
		},
		{
			name: "extts mode with pin name",
			args: []string{"-i", "--pin", "SYNC_OUT", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "SYNC_OUT",
				Chan:      0,
			},
		},
		{
			name: "extts mode with pin index",
			args: []string{"-i", "--pin", "2", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "2",
				Chan:      0,
			},
		},
		{
			name: "extts mode with channel",
			args: []string{"-i", "--chan", "1", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "0",
				Chan:      1,
			},
		},
		{
			name: "extts mode with all options",
			args: []string{"-i", "-t", "10", "--show-stale", "--pin", "3", "--chan", "2", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   10 * time.Second,
				ShowStale: true,
				Pin:       "3",
				Chan:      2,
			},
		},
		{
			name: "extts mode with flags AFTER interface (real usage pattern)",
			args: []string{"-i", "eth0", "--pin", "1", "--timeout", "20", "--chan", "0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   20 * time.Second,
				Pin:       "1",
				Chan:      0,
			},
		},
		{
			name: "extts mode with all flags after interface",
			args: []string{"-i", "eth0", "--show-stale", "--pin", "2", "--timeout", "30", "--chan", "1"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				Timeout:   30 * time.Second,
				ShowStale: true,
				Pin:       "2",
				Chan:      1,
			},
		},
		{
			name: "jsonl flag",
			args: []string{"-j", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeShowPins,
				Interface: "eth0",
				JSONL:     true,
				Timeout:   2 * time.Second,
				Pin:       "",
				Chan:      0,
			},
		},
		{
			name: "jsonl flag with extts",
			args: []string{"-j", "-i", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeExtts,
				Interface: "eth0",
				JSONL:     true,
				Timeout:   2 * time.Second,
				Pin:       "0",
				Chan:      0,
			},
		},
		{
			name: "perout mode basic",
			args: []string{"-o", "0.1", "eth0"},
			expected: &FlagConfig{
				Mode:         ModePerout,
				Interface:    "eth0",
				Timeout:      2 * time.Second,
				Pin:          "0",
				Chan:         0,
				PeroutWidth:  0.1,
				PeroutPeriod: 1.0,
				PeroutPhase:  0,
			},
		},
		{
			name: "perout mode with period",
			args: []string{"-o", "0.01", "--period", "0.1", "eth0"},
			expected: &FlagConfig{
				Mode:         ModePerout,
				Interface:    "eth0",
				Timeout:      2 * time.Second,
				Pin:          "0",
				Chan:         0,
				PeroutWidth:  0.01,
				PeroutPeriod: 0.1,
				PeroutPhase:  0,
			},
		},
		{
			name: "perout mode with exponential notation - microsecond width",
			args: []string{"-o", "1e-6", "eth0"},
			expected: &FlagConfig{
				Mode:         ModePerout,
				Interface:    "eth0",
				Timeout:      2 * time.Second,
				Pin:          "0",
				Chan:         0,
				PeroutWidth:  1e-6,
				PeroutPeriod: 1.0,
				PeroutPhase:  0,
			},
		},
		{
			name: "perout mode with exponential notation - nanosecond width",
			args: []string{"-o", "1e-9", "eth0"},
			expected: &FlagConfig{
				Mode:         ModePerout,
				Interface:    "eth0",
				Timeout:      2 * time.Second,
				Pin:          "0",
				Chan:         0,
				PeroutWidth:  1e-9,
				PeroutPeriod: 1.0,
				PeroutPhase:  0,
			},
		},
		{
			name: "perout mode with exponential notation for all time values",
			args: []string{"-o", "1e-3", "--period", "1e-1", "--phase", "5e-2", "eth0"},
			expected: &FlagConfig{
				Mode:         ModePerout,
				Interface:    "eth0",
				Timeout:      2 * time.Second,
				Pin:          "0",
				Chan:         0,
				PeroutWidth:  1e-3,
				PeroutPeriod: 1e-1,
				PeroutPhase:  5e-2,
			},
		},
		{
			name: "disable mode",
			args: []string{"--disable", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeDisable,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "0",
				Chan:      0,
			},
		},
		{
			name: "disable mode with pin",
			args: []string{"--disable", "--pin", "2", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeDisable,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "2",
				Chan:      0,
			},
		},
		{
			name: "explicit show mode",
			args: []string{"--show", "eth0"},
			expected: &FlagConfig{
				Mode:      ModeShowPins,
				Interface: "eth0",
				Timeout:   2 * time.Second,
				Pin:       "",
				Chan:      0,
			},
		},
		{
			name: "show mode without interface",
			args: []string{"--show"},
			expected: &FlagConfig{
				Mode:    ModeShowIfaces,
				Timeout: 2 * time.Second,
				Pin:     "",
				Chan:    0,
			},
		},
		// Error cases
		{
			name:        "extts without interface",
			args:        []string{"-i"},
			expectError: true,
		},
		{
			name:        "perout without interface",
			args:        []string{"-o", "0.1"},
			expectError: true,
		},
		{
			name:        "disable without interface",
			args:        []string{"--disable"},
			expectError: true,
		},
		{
			name:        "conflicting modes - extts and perout",
			args:        []string{"-i", "-o", "0.1", "eth0"},
			expectError: true,
		},
		{
			name:        "conflicting modes - extts and disable",
			args:        []string{"-i", "--disable", "eth0"},
			expectError: true,
		},
		{
			name:        "conflicting modes - perout and disable",
			args:        []string{"-o", "0.1", "--disable", "eth0"},
			expectError: true,
		},
		{
			name:        "invalid flag",
			args:        []string{"--invalid-flag"},
			expectError: true,
		},
		{
			name:        "too many positional arguments",
			args:        []string{"eth0", "eth1"},
			expectError: true,
		},
		{
			name:        "too many positional arguments with mode",
			args:        []string{"-i", "eth0", "eth1", "eth2"},
			expectError: true,
		},
		{
			name:        "perout width too large",
			args:        []string{"-o", "1.5", "eth0"},
			expectError: true,
		},
		{
			name:        "perout width equals period",
			args:        []string{"-o", "1.0", "--period", "1.0", "eth0"},
			expectError: true,
		},
		{
			name:        "perout negative width",
			args:        []string{"-o", "-0.1", "eth0"},
			expectError: true,
		},
		{
			name:        "perout negative period",
			args:        []string{"-o", "0.1", "--period", "-1.0", "eth0"},
			expectError: true,
		},
		{
			name:        "perout negative phase",
			args:        []string{"-o", "0.1", "--phase", "-0.5", "eth0"},
			expectError: true,
		},
		// Mode-specific flag validation tests
		{
			name:        "period flag without perout mode",
			args:        []string{"--period", "2.0", "eth0"},
			expectError: true,
		},
		{
			name:        "phase flag without perout mode",
			args:        []string{"--phase", "0.5", "eth0"},
			expectError: true,
		},
		{
			name:        "period flag with extts mode",
			args:        []string{"-i", "--period", "2.0", "eth0"},
			expectError: true,
		},
		{
			name:        "phase flag with extts mode",
			args:        []string{"-i", "--phase", "0.5", "eth0"},
			expectError: true,
		},
		{
			name:        "period flag with disable mode",
			args:        []string{"--disable", "--period", "2.0", "eth0"},
			expectError: true,
		},
		{
			name:        "phase flag with disable mode",
			args:        []string{"--disable", "--phase", "0.5", "eth0"},
			expectError: true,
		},
		{
			name:        "timeout flag without extts mode",
			args:        []string{"--timeout", "5", "eth0"},
			expectError: true,
		},
		{
			name:        "show-stale flag without extts mode",
			args:        []string{"--show-stale", "eth0"},
			expectError: true,
		},
		{
			name:        "timeout flag with perout mode",
			args:        []string{"-o", "0.1", "--timeout", "5", "eth0"},
			expectError: true,
		},
		{
			name:        "show-stale flag with perout mode",
			args:        []string{"-o", "0.1", "--show-stale", "eth0"},
			expectError: true,
		},
		{
			name:        "timeout flag with disable mode",
			args:        []string{"--disable", "--timeout", "5", "eth0"},
			expectError: true,
		},
		{
			name:        "show-stale flag with disable mode",
			args:        []string{"--disable", "--show-stale", "eth0"},
			expectError: true,
		},
		{
			name:        "timeout flag with show mode",
			args:        []string{"--show", "--timeout", "10", "eth0"},
			expectError: true,
		},
		{
			name:        "show-stale flag with show all mode",
			args:        []string{"--show-stale"},
			expectError: true,
		},
		{
			name:        "period and phase without perout in show mode",
			args:        []string{"--period", "1", "--phase", "0.5", "eth0"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, help, _, err := parseFlags("sdp", tt.args)
			
			if help {
				// Help flag is special case, not an error
				return
			}
			
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			
			if cfg == nil {
				t.Fatal("cfg is nil")
			}
			
			// Compare fields
			if cfg.Mode != tt.expected.Mode {
				t.Errorf("Mode: got %v, want %v", cfg.Mode, tt.expected.Mode)
			}
			if cfg.Interface != tt.expected.Interface {
				t.Errorf("Interface: got %q, want %q", cfg.Interface, tt.expected.Interface)
			}
			if cfg.JSONL != tt.expected.JSONL {
				t.Errorf("JSONL: got %v, want %v", cfg.JSONL, tt.expected.JSONL)
			}
			if cfg.Timeout != tt.expected.Timeout {
				t.Errorf("Timeout: got %v, want %v", cfg.Timeout, tt.expected.Timeout)
			}
			if cfg.ShowStale != tt.expected.ShowStale {
				t.Errorf("ShowStale: got %v, want %v", cfg.ShowStale, tt.expected.ShowStale)
			}
			if cfg.Pin != tt.expected.Pin {
				t.Errorf("Pin: got %q, want %q", cfg.Pin, tt.expected.Pin)
			}
			if cfg.Chan != tt.expected.Chan {
				t.Errorf("Chan: got %d, want %d", cfg.Chan, tt.expected.Chan)
			}
			if cfg.Mode == ModePerout {
				if cfg.PeroutWidth != tt.expected.PeroutWidth {
					t.Errorf("PeroutWidth: got %v, want %v", cfg.PeroutWidth, tt.expected.PeroutWidth)
				}
				if cfg.PeroutPeriod != tt.expected.PeroutPeriod {
					t.Errorf("PeroutPeriod: got %v, want %v", cfg.PeroutPeriod, tt.expected.PeroutPeriod)
				}
				if cfg.PeroutPhase != tt.expected.PeroutPhase {
					t.Errorf("PeroutPhase: got %v, want %v", cfg.PeroutPhase, tt.expected.PeroutPhase)
				}
			}
		})
	}
}

func TestParseFlagsHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "short help flag",
			args: []string{"-h"},
		},
		{
			name: "long help flag",
			args: []string{"--help"},
		},
		{
			name: "help with interface",
			args: []string{"-h", "eth0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, help, usageFunc, err := parseFlags("sdp", tt.args)
			
			if !help {
				t.Errorf("expected help flag to be true")
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			
			if usageFunc == nil {
				t.Errorf("usageFunc is nil")
			} else {
				usage := usageFunc("satpulsetool")
				if usage == "" {
					t.Errorf("usage string is empty")
				}
			}
		})
	}
}