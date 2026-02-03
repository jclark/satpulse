package unc

import (
	"maps"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// allSignalsExceptSBAS represents all signals except SBAS, useful for testing
// signal configurations without triggering SBAS-related commands
const allSignalsExceptSBAS = gpsprot.SigSetAll &^ gpsprot.SigSetSBAS

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
			for _, nativeCmd := range commands {
				gotSet[nativeCmd.cmd] = true
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
			for _, nativeCmd := range commands {
				if err := roundTrip.updateFromCommand(nativeCmd.cmd); err != nil {
					t.Fatalf("updateFromCommand(%q) failed: %v", nativeCmd.cmd, err)
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
			for _, nativeCmd := range commands {
				gotSet[nativeCmd.cmd] = true
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
		"UNMASK 5.0",     // UNMASK with elevation not allowed
		"UNMASK -10",     // UNMASK with negative elevation not allowed
	}

	for _, cmd := range invalidCommands {
		if err := prop.updateFromCommand(cmd); err == nil {
			t.Errorf("updateFromCommand(%q) should have failed but didn't", cmd)
		}
	}
}

func TestConfigMask(t *testing.T) {
	tc := []nativeConfigPropsTestCase{
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
				"MASK GPSL5", // GPS L5 frequency is masked (use response form)
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
		{
			name: "Parse MASK query response with firmware aliases - mask all GPS",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				// Firmware uses these aliases in MASK query responses
				"MASK GPSL1CA",
				"MASK GPSL1C",
				"MASK GPSL2C",
				"MASK GPSL2P",
				"MASK GPSL5",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Enable everything except SBAS
				props.SetSignalsEnabled(allSignalsExceptSBAS)
			},
			expectedCmds: []string{
				"UNMASK GPS", // Should recognize all GPS signals need to be unmasked
			},
		},
		{
			name: "Parse MASK query response with firmware aliases - mask all GLONASS",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				// Firmware uses these aliases in MASK query responses
				"MASK GLOL1",
				"MASK GLOL2",
				"MASK GLOL3",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Enable everything except SBAS
				props.SetSignalsEnabled(allSignalsExceptSBAS)
			},
			expectedCmds: []string{
				"UNMASK GLO", // Should recognize all GLONASS signals need to be unmasked
			},
		},
		{
			name: "no change with mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 1",
				"MASK GLO",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
			},
			expectedCmds: []string{},
		},
		{
			name: "set min elevation mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetMinElevation(gpsprot.DegreesFromFloat(10.5))
			},
			expectedCmds: []string{
				"MASK 10.5",
			},
		},
		{
			name: "change min elevation mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK 5",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetMinElevation(gpsprot.DegreesFromFloat(15))
			},
			expectedCmds: []string{
				"MASK 15",
			},
		},
		{
			name: "no change in elevation mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK 10",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetMinElevation(gpsprot.DegreesFromFloat(10))
			},
			expectedCmds: []string{},
		},
		{
			name: "min elevation with signal masks",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK 5",
				"MASK GLO",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetMinElevation(gpsprot.DegreesFromFloat(10))
				props.SetSignalsEnabled(gpsprot.SigSetGPS | gpsprot.SigSetGAL)
			},
			expectedCmds: []string{
				"MASK 10",
				"MASK BDS",
				"MASK GLO",
				"MASK QZSS",
				"MASK IRNSS",
			},
		},
		{
			name: "set zero degree elevation mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetMinElevation(gpsprot.DegreesFromFloat(0))
			},
			expectedCmds: []string{
				"MASK 0",
			},
		},
		{
			name: "set negative elevation mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetMinElevation(gpsprot.DegreesFromFloat(-5))
			},
			expectedCmds: []string{
				"MASK -5",
			},
		},
		{
			name: "change from positive to negative elevation mask",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK 10",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				props.SetMinElevation(gpsprot.DegreesFromFloat(-10.5))
			},
			expectedCmds: []string{
				"MASK -10.5",
			},
		},
		{
			name: "parse negative elevation mask from receiver",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"MASK -15",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Just getting the current state, no changes
			},
			expectedCmds: []string{},
		},
	}
	testNativeConfigProps(t, tc)
}

func TestSBASProp(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		expectError bool
		enabled     bool
	}{
		{
			name:        "SBAS disabled",
			command:     "CONFIG SBAS DISABLE",
			expectError: false,
			enabled:     false,
		},
		{
			name:        "SBAS enabled AUTO",
			command:     "CONFIG SBAS ENABLE AUTO",
			expectError: false,
			enabled:     true,
		},
		{
			name:        "SBAS enabled WAAS",
			command:     "CONFIG SBAS ENABLE WAAS",
			expectError: false,
			enabled:     true,
		},
		{
			name:        "SBAS enabled EGNOS",
			command:     "CONFIG SBAS ENABLE EGNOS",
			expectError: false,
			enabled:     true,
		},
		{
			name:        "SBAS timeout 600",
			command:     "CONFIG SBAS TIMEOUT 600",
			expectError: false,
			enabled:     false, // timeout alone doesn't enable SBAS
		},
		{
			name:        "SBAS timeout 0",
			command:     "CONFIG SBAS TIMEOUT 0",
			expectError: false,
			enabled:     false, // timeout 0 disables
		},
		{
			name:        "invalid SBAS command",
			command:     "CONFIG SBAS INVALID",
			expectError: true,
			enabled:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prop := sbasProp{}
			err := prop.updateFromCommand(tc.command)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for command %q, but got none", tc.command)
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error for command %q: %v", tc.command, err)
				return
			}
			
			if prop.enabled() != tc.enabled {
				t.Errorf("for command %q, expected enabled=%v, got %v", tc.command, tc.enabled, prop.enabled())
			}
		})
	}
}

func TestSBASIntegration(t *testing.T) {
	// Tests that verify SBAS is properly integrated with signal configuration
	t.Run("SBAS enabled adds SBAS signals", func(t *testing.T) {
		np := makeNativeProps()
		
		// Set up signal group 2
		if err := np.signalGroup.updateFromCommand("CONFIG SIGNALGROUP 2"); err != nil {
			t.Fatal(err)
		}
		// Enable SBAS
		if err := np.sbas.updateFromCommand("CONFIG SBAS ENABLE AUTO"); err != nil {
			t.Fatal(err)
		}
		
		// Convert to ConfigProps
		props := &gpsprot.ConfigProps{}
		np.convertToProps(props)
		
		sigs, ok := props.GetSignalsEnabled()
		if !ok {
			t.Fatal("expected SignalsEnabled to be set")
		}
		// Should include SBAS signals
		if sigs&gpsprot.SigSetSBAS == 0 {
			t.Errorf("expected SBAS signals to be enabled, got %v", sigs)
		}
	})
	
	t.Run("SBAS disabled removes SBAS signals", func(t *testing.T) {
		np := makeNativeProps()
		
		// Set up signal group 2
		if err := np.signalGroup.updateFromCommand("CONFIG SIGNALGROUP 2"); err != nil {
			t.Fatal(err)
		}
		// Disable SBAS
		if err := np.sbas.updateFromCommand("CONFIG SBAS DISABLE"); err != nil {
			t.Fatal(err)
		}
		
		// Convert to ConfigProps
		props := &gpsprot.ConfigProps{}
		np.convertToProps(props)
		
		sigs, ok := props.GetSignalsEnabled()
		if !ok {
			t.Fatal("expected SignalsEnabled to be set")
		}
		// Should not include SBAS signals
		if sigs&gpsprot.SigSetSBAS != 0 {
			t.Errorf("expected SBAS signals to be disabled, got %v", sigs)
		}
	})

	tc := []nativeConfigPropsTestCase{
		{
			name: "enable SBAS when requesting SBAS signals",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"CONFIG SBAS DISABLE",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Request GPS and SBAS signals
				props.SetSignalsEnabled(gpsprot.SigSetGPS | gpsprot.SigSetSBAS)
			},
			expectedCmds: []string{
				"MASK GLO",
				"MASK GAL",
				"MASK BDS",
				"MASK QZSS",
				"MASK IRNSS",
				"CONFIG SBAS ENABLE AUTO", // Should enable SBAS
			},
		},
		{
			name: "disable SBAS when not requesting SBAS signals",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
				"CONFIG SBAS ENABLE WAAS",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Request only GPS signals (no SBAS)
				props.SetSignalsEnabled(gpsprot.SigSetGPS)
			},
			expectedCmds: []string{
				"MASK GLO",
				"MASK GAL",
				"MASK BDS",
				"MASK QZSS",
				"MASK IRNSS",
				"CONFIG SBAS DISABLE", // Should disable SBAS
			},
		},
		{
			name: "preserve SBAS state when changing signal groups",
			currentState: []string{
				"CONFIG SIGNALGROUP 1",
				"CONFIG SBAS ENABLE EGNOS",
			},
			targetProps: func(props *gpsprot.ConfigProps) {
				// Just enable GPS but preserve SBAS
				props.SetSignalsEnabled(gpsprot.SigSetGPS | gpsprot.SigSetSBAS)
			},
			expectedCmds: []string{
				"MASK GLO",
				"MASK GAL",
				"MASK BDS",
				"MASK QZSS",
				// No SBAS command - already enabled
			},
		},
	}
	testNativeConfigProps(t, tc)
}
