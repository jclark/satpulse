package unc

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestConfigMode(t *testing.T) {
	testNativeConfigProps(t, modeTestCases)
}

var modeTestCases = []nativeConfigPropsTestCase{
	{
		name: "SetStatic with RTCMBaseID when already in survey mode",
		currentState: []string{
			"MODE BASE TIME 2000 0 0",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetRTCMBaseID(1234)
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.SetStatic = true
			opts.Survey.MinDur = 2000 * time.Second
		},
		expectedCmds: []string{"MODE BASE 1234 TIME 2000 0 0"},
	},
	{
		name: "Change RTCMBaseID in survey mode",
		currentState: []string{
			"MODE BASE 123 TIME 2000",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeNone})
			props.SetRTCMBaseID(456)
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 2000 * time.Second
		},
		expectedCmds: []string{"MODE BASE 456 TIME 2000"},
	},
	{
		name: "SetStatic preserves existing RTCMBaseID when not specified",
		currentState: []string{
			"MODE BASE 123 TIME 2000",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			// No RTCMBaseID set - should preserve existing
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.SetStatic = true
			opts.Survey.MinDur = 2000 * time.Second
		},
		expectedCmds: []string{}, // No change needed
	},
	{
		name: "SetStatic true forces survey mode",
		currentState: []string{
			"MODE ROVER SURVEY",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			// Don't set any mode properties - let SetStatic=true drive the behavior
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.SetStatic = true
			opts.Survey.MinDur = 60 * time.Second
		},
		expectedCmds: []string{"MODE BASE TIME 60"},
	},
	{
		name: "Survey configuration with explicit survey parameters",
		currentState: []string{
			"MODE ROVER",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone, // Survey mode
			})
			props.SetRTCMBaseID(123)
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 120 * time.Second
		},
		expectedCmds: []string{"MODE BASE 123 TIME 120"},
	},
	{
		name: "MODE fixed LLH coordinates",
		currentState: []string{
			"MODE ROVER",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeLLH,
				FixedPosLLH: [2]gpsprot.Angle{
					gpsprot.DegreesFromFloat(40.45628476579),
					gpsprot.DegreesFromFloat(116.2859754968),
				},
				Height: gpsprot.Meters(58.0984),
			})
			props.SetRTCMBaseID(456)
		},
		expectedCmds: []string{"MODE BASE 456 40.45628476600 116.28597549700 58.098400"},
	},
	{
		name: "MODE fixed ECEF coordinates",
		currentState: []string{
			"MODE ROVER SURVEY",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeECEF,
				FixedPosECEF: gpsprot.Point3D{
					gpsprot.Meters(-2160489.0276),
					gpsprot.Meters(4383620.1006),
					gpsprot.Meters(4084738.1110),
				},
			})
		},
		expectedCmds: []string{"MODE BASE -2160489.0276 4383620.1006 4084738.1110"},
	},
	{
		name: "MODE change from BASE to ROVER",
		currentState: []string{
			"MODE BASE 123 TIME 60",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static: false,
			})
		},
		expectedCmds: []string{"MODE ROVER"},
	},
	{
		name: "MODE preserve existing ROVER qualifiers",
		currentState: []string{
			"MODE ROVER SURVEY MOW",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static: false,
			})
		},
		expectedCmds: []string{}, // Should preserve existing ROVER SURVEY MOW
	},
	{
		name: "MODE no change needed - already correct BASE mode",
		currentState: []string{
			"MODE BASE 123 TIME 60",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone,
			})
			props.SetRTCMBaseID(123)
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 60 * time.Second
		},
		expectedCmds: []string{}, // No change needed
	},
	{
		name: "MODE BASE without ID - survey mode",
		currentState: []string{
			"MODE ROVER",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone, // Survey mode
			})
			// No RTCM base ID set
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 30 * time.Second
		},
		expectedCmds: []string{"MODE BASE TIME 30"},
	},
	{
		name: "MODE BASE without ID - fixed coordinates",
		currentState: []string{
			"MODE ROVER UAV",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeECEF,
				FixedPosECEF: gpsprot.Point3D{
					gpsprot.Meters(1000000.0),
					gpsprot.Meters(2000000.0),
					gpsprot.Meters(3000000.0),
				},
			})
			// No RTCM base ID set
		},
		expectedCmds: []string{"MODE BASE 1000000.0000 2000000.0000 3000000.0000"},
	},
	{
		name: "SetStatic rounds fixed coordinates",
		currentState: []string{
			"MODE BASE 123 40.12345678901 116.98765432109 100.123456",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			// Don't set any mode properties - SetStatic should preserve existing fixed coordinates
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.SetStatic = true
		},
		expectedCmds: []string{"MODE BASE 123 40.12345678900 116.98765432100 100.123456"}, // Rounded to 9 digits
	},
	{
		name: "SetStatic preserves fixed coordinates with 9 decimal places",
		currentState: []string{
			"MODE BASE 123 40.12345678900 116.98765432100 100.123456",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			// Don't set any mode properties - SetStatic should preserve existing fixed coordinates
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.SetStatic = true
		},
		expectedCmds: []string{}, // No change
	},
	{
		name: "Survey mode unchanged",
		currentState: []string{
			"MODE BASE TIME 2000",
		},
		targetProps:  func(props *gpsprot.ConfigProps) {},
		expectedCmds: []string{}, // No change needed
	},
	{
		name: "MODE preserve survey distance parameter when time changes",
		currentState: []string{
			"MODE BASE 456 TIME 120 5", // TIME with distance parameter (5)
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone, // Survey mode
			})
			props.SetRTCMBaseID(456)
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 180 * time.Second  // Change time from 120 to 180
			opts.Survey.Flags = gpsprot.SurveyAgain // Force new survey with new duration
		},
		expectedCmds: []string{"MODE BASE 456 TIME 180 5"}, // Preserves distance parameter (5)
	},
	{
		name: "MODE HEADING2 behaves like ROVER from gpsprot perspective",
		currentState: []string{
			"MODE HEADING2 STATIC",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static: false, // Should result in maintaining HEADING2 mode
			})
		},
		expectedCmds: []string{}, // Should preserve existing HEADING2 STATIC
	},
	{
		name: "SetStatic=false when already in survey mode should not preserve survey parameters",
		currentState: []string{
			"MODE BASE 123 TIME 180 5", // Currently in survey mode with 180 seconds
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			// Explicitly set non-static mode (ROVER)
			props.SetMode(gpsprot.Mode{
				Static:  false,
				PosType: gpsprot.PosTypeNone, // Should be None when Static=false
			})
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			// Even though we have survey options, they shouldn't matter since we're going to ROVER
			opts.Survey.MinDur = 300 * time.Second
		},
		expectedCmds: []string{"MODE ROVER"},
	},
	{
		name: "Survey to survey with same time but SurveyAgain not set should not regenerate MODE",
		currentState: []string{
			"MODE BASE 123 TIME 60", // Currently in survey mode with 60 seconds
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone, // Survey mode
			})
			props.SetRTCMBaseID(123) // Same base ID
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 60 * time.Second // Same survey duration
			// Note: SurveyAgain is NOT set
		},
		expectedCmds: []string{}, // Should NOT regenerate MODE command (would restart survey)
	},
	{
		name: "Survey to survey with same time but SurveyAgain IS set should regenerate MODE",
		currentState: []string{
			"MODE BASE 123 TIME 60", // Currently in survey mode with 60 seconds
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone, // Survey mode
			})
			props.SetRTCMBaseID(123) // Same base ID
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 60 * time.Second   // Same survey duration
			opts.Survey.Flags = gpsprot.SurveyAgain // Force new survey
		},
		expectedCmds: []string{"MODE BASE 123 TIME 60"}, // Should regenerate MODE to restart survey
	},
	{
		name: "SetStatic with explicit fixed coordinates should use those coordinates",
		currentState: []string{
			"MODE ROVER", // Currently in non-static mode
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeECEF,
				FixedPosECEF: gpsprot.Point3D{
					gpsprot.Meters(1000000.0),
					gpsprot.Meters(2000000.0),
					gpsprot.Meters(3000000.0),
				},
			})
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.SetStatic = true                  // Redundant with Mode.Static=true but should not interfere
			opts.Survey.MinDur = 120 * time.Second // Should be ignored since we have fixed coords
		},
		expectedCmds: []string{"MODE BASE 1000000.0000 2000000.0000 3000000.0000"},
	},
	{
		name: "Transition from survey to fixed position should use new coordinates",
		currentState: []string{
			"MODE BASE 123 TIME 180 5", // Currently in survey mode
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeLLH,
				FixedPosLLH: [2]gpsprot.Angle{
					gpsprot.DegreesFromFloat(40.0),
					gpsprot.DegreesFromFloat(116.0),
				},
				Height: gpsprot.Meters(50.0),
			})
			props.SetRTCMBaseID(456) // Different base ID
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			// Survey options shouldn't matter since we're going to fixed position
			opts.Survey.MinDur = 60 * time.Second
		},
		expectedCmds: []string{"MODE BASE 456 40.00000000000 116.00000000000 50.000000"},
	},
	{
		name: "Survey time clamped to 3600 for UM980",
		currentState: []string{
			"MODE ROVER",
		},
		targetProps: func(props *gpsprot.ConfigProps) {
			props.SetMode(gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone,
			})
		},
		targetOpts: func(opts *gpsprot.ConfigOptions) {
			opts.Survey.MinDur = 5000 * time.Second
		},
		expectedCmds: []string{"MODE BASE TIME 3600"},
	},
}


func TestModeRegexp(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		expectMatch  bool
		expectGroups map[int]string // capture group index -> expected value
	}{
		// ROVER variants
		{
			name:         "ROVER basic",
			command:      "MODE ROVER",
			expectMatch:  true,
			expectGroups: map[int]string{1: "ROVER"},
		},
		{
			name:         "ROVER SURVEY MOW",
			command:      "MODE ROVER SURVEY MOW",
			expectMatch:  true,
			expectGroups: map[int]string{1: "ROVER SURVEY MOW"},
		},
		{
			name:         "ROVER UAV HIGHDYN",
			command:      "MODE ROVER UAV HIGHDYN",
			expectMatch:  true,
			expectGroups: map[int]string{1: "ROVER UAV HIGHDYN"},
		},

		// HEADING2 variants
		{
			name:         "HEADING2 basic",
			command:      "MODE HEADING2",
			expectMatch:  true,
			expectGroups: map[int]string{2: "HEADING2"},
		},
		{
			name:         "HEADING2 STATIC",
			command:      "MODE HEADING2 STATIC",
			expectMatch:  true,
			expectGroups: map[int]string{2: "HEADING2 STATIC"},
		},

		// BASE variants - no parameters
		{
			name:         "BASE default",
			command:      "MODE BASE",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE"},
		},

		// BASE with ID only
		{
			name:         "BASE with ID 123",
			command:      "MODE BASE 123",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE", 4: "123"},
		},

		// BASE with TIME (survey)
		{
			name:         "BASE TIME 60",
			command:      "MODE BASE TIME 60",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE", 5: "60"},
		},
		{
			name:         "BASE ID TIME distance",
			command:      "MODE BASE 123 TIME 60 5",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE", 4: "123", 5: "60", 6: "5"},
		},
		{
			name:         "BASE ID TIME extra param",
			command:      "MODE BASE 123 TIME 60 0 0",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE", 4: "123", 5: "60", 6: "0 0"},
		},

		// BASE with coordinates
		{
			name:         "BASE coords",
			command:      "MODE BASE 40.0 116.0 50.0",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE", 7: "40.0", 8: "116.0", 9: "50.0"},
		},
		{
			name:         "BASE ID coords",
			command:      "MODE BASE 123 -2160489.0 4383620.1 4084738.1",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE", 4: "123", 7: "-2160489.0", 8: "4383620.1", 9: "4084738.1"},
		},
		{
			name:         "BASE with ID and coords",
			command:      "MODE BASE 1 2 3 4",
			expectMatch:  true,
			expectGroups: map[int]string{3: "BASE", 4: "1", 7: "2", 8: "3", 9: "4"},
		},

		// Invalid cases
		{
			name:        "Empty",
			command:     "",
			expectMatch: false,
		},
		{
			name:        "Just MODE",
			command:     "MODE",
			expectMatch: false,
		},
		{
			name:        "Unknown mode",
			command:     "MODE UNKNOWN",
			expectMatch: false,
		},
		{
			name:        "ROVER with number",
			command:     "MODE ROVER 123",
			expectMatch: false,
		},
		{
			name:        "BASE TIME without duration",
			command:     "MODE BASE TIME",
			expectMatch: false,
		},
		{
			name:        "Too many args",
			command:     "MODE BASE 1 2 3 4 5",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := modeRegexp.FindStringSubmatch(tt.command)
			got := matches != nil
			if got != tt.expectMatch {
				t.Errorf("modeRegexp.MatchString(%q) = %v, expect %v", tt.command, got, tt.expectMatch)
			}

			if tt.expectMatch && matches != nil {
				// Check expected capture groups
				for groupIdx, expectValue := range tt.expectGroups {
					if groupIdx >= len(matches) {
						t.Errorf("Expected group %d but only got %d groups", groupIdx, len(matches))
						continue
					}
					if matches[groupIdx] != expectValue {
						t.Errorf("Group %d: got %q, expect %q", groupIdx, matches[groupIdx], expectValue)
					}
				}
			}
		})
	}
}
