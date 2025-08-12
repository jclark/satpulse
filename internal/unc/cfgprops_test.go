package unc

import (
	"maps"
	"strings"
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
			pps := ppsProp{}
			err := pps.updateFromProps(&expectedProps)
			if err != nil {
				t.Fatalf("updateFromProps failed: %v", err)
			}

			if pps.command != tt.command {
				t.Errorf("updateFromProps command mismatch:\ngot:  %q\nwant: %q", pps.command, tt.command)
			}

			// Test convertToProps: Unicore command -> gpsprot.ConfigProps
			var actualProps gpsprot.ConfigProps
			pps2 := pps
			pps2.command = tt.command // Set command to parse
			pps2.convertToProps(&actualProps)

			// Verify round-trip conversion by comparing actual vs expected
			if actualProps != expectedProps {
				t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", actualProps, expectedProps)
			}
		})
	}
}

func TestPPSUserDelayPreservation(t *testing.T) {
	// Test that userDelay is preserved when cloning and updating props

	// Start with a PPS command that has a non-zero userDelay
	originalPPS := ppsProp{}
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
	cloned := originalPPS

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
	configProps.SetTimeGNSS(gpsprot.BDS)                   // different GNSS
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
			prop := maskProp{signalMask: tc.signalSet}
			prev := maskProp{}

			commands := prop.generateCommands(&prev, gpsprot.SigSetAll)

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
			roundTrip := maskProp{}
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
			name:        "add more masks to existing",
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
			currSignals:  gpsprot.SignalSetOf(gpsprot.SigGPSL1CA),
			expectedCmds: []string{"MASK L1CA", "UNMASK L1C", "UNMASK L2C"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prev := &maskProp{signalMask: tc.prevSignals}
			curr := &maskProp{signalMask: tc.currSignals}

			commands := curr.generateCommands(prev, gpsprot.SigSetAll)

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
	prop := maskProp{}
	if err := prop.updateFromCommand("MASK 5.0"); err != nil {
		t.Fatalf("updateFromCommand failed: %v", err)
	}
	if prop.elevationMask != "5.0" {
		t.Errorf("elevationMask = %q, want %q", prop.elevationMask, "5.0")
	}

	// Test integer elevation mask
	prop2 := maskProp{}
	if err := prop2.updateFromCommand("MASK 10"); err != nil {
		t.Fatalf("updateFromCommand failed: %v", err)
	}
	if prop2.elevationMask != "10" {
		t.Errorf("elevationMask = %q, want %q", prop2.elevationMask, "10")
	}

	// Test elevation mask generation
	prev := maskProp{}
	commands := prop.generateCommands(&prev, gpsprot.SigSetAll)
	expected := []string{"MASK 5.0"}

	// For elevation, order should be deterministic (only one command)
	if len(commands) != len(expected) || commands[0] != expected[0] {
		t.Errorf("generateCommands() = %v, want %v", commands, expected)
	}

	// Test elevation mask change
	prev2 := &maskProp{elevationMask: "10"}
	commands2 := prop.generateCommands(prev2, gpsprot.SigSetAll)
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
	prop := maskProp{}

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
	prop := maskProp{}

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

func TestNativeConfigProps(t *testing.T) {
	tests := []struct {
		name         string
		currentState []string                   // commands representing current receiver state
		targetProps  func(*gpsprot.ConfigProps) // function to set up target properties
		expectedCmds []string
	}{
		{
			name: "enable PPS from disabled state",
			currentState: []string{
				"CONFIG PPS DISABLE",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetTimePulse(gpsprot.TimePulse{
					Width:          100 * time.Microsecond,
					Period:         time.Second,
					PolarityRising: true,
					OnlyWhenLocked: true,
					AlignToGNSS:    true,
				})
				props.SetTimeGNSS(gpsprot.GPS)
			},
			expectedCmds: []string{"CONFIG PPS ENABLE GPS POSITIVE 100 1000 0 0"},
		},
		{
			name: "change PPS from GPS to BDS with cable delay",
			currentState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100 1000 0 0",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetTimePulse(gpsprot.TimePulse{
					Width:          200 * time.Microsecond,
					Period:         2 * time.Second,
					PolarityRising: false,
					OnlyWhenLocked: false,
					AlignToGNSS:    true,
				})
				props.SetTimeGNSS(gpsprot.BDS)
				props.SetAntennaCableDelay(50 * time.Nanosecond)
			},
			expectedCmds: []string{"CONFIG PPS ENABLE2 BDS NEGATIVE 200 2000 50 0"},
		},
		{
			name: "disable PPS",
			currentState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100 1000 0 0",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetTimePulse(gpsprot.TimePulse{
					Width:          0, // Disabled
					Period:         time.Second,
					PolarityRising: true,
					OnlyWhenLocked: true,
					AlignToGNSS:    true,
				})
			},
			expectedCmds: []string{"CONFIG PPS DISABLE"},
		},
		{
			name: "no change needed",
			currentState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100 1000 0 0",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetTimePulse(gpsprot.TimePulse{
					Width:          100 * time.Microsecond,
					Period:         time.Second,
					PolarityRising: true,
					OnlyWhenLocked: true,
					AlignToGNSS:    true,
				})
				props.SetTimeGNSS(gpsprot.GPS)
			},
			expectedCmds: []string{},
		},
		{
			name: "PPS preserving userDelay",
			currentState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100 1000 0 456",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetTimePulse(gpsprot.TimePulse{
					Width:          200 * time.Microsecond, // changed
					Period:         time.Second,
					PolarityRising: false, // changed
					OnlyWhenLocked: true,
					AlignToGNSS:    true,
				})
				props.SetTimeGNSS(gpsprot.BDS)                    // changed
				props.SetAntennaCableDelay(100 * time.Nanosecond) // changed
			},
			expectedCmds: []string{"CONFIG PPS ENABLE BDS NEGATIVE 200 1000 100 456"}, // userDelay 456 preserved
		},
		{
			name: "set just antenna cable delay",
			currentState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100 1000 0 0",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetAntennaCableDelay(75 * time.Nanosecond)
			},
			expectedCmds: []string{"CONFIG PPS ENABLE GPS POSITIVE 100 1000 75 0"},
		},
		{
			name: "set just timeGNSS",
			currentState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100 1000 50 0",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetTimeGNSS(gpsprot.GAL)
			},
			expectedCmds: []string{"CONFIG PPS ENABLE GAL POSITIVE 100 1000 50 0"},
		},
		{
			name: "preserve antenna cable delay when time pulse is set",
			currentState: []string{
				"CONFIG PPS ENABLE BDS NEGATIVE 500 2000 150 0",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetTimePulse(gpsprot.TimePulse{
					Width:          300 * time.Microsecond,
					Period:         500 * time.Millisecond,
					PolarityRising: true,
					OnlyWhenLocked: false,
					AlignToGNSS:    false,
				})
			},
			expectedCmds: []string{"CONFIG PPS ENABLE3 BDS POSITIVE 300 500 150 0"},
		},
		{
			name: "preserve everything when no properties are set",
			currentState: []string{
				"CONFIG PPS ENABLE2 GAL NEGATIVE 750 3000 999 789",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Set no properties - everything should be preserved
			},
			expectedCmds: []string{},
		},
		{
			name: "mask multiple constellations with signal group 2",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetSignalsEnabled(gpsprot.SigSetGPS)
			},
			expectedCmds: []string{
				"MASK GLO",
				"MASK GAL",
				"MASK BDS",
				"MASK QZSS",
				"MASK IRNSS",
			},
		},
		{
			name: "mask multiple constellations with signal group 1",
			currentState: []string{
				"CONFIG SIGNALGROUP 1",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetSignalsEnabled(gpsprot.SigSetBDS)
			},
			expectedCmds: []string{
				"MASK GPS",
				"MASK GAL",
				"MASK GLO",
				"MASK QZSS",
				// no MASK IRNSS since signal group 1 does not include IRNSS
			},
		},
		{
			name: "--gnss GPS,BDS --band L1 equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // has GPS, BDS, GAL, GLO, QZSS, NAVIC
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS,BDS --band L1
				props.SetSignalsEnabled(gpsprot.BandL1.SignalSet(gpsprot.GPS, gpsprot.BDS))
			},
			expectedCmds: []string{
				"MASK L2", "MASK L5", // mask GPS non-L1 frequencies
				"MASK B2", "MASK B3", // mask BDS non-L1 frequencies
				"MASK GLO", "MASK GAL", "MASK QZSS", "MASK IRNSS", // mask entire constellations not in target
			},
		},
		{
			name: "--gnss GPS,GAL --band L5 equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 1", // has GPS, BDS, GAL, GLO, QZSS (no NAVIC)
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS,GAL --band L5
				props.SetSignalsEnabled(gpsprot.BandL5.SignalSet(gpsprot.GPS, gpsprot.GAL))
			},
			expectedCmds: []string{
				"MASK L1", "MASK L2", // mask GPS non-L5 frequencies
				"MASK E1", "MASK E5b", // mask Galileo non-L5 frequencies (E6 not in signal group 1)
				"MASK BDS", "MASK GLO", "MASK QZSS", // mask entire constellations not in target
			},
		},
		{
			name: "--gnss GPS --band E5 equivalent (L5+E5b)",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS --band E5 (which is BandL5 | BandE5b)
				props.SetSignalsEnabled((gpsprot.BandL5 | gpsprot.BandE5b).SignalSet(gpsprot.GPS))
			},
			expectedCmds: []string{
				"MASK L1", "MASK L2", // mask GPS non-L5 frequencies (GPS doesn't have E5b signals)
				"MASK GLO", "MASK BDS", "MASK GAL", "MASK QZSS", "MASK IRNSS", // mask all other constellations
			},
		},
		{
			name: "current state has constellation masked, target wants it - --gnss GPS,BDS equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // has GPS, BDS, GAL, GLO, QZSS, NAVIC
				"MASK GPS",             // GPS is currently masked
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS,BDS (normal case - target constellations)
				props.SetSignalsEnabled(gpsprot.SigSetGPS | gpsprot.SigSetBDS)
			},
			expectedCmds: []string{
				"UNMASK GPS",                                      // need to unmask GPS constellation
				"MASK GAL", "MASK GLO", "MASK QZSS", "MASK IRNSS", // mask unwanted constellations
			},
		},
		{
			name: "normal case - --gnss GPS,GAL equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 1", // has GPS, BDS, GAL, GLO, QZSS
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS,GAL (normal constellation targeting)
				props.SetSignalsEnabled(gpsprot.SigSetGPS | gpsprot.SigSetGAL)
			},
			expectedCmds: []string{
				"MASK BDS", "MASK GLO", "MASK QZSS", // mask unwanted constellations
			},
		},
		{
			name: "normal case - --gnss BDS equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // has GPS, BDS, GAL, GLO, QZSS, NAVIC
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss BDS (single constellation)
				props.SetSignalsEnabled(gpsprot.SigSetBDS)
			},
			expectedCmds: []string{
				"MASK GPS", "MASK GAL", "MASK GLO", "MASK QZSS", "MASK IRNSS", // mask unwanted constellations
			},
		},
		{
			name: "multiple constellations masked, target wants subset",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK GPS", "MASK GAL", // two constellations masked
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Want GPS and BDS, so need to unmask GPS but keep GAL masked
				props.SetSignalsEnabled(gpsprot.SigSetGPS | gpsprot.SigSetBDS)
			},
			expectedCmds: []string{
				// Note: MASK GAL is generated even though it's already masked (implementation doesn't optimize this)
				"MASK GAL", "MASK GLO", "MASK QZSS", "MASK IRNSS", // mask unwanted constellations
				"UNMASK GPS", // unmask GPS since we want it
			},
		},
		{
			name: "current state has frequency masked, target needs it - --gnss GPS,BDS --band L1,L5 equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK L5", // L5 frequency is masked across all constellations
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS,BDS --band L1,L5
				props.SetSignalsEnabled((gpsprot.BandL1 | gpsprot.BandL5).SignalSet(gpsprot.GPS, gpsprot.BDS))
			},
			expectedCmds: []string{
				"MASK L2",                            // mask GPS L2
				"MASK B2I", "MASK BD3B2B", "MASK B3", // mask unwanted BDS frequencies
				"MASK GAL", "MASK GLO", "MASK QZSS", "MASK IRNSS", // mask unwanted constellations
				"UNMASK L5", // need to unmask L5 frequency
			},
		},
		{
			name: "current state has mixed masks - --gnss GAL,QZSS --band L1,E6 equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK E1",   // specific Galileo signal masked
				"MASK QZSS", // whole QZSS constellation masked
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GAL,QZSS --band L1,E6
				props.SetSignalsEnabled((gpsprot.BandL1 | gpsprot.BandE6).SignalSet(gpsprot.GAL, gpsprot.QZSS))
			},
			expectedCmds: []string{
				"MASK E5a", "MASK E5b", // mask non-target Galileo frequencies
				"MASK Q2", "MASK Q5", // mask non-target QZSS frequencies (want L1 and L6/E6)
				"MASK GPS", "MASK BDS", "MASK GLO", "MASK IRNSS", // mask unwanted constellations
				"UNMASK E1", // need to unmask specific Galileo E1 signal (it's on L1 band)
				"UNMASK Q1", // need to unmask QZSS Q1 (L1 band signals)
			},
		},
		{
			name: "signal group 8 - --gnss GPS equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // GPS, BDS (limited), GAL - no GLO, QZSS, NAVIC
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS (single constellation from group 8)
				props.SetSignalsEnabled(gpsprot.SigSetGPS)
			},
			expectedCmds: []string{
				"MASK BDS", "MASK GAL", // mask other constellations in signal group 8
			},
		},
		{
			name: "signal group 8 - --gnss BDS,GAL equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // GPS, BDS, GAL only
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss BDS,GAL (subset of group 8)
				props.SetSignalsEnabled(gpsprot.SigSetBDS | gpsprot.SigSetGAL)
			},
			expectedCmds: []string{
				"MASK GPS", // mask GPS, keep BDS and GAL
			},
		},
		{
			name: "signal group 10 - --gnss GPS,QZSS equivalent",
			currentState: []string{
				"CONFIG SIGNALGROUP 10", // includes QZSS L6, full constellation set
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Equivalent to: --gnss GPS,QZSS (includes QZSS L6 support)
				props.SetSignalsEnabled(gpsprot.SigSetGPS | gpsprot.SigSetQZSS)
			},
			expectedCmds: []string{
				"MASK BDS", "MASK GLO", "MASK GAL", // mask unwanted constellations
				// QZSS includes L6 in signal group 10
			},
		},
		{
			name: "signal group 10 with specific frequency mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 10",
				"MASK Q5", // QZSS L5 is masked
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Want all QZSS signals including L6 (which is only in group 10)
				props.SetSignalsEnabled(gpsprot.SigSetQZSS)
			},
			expectedCmds: []string{
				"MASK GPS", "MASK BDS", "MASK GLO", "MASK GAL", // mask unwanted constellations first
				"UNMASK Q5", // then unmask QZSS L5 (UNMASK comes after MASK)
			},
		},
		{
			name: "target single specific signal - GPS L5 only",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // has full constellation set
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Target only GPS L5 signal
				props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL5))
			},
			expectedCmds: []string{
				"MASK L1", "MASK L2", // mask other GPS frequencies
				"MASK BDS", "MASK GLO", "MASK GAL", "MASK QZSS", "MASK IRNSS", // mask all other constellations
			},
		},
		{
			name: "target single specific signal - BDS B1I only",
			currentState: []string{
				"CONFIG SIGNALGROUP 1", // has BDS support
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Target only BDS B1I signal
				props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigBDSB1I))
			},
			expectedCmds: []string{
				"MASK BD3B1C", "MASK B2", "MASK B3", // mask other BDS frequencies (keep only B1I)
				"MASK GPS", "MASK GLO", "MASK GAL", "MASK QZSS", // mask all other constellations
			},
		},
		{
			name: "target specific signal with MASK/UNMASK overlap",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK GPS", // GPS system is masked
				"MASK L5",  // L5 frequency is separately masked (redundant but possible)
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Target only GPS L5 - need to unmask both GPS and L5
				props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL5))
			},
			expectedCmds: []string{
				// First all MASK commands are generated
				"MASK BDS", "MASK GLO", "MASK GAL", "MASK QZSS", "MASK IRNSS", // mask other constellations
				"MASK L1", "MASK L2", // mask other GPS frequencies
				// Then all UNMASK commands (UNMASK always comes after MASK)
				"UNMASK L5", // unmask L5 frequency (GPS system unmask not generated when we target specific signals)
			},
		},
		{
			name: "target specific signal with frequency conflict",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK GAL", // But also whole Galileo system is masked
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Target only Galileo E1 signal
				props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGALE1))
			},
			expectedCmds: []string{
				// MASK commands first
				"MASK GPS", "MASK BDS", "MASK GLO", "MASK QZSS", "MASK IRNSS", // mask other constellations
				"MASK E5a", "MASK E5b", "MASK E6C", // mask other Galileo frequencies
				// UNMASK commands after (only E1 since implementation doesn't generate UNMASK GAL for specific signals)
				"UNMASK E1", // unmask E1 frequency
			},
		},
		{
			name: "target multiple specific signals from different constellations",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK L1",  // GPS L1 frequencies masked
				"MASK BDS", // BDS constellation masked
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Target GPS L1C/A and BDS B2a specifically
				props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigBDSB2a))
			},
			expectedCmds: []string{
				// MASK commands
				"MASK GLO", "MASK GAL", "MASK QZSS", "MASK IRNSS", // mask unwanted constellations
				"MASK L1C", "MASK L2", "MASK L5", // mask unwanted GPS frequencies (keep only L1CA)
				"MASK B1", "MASK B3", "MASK B2I", "MASK BD3B2B", // mask unwanted BDS frequencies (keep only B2a)
				// UNMASK commands after
				"UNMASK L1CA", "UNMASK BD3B2A", // unmask specific GPS L1C/A and BDS B2a
			},
		},
	}

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
			tt.targetProps(&targetProps)

			// Generate commands for target
			commands := np.generateConfigCommands(&targetProps)

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
			for _, cmd := range commands {
				if _, exists := expectedSet[cmd]; !exists {
					t.Errorf("generateConfigCommands() returned unexpected command: %s", cmd)
				}
				
				// Check ordering: MASK commands must come before UNMASK commands
				if strings.HasPrefix(cmd, "UNMASK") {
					seenUnmask = true
				} else if strings.HasPrefix(cmd, "MASK") && seenUnmask {
					t.Errorf("generateConfigCommands() returned MASK command after UNMASK command: %s", cmd)
				}
			}
		})
	}
}
