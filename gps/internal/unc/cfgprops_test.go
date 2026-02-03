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
