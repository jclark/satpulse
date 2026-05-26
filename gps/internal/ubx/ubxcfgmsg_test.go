package ubx

import (
	"slices"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

var testVers = struct {
	f9p, f9p51, m8f, m8p, lea6t, f10s, f9t, f9t25, x20p Version
}{
	f9p: Version{
		Mod:  "ZED-F9P",
		Prot: &ProtVer{Major: 27, Minor: 11},
		FW:   &FWVer{ProductCategory: "HPG", Major: 1, Minor: 12},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS, gpsprot.SBAS),
	},
	f9p51: Version{
		Mod:  "ZED-F9P",
		Prot: &ProtVer{Major: 27, Minor: 50},
		FW:   &FWVer{ProductCategory: "HPG", Major: 1, Minor: 51},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS, gpsprot.SBAS),
	},
	m8f: Version{
		Mod:  "LEA-M8F-0",
		Prot: &ProtVer{Major: 16, Minor: 0},
		FW:   &FWVer{ProductCategory: "FTS", Major: 1, Minor: 1},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS),
	},
	m8p: Version{ // don't have one of these, so I am guessing a bit
		Mod:  "NEO-M8P-0",
		Prot: &ProtVer{Major: 20, Minor: 0},
		FW:   &FWVer{ProductCategory: "HPG", Major: 1, Minor: 40},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS, gpsprot.SBAS),
	},
	lea6t: Version{
		Prot: &ProtVer{Major: 12, Minor: 2}, // this is inferred
		SW:   "6.02 (36023)",
	},
	f10s: Version{
		Mod:  "MAX-F10S",
		Prot: &ProtVer{Major: 40, Minor: 0},
		FW:   &FWVer{ProductCategory: "SPGL1L5", Major: 6, Minor: 0},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS, gpsprot.NAVIC),
	},
	f9t: Version{
		Mod:  "ZED-F9T",
		Prot: &ProtVer{Major: 29, Minor: 0},
		FW:   &FWVer{ProductCategory: "TIM", Major: 2, Minor: 1},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS),
	},
	f9t25: Version{
		Mod:  "ZED-F9T",
		Prot: &ProtVer{Major: 29, Minor: 25},
		FW:   &FWVer{ProductCategory: "TIM", Major: 2, Minor: 25},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS),
	},
	x20p: Version{
		SW:            "EXT HPG 2.00 (1e963fM)",
		HW:            "000B0000",
		Mod:           "ZED-X20P",
		Prot:          &ProtVer{Major: 50, Minor: 2},
		FW:            &FWVer{ProductCategory: "HPG", Major: 2, Minor: 0},
		RunsFromFlash: true,
		GNSS:          gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS, gpsprot.SBAS, gpsprot.NAVIC),
	},
}

// TestMsgChangesPVT tests msgChanges.pvt
// But without the PVTMsgOff
func TestMsgChangesPVT(t *testing.T) {
	tests := []struct {
		name     string
		flags    gpsprot.PVTMsgFlags
		version  Version
		expected []ubxbin.MsgID
	}{
		{
			name:     "Time",
			flags:    gpsprot.PVTMsgTime,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavTimeUTCID},
		},
		{
			name:     "Pos (F10S)",
			flags:    gpsprot.PVTMsgPos,
			version:  testVers.f10s,
			expected: []ubxbin.MsgID{ubxbin.NavPosLLHID},
		},
		{
			name:     "Vel",
			flags:    gpsprot.PVTMsgVel,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavVelNEDID},
		},
		{
			name:     "Pos,ECEF (F10S)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.f10s,
			expected: []ubxbin.MsgID{ubxbin.NavPosECEFID},
		},
		{
			name:     "Vel,ECEF",
			flags:    gpsprot.PVTMsgVel | gpsprot.PVTMsgECEF,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavVelECEFID},
		},
		{
			name:     "Time,Pos (HPG F9P)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavTimeUTCID, ubxbin.NavHPPosLLHID},
		},
		{
			name:     "Time,Vel (NAV-PVT supported)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgVel,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavPVTID},
		},
		{
			name:     "Pos,Vel (HPG F9P)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgVel,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavVelNEDID, ubxbin.NavHPPosLLHID},
		},
		{
			name:     "Time,Pos,Vel (HPG F9P)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgPos | gpsprot.PVTMsgVel,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavPVTID, ubxbin.NavHPPosLLHID},
		},
		{
			name:     "TimePulse",
			flags:    gpsprot.PVTMsgTimePulse,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID},
		},
		{
			name:     "Time,TAI",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavTimeGPSID},
		},
		{
			name:     "TimePulse,TAI",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID},
		},
		{
			name:     "LeapSecond",
			flags:    gpsprot.PVTMsgLeapSecond,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavTimeLSID},
		},
		{
			name:     "TimePulse,Pos",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavHPPosLLHID},
		},
		{
			name:     "TimePulse,Vel",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgVel,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavVelNEDID},
		},
		{
			name:     "TimePulse,TAI,Pos",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavHPPosLLHID},
		},
		{
			name:     "TimePulse,TAI,LeapSecond",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI | gpsprot.PVTMsgLeapSecond,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavTimeLSID},
		},
		{
			name:     "TimePulse,After",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTimePulseAfter,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavTimeUTCID},
		},
		{
			name:     "TimePulse,After,TAI",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTimePulseAfter | gpsprot.PVTMsgTAI,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavTimeGPSID},
		},
		{
			name:     "Quality (F9P)",
			flags:    gpsprot.PVTMsgQuality,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavPVTID, ubxbin.NavDOPID},
		},
		{
			name:     "Quality (LEA-6T, no NAV-PVT)",
			flags:    gpsprot.PVTMsgQuality,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavDOPID},
		},
		{
			name:     "Quality,Time (F9P)",
			flags:    gpsprot.PVTMsgQuality | gpsprot.PVTMsgTime,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavPVTID, ubxbin.NavDOPID},
		},
		{
			name:     "Quality,Time,Pos (HPG F9P)",
			flags:    gpsprot.PVTMsgQuality | gpsprot.PVTMsgTime | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavPVTID, ubxbin.NavDOPID, ubxbin.NavHPPosLLHID},
		},
		{
			name:     "Epoch (F9P)",
			flags:    gpsprot.PVTMsgEpoch,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavEOEID},
		},
		{
			name:     "Epoch (LEA-6T - not supported)",
			flags:    gpsprot.PVTMsgEpoch,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{},
		},
		{
			name:     "Epoch,Time,Pos (HPG F9P)",
			flags:    gpsprot.PVTMsgEpoch | gpsprot.PVTMsgTime | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavTimeUTCID, ubxbin.NavHPPosLLHID, ubxbin.NavEOEID},
		},
		// FTS (M8F) specific tests - uses TIM-TOS instead of TIM-TP
		{
			name:     "TimePulse (FTS)",
			flags:    gpsprot.PVTMsgTimePulse,
			version:  testVers.m8f,
			expected: []ubxbin.MsgID{ubxbin.TimTosID},
		},
		{
			name:     "Time (FTS)",
			flags:    gpsprot.PVTMsgTime,
			version:  testVers.m8f,
			expected: []ubxbin.MsgID{ubxbin.NavTimeUTCID},
		},
		{
			name:     "TimePulse,TAI (FTS)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI,
			version:  testVers.m8f,
			expected: []ubxbin.MsgID{ubxbin.TimTosID},
		},
		{
			name:     "Time,TAI (FTS)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			version:  testVers.m8f,
			expected: []ubxbin.MsgID{ubxbin.NavTimeGPSID},
		},
		{
			name:     "TimePulse,Time (FTS)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTime,
			version:  testVers.m8f,
			expected: []ubxbin.MsgID{ubxbin.TimTosID},
		},
		{
			name:     "TimePulse,After (FTS)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTimePulseAfter,
			version:  testVers.m8f,
			expected: []ubxbin.MsgID{ubxbin.TimTosID},
		},
		// LEA-6T specific tests - no NAV-PVT support
		{
			name:     "Time,Pos (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgPos,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavTimeUTCID, ubxbin.NavPosLLHID},
		},
		{
			name:     "Vel (no NAV-PVT)",
			flags:    gpsprot.PVTMsgVel,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavVelNEDID},
		},
		{
			name:     "Time,Vel (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgVel,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavTimeUTCID, ubxbin.NavVelNEDID},
		},
		{
			name:     "Pos,Vel (no NAV-PVT)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgVel,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavPosLLHID, ubxbin.NavVelNEDID},
		},
		{
			name:     "TimePulse,Pos (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgPos,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavPosLLHID},
		},
		{
			name:     "Time,TAI (LEA-6T)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavTimeGPSID},
		},
		{
			name:     "TimePulse,TAI (LEA-6T)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.TimTPID},
		},
		// HPG specific tests - uses HP position messages
		{
			name:     "Pos (HPG F9P)",
			flags:    gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavHPPosLLHID},
		},
		{
			name:     "Pos,ECEF (HPG F9P)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.f9p,
			expected: []ubxbin.MsgID{ubxbin.NavHPPosECEFID},
		},
		{
			name:     "Pos (HPG M8P)",
			flags:    gpsprot.PVTMsgPos,
			version:  testVers.m8p,
			expected: []ubxbin.MsgID{ubxbin.NavHPPosLLHID},
		},
		{
			name:     "Pos,ECEF (HPG M8P)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.m8p,
			expected: []ubxbin.MsgID{ubxbin.NavHPPosECEFID},
		},
		{
			name:     "Pos (HPG X20P)",
			flags:    gpsprot.PVTMsgPos,
			version:  testVers.x20p,
			expected: []ubxbin.MsgID{ubxbin.NavHPPosLLHID},
		},
		{
			name:     "Pos,ECEF (HPG X20P)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.x20p,
			expected: []ubxbin.MsgID{ubxbin.NavHPPosECEFID},
		},
		{
			name:     "Pos,ECEF (LEA-6T)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavPosECEFID},
		},
		{
			name:     "Vel,ECEF (LEA-6T)",
			flags:    gpsprot.PVTMsgVel | gpsprot.PVTMsgECEF,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavVelECEFID},
		},
		{
			name:     "Time,Pos,TAI (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgPos | gpsprot.PVTMsgTAI,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.NavTimeGPSID, ubxbin.NavPosLLHID},
		},
		{
			name:     "TimePulse,Pos,ECEF (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.lea6t,
			expected: []ubxbin.MsgID{ubxbin.TimTPID, ubxbin.NavPosECEFID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			mc.pvt(tt.flags, &tt.version)

			// Collect enabled messages (rate > 0)
			var enabled []ubxbin.MsgID
			for msgID, rate := range mc.rate {
				if rate > 0 {
					enabled = append(enabled, msgID)
				}
			}

			// Sort for consistent comparison
			slices.Sort(enabled)
			slices.Sort(tt.expected)

			// Compare
			if !slices.Equal(enabled, tt.expected) {
				t.Errorf("enabled messages = %v, want %v", enabled, tt.expected)
			}
		})
	}
}

// TestMsgChangesSurvey tests msgChanges.survey
func TestMsgChangesSurvey(t *testing.T) {
	tests := []struct {
		name            string
		flags           gpsprot.PVTMsgFlags
		version         Version
		surveyRequested bool
		expectedMsgID   ubxbin.MsgID
		expectedRate    MsgRate
	}{
		// Early models that don't support survey (protocol < 18)
		{
			name:            "LEA-6T no survey support",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.lea6t,
			surveyRequested: true,
		},
		// Models with tmode2 (protocol >= 18, < 20)
		{
			name:            "M8F survey enabled",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.m8f,
			surveyRequested: true,
			expectedMsgID:   ubxbin.TimSvinID,
			expectedRate:    1,
		},
		{
			name:            "M8F survey flag not set",
			flags:           gpsprot.PVTMsgTimePulse,
			version:         testVers.m8f,
			surveyRequested: true,
		},
		{
			name:            "M8F survey not requested",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.m8f,
			surveyRequested: false,
		},
		{
			name:            "M8F survey with Off flag",
			flags:           gpsprot.PVTMsgSurvey | gpsprot.PVTMsgOff,
			version:         testVers.m8f,
			surveyRequested: true,
			expectedMsgID:   ubxbin.TimSvinID,
			expectedRate:    1,
		},
		{
			name:            "M8P survey enabled",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.m8p,
			surveyRequested: true,
			expectedMsgID:   ubxbin.NavSvinID,
			expectedRate:    1,
		},
		// Models with tmode3 (protocol >= 20)
		{
			name:            "F9P survey enabled",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.f9p,
			surveyRequested: true,
			expectedMsgID:   ubxbin.NavSvinID,
			expectedRate:    1,
		},
		{
			name:            "F9P survey flag not set",
			flags:           gpsprot.PVTMsgTime,
			version:         testVers.f9p,
			surveyRequested: true,
		},
		{
			name:            "F9P survey not requested",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.f9p,
			surveyRequested: false,
		},
		{
			name:            "F9P survey with Off flag",
			flags:           gpsprot.PVTMsgSurvey | gpsprot.PVTMsgOff,
			version:         testVers.f9p,
			surveyRequested: true,
			expectedMsgID:   ubxbin.NavSvinID,
			expectedRate:    1,
		},
		{
			name:            "F9T survey enabled",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.f9t,
			surveyRequested: true,
			expectedMsgID:   ubxbin.TimSvinID,
			expectedRate:    1,
		},
		{
			name:            "F9T20 survey enabled",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.f9t25,
			surveyRequested: true,
			expectedMsgID:   ubxbin.TimSvinID,
			expectedRate:    1,
		},
		// Model that doesn't support tmode
		{
			name:            "F10S no tmode support",
			flags:           gpsprot.PVTMsgSurvey,
			version:         testVers.f10s,
			surveyRequested: true,
		},
		// Survey with multiple flags
		{
			name:            "F9P survey with other PVT flags",
			flags:           gpsprot.PVTMsgSurvey | gpsprot.PVTMsgTime | gpsprot.PVTMsgPos,
			version:         testVers.f9p,
			surveyRequested: true,
			expectedMsgID:   ubxbin.NavSvinID,
			expectedRate:    1,
		},
		// Off flag without survey flag
		{
			name:            "F9P Off flag only",
			flags:           gpsprot.PVTMsgOff,
			version:         testVers.f9p,
			surveyRequested: true,
			expectedMsgID:   ubxbin.NavSvinID,
			expectedRate:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			mc.survey(tt.flags, &tt.version, tt.surveyRequested)

			if tt.expectedMsgID == 0 {
				// Verify no message was set
				if len(mc.rate) > 0 {
					t.Errorf("expected no messages to be set, but got %v", mc.rate)
				}
				return
			}

			// Check that the expected message was set with the correct rate
			if actualRate, ok := mc.rate[tt.expectedMsgID]; !ok {
				t.Errorf("expected message %v to be set, but it wasn't", tt.expectedMsgID)
			} else if actualRate != tt.expectedRate {
				t.Errorf("message %v: got rate %v, want %v", tt.expectedMsgID, actualRate, tt.expectedRate)
			}

			// Check that only the expected message was set
			if len(mc.rate) != 1 {
				t.Errorf("expected exactly 1 message to be set, but got %d: %v", len(mc.rate), mc.rate)
			}
		})
	}
}

// TestMsgChangesRaw tests msgChanges.raw
func TestMsgChangesRaw(t *testing.T) {
	tests := []struct {
		name     string
		flags    gpsprot.RawMsgFlags
		version  Version
		expected map[ubxbin.MsgID]MsgRate
		wantErr  bool
	}{
		{
			name:     "Obs (LEA-6T)",
			flags:    gpsprot.RawMsgObs,
			version:  testVers.lea6t,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawID: 1, ubxbin.RxmSfrbID: 0},
		},
		{
			name:     "NavData (LEA-6T)",
			flags:    gpsprot.RawMsgNavData,
			version:  testVers.lea6t,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawID: 0, ubxbin.RxmSfrbID: 1},
		},
		{
			name:     "Obs,NavData (LEA-6T)",
			flags:    gpsprot.RawMsgObs | gpsprot.RawMsgNavData,
			version:  testVers.lea6t,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawID: 1, ubxbin.RxmSfrbID: 1},
		},
		{
			name:     "None (LEA-6T)",
			flags:    gpsprot.RawMsgNone,
			version:  testVers.lea6t,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawID: 0, ubxbin.RxmSfrbID: 0},
		},
		{
			name:     "Obs (F9P)",
			flags:    gpsprot.RawMsgObs,
			version:  testVers.f9p,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawxID: 1, ubxbin.RxmSfrbxID: 0},
		},
		{
			name:     "NavData (F9P)",
			flags:    gpsprot.RawMsgNavData,
			version:  testVers.f9p,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawxID: 0, ubxbin.RxmSfrbxID: 1},
		},
		{
			name:     "Obs,NavData (F9P)",
			flags:    gpsprot.RawMsgObs | gpsprot.RawMsgNavData,
			version:  testVers.f9p,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawxID: 1, ubxbin.RxmSfrbxID: 1},
		},
		{
			name:     "None (F9P)",
			flags:    gpsprot.RawMsgNone,
			version:  testVers.f9p,
			expected: map[ubxbin.MsgID]MsgRate{ubxbin.RxmRawxID: 0, ubxbin.RxmSfrbxID: 0},
		},
		{
			name:    "Obs (F10S - not supported)",
			flags:   gpsprot.RawMsgObs,
			version: testVers.f10s,
			wantErr: true,
		},
		{
			name:     "None (F10S - not supported)",
			flags:    gpsprot.RawMsgNone,
			version:  testVers.f10s,
			expected: map[ubxbin.MsgID]MsgRate{},
			wantErr:  false, // not an error to disable raw messages for something that doesn't support them
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			err := mc.raw(tt.flags, &tt.version)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check that all expected messages have correct rates
			for msgID, expectedRate := range tt.expected {
				if actualRate, ok := mc.rate[msgID]; !ok || actualRate != expectedRate {
					t.Errorf("message %v: got rate %v, want %v", msgID, actualRate, expectedRate)
				}
			}

			// Check that no unexpected messages were set
			for msgID := range mc.rate {
				if _, expected := tt.expected[msgID]; !expected {
					t.Errorf("unexpected message %v with rate %v", msgID, mc.rate[msgID])
				}
			}
		})
	}
}

// TestVersionRtcmSupport tests Version.rtcmSupport
func TestVersionRtcmSupport(t *testing.T) {
	tests := []struct {
		name        string
		version     Version
		expectGNSS  gpsprot.GNSSSet
		expectFlags gpsprot.RTCMMsgFlags
		expectDF003 bool
	}{
		{
			name:        "f9p",
			version:     testVers.f9p,
			expectGNSS:  gpsprot.MajorGNSSSet,
			expectFlags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			expectDF003: false,
		},
		{
			name:        "f9p51",
			version:     testVers.f9p51,
			expectGNSS:  gpsprot.MajorGNSSSet,
			expectFlags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			expectDF003: true,
		},
		{
			name:        "m8f",
			version:     testVers.m8f,
			expectGNSS:  0,
			expectFlags: 0,
			expectDF003: false,
		},
		{
			name:        "m8p",
			version:     testVers.m8p,
			expectGNSS:  gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GLO, gpsprot.BDS),
			expectFlags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			expectDF003: false,
		},
		{
			name:        "lea6t",
			version:     testVers.lea6t,
			expectGNSS:  0,
			expectFlags: 0,
			expectDF003: false,
		},
		{
			name:        "f10s",
			version:     testVers.f10s,
			expectGNSS:  0,
			expectFlags: 0,
			expectDF003: false,
		},
		{
			name:        "f9t",
			version:     testVers.f9t,
			expectGNSS:  gpsprot.MajorGNSSSet,
			expectFlags: gpsprot.RTCMMsgMSM7,
			expectDF003: false,
		},
		{
			name:        "f9t25",
			version:     testVers.f9t25,
			expectGNSS:  gpsprot.MajorGNSSSet,
			expectFlags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			expectDF003: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sup := tt.version.rtcmSupport()
			if sup.gnss != tt.expectGNSS {
				t.Errorf("GNSS support: got %v, want %v", sup.gnss, tt.expectGNSS)
			}
			if sup.msgs != tt.expectFlags {
				t.Errorf("MSM flags: got %v, want %v", sup.msgs, tt.expectFlags)
			}
			if sup.df003Out != tt.expectDF003 {
				t.Errorf("DF003 support: got %v, want %v", sup.df003Out, tt.expectDF003)
			}
		})
	}
}

// TestMsgChangesRtcm tests msgChanges.rtcm
func TestMsgChangesRtcm(t *testing.T) {
	tests := []struct {
		name            string
		flags           gpsprot.RTCMMsgFlags
		version         Version
		enabledGNSS     gpsprot.GNSSSet
		expectedRates   map[ubxbin.MsgID]MsgRate
		expectedEnable  ubxbin.CfgPrtProtoMask
		expectedDisable ubxbin.CfgPrtProtoMask
		wantErr         bool
	}{
		{
			name:        "MSM4 GPS (F9P)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 1, // GPS MSM4
				ubxbin.Rtcm1077ID: 0, // GPS MSM7 disabled
				ubxbin.Rtcm1084ID: 0, // GLO MSM4 disabled
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1094ID: 0, // GAL MSM4 disabled
				ubxbin.Rtcm1097ID: 0, // GAL MSM7 disabled
				ubxbin.Rtcm1124ID: 0, // BDS MSM4 disabled
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM7 GPS,GLO (F9P)",
			flags:       gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GLO),
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 0, // GPS MSM4 disabled
				ubxbin.Rtcm1077ID: 1, // GPS MSM7
				ubxbin.Rtcm1084ID: 0, // GLO MSM4 disabled
				ubxbin.Rtcm1087ID: 1, // GLO MSM7
				ubxbin.Rtcm1094ID: 0, // GAL MSM4 disabled
				ubxbin.Rtcm1097ID: 0, // GAL MSM7 disabled
				ubxbin.Rtcm1124ID: 0, // BDS MSM4 disabled
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4+MSM7 GPS (F9P)",
			flags:       gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 1,
				ubxbin.Rtcm1077ID: 1,
				ubxbin.Rtcm1084ID: 0, // GLO disabled
				ubxbin.Rtcm1087ID: 0, // GLO disabled
				ubxbin.Rtcm1094ID: 0, // GAL disabled
				ubxbin.Rtcm1097ID: 0, // GAL disabled
				ubxbin.Rtcm1124ID: 0, // BDS disabled
				ubxbin.Rtcm1127ID: 0, // BDS disabled
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "ARP (F9P)",
			flags:       gpsprot.RTCMMsgARP,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1005ID: 1, // ARP
				ubxbin.Rtcm1074ID: 0, // GPS MSM4 disabled
				ubxbin.Rtcm1077ID: 0, // GPS MSM7 disabled
				ubxbin.Rtcm1084ID: 0, // GLO MSM4 disabled
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1094ID: 0, // GAL MSM4 disabled
				ubxbin.Rtcm1097ID: 0, // GAL MSM7 disabled
				ubxbin.Rtcm1124ID: 0, // BDS MSM4 disabled
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:            "None (F9P)",
			flags:           gpsprot.RTCMMsgNone,
			version:         testVers.f9p,
			enabledGNSS:     gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedDisable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4 no GNSS enabled (F9P)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.f9p,
			enabledGNSS: 0,
			wantErr:     true,
		},
		{
			name:        "MSM4 GPS (M8P - no GAL support)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.m8p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 1, // GPS MSM4
				ubxbin.Rtcm1077ID: 0, // GPS MSM7 disabled
				ubxbin.Rtcm1084ID: 0, // GLO MSM4 disabled
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1124ID: 0, // BDS MSM4 disabled
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4 GAL (M8P - not supported)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.m8p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GAL),
			wantErr:     true,
		},
		{
			name:        "MSM7 GPS (F9T)",
			flags:       gpsprot.RTCMMsgMSM7,
			version:     testVers.f9t,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1077ID: 1, // GPS MSM7
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1097ID: 0, // GAL MSM7 disabled
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4 GPS (F9T - not supported)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.f9t,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			wantErr:     true,
		},
		{
			name:        "MSM4 GPS (LEA-6T - not supported)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.lea6t,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			wantErr:     true,
		},
		{
			name:        "MSM4 all major GNSS (F9P)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 1, // GPS MSM4
				ubxbin.Rtcm1077ID: 0, // GPS MSM7 disabled
				ubxbin.Rtcm1084ID: 1, // GLO MSM4
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1094ID: 1, // GAL MSM4
				ubxbin.Rtcm1097ID: 0, // GAL MSM7 disabled
				ubxbin.Rtcm1124ID: 1, // BDS MSM4
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM7 all major GNSS (F9P)",
			flags:       gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 0, // GPS MSM4 disabled
				ubxbin.Rtcm1077ID: 1, // GPS MSM7
				ubxbin.Rtcm1084ID: 0, // GLO MSM4 disabled
				ubxbin.Rtcm1087ID: 1, // GLO MSM7
				ubxbin.Rtcm1094ID: 0, // GAL MSM4 disabled
				ubxbin.Rtcm1097ID: 1, // GAL MSM7
				ubxbin.Rtcm1124ID: 0, // BDS MSM4 disabled
				ubxbin.Rtcm1127ID: 1, // BDS MSM7
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4+MSM7 all major GNSS (F9P)",
			flags:       gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 1, // GPS MSM4
				ubxbin.Rtcm1077ID: 1, // GPS MSM7
				ubxbin.Rtcm1084ID: 1, // GLO MSM4
				ubxbin.Rtcm1087ID: 1, // GLO MSM7
				ubxbin.Rtcm1094ID: 1, // GAL MSM4
				ubxbin.Rtcm1097ID: 1, // GAL MSM7
				ubxbin.Rtcm1124ID: 1, // BDS MSM4
				ubxbin.Rtcm1127ID: 1, // BDS MSM7
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4 all major GNSS (M8P - no GAL)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.m8p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 1, // GPS MSM4
				ubxbin.Rtcm1077ID: 0, // GPS MSM7 disabled
				ubxbin.Rtcm1084ID: 1, // GLO MSM4
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1124ID: 1, // BDS MSM4
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM7 all major GNSS (F9T20)",
			flags:       gpsprot.RTCMMsgMSM7,
			version:     testVers.f9t25,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 0, // GPS MSM4 disabled
				ubxbin.Rtcm1077ID: 1, // GPS MSM7
				ubxbin.Rtcm1084ID: 0, // GLO MSM4 disabled
				ubxbin.Rtcm1087ID: 1, // GLO MSM7
				ubxbin.Rtcm1094ID: 0, // GAL MSM4 disabled
				ubxbin.Rtcm1097ID: 1, // GAL MSM7
				ubxbin.Rtcm1124ID: 0, // BDS MSM4 disabled
				ubxbin.Rtcm1127ID: 1, // BDS MSM7
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "None (M8F - no RTCM support)",
			flags:       gpsprot.RTCMMsgNone,
			version:     testVers.m8f,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			// No expected rates or protocol changes
			wantErr: false, // Not an error to disable RTCM on devices that don't support it
		},
		{
			name:        "None (LEA-6T - no RTCM support)",
			flags:       gpsprot.RTCMMsgNone,
			version:     testVers.lea6t,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			// No expected rates or protocol changes
			wantErr: false, // Not an error to disable RTCM on devices that don't support it
		},
		// Lax flag tests
		{
			name:        "MSM4+Lax all major GNSS (F9P - no fallback needed)",
			flags:       gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgLax,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 1, // GPS MSM4
				ubxbin.Rtcm1077ID: 0, // GPS MSM7 disabled
				ubxbin.Rtcm1084ID: 1, // GLO MSM4
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1094ID: 1, // GAL MSM4
				ubxbin.Rtcm1097ID: 0, // GAL MSM7 disabled
				ubxbin.Rtcm1124ID: 1, // BDS MSM4
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4+Lax all major GNSS (F9T - fallback to MSM7)",
			flags:       gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgLax,
			version:     testVers.f9t,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1077ID: 1, // GPS MSM7 (fallback from MSM4)
				ubxbin.Rtcm1087ID: 1, // GLO MSM7 (fallback from MSM4)
				ubxbin.Rtcm1097ID: 1, // GAL MSM7 (fallback from MSM4)
				ubxbin.Rtcm1127ID: 1, // BDS MSM7 (fallback from MSM4)
				ubxbin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM7+Lax GPS (F9T20 - no fallback needed)",
			flags:       gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgLax,
			version:     testVers.f9t25,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.Rtcm1074ID: 0, // GPS MSM4 disabled
				ubxbin.Rtcm1077ID: 1, // GPS MSM7
				ubxbin.Rtcm1084ID: 0, // GLO MSM4 disabled
				ubxbin.Rtcm1087ID: 0, // GLO MSM7 disabled
				ubxbin.Rtcm1094ID: 0, // GAL MSM4 disabled
				ubxbin.Rtcm1097ID: 0, // GAL MSM7 disabled
				ubxbin.Rtcm1124ID: 0, // BDS MSM4 disabled
				ubxbin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: ubxbin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4+Lax without Lax (F9T - should fail)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.f9t,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			wantErr:     true, // MSM4 not supported on F9T, no Lax fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			err := mc.rtcm(tt.flags, tt.version.rtcmSupport(), tt.enabledGNSS)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check message rates
			if tt.expectedRates != nil {
				for msgID, expectedRate := range tt.expectedRates {
					if actualRate, ok := mc.rate[msgID]; !ok || actualRate != expectedRate {
						t.Errorf("message %v: got rate %v, want %v", msgID, actualRate, expectedRate)
					}
				}
				// Check no unexpected messages
				for msgID := range mc.rate {
					if _, expected := tt.expectedRates[msgID]; !expected {
						t.Errorf("unexpected message %v with rate %v", msgID, mc.rate[msgID])
					}
				}
			}

			// Check protocol enable/disable
			if tt.expectedEnable != 0 && mc.protoEnable&tt.expectedEnable == 0 {
				t.Errorf("expected proto enable %v, got %v", tt.expectedEnable, mc.protoEnable)
			}
			if tt.expectedDisable != 0 && mc.protoDisable&tt.expectedDisable == 0 {
				t.Errorf("expected proto disable %v, got %v", tt.expectedDisable, mc.protoDisable)
			}
		})
	}
}

// TestMsgChangesSats tests msgChanges.sats
func TestMsgChangesSats(t *testing.T) {
	tests := []struct {
		name          string
		flags         gpsprot.SatsMsgFlags
		version       Version
		expectedRates map[ubxbin.MsgID]MsgRate
	}{
		{
			name:    "SV (F9P - uses NAV-SAT)",
			flags:   gpsprot.SatsMsgSat,
			version: testVers.f9p,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NavSatID: 1,
				ubxbin.NavSigID: 0,
			},
		},
		{
			name:    "None (F9P)",
			flags:   gpsprot.SatsMsgNone,
			version: testVers.f9p,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NavSatID: 0,
				ubxbin.NavSigID: 0,
			},
		},
		{
			name:    "SV (LEA-6T - uses NAV-SVINFO)",
			flags:   gpsprot.SatsMsgSat,
			version: testVers.lea6t,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NavSVInfoID: 1,
			},
		},
		{
			name:    "None (LEA-6T)",
			flags:   gpsprot.SatsMsgNone,
			version: testVers.lea6t,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NavSVInfoID: 0,
			},
		},
		{
			name:    "Signal (F9P - uses NAV-SIG)",
			flags:   gpsprot.SatsMsgSignal,
			version: testVers.f9p,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NavSatID: 0,
				ubxbin.NavSigID: 1,
			},
		},
		{
			name:    "SV+Signal (F9P - uses both NAV-SAT and NAV-SIG)",
			flags:   gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal,
			version: testVers.f9p,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NavSatID: 1,
				ubxbin.NavSigID: 1,
			},
		},
		{
			name:    "Signal (LEA-6T - NAV-SIG not supported)",
			flags:   gpsprot.SatsMsgSignal,
			version: testVers.lea6t,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NavSVInfoID: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			mc.sats(tt.flags, &tt.version)

			// Check message rates
			for msgID, expectedRate := range tt.expectedRates {
				if actualRate, ok := mc.rate[msgID]; !ok || actualRate != expectedRate {
					t.Errorf("message %v: got rate %v, want %v", msgID, actualRate, expectedRate)
				}
			}

			// Check no unexpected messages
			for msgID := range mc.rate {
				if _, expected := tt.expectedRates[msgID]; !expected {
					t.Errorf("unexpected message %v with rate %v", msgID, mc.rate[msgID])
				}
			}
		})
	}
}

// TestMsgChangesNMEA tests msgChanges.nmea
func TestMsgChangesNMEA(t *testing.T) {
	tests := []struct {
		name            string
		flags           gpsprot.NMEAMsgFlags
		expectedRates   map[ubxbin.MsgID]MsgRate
		expectedEnable  ubxbin.CfgPrtProtoMask
		expectedDisable ubxbin.CfgPrtProtoMask
	}{
		{
			name:            "None - disable protocol",
			flags:           gpsprot.NMEAMsgNone,
			expectedRates:   map[ubxbin.MsgID]MsgRate{},
			expectedEnable:  0,
			expectedDisable: ubxbin.CfgPrtProtoNMEA,
		},
		{
			name:            "RMC only",
			flags:           gpsprot.NMEAMsgRMC,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 1,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "GGA only",
			flags:           gpsprot.NMEAMsgGGA,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 0,
				ubxbin.NmeaGgaID: 1,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "GSA only",
			flags:           gpsprot.NMEAMsgGSA,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 0,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 1,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "GSV only",
			flags:           gpsprot.NMEAMsgGSV,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 0,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 1,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "ZDA only",
			flags:           gpsprot.NMEAMsgZDA,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 0,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 1,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "VTG only",
			flags:           gpsprot.NMEAMsgVTG,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 0,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 1,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "GLL only",
			flags:           gpsprot.NMEAMsgGLL,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 0,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 1,
			},
		},
		{
			name:            "Multiple messages",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSV,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 1,
				ubxbin.NmeaGgaID: 1,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 1,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "All standard messages",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV | gpsprot.NMEAMsgZDA | gpsprot.NMEAMsgVTG | gpsprot.NMEAMsgGLL,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 1,
				ubxbin.NmeaGgaID: 1,
				ubxbin.NmeaGsaID: 1,
				ubxbin.NmeaGsvID: 1,
				ubxbin.NmeaZdaID: 1,
				ubxbin.NmeaVtgID: 1,
				ubxbin.NmeaGllID: 1,
			},
		},
		{
			name:            "Other flag only - enables protocol but no specific messages",
			flags:           gpsprot.NMEAMsgOther,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 0,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "RMC with Other flag",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgOther,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 1,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 0,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "RMC with VTG",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgVTG,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 1,
				ubxbin.NmeaGgaID: 0,
				ubxbin.NmeaGsaID: 0,
				ubxbin.NmeaGsvID: 0,
				ubxbin.NmeaZdaID: 0,
				ubxbin.NmeaVtgID: 1,
				ubxbin.NmeaGllID: 0,
			},
		},
		{
			name:            "All messages with Other flag",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV | gpsprot.NMEAMsgZDA | gpsprot.NMEAMsgVTG | gpsprot.NMEAMsgGLL | gpsprot.NMEAMsgOther,
			expectedEnable:  ubxbin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[ubxbin.MsgID]MsgRate{
				ubxbin.NmeaRmcID: 1,
				ubxbin.NmeaGgaID: 1,
				ubxbin.NmeaGsaID: 1,
				ubxbin.NmeaGsvID: 1,
				ubxbin.NmeaZdaID: 1,
				ubxbin.NmeaVtgID: 1,
				ubxbin.NmeaGllID: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			mc.nmea(tt.flags, nil)

			// Check protocol enable/disable
			if mc.protoEnable != tt.expectedEnable {
				t.Errorf("protoEnable = %v, want %v", mc.protoEnable, tt.expectedEnable)
			}
			if mc.protoDisable != tt.expectedDisable {
				t.Errorf("protoDisable = %v, want %v", mc.protoDisable, tt.expectedDisable)
			}

			// Check message rates
			for msgID, expectedRate := range tt.expectedRates {
				if actualRate, ok := mc.rate[msgID]; !ok || actualRate != expectedRate {
					t.Errorf("message %v: got rate %v, want %v", msgID, actualRate, expectedRate)
				}
			}

			// Check no unexpected messages
			for msgID := range mc.rate {
				if _, expected := tt.expectedRates[msgID]; !expected {
					t.Errorf("unexpected message %v with rate %v", msgID, mc.rate[msgID])
				}
			}
		})
	}
}

// TestMsgChangesItemsPortKeys verifies that msgChanges.items emits the
// correct port-specific OUTPROT keys for each port, and never emits an
// item with a zero key. Regression test for the I2C/SPI VALSET NACK
// caused by portOutprotNmeaKey/portOutprotRtcm3xKey returning 0.
func TestMsgChangesItemsPortKeys(t *testing.T) {
	tests := []struct {
		port     ucv.Port
		nmeaKey  ucv.Key
		rtcm3Key ucv.Key
	}{
		{ucv.I2C, ucv.KI2coutprotNmea.Key(), ucv.KI2coutprotRtcm3x.Key()},
		{ucv.UART1, ucv.KUart1outprotNmea.Key(), ucv.KUart1outprotRtcm3x.Key()},
		{ucv.UART2, ucv.KUart2outprotNmea.Key(), ucv.KUart2outprotRtcm3x.Key()},
		{ucv.USB, ucv.KUsboutprotNmea.Key(), ucv.KUsboutprotRtcm3x.Key()},
		{ucv.SPI, ucv.KSpioutprotNmea.Key(), ucv.KSpioutprotRtcm3x.Key()},
	}
	for _, tt := range tests {
		name, _ := portName(tt.port)
		t.Run(name, func(t *testing.T) {
			mc := newMsgChanges()
			mc.protoDisable = ubxbin.CfgPrtProtoNMEA
			mc.protoEnable = ubxbin.CfgPrtProtoRTCM3
			items := mc.items(tt.port, true)
			for _, it := range items {
				if it.Key == 0 {
					t.Errorf("item with zero key: %+v", it)
				}
			}
			if !hasItem(items, tt.nmeaKey, 0) {
				t.Errorf("missing NMEA disable item for key 0x%08x", uint32(tt.nmeaKey))
			}
			if !hasItem(items, tt.rtcm3Key, 1) {
				t.Errorf("missing RTCM3 enable item for key 0x%08x", uint32(tt.rtcm3Key))
			}
		})
	}
}

func hasItem(items []ucv.Item, key ucv.Key, value uint64) bool {
	for _, it := range items {
		if it.Key == key && it.Value == value {
			return true
		}
	}
	return false
}
