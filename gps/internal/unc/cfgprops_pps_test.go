package unc

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
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

func TestConfigPPS(t *testing.T) {
	testNativeConfigProps(t, ppsTestCases)
}

var ppsTestCases = []nativeConfigPropsTestCase{
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
		name: "re-enable PPS from disabled with BDS-only signals",
		currentState: []string{
			"CONFIG SIGNALGROUP 8",
			"MASK GPS",
			"MASK GAL",
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
		},
		expectedCmds: []string{"CONFIG PPS ENABLE BDS POSITIVE 100 1000 0 0"},
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
}

