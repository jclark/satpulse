package ubx

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
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
				UsedValidity: gpsprot.SatelliteUsedSV,
			},
		},
		{
			name: "single satellite with signals",
			input: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "UBX-NAV-SAT",
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
				NativeMsgID: "UBX-NAV-SAT",
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
		name string
		sats *gpsprot.SatellitesMsg
		sigs *gpsprot.SatellitesMsg
	}{
		{
			name: "both nil",
			sats: nil,
			sigs: nil,
		},
		{
			name: "sats nil, sigs not nil",
			sats: nil,
			sigs: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "UBX-NAV-SIG",
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}},
				},
			},
		},
		{
			name: "sats not nil, sigs nil",
			sats: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "UBX-NAV-SAT",
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}},
				},
			},
			sigs: nil,
		},
		{
			name: "combine overlapping satellites",
			sats: &gpsprot.SatellitesMsg{
				Tag:         Tag,
				NativeMsgID: "UBX-NAV-SAT",
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
				NativeMsgID: "UBX-NAV-SIG",
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 42, Used: true},
							{CN0: 38, Used: false},
						},
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 3},
						Signals: []gpsprot.SignalInfo{
							{CN0: 35, Used: true},
						},
					},
				},
			},
		},
		{
			name: "combine non-overlapping satellites",
			sats: &gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID:      gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{{CN0: 45}},
					},
				},
			},
			sigs: &gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID:      gpsprot.SVID{GNSS: gpsprot.GAL, Num: 2},
						Signals: []gpsprot.SignalInfo{{CN0: 40}},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := satellitesCombine(tt.sats, tt.sigs)

			if tt.sats == nil && tt.sigs == nil {
				if result != nil {
					t.Error("expected nil when both inputs are nil")
				}
				return
			}

			if tt.sats == nil {
				if result != tt.sigs {
					t.Error("expected sigs to be returned when sats is nil")
				}
				return
			}

			if tt.sigs == nil {
				if result != tt.sats {
					t.Error("expected sats to be returned when sigs is nil")
				}
				return
			}

			// When both are non-nil, result should be a copy of sats
			if result == tt.sats {
				t.Error("result should be a copy of sats, not the same pointer")
			}

			// Calculate expected number of satellites
			expectedCount := len(tt.sats.SVs)
			for _, sigSV := range tt.sigs.SVs {
				found := false
				for _, satSV := range tt.sats.SVs {
					if sigSV.ID == satSV.ID {
						found = true
						break
					}
				}
				if !found {
					expectedCount++
				}
			}

			if len(result.SVs) != expectedCount {
				t.Errorf("expected %d SVs, got %d", expectedCount, len(result.SVs))
			}

			// Verify satellites from sigs are present and signals are replaced
			for _, sigSV := range tt.sigs.SVs {
				found := false
				for _, resSV := range result.SVs {
					if resSV.ID == sigSV.ID {
						found = true
						if len(resSV.Signals) != len(sigSV.Signals) {
							t.Errorf("SV %v: expected %d signals, got %d", resSV.ID, len(sigSV.Signals), len(resSV.Signals))
						}
						for i, signal := range sigSV.Signals {
							if resSV.Signals[i] != signal {
								t.Errorf("SV %v signal %d: expected %+v, got %+v", resSV.ID, i, signal, resSV.Signals[i])
							}
						}
						break
					}
				}
				if !found {
					t.Errorf("SV %v from sigs not found in result", sigSV.ID)
				}
			}

			// Verify satellites from sats that don't overlap preserve LookAngles
			for _, satSV := range tt.sats.SVs {
				for _, resSV := range result.SVs {
					if resSV.ID == satSV.ID {
						if resSV.LookAngles != satSV.LookAngles {
							t.Errorf("SV %v: LookAngles should be preserved from sats", resSV.ID)
						}
						break
					}
				}
			}
		})
	}
}

func TestSatellitesNavSat(t *testing.T) {
	tests := []struct {
		name     string
		input    bin.NavSat
		expected gpsprot.SatellitesMsg
	}{
		{
			name: "empty satellites",
			input: bin.NavSat{
				NavSatFixed: bin.NavSatFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  0,
				},
				SVs: []bin.NavSatSV{},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "single satellite with good quality",
			input: bin.NavSat{
				NavSatFixed: bin.NavSatFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  1,
				},
				SVs: []bin.NavSatSV{
					{
						GNSSID: bin.GPS,
						SVID:   1,
						CNO:    45,
						Elev:   30,
						Azim:   90,
						PRRes:  100,
						Flags:  bin.NavSatQualityCodeLocked | bin.NavSatSVUsed,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "satellite with poor quality (filtered out)",
			input: bin.NavSat{
				NavSatFixed: bin.NavSatFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  1,
				},
				SVs: []bin.NavSatSV{
					{
						GNSSID: bin.GPS,
						SVID:   1,
						CNO:    20,
						Elev:   30,
						Azim:   90,
						PRRes:  100,
						Flags:  bin.NavSatQualitySearchingSignal,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites mixed quality",
			input: bin.NavSat{
				NavSatFixed: bin.NavSatFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 1,
					NumSVs:  5,
				},
				SVs: []bin.NavSatSV{
					{
						GNSSID: bin.GPS,
						SVID:   1,
						CNO:    45,
						Elev:   30,
						Azim:   90,
						PRRes:  100,
						Flags:  bin.NavSatQualityCodeLocked | bin.NavSatSVUsed,
					},
					{
						GNSSID: bin.GAL,
						SVID:   5,
						CNO:    25,
						Elev:   45,
						Azim:   180,
						PRRes:  -50,
						Flags:  bin.NavSatQualitySignalAcquired,
					},
					{
						GNSSID: bin.BDS,
						SVID:   10,
						CNO:    40,
						Elev:   60,
						Azim:   270,
						PRRes:  200,
						Flags:  bin.NavSatQualityCodeAndCarrierLocked1,
					},
					{
						GNSSID: bin.SBAS,
						SVID:   130, // SBAS satellite (130 - 100 = 30)
						CNO:    35,
						Elev:   70,
						Azim:   0,
						PRRes:  -75,
						Flags:  bin.NavSatQualityCodeLocked | bin.NavSatSVUsed,
					},
					{
						GNSSID: bin.GLO,
						SVID:   255, // GLONASS unknown SVID
						CNO:    30,
						Elev:   20,
						Azim:   315,
						PRRes:  150,
						Flags:  bin.NavSatQualityCodeAndCarrierLocked2,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 45, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 270, Elevation: 60},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 30},
						Signals: []gpsprot.SignalInfo{
							{CN0: 35, Used: true},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 0, Elevation: 70},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: gpsprot.GLOUnknown},
						Signals: []gpsprot.SignalInfo{
							{CN0: 30, Used: false},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 315, Elevation: 20},
						Used:       false,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SAT",
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
		input    bin.NavSig
		expected gpsprot.SatellitesMsg
	}{
		{
			name: "empty signals",
			input: bin.NavSig{
				NavSigFixed: bin.NavSigFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 0,
				},
				Signals: []bin.NavSigSignal{},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "single signal with good quality",
			input: bin.NavSig{
				NavSigFixed: bin.NavSigFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 1,
				},
				Signals: []bin.NavSigSignal{
					{
						GNSSID:     bin.GPS,
						SVID:       1,
						SigID:      0, // L1 C/A
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   bin.NavSigPrUsed,
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
				NativeMsgID:  "UBX-NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "signal with poor quality (filtered out)",
			input: bin.NavSig{
				NavSigFixed: bin.NavSigFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 1,
				},
				Signals: []bin.NavSigSignal{
					{
						GNSSID:     bin.GPS,
						SVID:       1,
						SigID:      0,
						FreqID:     0,
						PRRes:      100,
						CNO:        20,
						QualityInd: bin.NavSigQualitySearching, // Below minimum quality
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   0,
					},
				},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple signals same satellite",
			input: bin.NavSig{
				NavSigFixed: bin.NavSigFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 2,
				},
				Signals: []bin.NavSigSignal{
					{
						GNSSID:     bin.GPS,
						SVID:       1,
						SigID:      0, // L1 C/A
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   bin.NavSigPrUsed,
					},
					{
						GNSSID:     bin.GPS,
						SVID:       1,
						SigID:      6, // L5 I
						FreqID:     0,
						PRRes:      150,
						CNO:        42,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
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
				NativeMsgID:  "UBX-NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites mixed quality",
			input: bin.NavSig{
				NavSigFixed: bin.NavSigFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 3,
				},
				Signals: []bin.NavSigSignal{
					{
						GNSSID:     bin.GPS,
						SVID:       1,
						SigID:      0,
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   bin.NavSigPrUsed,
					},
					{
						GNSSID:     bin.GAL,
						SVID:       5,
						SigID:      0, // E1 C
						FreqID:     0,
						PRRes:      -50,
						CNO:        25,
						QualityInd: bin.NavSigQualityAcquired, // Below minimum quality
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   0,
					},
					{
						GNSSID:     bin.BDS,
						SVID:       10,
						SigID:      0, // B1I D1
						FreqID:     0,
						PRRes:      200,
						CNO:        40,
						QualityInd: bin.NavSigQualityCodeCarrierLocked5,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
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
				NativeMsgID:  "UBX-NAV-SIG",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multiple satellites each with multiple signals",
			input: bin.NavSig{
				NavSigFixed: bin.NavSigFixed{
					NavITOW: bin.NavITOW{ITOW: 12345},
					Version: 0,
					NumSigs: 4,
				},
				Signals: []bin.NavSigSignal{
					{
						GNSSID:     bin.GPS,
						SVID:       1,
						SigID:      0, // L1 C/A
						FreqID:     0,
						PRRes:      100,
						CNO:        45,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   bin.NavSigPrUsed,
					},
					{
						GNSSID:     bin.GPS,
						SVID:       1,
						SigID:      6, // L5 I
						FreqID:     0,
						PRRes:      150,
						CNO:        42,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   0, // Not used
					},
					{
						GNSSID:     bin.GAL,
						SVID:       5,
						SigID:      0, // E1 C
						FreqID:     0,
						PRRes:      -50,
						CNO:        38,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
						SigFlags:   bin.NavSigPrUsed,
					},
					{
						GNSSID:     bin.GAL,
						SVID:       5,
						SigID:      3, // E5a I
						FreqID:     0,
						PRRes:      75,
						CNO:        35,
						QualityInd: bin.NavSigQualityCodeLocked,
						CorrSource: bin.NavSigCorrSourceNone,
						IonoModel:  bin.NavSigIonoModelNone,
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
				NativeMsgID:  "UBX-NAV-SIG",
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

func TestSatellitesNavSVInfo(t *testing.T) {
	tests := []struct {
		name     string
		input    bin.NavSVInfo
		expected gpsprot.SatellitesMsg
	}{
		{
			name: "empty satellites",
			input: bin.NavSVInfo{
				NavSVInfoFixed: bin.NavSVInfoFixed{
					NavITOW:     bin.NavITOW{ITOW: 12345},
					NumCh:       0,
					GlobalFlags: bin.NavSVInfoUblox6,
				},
				SVs: []bin.NavSVInfoSV{},
			},
			expected: gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSV,
			},
		},
		{
			name: "single satellite with good quality",
			input: bin.NavSVInfo{
				NavSVInfoFixed: bin.NavSVInfoFixed{
					NavITOW:     bin.NavITOW{ITOW: 12345},
					NumCh:       1,
					GlobalFlags: bin.NavSVInfoUblox6,
				},
				SVs: []bin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    1, // GPS satellite 1
						Flags:   bin.NavSVInfoSVUsed,
						Quality: bin.NavSVInfoQualityCodeLockOnSignal,
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
							{CN0: 45},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSV,
			},
		},
		{
			name: "satellite with poor quality (filtered out)",
			input: bin.NavSVInfo{
				NavSVInfoFixed: bin.NavSVInfoFixed{
					NavITOW:     bin.NavITOW{ITOW: 12345},
					NumCh:       1,
					GlobalFlags: bin.NavSVInfoUblox6,
				},
				SVs: []bin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    1,
						Flags:   0,
						Quality: bin.NavSVInfoQualitySignalAcquired, // Below minimum quality
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
				NativeMsgID:  "UBX-NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSV,
			},
		},
		{
			name: "multiple satellites mixed quality and usage",
			input: bin.NavSVInfo{
				NavSVInfoFixed: bin.NavSVInfoFixed{
					NavITOW:     bin.NavITOW{ITOW: 12345},
					NumCh:       4,
					GlobalFlags: bin.NavSVInfoUblox6,
				},
				SVs: []bin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    1, // GPS satellite 1
						Flags:   bin.NavSVInfoSVUsed,
						Quality: bin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     45,
						Elev:    30,
						Azim:    90,
						PRRes:   100,
					},
					{
						ChN:     2,
						SVID:    5, // GPS satellite 5
						Flags:   0, // Not used
						Quality: bin.NavSVInfoCodeAndCarrierLocked1,
						CNO:     40,
						Elev:    45,
						Azim:    180,
						PRRes:   -50,
					},
					{
						ChN:     3,
						SVID:    130, // SBAS satellite (130 - 100 = 30)
						Flags:   bin.NavSVInfoSVUsed,
						Quality: bin.NavSVInfoCodeAndCarrierLocked2,
						CNO:     35,
						Elev:    60,
						Azim:    270,
						PRRes:   200,
					},
					{
						ChN:     4,
						SVID:    193, // QZSS satellite (193 - 192 = 1)
						Flags:   0,   // Not used, poor quality
						Quality: bin.NavSVInfoQualitySearching,
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
							{CN0: 45},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 30},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 180, Elevation: 45},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 30},
						Signals: []gpsprot.SignalInfo{
							{CN0: 35},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 270, Elevation: 60},
						Used:       true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSV,
			},
		},
		{
			name: "comprehensive SVID ranges coverage",
			input: bin.NavSVInfo{
				NavSVInfoFixed: bin.NavSVInfoFixed{
					NavITOW:     bin.NavITOW{ITOW: 12345},
					NumCh:       7,
					GlobalFlags: bin.NavSVInfoUblox6,
				},
				SVs: []bin.NavSVInfoSV{
					{
						ChN:     1,
						SVID:    32, // GPS satellite 32 (max typical GPS)
						Flags:   bin.NavSVInfoSVUsed,
						Quality: bin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     42,
						Elev:    25,
						Azim:    45,
						PRRes:   50,
					},
					{
						ChN:     2,
						SVID:    65, // GLONASS satellite 1 (65 - 64 = 1)
						Flags:   bin.NavSVInfoSVUsed,
						Quality: bin.NavSVInfoCodeAndCarrierLocked1,
						CNO:     38,
						Elev:    35,
						Azim:    135,
						PRRes:   -25,
					},
					{
						ChN:     3,
						SVID:    96, // GLONASS satellite 32 (96 - 64 = 32, max)
						Flags:   0,
						Quality: bin.NavSVInfoCodeAndCarrierLocked2,
						CNO:     33,
						Elev:    55,
						Azim:    225,
						PRRes:   75,
					},
					{
						ChN:     4,
						SVID:    120, // SBAS satellite 20 (120 - 100 = 20, min)
						Flags:   bin.NavSVInfoSVUsed,
						Quality: bin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     30,
						Elev:    65,
						Azim:    315,
						PRRes:   100,
					},
					{
						ChN:     5,
						SVID:    158, // SBAS satellite 58 (158 - 100 = 58, max)
						Flags:   0,
						Quality: bin.NavSVInfoCodeAndCarrierLocked3,
						CNO:     28,
						Elev:    70,
						Azim:    0,
						PRRes:   -100,
					},
					{
						ChN:     6,
						SVID:    197, // QZSS satellite 5 (197 - 192 = 5, max)
						Flags:   bin.NavSVInfoSVUsed,
						Quality: bin.NavSVInfoQualityCodeLockOnSignal,
						CNO:     40,
						Elev:    40,
						Azim:    180,
						PRRes:   25,
					},
					{
						ChN:     7,
						SVID:    255, // GLONASS unknown
						Flags:   0,
						Quality: bin.NavSVInfoCodeAndCarrierLocked1,
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
							{CN0: 42},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 45, Elevation: 25},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 1},
						Signals: []gpsprot.SignalInfo{
							{CN0: 38},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 135, Elevation: 35},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 32},
						Signals: []gpsprot.SignalInfo{
							{CN0: 33},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 225, Elevation: 55},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 20},
						Signals: []gpsprot.SignalInfo{
							{CN0: 30},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 315, Elevation: 65},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 58},
						Signals: []gpsprot.SignalInfo{
							{CN0: 28},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 0, Elevation: 70},
						Used:       false,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 5},
						Signals: []gpsprot.SignalInfo{
							{CN0: 40},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 180, Elevation: 40},
						Used:       true,
					},
					{
						ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: gpsprot.GLOUnknown},
						Signals: []gpsprot.SignalInfo{
							{CN0: 20},
						},
						LookAngles: &gpsprot.LookAngles{Azimuth: 90, Elevation: 10},
						Used:       false,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "UBX-NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSV,
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
