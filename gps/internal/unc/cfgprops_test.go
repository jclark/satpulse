package unc

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

type nativeConfigPropsTestCase struct {
	name         string
	currentState []string                     // commands representing current receiver state
	targetProps  func(*gpsprot.ConfigProps)   // function to set up target properties
	targetOpts   func(*gpsprot.ConfigOptions) // function to set up target options (optional)
	expectedCmds []string
}

func testNativeConfigProps(t *testing.T, tests []nativeConfigPropsTestCase) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up current state
			np := makeNativeProps()
			for _, cmd := range tt.currentState {
				// Extract key from command: if starts with CONFIG, use second field, otherwise first field
				fields := strings.Fields(cmd)
				if len(fields) == 0 {
					t.Fatalf("empty command in current state")
				}
				var key string
				if fields[0] == "CONFIG" && len(fields) > 1 {
					key = fields[1]
				} else {
					key = fields[0]
				}

				err := np.updateFromQueryResponse(key, cmd)
				if err != nil {
					t.Fatalf("failed to set up current state for command %s: %v", cmd, err)
				}
			}

			// Set up target properties
			var targetProps gpsprot.ConfigProps
			if tt.targetProps != nil {
				tt.targetProps(&targetProps)
			}

			// Set up target options
			var targetOpts gpsprot.ConfigOptions
			if tt.targetOpts != nil {
				tt.targetOpts(&targetOpts)
			}

			// Create ConfigTarget with the props and options
			target := &gpsprot.ConfigTarget{Props: targetProps, Opts: targetOpts}

			// Generate commands for target
			commands := np.generateTargetCommands(target)

			// Convert expected commands to set for order-independent comparison
			expectedSet := make(map[string]struct{})
			for _, cmd := range tt.expectedCmds {
				expectedSet[cmd] = struct{}{}
			}

			// Check length first
			if len(commands) != len(tt.expectedCmds) {
				t.Errorf("generateConfigCommands() returned %d commands, want %d", len(commands), len(tt.expectedCmds))
				t.Errorf("got: %v", commands)
				t.Errorf("want: %v", tt.expectedCmds)
				return
			}

			// Check that each got command is in expected set and validate ordering
			seenUnmask := false
			for _, nativeCmd := range commands {
				if _, exists := expectedSet[nativeCmd.cmd]; !exists {
					t.Errorf("generateConfigCommands() returned unexpected command: %s", nativeCmd.cmd)
				}

				// Check ordering: MASK commands must come before UNMASK commands
				if strings.HasPrefix(nativeCmd.cmd, "UNMASK") {
					seenUnmask = true
				} else if strings.HasPrefix(nativeCmd.cmd, "MASK") && seenUnmask {
					t.Errorf("generateConfigCommands() returned MASK command after UNMASK command: %s", nativeCmd.cmd)
				}
			}
		})
	}
}

func TestComPropUpdateFromCommand(t *testing.T) {
	tests := []struct {
		name      string
		cmd       string
		expect    [3]uint32
		expectErr bool
	}{
		{
			name:   "plain baud rate",
			cmd:    "CONFIG COM1 115200",
			expect: [3]uint32{115200, 0, 0},
		},
		{
			name:   "baud rate with framing parameters",
			cmd:    "CONFIG COM2 460800 8 n 1",
			expect: [3]uint32{0, 460800, 0},
		},
		{
			name:   "COM3 high speed",
			cmd:    "CONFIG COM3 921600",
			expect: [3]uint32{0, 0, 921600},
		},
		{
			name:   "untracked port ignored",
			cmd:    "CONFIG COM4 115200",
			expect: [3]uint32{},
		},
		{
			name:      "not a COM port config",
			cmd:       "CONFIG PPS DISABLE",
			expectErr: true,
		},
		{
			name:      "missing baud rate",
			cmd:       "CONFIG COM1",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p comProp
			err := p.updateFromCommand(tt.cmd)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.speeds != tt.expect {
				t.Errorf("speeds = %v, want %v", p.speeds, tt.expect)
			}
		})
	}
}
