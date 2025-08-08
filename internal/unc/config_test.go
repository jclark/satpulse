package unc

import (
	"maps"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestPPS(t *testing.T) {
	const (
		// Sentinel values to indicate unset properties in test cases
		// These allow us to distinguish between "should be set to zero value" vs "should not be set at all"
		unsetGNSS       = gpsprot.GNSS(0) // Invalid GNSS value (0 is not a valid GNSS constellation)
		unsetCableDelay = -time.Second    // Out of range cable delay (negative values are invalid for cable delay)
	)
	tests := []struct {
		name              string
		timePulse         gpsprot.TimePulse
		antennaCableDelay time.Duration
		timeGNSS          gpsprot.GNSS
		command           string
	}{
		{
			name: "disabled PPS",
			timePulse: gpsprot.TimePulse{
				Width:          0, // Disabled
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: unsetCableDelay, // unset for DISABLE
			timeGNSS:          unsetGNSS,       // unset for DISABLE
			command:           "CONFIG PPS DISABLE",
		},
		{
			name: "basic GPS PPS",
			timePulse: gpsprot.TimePulse{
				Width:          time.Millisecond,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 0,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE GPS POSITIVE 1000 1000 0 0",
		},
		{
			name: "Galileo PPS with cable delay",
			timePulse: gpsprot.TimePulse{
				Width:          500 * time.Microsecond,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 50 * time.Nanosecond,
			timeGNSS:          gpsprot.GAL,
			command:           "CONFIG PPS ENABLE GAL POSITIVE 500 1000 50 0",
		},
		{
			name: "BDS negative polarity unlocked",
			timePulse: gpsprot.TimePulse{
				Width:          10 * time.Millisecond,
				Period:         2 * time.Second,
				PolarityRising: false,
				OnlyWhenLocked: false,
				AlignToGNSS:    true,
			},
			antennaCableDelay: -100 * time.Nanosecond,
			timeGNSS:          gpsprot.BDS,
			command:           "CONFIG PPS ENABLE2 BDS NEGATIVE 10000 2000 -100 0",
		},
		{
			name: "manual example",
			timePulse: gpsprot.TimePulse{
				Width:          time.Second / 2,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 0,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0",
		},
		{
			name: "typical example",
			timePulse: gpsprot.TimePulse{
				Width:          time.Second / 10,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 50,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE GPS POSITIVE 100000 1000 50 0",
		},
		{
			name: "ENABLE3",
			timePulse: gpsprot.TimePulse{
				Width:          time.Second / 10,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: false,
				AlignToGNSS:    false,
			},
			antennaCableDelay: 0,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE3 GPS POSITIVE 100000 1000 0 0",
		},
		{
			name: "GLONASS high frequency",
			timePulse: gpsprot.TimePulse{
				Width:          time.Microsecond,
				Period:         100 * time.Millisecond,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 32767 * time.Nanosecond, // max int16
			timeGNSS:          gpsprot.GLO,
			command:           "CONFIG PPS ENABLE GLO POSITIVE 1 100 32767 0",
		},
	}

	props := NewNativeConfigProps()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Construct expected ConfigProps using conditional logic once
			var expectedProps gpsprot.ConfigProps
			expectedProps.SetTimePulse(tt.timePulse)
			if tt.timeGNSS != unsetGNSS {
				expectedProps.SetTimeGNSS(tt.timeGNSS)
			}
			if tt.antennaCableDelay != unsetCableDelay {
				expectedProps.SetAntennaCableDelay(tt.antennaCableDelay)
			}

			// Test updateFromProps: Unicore command <- gpsprot.ConfigProps 
			pps := props[idPropPPS].clone().(*ppsProp)
			err := pps.updateFromProps(&expectedProps)
			if err != nil {
				t.Fatalf("updateFromProps failed: %v", err)
			}

			if pps.command != tt.command {
				t.Errorf("updateFromProps command mismatch:\ngot:  %q\nwant: %q", pps.command, tt.command)
			}

			// Test convertToProps: Unicore command -> gpsprot.ConfigProps
			var actualProps gpsprot.ConfigProps
			pps2 := pps.clone().(*ppsProp)
			pps2.command = tt.command // Set command to parse
			err = pps2.convertToProps(&actualProps)
			if err != nil {
				t.Fatalf("convertToProps failed: %v", err)
			}

			// Verify round-trip conversion by comparing actual vs expected
			if actualProps != expectedProps {
				t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", actualProps, expectedProps)
			}
		})
	}
}

func TestPPSUserDelayPreservation(t *testing.T) {
	// Test that userDelay is preserved when cloning and updating props
	props := NewNativeConfigProps()
	
	// Start with a PPS command that has a non-zero userDelay
	originalPPS := props[idPropPPS].clone().(*ppsProp)
	err := originalPPS.updateFromCommand("CONFIG PPS ENABLE GPS POSITIVE 1000 1000 0 123")
	if err != nil {
		t.Fatalf("updateFromCommand failed: %v", err)
	}
	
	// Verify the command was stored correctly
	expectedOriginalCommand := "CONFIG PPS ENABLE GPS POSITIVE 1000 1000 0 123"
	if originalPPS.command != expectedOriginalCommand {
		t.Fatalf("command not stored correctly: got %q, want %q", originalPPS.command, expectedOriginalCommand)
	}
	
	// Clone the property
	cloned := originalPPS.clone().(*ppsProp)
	
	// Verify command was preserved in clone
	if cloned.command != expectedOriginalCommand {
		t.Fatalf("command not preserved in clone: got %q, want %q", cloned.command, expectedOriginalCommand)
	}
	
	// Now update from props - userDelay should be preserved from the existing command
	var configProps gpsprot.ConfigProps
	timePulse := gpsprot.TimePulse{
		Width:          2 * time.Millisecond, // different width
		Period:         time.Second,
		PolarityRising: false, // different polarity
		OnlyWhenLocked: true,
		AlignToGNSS:    true,
	}
	configProps.SetTimePulse(timePulse)
	configProps.SetTimeGNSS(gpsprot.BDS) // different GNSS
	configProps.SetAntennaCableDelay(50 * time.Nanosecond) // different delay
	
	err = cloned.updateFromProps(&configProps)
	if err != nil {
		t.Fatalf("updateFromProps failed: %v", err)
	}
	
	// The command should have updated values but preserved userDelay (123)
	expectedCommand := "CONFIG PPS ENABLE BDS NEGATIVE 2000 1000 50 123"
	if cloned.command != expectedCommand {
		t.Errorf("updateFromProps did not preserve userDelay:\ngot:  %q\nwant: %q", cloned.command, expectedCommand)
	}
}

func TestMaskPropRoundTrip(t *testing.T) {
	maskTestCases := []struct {
		name      string
		signalSet gpsprot.SignalSet
		commands  []string // Expected command set (order doesn't matter)
	}{
		{
			name:      "empty mask",
			signalSet: 0,
			commands:  []string{},
		},
		{
			name: "mask entire GPS system",
			signalSet: gpsprot.SignalSetOf(
				gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2C,
				gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
			),
			commands: []string{"MASK GPS"},
		},
		{
			name:      "mask GPS L1 frequencies",
			signalSet: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C),
			commands:  []string{"MASK L1"},
		},
		{
			name:      "mask individual GPS frequencies",
			signalSet: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C),
			commands:  []string{"MASK L1CA", "MASK L2C"},
		},
		{
			name: "mask multiple systems",
			signalSet: gpsprot.SignalSetOf(
				gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2C,
				gpsprot.SigGPSL2P, gpsprot.SigGPSL5, // All GPS
				gpsprot.SigGLOL1, gpsprot.SigGLOL2, gpsprot.SigGLOL3, // All GLONASS
			),
			commands: []string{"MASK GPS", "MASK GLO"},
		},
		{
			name: "mask mixed signals requiring optimization",
			signalSet: gpsprot.SignalSetOf(
				gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, // L1 superset
				gpsprot.SigBDSB1I, gpsprot.SigBDSB1C, // B1 superset
				gpsprot.SigGALE1, // Individual signal
			),
			commands: []string{"MASK L1", "MASK B1", "MASK E1"},
		},
		{
			name: "mask BDS B2 frequencies",
			signalSet: gpsprot.SignalSetOf(
				gpsprot.SigBDSB2I, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
			),
			commands: []string{"MASK B2"},
		},
		{
			name: "mask partial BDS frequencies",
			signalSet: gpsprot.SignalSetOf(
				gpsprot.SigBDSB1I, gpsprot.SigBDSB2I,
			),
			commands: []string{"MASK B1I", "MASK B2I"},
		},
	}

	for _, tc := range maskTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test command generation
			prop := &maskProp{signalMask: tc.signalSet}
			prev := newMaskProp()

			commands := prop.generateCommands(prev)

			// Convert to sets for comparison
			gotSet := make(map[string]bool)
			for _, cmd := range commands {
				gotSet[cmd] = true
			}

			wantSet := make(map[string]bool)
			for _, cmd := range tc.commands {
				wantSet[cmd] = true
			}

			// Verify expected commands as sets
			if !maps.Equal(gotSet, wantSet) {
				t.Errorf("generateCommands() = %v, want %v", commands, tc.commands)
			}

			// Test round-trip: parse commands back
			roundTrip := newMaskProp()
			for _, cmd := range commands {
				if err := roundTrip.updateFromCommand(cmd); err != nil {
					t.Fatalf("updateFromCommand(%q) failed: %v", cmd, err)
				}
			}

			// Verify round-trip result
			if roundTrip.signalMask != tc.signalSet {
				t.Errorf("Round-trip failed: got signalMask %#x, want %#x",
					roundTrip.signalMask, tc.signalSet)
			}
		})
	}
}

func TestMaskPropDifferentialUpdate(t *testing.T) {
	testCases := []struct {
		name         string
		prevSignals  gpsprot.SignalSet
		currSignals  gpsprot.SignalSet
		expectedCmds []string
	}{
		{
			name:         "add masks to empty",
			prevSignals:  0,
			currSignals:  gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C),
			expectedCmds: []string{"MASK L1"},
		},
		{
			name:         "remove all masks",
			prevSignals:  gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C),
			currSignals:  0,
			expectedCmds: []string{"UNMASK L1"},
		},
		{
			name: "change from GPS to BDS",
			prevSignals: gpsprot.SignalSetOf(
				gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2C,
				gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
			),
			currSignals: gpsprot.SignalSetOf(
				gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I,
				gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
			),
			expectedCmds: []string{"MASK BDS", "UNMASK GPS"},
		},
		{
			name:         "partial overlap",
			prevSignals:  gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C),
			currSignals:  gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL5),
			expectedCmds: []string{"MASK L1CA", "MASK L5", "UNMASK L2C"},
		},
		{
			name: "add more masks to existing",
			prevSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA),
			currSignals: gpsprot.SignalSetOf(
				gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2C,
			),
			expectedCmds: []string{"MASK L1", "MASK L2C"},
		},
		{
			name: "remove some masks",
			prevSignals: gpsprot.SignalSetOf(
				gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2C,
			),
			currSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA),
			expectedCmds: []string{"MASK L1CA", "UNMASK L1C", "UNMASK L2C"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prev := &maskProp{signalMask: tc.prevSignals}
			curr := &maskProp{signalMask: tc.currSignals}

			commands := curr.generateCommands(prev)

			// Convert to sets for comparison
			gotSet := make(map[string]bool)
			for _, cmd := range commands {
				gotSet[cmd] = true
			}

			wantSet := make(map[string]bool)
			for _, cmd := range tc.expectedCmds {
				wantSet[cmd] = true
			}

			if !maps.Equal(gotSet, wantSet) {
				t.Errorf("generateCommands() = %v, want %v", commands, tc.expectedCmds)
			}
		})
	}
}

func TestMaskPropElevation(t *testing.T) {
	// Test elevation mask parsing
	prop := newMaskProp()
	if err := prop.updateFromCommand("MASK 5.0"); err != nil {
		t.Fatalf("updateFromCommand failed: %v", err)
	}
	if prop.elevationMask != "5.0" {
		t.Errorf("elevationMask = %q, want %q", prop.elevationMask, "5.0")
	}

	// Test integer elevation mask
	prop2 := newMaskProp()
	if err := prop2.updateFromCommand("MASK 10"); err != nil {
		t.Fatalf("updateFromCommand failed: %v", err)
	}
	if prop2.elevationMask != "10" {
		t.Errorf("elevationMask = %q, want %q", prop2.elevationMask, "10")
	}

	// Test elevation mask generation
	prev := newMaskProp()
	commands := prop.generateCommands(prev)
	expected := []string{"MASK 5.0"}

	// For elevation, order should be deterministic (only one command)
	if len(commands) != len(expected) || commands[0] != expected[0] {
		t.Errorf("generateCommands() = %v, want %v", commands, expected)
	}

	// Test elevation mask change
	prev2 := &maskProp{elevationMask: "10"}
	commands2 := prop.generateCommands(prev2)
	expected2 := []string{"MASK 5.0"}
	if len(commands2) != len(expected2) || commands2[0] != expected2[0] {
		t.Errorf("generateCommands() = %v, want %v", commands2, expected2)
	}

	// Verify UNMASK elevation is rejected by regex
	if err := prop.updateFromCommand("UNMASK 5.0"); err == nil {
		t.Error("UNMASK elevation should fail but didn't")
	}
}

func TestMaskPropPRN(t *testing.T) {
	// Test that PRN masks are accepted but ignored
	prop := newMaskProp()
	
	// Should parse without error
	if err := prop.updateFromCommand("MASK GPS PRN 10"); err != nil {
		t.Fatalf("updateFromCommand failed: %v", err)
	}
	
	// Should not affect signalMask
	if prop.signalMask != 0 {
		t.Errorf("PRN mask should be ignored, but signalMask = %#x", prop.signalMask)
	}
	
	// Test UNMASK PRN
	if err := prop.updateFromCommand("UNMASK GPS PRN 10"); err != nil {
		t.Fatalf("updateFromCommand failed: %v", err)
	}
	
	if prop.signalMask != 0 {
		t.Errorf("PRN unmask should be ignored, but signalMask = %#x", prop.signalMask)
	}
}

func TestMaskPropInvalidCommands(t *testing.T) {
	prop := newMaskProp()
	
	invalidCommands := []string{
		"MASK",           // Missing argument
		"UNMASK",         // Missing argument
		"MASK INVALID",   // Unknown system/frequency
		"UNMASK INVALID", // Unknown system/frequency
		"MASK GPS BDS",   // Multiple systems
		"ENABLE GPS",     // Wrong command
	}
	
	for _, cmd := range invalidCommands {
		if err := prop.updateFromCommand(cmd); err == nil {
			t.Errorf("updateFromCommand(%q) should have failed but didn't", cmd)
		}
	}
}

