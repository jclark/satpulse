package ubx

import (
	"slices"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

var testVers = struct {
	f9p, m8f, m8p, lea6t, f10s, f9t, f9t20 Version
}{
	f9p: Version{
		Mod:  "ZED-F9P",
		Prot: &ProtVer{Major: 27, Minor: 11},
		FW:   &FWVer{ProductCategory: "HPG", Major: 1, Minor: 12},
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
	f9t20: Version{
		Mod:  "ZED-F9T",
		Prot: &ProtVer{Major: 29, Minor: 25},
		FW:   &FWVer{ProductCategory: "TIM", Major: 2, Minor: 25},
		GNSS: gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS),
	},
}

// TestMsgChangesPVT tests msgChanges.pvt
// But without the PVTMsgOff
func TestMsgChangesPVT(t *testing.T) {
	tests := []struct {
		name     string
		flags    gpsprot.PVTMsgFlags
		version  Version
		expected []bin.MsgID
	}{
		{
			name:     "Time",
			flags:    gpsprot.PVTMsgTime,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.NavTimeUTCID},
		},
		{
			name:     "Pos",
			flags:    gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.NavPosLLHID},
		},
		{
			name:     "Pos,ECEF",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.NavPosECEFID},
		},
		{
			name:     "Time,Pos (NAV-PVT supported)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.NavPVTID},
		},
		{
			name:     "TimePulse",
			flags:    gpsprot.PVTMsgTimePulse,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.TimTPID, bin.NavTimeUTCID},
		},
		{
			name:     "Time,TAI",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.NavTimeGPSID},
		},
		{
			name:     "TimePulse,TAI",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.TimTPID, bin.NavTimeGPSID},
		},
		{
			name:     "LeapSecond",
			flags:    gpsprot.PVTMsgLeapSecond,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.NavTimeLSID},
		},
		{
			name:     "TimePulse,Pos",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.TimTPID, bin.NavPVTID},
		},
		{
			name:     "TimePulse,TAI,Pos",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI | gpsprot.PVTMsgPos,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.TimTPID, bin.NavTimeGPSID, bin.NavPosLLHID},
		},
		{
			name:     "TimePulse,TAI,LeapSecond",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI | gpsprot.PVTMsgLeapSecond,
			version:  testVers.f9p,
			expected: []bin.MsgID{bin.TimTPID, bin.NavTimeGPSID, bin.NavTimeLSID},
		},
		// FTS (M8F) specific tests - uses TIM-TOS instead of TIM-TP
		{
			name:     "TimePulse (FTS)",
			flags:    gpsprot.PVTMsgTimePulse,
			version:  testVers.m8f,
			expected: []bin.MsgID{bin.TimTosID},
		},
		{
			name:     "Time (FTS)",
			flags:    gpsprot.PVTMsgTime,
			version:  testVers.m8f,
			expected: []bin.MsgID{bin.NavTimeUTCID},
		},
		{
			name:     "TimePulse,TAI (FTS)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI,
			version:  testVers.m8f,
			expected: []bin.MsgID{bin.TimTosID},
		},
		{
			name:     "Time,TAI (FTS)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			version:  testVers.m8f,
			expected: []bin.MsgID{bin.NavTimeGPSID},
		},
		{
			name:     "TimePulse,Time (FTS)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTime,
			version:  testVers.m8f,
			expected: []bin.MsgID{bin.TimTosID},
		},
		// LEA-6T specific tests - no NAV-PVT support
		{
			name:     "Time,Pos (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgPos,
			version:  testVers.lea6t,
			expected: []bin.MsgID{bin.NavTimeUTCID, bin.NavPosLLHID},
		},
		{
			name:     "TimePulse,Pos (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgPos,
			version:  testVers.lea6t,
			expected: []bin.MsgID{bin.TimTPID, bin.NavTimeUTCID, bin.NavPosLLHID},
		},
		{
			name:     "Time,TAI (LEA-6T)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			version:  testVers.lea6t,
			expected: []bin.MsgID{bin.NavTimeGPSID},
		},
		{
			name:     "TimePulse,TAI (LEA-6T)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI,
			version:  testVers.lea6t,
			expected: []bin.MsgID{bin.TimTPID, bin.NavTimeGPSID},
		},
		{
			name:     "Pos,ECEF (LEA-6T)",
			flags:    gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.lea6t,
			expected: []bin.MsgID{bin.NavPosECEFID},
		},
		{
			name:     "Time,Pos,TAI (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTime | gpsprot.PVTMsgPos | gpsprot.PVTMsgTAI,
			version:  testVers.lea6t,
			expected: []bin.MsgID{bin.NavTimeGPSID, bin.NavPosLLHID},
		},
		{
			name:     "TimePulse,Pos,ECEF (no NAV-PVT)",
			flags:    gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			version:  testVers.lea6t,
			expected: []bin.MsgID{bin.TimTPID, bin.NavTimeUTCID, bin.NavPosECEFID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			mc.pvt(tt.flags, &tt.version)

			// Collect enabled messages (rate > 0)
			var enabled []bin.MsgID
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

// TestMsgChangesRaw tests msgChanges.raw
func TestMsgChangesRaw(t *testing.T) {
	tests := []struct {
		name     string
		flags    gpsprot.RawMsgFlags
		version  Version
		expected map[bin.MsgID]uint8
		wantErr  bool
	}{
		{
			name:     "Obs (LEA-6T)",
			flags:    gpsprot.RawMsgObs,
			version:  testVers.lea6t,
			expected: map[bin.MsgID]uint8{bin.RxmRawID: 1, bin.RxmSfrbID: 0},
		},
		{
			name:     "NavData (LEA-6T)",
			flags:    gpsprot.RawMsgNavData,
			version:  testVers.lea6t,
			expected: map[bin.MsgID]uint8{bin.RxmRawID: 0, bin.RxmSfrbID: 1},
		},
		{
			name:     "Obs,NavData (LEA-6T)",
			flags:    gpsprot.RawMsgObs | gpsprot.RawMsgNavData,
			version:  testVers.lea6t,
			expected: map[bin.MsgID]uint8{bin.RxmRawID: 1, bin.RxmSfrbID: 1},
		},
		{
			name:     "None (LEA-6T)",
			flags:    gpsprot.RawMsgNone,
			version:  testVers.lea6t,
			expected: map[bin.MsgID]uint8{bin.RxmRawID: 0, bin.RxmSfrbID: 0},
		},
		{
			name:     "Obs (F9P)",
			flags:    gpsprot.RawMsgObs,
			version:  testVers.f9p,
			expected: map[bin.MsgID]uint8{bin.RxmRawxID: 1, bin.RxmSfrbxID: 0},
		},
		{
			name:     "NavData (F9P)",
			flags:    gpsprot.RawMsgNavData,
			version:  testVers.f9p,
			expected: map[bin.MsgID]uint8{bin.RxmRawxID: 0, bin.RxmSfrbxID: 1},
		},
		{
			name:     "Obs,NavData (F9P)",
			flags:    gpsprot.RawMsgObs | gpsprot.RawMsgNavData,
			version:  testVers.f9p,
			expected: map[bin.MsgID]uint8{bin.RxmRawxID: 1, bin.RxmSfrbxID: 1},
		},
		{
			name:     "None (F9P)",
			flags:    gpsprot.RawMsgNone,
			version:  testVers.f9p,
			expected: map[bin.MsgID]uint8{bin.RxmRawxID: 0, bin.RxmSfrbxID: 0},
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
			expected: map[bin.MsgID]uint8{},
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
	}{
		{
			name:        "f9p",
			version:     testVers.f9p,
			expectGNSS:  gpsprot.MajorGNSSSet,
			expectFlags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
		},
		{
			name:        "m8f",
			version:     testVers.m8f,
			expectGNSS:  0,
			expectFlags: 0,
		},
		{
			name:        "m8p",
			version:     testVers.m8p,
			expectGNSS:  gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GLO, gpsprot.BDS),
			expectFlags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
		},
		{
			name:        "lea6t",
			version:     testVers.lea6t,
			expectGNSS:  0,
			expectFlags: 0,
		},
		{
			name:        "f10s",
			version:     testVers.f10s,
			expectGNSS:  0,
			expectFlags: 0,
		},
		{
			name:        "f9t",
			version:     testVers.f9t,
			expectGNSS:  gpsprot.MajorGNSSSet,
			expectFlags: gpsprot.RTCMMsgMSM7,
		},
		{
			name:        "f9t20",
			version:     testVers.f9t20,
			expectGNSS:  gpsprot.MajorGNSSSet,
			expectFlags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gnss, flags := tt.version.rtcmSupport()
			if gnss != tt.expectGNSS {
				t.Errorf("GNSS support: got %v, want %v", gnss, tt.expectGNSS)
			}
			if flags != tt.expectFlags {
				t.Errorf("MSM flags: got %v, want %v", flags, tt.expectFlags)
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
		expectedRates   map[bin.MsgID]uint8
		expectedEnable  bin.CfgPrtProtoMask
		expectedDisable bin.CfgPrtProtoMask
		wantErr         bool
	}{
		{
			name:        "MSM4 GPS (F9P)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 1, // GPS MSM4
				bin.Rtcm1077ID: 0, // GPS MSM7 disabled
				bin.Rtcm1084ID: 0, // GLO MSM4 disabled
				bin.Rtcm1087ID: 0, // GLO MSM7 disabled
				bin.Rtcm1094ID: 0, // GAL MSM4 disabled
				bin.Rtcm1097ID: 0, // GAL MSM7 disabled
				bin.Rtcm1124ID: 0, // BDS MSM4 disabled
				bin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM7 GPS,GLO (F9P)",
			flags:       gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GLO),
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 0, // GPS MSM4 disabled
				bin.Rtcm1077ID: 1, // GPS MSM7
				bin.Rtcm1084ID: 0, // GLO MSM4 disabled
				bin.Rtcm1087ID: 1, // GLO MSM7
				bin.Rtcm1094ID: 0, // GAL MSM4 disabled
				bin.Rtcm1097ID: 0, // GAL MSM7 disabled
				bin.Rtcm1124ID: 0, // BDS MSM4 disabled
				bin.Rtcm1127ID: 0, // BDS MSM7 disabled
				bin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4+MSM7 GPS (F9P)",
			flags:       gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 1,
				bin.Rtcm1077ID: 1,
				bin.Rtcm1084ID: 0, // GLO disabled
				bin.Rtcm1087ID: 0, // GLO disabled
				bin.Rtcm1094ID: 0, // GAL disabled
				bin.Rtcm1097ID: 0, // GAL disabled
				bin.Rtcm1124ID: 0, // BDS disabled
				bin.Rtcm1127ID: 0, // BDS disabled
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:        "ARP (F9P)",
			flags:       gpsprot.RTCMMsgARP,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1005ID: 1, // ARP
				bin.Rtcm1074ID: 0, // GPS MSM4 disabled
				bin.Rtcm1077ID: 0, // GPS MSM7 disabled
				bin.Rtcm1084ID: 0, // GLO MSM4 disabled
				bin.Rtcm1087ID: 0, // GLO MSM7 disabled
				bin.Rtcm1094ID: 0, // GAL MSM4 disabled
				bin.Rtcm1097ID: 0, // GAL MSM7 disabled
				bin.Rtcm1124ID: 0, // BDS MSM4 disabled
				bin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:            "None (F9P)",
			flags:           gpsprot.RTCMMsgNone,
			version:         testVers.f9p,
			enabledGNSS:     gpsprot.GNSSSetOf(gpsprot.GPS),
			expectedDisable: bin.CfgPrtProtoRTCM3,
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
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 1, // GPS MSM4
				bin.Rtcm1077ID: 0, // GPS MSM7 disabled
				bin.Rtcm1084ID: 0, // GLO MSM4 disabled
				bin.Rtcm1087ID: 0, // GLO MSM7 disabled
				bin.Rtcm1124ID: 0, // BDS MSM4 disabled
				bin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
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
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1077ID: 1, // GPS MSM7
				bin.Rtcm1087ID: 0, // GLO MSM7 disabled
				bin.Rtcm1097ID: 0, // GAL MSM7 disabled
				bin.Rtcm1127ID: 0, // BDS MSM7 disabled
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
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
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 1, // GPS MSM4
				bin.Rtcm1077ID: 0, // GPS MSM7 disabled
				bin.Rtcm1084ID: 1, // GLO MSM4
				bin.Rtcm1087ID: 0, // GLO MSM7 disabled
				bin.Rtcm1094ID: 1, // GAL MSM4
				bin.Rtcm1097ID: 0, // GAL MSM7 disabled
				bin.Rtcm1124ID: 1, // BDS MSM4
				bin.Rtcm1127ID: 0, // BDS MSM7 disabled
				bin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM7 all major GNSS (F9P)",
			flags:       gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 0, // GPS MSM4 disabled
				bin.Rtcm1077ID: 1, // GPS MSM7
				bin.Rtcm1084ID: 0, // GLO MSM4 disabled
				bin.Rtcm1087ID: 1, // GLO MSM7
				bin.Rtcm1094ID: 0, // GAL MSM4 disabled
				bin.Rtcm1097ID: 1, // GAL MSM7
				bin.Rtcm1124ID: 0, // BDS MSM4 disabled
				bin.Rtcm1127ID: 1, // BDS MSM7
				bin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4+MSM7 all major GNSS (F9P)",
			flags:       gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			version:     testVers.f9p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 1, // GPS MSM4
				bin.Rtcm1077ID: 1, // GPS MSM7
				bin.Rtcm1084ID: 1, // GLO MSM4
				bin.Rtcm1087ID: 1, // GLO MSM7
				bin.Rtcm1094ID: 1, // GAL MSM4
				bin.Rtcm1097ID: 1, // GAL MSM7
				bin.Rtcm1124ID: 1, // BDS MSM4
				bin.Rtcm1127ID: 1, // BDS MSM7
				bin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM4 all major GNSS (M8P - no GAL)",
			flags:       gpsprot.RTCMMsgMSM4,
			version:     testVers.m8p,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 1, // GPS MSM4
				bin.Rtcm1077ID: 0, // GPS MSM7 disabled
				bin.Rtcm1084ID: 1, // GLO MSM4
				bin.Rtcm1087ID: 0, // GLO MSM7 disabled
				bin.Rtcm1124ID: 1, // BDS MSM4
				bin.Rtcm1127ID: 0, // BDS MSM7 disabled
				bin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
		},
		{
			name:        "MSM7 all major GNSS (F9T20)",
			flags:       gpsprot.RTCMMsgMSM7,
			version:     testVers.f9t20,
			enabledGNSS: gpsprot.MajorGNSSSet,
			expectedRates: map[bin.MsgID]uint8{
				bin.Rtcm1074ID: 0, // GPS MSM4 disabled
				bin.Rtcm1077ID: 1, // GPS MSM7
				bin.Rtcm1084ID: 0, // GLO MSM4 disabled
				bin.Rtcm1087ID: 1, // GLO MSM7
				bin.Rtcm1094ID: 0, // GAL MSM4 disabled
				bin.Rtcm1097ID: 1, // GAL MSM7
				bin.Rtcm1124ID: 0, // BDS MSM4 disabled
				bin.Rtcm1127ID: 1, // BDS MSM7
				bin.Rtcm1230ID: 1, // GLO bias
			},
			expectedEnable: bin.CfgPrtProtoRTCM3,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMsgChanges()
			err := mc.rtcm(tt.flags, &tt.version, tt.enabledGNSS)

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
		expectedRates map[bin.MsgID]uint8
	}{
		{
			name:    "SV (F9P - uses NAV-SAT)",
			flags:   gpsprot.SatsMsgSV,
			version: testVers.f9p,
			expectedRates: map[bin.MsgID]uint8{
				bin.NavSatID: 1,
			},
		},
		{
			name:    "None (F9P)",
			flags:   gpsprot.SatsMsgNone,
			version: testVers.f9p,
			expectedRates: map[bin.MsgID]uint8{
				bin.NavSatID: 0,
			},
		},
		{
			name:    "SV (LEA-6T - uses NAV-SVINFO)",
			flags:   gpsprot.SatsMsgSV,
			version: testVers.lea6t,
			expectedRates: map[bin.MsgID]uint8{
				bin.NavSVInfoID: 1,
			},
		},
		{
			name:    "None (LEA-6T)",
			flags:   gpsprot.SatsMsgNone,
			version: testVers.lea6t,
			expectedRates: map[bin.MsgID]uint8{
				bin.NavSVInfoID: 0,
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
		name             string
		flags            gpsprot.NMEAMsgFlags
		expectedRates    map[bin.MsgID]uint8
		expectedEnable   bin.CfgPrtProtoMask
		expectedDisable  bin.CfgPrtProtoMask
	}{
		{
			name:             "None - disable protocol",
			flags:            gpsprot.NMEAMsgNone,
			expectedRates:    map[bin.MsgID]uint8{},
			expectedEnable:   0,
			expectedDisable:  bin.CfgPrtProtoNMEA,
		},
		{
			name:            "RMC only",
			flags:           gpsprot.NMEAMsgRMC,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 1,
				bin.NmeaGgaID: 0,
				bin.NmeaGsaID: 0,
				bin.NmeaGsvID: 0,
				bin.NmeaZdaID: 0,
			},
		},
		{
			name:            "GGA only",
			flags:           gpsprot.NMEAMsgGGA,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 0,
				bin.NmeaGgaID: 1,
				bin.NmeaGsaID: 0,
				bin.NmeaGsvID: 0,
				bin.NmeaZdaID: 0,
			},
		},
		{
			name:            "GSA only",
			flags:           gpsprot.NMEAMsgGSA,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 0,
				bin.NmeaGgaID: 0,
				bin.NmeaGsaID: 1,
				bin.NmeaGsvID: 0,
				bin.NmeaZdaID: 0,
			},
		},
		{
			name:            "GSV only",
			flags:           gpsprot.NMEAMsgGSV,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 0,
				bin.NmeaGgaID: 0,
				bin.NmeaGsaID: 0,
				bin.NmeaGsvID: 1,
				bin.NmeaZdaID: 0,
			},
		},
		{
			name:            "ZDA only",
			flags:           gpsprot.NMEAMsgZDA,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 0,
				bin.NmeaGgaID: 0,
				bin.NmeaGsaID: 0,
				bin.NmeaGsvID: 0,
				bin.NmeaZdaID: 1,
			},
		},
		{
			name:            "Multiple messages",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSV,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 1,
				bin.NmeaGgaID: 1,
				bin.NmeaGsaID: 0,
				bin.NmeaGsvID: 1,
				bin.NmeaZdaID: 0,
			},
		},
		{
			name:            "All standard messages",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV | gpsprot.NMEAMsgZDA,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 1,
				bin.NmeaGgaID: 1,
				bin.NmeaGsaID: 1,
				bin.NmeaGsvID: 1,
				bin.NmeaZdaID: 1,
			},
		},
		{
			name:            "Other flag only - enables protocol but no specific messages",
			flags:           gpsprot.NMEAMsgOther,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 0,
				bin.NmeaGgaID: 0,
				bin.NmeaGsaID: 0,
				bin.NmeaGsvID: 0,
				bin.NmeaZdaID: 0,
			},
		},
		{
			name:            "RMC with Other flag",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgOther,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 1,
				bin.NmeaGgaID: 0,
				bin.NmeaGsaID: 0,
				bin.NmeaGsvID: 0,
				bin.NmeaZdaID: 0,
			},
		},
		{
			name:            "All messages with Other flag",
			flags:           gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV | gpsprot.NMEAMsgZDA | gpsprot.NMEAMsgOther,
			expectedEnable:  bin.CfgPrtProtoNMEA,
			expectedDisable: 0,
			expectedRates: map[bin.MsgID]uint8{
				bin.NmeaRmcID: 1,
				bin.NmeaGgaID: 1,
				bin.NmeaGsaID: 1,
				bin.NmeaGsvID: 1,
				bin.NmeaZdaID: 1,
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

