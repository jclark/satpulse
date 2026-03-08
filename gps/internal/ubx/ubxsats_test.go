package ubx

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestSatellitesCopy(t *testing.T) {
	tests := []struct {
		name  string
		input *gpsprot.SatellitesMsg
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name: "empty satellites msg",
			input: &gpsprot.SatellitesMsg{
				Tag:          Tag,
				NativeMsgID:  "TEST",
				SVs:          []gpsprot.SVInfo{},
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "single satellite with signals",
			input: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true},
							{CN0: 35, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45},
						Used:       true,
					},
				},
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites",
			input: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 5},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40, Used: false},
							{CN0: 38, Used: true},
						},
						Used: true,
					},
				},
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := satellitesCopy(tt.input)

			if tt.input == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			// Verify it's a different pointer
			if result == tt.input {
				t.Error("copy should return a different pointer")
			}

			// Verify top-level fields are copied correctly
			if result.Tag != tt.input.Tag {
				t.Errorf("expected Tag %v, got %v", tt.input.Tag, result.Tag)
			}
			if result.NativeMsgID != tt.input.NativeMsgID {
				t.Errorf("expected NativeMsgID %s, got %s", tt.input.NativeMsgID, result.NativeMsgID)
			}
			if result.UsedValidity != tt.input.UsedValidity {
				t.Errorf("expected UsedValidity %v, got %v", tt.input.UsedValidity, result.UsedValidity)
			}

			// Verify SVs slice is copied
			if len(result.SVs) != len(tt.input.SVs) {
				t.Errorf("expected %d SVs, got %d", len(tt.input.SVs), len(result.SVs))
			}
			if len(tt.input.SVs) > 0 && &result.SVs == &tt.input.SVs {
				t.Error("SVs slice should be copied")
			}

			// Verify each SVInfo is copied properly
			for i := range tt.input.SVs {
				if &result.SVs[i] == &tt.input.SVs[i] {
					t.Errorf("SVInfo %d should be copied", i)
				}
				if result.SVs[i].ID != tt.input.SVs[i].ID {
					t.Errorf("SV %d: expected ID %v, got %v", i, tt.input.SVs[i].ID, result.SVs[i].ID)
				}
				if result.SVs[i].Used != tt.input.SVs[i].Used {
					t.Errorf("SV %d: expected Used %v, got %v", i, tt.input.SVs[i].Used, result.SVs[i].Used)
				}

				// Verify LookAngles is NOT deep copied
				if result.SVs[i].LookAngles != tt.input.SVs[i].LookAngles {
					t.Errorf("SV %d: LookAngles should point to same object", i)
				}

				// Verify Signals slice is copied
				if len(result.SVs[i].Signals) != len(tt.input.SVs[i].Signals) {
					t.Errorf("SV %d: expected %d signals, got %d", i, len(tt.input.SVs[i].Signals), len(result.SVs[i].Signals))
				}
				if len(tt.input.SVs[i].Signals) > 0 && &result.SVs[i].Signals == &tt.input.SVs[i].Signals {
					t.Errorf("SV %d: Signals slice should be copied", i)
				}

				// Verify individual signals are copied correctly
				for j := range tt.input.SVs[i].Signals {
					if result.SVs[i].Signals[j] != tt.input.SVs[i].Signals[j] {
						t.Errorf("SV %d signal %d: expected %+v, got %+v", i, j, tt.input.SVs[i].Signals[j], result.SVs[i].Signals[j])
					}
				}
			}

			// Test data independence by modifying copied data
			if len(result.SVs) > 0 && len(result.SVs[0].Signals) > 0 {
				originalCN0 := tt.input.SVs[0].Signals[0].CN0
				result.SVs[0].Signals[0].CN0 = 99
				if tt.input.SVs[0].Signals[0].CN0 != originalCN0 {
					t.Error("modifying copied data should not affect original")
				}
			}
		})
	}
}

func TestSatellitesCombine(t *testing.T) {
	tests := []struct {
		name     string
		sats     *gpsprot.SatellitesMsg
		sigs     *gpsprot.SatellitesMsg
		expected *gpsprot.SatellitesMsg
	}{
		{
			name:     "both nil",
			sats:     nil,
			sigs:     nil,
			expected: nil,
		},
		{
			name: "sats nil, sigs not nil",
			sats: nil,
			sigs: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SIG",
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}},
				},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SIG",
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}},
				},
			},
		},
		{
			name: "sats not nil, sigs nil",
			sats: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}},
				},
			},
			sigs: nil,
			expected: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}},
				},
			},
		},
		{
			name: "used flag should be copied from sigs when signals are replaced",
			sats: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true}, // SAT says it's used
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45},
						Used:       true, // SAT says satellite is used
					},
				},
			},
			sigs: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SIG",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 42, Used: false}, // SIG says L1 not used
							{CN0: 38, Used: false}, // SIG says L5 not used
						},
						Used: false, // SIG correctly sets this to false (OR of signal Used flags)
					},
				},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 42, Used: false}, // Signals from SIG
							{CN0: 38, Used: false}, // Signals from SIG
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45}, // LookAngles from SAT
						Used:       false,                                           // Should be false (from SIG), not true (from SAT)
					},
				},
			},
		},
		{
			name: "combine overlapping satellites",
			sats: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 2},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 180, Elevation: 30},
						Used:       false,
					},
				},
			},
			sigs: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SIG",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 42, Used: true},
							{CN0: 38, Used: false},
						},
						Used: true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 3},
						Signals: []gpsprot.SignalInfo{
							{CN0: 35, Used: true},
						},
						Used: true,
					},
				},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 42, Used: true},
							{CN0: 38, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 2},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 180, Elevation: 30},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 3},
						Signals: []gpsprot.SignalInfo{
							{CN0: 35, Used: true},
						},
						Used: true,
					},
				},
			},
		},
		{
			name: "combine non-overlapping satellites",
			sats: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45},
						Used:       true,
					},
				},
			},
			sigs: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SIG",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 2},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40, Used: false},
						},
						Used: false,
					},
				},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 45},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 2},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40, Used: false},
						},
						Used: false,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := satellitesCombine(tt.sats, tt.sigs)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("satellitesCombine() = %+v, expected %+v", result, tt.expected)
			}
		})
	}
}

func TestSatellitesNavSat(t *testing.T) {
	tests := []struct {
		name     string
		input    ubxbin.NavSat
		expected gpsprot.SatellitesMsg
	}{
		{
			name: "empty satellites",
			input: ubxbin.NavSat{
				NavSatFixed: ubxbin.NavSatFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  0,
				},
				SVs: []ubxbin.NavSatSV{},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "NAV-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "single satellite with good quality",
			input: ubxbin.NavSat{
				NavSatFixed: ubxbin.NavSatFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  1,
				},
				SVs: []ubxbin.NavSatSV{
					{
						GNSSID: ubxbin.GPS,
						SVID:   1,
						CNO:    45,
						Elev:   30,
						Azim:   90,
						PRRes:  100,
						Flags:  ubxbin.NavSatQualityCodeLocked | ubxbin.NavSatSVUsed,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "satellite with poor quality (filtered out)",
			input: ubxbin.NavSat{
				NavSatFixed: ubxbin.NavSatFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  1,
				},
				SVs: []ubxbin.NavSatSV{
					{
						GNSSID: ubxbin.GPS,
						SVID:   1,
						CNO:    20,
						Elev:   30,
						Azim:   90,
						PRRes:  100,
						Flags:  ubxbin.NavSatQualitySearchingSignal,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "NAV-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites mixed quality",
			input: ubxbin.NavSat{
				NavSatFixed: ubxbin.NavSatFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  5,
				},
				SVs: []ubxbin.NavSatSV{
					{
						GNSSID: ubxbin.GPS,
						SVID:   1,
						CNO:    45,
						Elev:   30,
						Azim:   90,
						PRRes:  100,
						Flags:  ubxbin.NavSatQualityCodeLocked | ubxbin.NavSatSVUsed,
					},
					{
						GNSSID: ubxbin.GAL,
						SVID:   5,
						CNO:    25,
						Elev:   45,
						Azim:   180,
						PRRes:  -50,
						Flags:  ubxbin.NavSatQualitySignalAcquired,
					},
					{
						GNSSID: ubxbin.BDS,
						SVID:   10,
						CNO:    40,
						Elev:   60,
						Azim:   270,
						PRRes:  200,
						Flags:  ubxbin.NavSatQualityCodeAndCarrierLocked1,
					},
					{
						GNSSID: ubxbin.SBAS,
						SVID:   130, // SBAS satellite (130 - 100 = 30)
						CNO:    35,
						Elev:   70,
						Azim:   0,
						PRRes:  -75,
						Flags:  ubxbin.NavSatQualityCodeLocked | ubxbin.NavSatSVUsed,
					},
					{
						GNSSID: ubxbin.GLO,
						SVID:   255, // GLONASS unknown SVID
						CNO:    30,
						Elev:   20,
						Azim:   315,
						PRRes:  150,
						Flags:  ubxbin.NavSatQualityCodeAndCarrierLocked2,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDBDSB1I, CN0: 40, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 270, Elevation: 60},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 30},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 35, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 0, Elevation: 70},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: gpsprot.GLOUnknown},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGLOL1, CN0: 30, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 315, Elevation: 20},
						Used:       false,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := satellitesNavSat(&tt.input)

			if !reflect.DeepEqual(*result, tt.expected) {
				t.Errorf("satellitesNavSat() = %+v, expected %+v", *result, tt.expected)
			}
		})
	}
}

func TestSatellitesNavSig(t *testing.T) {
	tests := []struct {
		name     string
		input    ubxbin.NavSig
		expected gpsprot.SatellitesMsg
	}{
		{
			name: "empty signals",
			input: ubxbin.NavSig{
				NavSigFixed: ubxbin.NavSigFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 0,
				},
				Signals: []ubxbin.NavSigSignal{},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "single signal with good quality",
			input: ubxbin.NavSig{
				NavSigFixed: ubxbin.NavSigFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 1,
				},
				Signals: []ubxbin.NavSigSignal{
					{
						GNSSID:     ubxbin.GPS,
						SVID:       1,
						SigID:      0, // L1 C/A
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   ubxbin.NavSigPrUsed,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
						},
						Used: true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "signal with poor quality (filtered out)",
			input: ubxbin.NavSig{
				NavSigFixed: ubxbin.NavSigFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 1,
				},
				Signals: []ubxbin.NavSigSignal{
					{
						GNSSID:     ubxbin.GPS,
						SVID:       1,
						SigID:      0,
						FreqID:     0,
						PRRes:      100,
						CNO:        20,
						QualityInd: ubxbin.NavSigQualitySearching, // Below minimum quality
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   0,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple signals same satellite",
			input: ubxbin.NavSig{
				NavSigFixed: ubxbin.NavSigFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 2,
				},
				Signals: []ubxbin.NavSigSignal{
					{
						GNSSID:     ubxbin.GPS,
						SVID:       1,
						SigID:      0, // L1 C/A
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   ubxbin.NavSigPrUsed,
					},
					{
						GNSSID:     ubxbin.GPS,
						SVID:       1,
						SigID:      6, // L5 I
						FreqID:     0,
						PRRes:      150,
						CNO:        42,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   0, // Not used
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
							{ID: gpsprot.SigIDGPSL5I, CN0: 42, Used: false},
						},
						Used: true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites mixed quality",
			input: ubxbin.NavSig{
				NavSigFixed: ubxbin.NavSigFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 3,
				},
				Signals: []ubxbin.NavSigSignal{
					{
						GNSSID:     ubxbin.GPS,
						SVID:       1,
						SigID:      0,
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   ubxbin.NavSigPrUsed,
					},
					{
						GNSSID:     ubxbin.GAL,
						SVID:       5,
						SigID:      0, // E1 C
						FreqID:     0,
						PRRes:      -50,
						CNO:        25,
						QualityInd: ubxbin.NavSigQualityAcquired, // Below minimum quality
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   0,
					},
					{
						GNSSID:     ubxbin.BDS,
						SVID:       10,
						SigID:      0, // B1I D1
						FreqID:     0,
						PRRes:      200,
						CNO:        40,
						QualityInd: ubxbin.NavSigQualityCodeCarrierLocked5,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   0,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
						},
						Used: true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDBDSB1ID1, CN0: 40, Used: false},
						},
						Used: false,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites each with multiple signals",
			input: ubxbin.NavSig{
				NavSigFixed: ubxbin.NavSigFixed{
					NavITOW: ubxbin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 4,
				},
				Signals: []ubxbin.NavSigSignal{
					{
						GNSSID:     ubxbin.GPS,
						SVID:       1,
						SigID:      0, // L1 C/A
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   ubxbin.NavSigPrUsed,
					},
					{
						GNSSID:     ubxbin.GPS,
						SVID:       1,
						SigID:      6, // L5 I
						FreqID:     0,
						PRRes:      150,
						CNO:        42,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   0, // Not used
					},
					{
						GNSSID:     ubxbin.GAL,
						SVID:       5,
						SigID:      0, // E1 C
						FreqID:     0,
						PRRes:      -50,
						CNO:        38,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   ubxbin.NavSigPrUsed,
					},
					{
						GNSSID:     ubxbin.GAL,
						SVID:       5,
						SigID:      3, // E5a I
						FreqID:     0,
						PRRes:      75,
						CNO:        35,
						QualityInd: ubxbin.NavSigQualityCodeLocked,
						CorrSource: ubxbin.NavSigCorrSourceNone,
						IonoModel:  ubxbin.NavSigIonoModelNone,
						SigFlags:   0, // Not used
					},
				},
			},
			// Should group signals by satellite, but current buggy implementation creates separate entries
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
							{ID: gpsprot.SigIDGPSL5I, CN0: 42, Used: false},
						},
						Used: true, // At least one signal is used
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 5},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGALE1C, CN0: 38, Used: true},
							{ID: gpsprot.SigIDGALE5aI, CN0: 35, Used: false},
						},
						Used: true, // At least one signal is used
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := satellitesNavSig(&tt.input)

			if !reflect.DeepEqual(*result, tt.expected) {
				t.Errorf("satellitesNavSig() = %+v, expected %+v", *result, tt.expected)
			}
		})
	}
}

func TestCorrKindNavSig(t *testing.T) {
	tests := []struct {
		name string
		sigs []ubxbin.NavSigSignal
		want gpsprot.CorrKind
	}{
		{
			name: "no signals",
			want: 0,
		},
		{
			name: "unused signal with RTCM correction",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceRTCM3OSR, SigFlags: 0},
			},
			want: 0,
		},
		{
			name: "no correction",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceNone, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: 0,
		},
		{
			name: "RTCM3 OSR",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceRTCM3OSR, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrOSR,
		},
		{
			name: "RTCM2",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceRTCM2, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrOSR,
		},
		{
			name: "RTCM3 SSR",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceRTCM3SSR, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrSSR,
		},
		{
			name: "SBAS",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceSBAS, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSBAS,
		},
		{
			name: "SPARTN",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceSPARTN, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSPARTN,
		},
		{
			name: "CLAS",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceCLAS, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS,
		},
		{
			name: "QZSS SLAS",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceQZSSSLAS, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS,
		},
		{
			name: "BeiDou",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceBeiDou, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR,
		},
		{
			name: "multiple signals same source",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceRTCM3OSR, SigFlags: ubxbin.NavSigPrUsed},
				{CorrSource: ubxbin.NavSigCorrSourceRTCM3OSR, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrOSR,
		},
		{
			name: "conflict: base-station and wide-area",
			sigs: []ubxbin.NavSigSignal{
				{CorrSource: ubxbin.NavSigCorrSourceRTCM3OSR, SigFlags: ubxbin.NavSigPrUsed},
				{CorrSource: ubxbin.NavSigCorrSourceSBAS, SigFlags: ubxbin.NavSigPrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ubxbin.NavSig{
				NavSigFixed: ubxbin.NavSigFixed{NumSigs: byte(len(tt.sigs))},
				Signals:     tt.sigs,
			}
			got := corrKindNavSig(m)
			if got != tt.want {
				t.Errorf("corrKindNavSig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCorrKindNavSat(t *testing.T) {
	tests := []struct {
		name string
		svs  []ubxbin.NavSatSV
		want gpsprot.CorrKind
	}{
		{
			name: "no SVs",
			want: 0,
		},
		{
			name: "unused SV with SBAS correction",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSbasCorrUsed},
			},
			want: 0,
		},
		{
			name: "SBAS correction",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatSbasCorrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSBAS,
		},
		{
			name: "RTCM correction",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatRtcmCorrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM,
		},
		{
			name: "SPARTN correction",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatSpartnCorrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSPARTN,
		},
		{
			name: "SLAS correction",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatSlasCorrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS,
		},
		{
			name: "CLAS correction",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatClasCorrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS,
		},
		{
			name: "RTCM and SBAS combined",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatRtcmCorrUsed | ubxbin.NavSatSbasCorrUsed},
			},
			// No conflict: NavSat RTCM doesn't set CorrOSR (OSR vs SSR unknown)
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrSSR | gpsprot.CorrSBAS,
		},
		{
			name: "multiple SVs, same correction",
			svs: []ubxbin.NavSatSV{
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatRtcmCorrUsed},
				{Flags: ubxbin.NavSatSVUsed | ubxbin.NavSatRtcmCorrUsed},
			},
			want: gpsprot.CorrUsed | gpsprot.CorrRTCM,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ubxbin.NavSat{
				NavSatFixed: ubxbin.NavSatFixed{NumSVs: byte(len(tt.svs))},
				SVs:         tt.svs,
			}
			got := corrKindNavSat(m)
			if got != tt.want {
				t.Errorf("corrKindNavSat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSatellitesNavSVInfo(t *testing.T) {
	tests := []struct {
		name     string
		input    ubxbin.NavSVInfo
		expected gpsprot.SatellitesMsg
	}{
		{
			name: "empty satellites",
			input: ubxbin.NavSVInfo{
				NavSVInfoFixed: ubxbin.NavSVInfoFixed{
					NavITOW:     ubxbin.NavITOW{ITOW: 12345},
					NumCh:       0,
					GlobalFlags: ubxbin.NavSVInfoUblox6,
				},
				SVs: []ubxbin.NavSVInfoSV{},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "single satellite with good quality",
			input: ubxbin.NavSVInfo{
				NavSVInfoFixed: ubxbin.NavSVInfoFixed{
					NavITOW:     ubxbin.NavITOW{ITOW: 12345},
					NumCh:       1,
					GlobalFlags: ubxbin.NavSVInfoUblox6,
				},
				SVs: []ubxbin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    1, // GPS satellite 1
						Flags:   ubxbin.NavSVInfoSVUsed,
						Quality: ubxbin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     45,
						Elev:    30,
						Azim:    90,
						PRRes:   100,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "satellite with poor quality (filtered out)",
			input: ubxbin.NavSVInfo{
				NavSVInfoFixed: ubxbin.NavSVInfoFixed{
					NavITOW:     ubxbin.NavITOW{ITOW: 12345},
					NumCh:       1,
					GlobalFlags: ubxbin.NavSVInfoUblox6,
				},
				SVs: []ubxbin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    1,
						Flags:   0,
						Quality: ubxbin.NavSVInfoQualitySignalAcquired, // Below minimum quality
						CNO:     25,
						Elev:    30,
						Azim:    90,
						PRRes:   100,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites mixed quality and usage",
			input: ubxbin.NavSVInfo{
				NavSVInfoFixed: ubxbin.NavSVInfoFixed{
					NavITOW:     ubxbin.NavITOW{ITOW: 12345},
					NumCh:       4,
					GlobalFlags: ubxbin.NavSVInfoUblox6,
				},
				SVs: []ubxbin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    1, // GPS satellite 1
						Flags:   ubxbin.NavSVInfoSVUsed,
						Quality: ubxbin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     45,
						Elev:    30,
						Azim:    90,
						PRRes:   100,
					},
					{
						ChN:     2,
						SVID:    5, // GPS satellite 5
						Flags:   0, // Not used
						Quality: ubxbin.NavSVInfoCodeAndCarrierLocked1,
						CNO:     40,
						Elev:    45,
						Azim:    180,
						PRRes:   -50,
					},
					{
						ChN:     3,
						SVID:    130, // SBAS satellite (130 - 100 = 30)
						Flags:   ubxbin.NavSVInfoSVUsed,
						Quality: ubxbin.NavSVInfoCodeAndCarrierLocked2,
						CNO:     35,
						Elev:    60,
						Azim:    270,
						PRRes:   200,
					},
					{
						ChN:     4,
						SVID:    193, // QZSS satellite (193 - 192 = 1)
						Flags:   0,   // Not used, poor quality
						Quality: ubxbin.NavSVInfoQualitySearching,
						CNO:     20,
						Elev:    15,
						Azim:    45,
						PRRes:   300,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 40},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 180, Elevation: 45},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 30},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 35, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 270, Elevation: 60},
						Used:       true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "comprehensive SVID ranges coverage",
			input: ubxbin.NavSVInfo{
				NavSVInfoFixed: ubxbin.NavSVInfoFixed{
					NavITOW:     ubxbin.NavITOW{ITOW: 12345},
					NumCh:       7,
					GlobalFlags: ubxbin.NavSVInfoUblox6,
				},
				SVs: []ubxbin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    32, // GPS satellite 32 (max typical GPS)
						Flags:   ubxbin.NavSVInfoSVUsed,
						Quality: ubxbin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     42,
						Elev:    25,
						Azim:    45,
						PRRes:   50,
					},
					{
						ChN:     2,
						SVID:    65, // GLONASS satellite 1 (65 - 64 = 1)
						Flags:   ubxbin.NavSVInfoSVUsed,
						Quality: ubxbin.NavSVInfoCodeAndCarrierLocked1,
						CNO:     38,
						Elev:    35,
						Azim:    135,
						PRRes:   -25,
					},
					{
						ChN:     3,
						SVID:    96, // GLONASS satellite 32 (96 - 64 = 32, max)
						Flags:   0,
						Quality: ubxbin.NavSVInfoCodeAndCarrierLocked2,
						CNO:     33,
						Elev:    55,
						Azim:    225,
						PRRes:   75,
					},
					{
						ChN:     4,
						SVID:    120, // SBAS satellite 20 (120 - 100 = 20, min)
						Flags:   ubxbin.NavSVInfoSVUsed,
						Quality: ubxbin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     30,
						Elev:    65,
						Azim:    315,
						PRRes:   100,
					},
					{
						ChN:     5,
						SVID:    158, // SBAS satellite 58 (158 - 100 = 58, max)
						Flags:   0,
						Quality: ubxbin.NavSVInfoCodeAndCarrierLocked3,
						CNO:     28,
						Elev:    70,
						Azim:    0,
						PRRes:   -100,
					},
					{
						ChN:     6,
						SVID:    197, // QZSS satellite 5 (197 - 192 = 5, max)
						Flags:   ubxbin.NavSVInfoSVUsed,
						Quality: ubxbin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     40,
						Elev:    40,
						Azim:    180,
						PRRes:   25,
					},
					{
						ChN:     7,
						SVID:    255, // GLONASS unknown
						Flags:   0,
						Quality: ubxbin.NavSVInfoCodeAndCarrierLocked1,
						CNO:     20,
						Elev:    10,
						Azim:    90,
						PRRes:   200,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 32},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 42, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 45, Elevation: 25},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGLOL1, CN0: 38, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 135, Elevation: 35},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 32},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGLOL1, CN0: 33},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 225, Elevation: 55},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 20},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 30, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 315, Elevation: 65},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 58},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGPSL1CA, CN0: 28},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 0, Elevation: 70},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 5},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDQZSSL1CA, CN0: 40, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 180, Elevation: 40},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: gpsprot.GLOUnknown},
						Signals: []gpsprot.SignalInfo{
							{ID: gpsprot.SigIDGLOL1, CN0: 20},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 10},
						Used:       false,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := satellitesNavSVInfo(&tt.input)

			if !reflect.DeepEqual(*result, tt.expected) {
				t.Errorf("satellitesNavSVInfo() = %+v, expected %+v", *result, tt.expected)
			}
		})
	}
}
