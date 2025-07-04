package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

func TestTmodeConfigRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		mode   gpsprot.Mode
		survey gpsprot.Survey
	}{
		{
			name: "disabled",
			mode: gpsprot.Mode{
				Static: false,
			},
			survey: gpsprot.Survey{},
		},
		{
			name: "survey_in",
			mode: gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone,
			},
			survey: gpsprot.Survey{
				MinDur:   120 * time.Second,
				AccLimit: 2 * gpsprot.Meter,
			},
		},
		{
			name: "fixed_ecef",
			mode: gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeECEF,
				FixedPosECEF: [3]gpsprot.Length{
					4194304 * gpsprot.Meter,
					837860 * gpsprot.Meter,
					4581200 * gpsprot.Meter,
				},
				FixedPosAcc: 10 * gpsprot.Millimeter,
			},
			survey: gpsprot.Survey{},
		},
		{
			name: "fixed_ecef_hp",
			mode: gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeECEF,
				FixedPosECEF: [3]gpsprot.Length{
					41943040053 * (gpsprot.Millimeter / 10),
					8378600127 * (gpsprot.Millimeter / 10),
					45812000089 * (gpsprot.Millimeter / 10),
				},
				FixedPosAcc: 5 * gpsprot.Millimeter,
			},
			survey: gpsprot.Survey{},
		},
		{
			name: "fixed_llh",
			mode: gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeLLH,
				FixedPosLLH: [2]gpsprot.Angle{
					47 * gpsprot.Degrees,
					8 * gpsprot.Degrees,
				},
				Height:      400 * gpsprot.Meter,
				FixedPosAcc: 20 * gpsprot.Millimeter,
			},
			survey: gpsprot.Survey{},
		},
		{
			name: "fixed_llh_hp",
			mode: gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeLLH,
				FixedPosLLH: [2]gpsprot.Angle{
					47123456789 * gpsprot.Nanodegrees,
					8987654321 * gpsprot.Nanodegrees,
				},
				Height:      4001234 * (gpsprot.Millimeter / 10),
				FixedPosAcc: 15 * gpsprot.Millimeter,
			},
			survey: gpsprot.Survey{},
		},
		{
			name: "survey_in_detailed",
			mode: gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeNone,
			},
			survey: gpsprot.Survey{
				MinDur:   300 * time.Second,
				AccLimit: 500 * gpsprot.Millimeter,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test gpsprot.Mode/Survey -> tmodeConfig -> gpsprot.Mode round-trip
			t.Run("mode_roundtrip", func(t *testing.T) {
				tc, err := newTmodeConfig(tt.mode, tt.survey)
				if err != nil {
					t.Fatalf("newTmodeConfig failed: %v", err)
				}

				gotMode := tc.getMode()
				if gotMode != tt.mode {
					t.Errorf("mode roundtrip failed: got %+v, want %+v", gotMode, tt.mode)
				}
			})

			// Test tmodeConfig -> TMODE3 -> tmodeConfig round-trip
			t.Run("tmode3_roundtrip", func(t *testing.T) {
				tc, err := newTmodeConfig(tt.mode, tt.survey)
				if err != nil {
					t.Fatalf("newTmodeConfig failed: %v", err)
				}

				var tmode3 bin.CfgTmode3
				tc.toTmode3(&tmode3, true)

				var tc2 tmodeConfig
				tc2.fromTmode3(&tmode3)

				if tc2 != *tc {
					t.Errorf("tmode3 roundtrip failed: got %+v, want %+v", tc2, *tc)
				}
			})

			// Test tmodeConfig -> TMODE2 -> tmodeConfig round-trip
			t.Run("tmode2_roundtrip", func(t *testing.T) {
				tc, err := newTmodeConfig(tt.mode, tt.survey)
				if err != nil {
					t.Fatalf("newTmodeConfig failed: %v", err)
				}

				var tmode2 bin.CfgTmode2
				tc.toTmode2(&tmode2, true)

				var tc2 tmodeConfig
				tc2.fromTmode2(&tmode2)

				// TMODE2 loses HP precision, so create expected result
				expected := *tc
				// HP fields are zeroed in TMODE2
				expected.ecefHP = [3]int8{0, 0, 0}
				expected.latLonHP = [2]int8{0, 0}
				expected.heightHP = 0
				// Accuracy fields lose precision (0.1mm -> mm -> 0.1mm)
				expected.fixedPosAcc = (tc.fixedPosAcc + 5) / 10 * 10
				expected.svinAccLimit = (tc.svinAccLimit + 5) / 10 * 10

				if tc2 != expected {
					t.Errorf("tmode2 roundtrip failed: got %+v, want %+v", tc2, expected)
				}
			})

			// Test tmodeConfig -> TMODE -> tmodeConfig round-trip
			t.Run("tmode_roundtrip", func(t *testing.T) {
				tc, err := newTmodeConfig(tt.mode, tt.survey)
				if err != nil {
					t.Fatalf("newTmodeConfig failed: %v", err)
				}

				var tmode bin.CfgTmode
				err = tc.toTmode(&tmode, true)
				if tt.mode.PosType == gpsprot.PosTypeLLH {
					// TMODE doesn't support LLH
					if err == nil {
						t.Errorf("expected error for LLH coordinates but got none")
					}
					return
				}
				if err != nil {
					t.Fatalf("toTmode failed: %v", err)
				}

				var tc2 tmodeConfig
				tc2.fromTmode(&tmode)

				// TMODE loses HP fields and only supports ECEF
				expected := *tc
				expected.useLLH = false
				expected.ecefHP = [3]int8{0, 0, 0}
				expected.latLon = [2]int32{0, 0}
				expected.latLonHP = [2]int8{0, 0}
				expected.height = 0
				expected.heightHP = 0

				if tc2 != expected {
					t.Errorf("tmode roundtrip failed: got %+v, want %+v", tc2, expected)
				}
			})

			// Test tmodeConfig -> items -> CfgVals -> tmodeConfig round-trip
			t.Run("cfgvals_roundtrip", func(t *testing.T) {
				tc, err := newTmodeConfig(tt.mode, tt.survey)
				if err != nil {
					t.Fatalf("newTmodeConfig failed: %v", err)
				}

				// Test with all=false (mode-specific items only)
				t.Run("mode_specific", func(t *testing.T) {
					// Convert tmodeConfig to items
					var items []ucv.Item
					tc.toItems(&items, false)

					// Convert items to CfgVals
					vals := MakeCfgVals()
					vals.AddItems(items)

					// Convert CfgVals back to tmodeConfig
					var tc2 tmodeConfig
					ok := tc2.fromCfgVals(&vals, false)
					if !ok {
						t.Fatalf("fromCfgVals failed with all=false")
					}

					if tc2 != *tc {
						t.Errorf("cfgvals roundtrip (all=false) failed: got %+v, want %+v", tc2, *tc)
					}
				})

				// Test with all=true (all items)
				t.Run("all_items", func(t *testing.T) {
					// Convert tmodeConfig to items
					var items []ucv.Item
					tc.toItems(&items, true)

					// Convert items to CfgVals
					vals := MakeCfgVals()
					vals.AddItems(items)

					// Convert CfgVals back to tmodeConfig
					var tc2 tmodeConfig
					ok := tc2.fromCfgVals(&vals, true)
					if !ok {
						t.Fatalf("fromCfgVals failed with all=true")
					}

					if tc2 != *tc {
						t.Errorf("cfgvals roundtrip (all=true) failed: got %+v, want %+v", tc2, *tc)
					}
				})
			})
		})
	}
}

func TestTmodeConfigsTargetOld(t *testing.T) {
	tests := []struct {
		name         string
		mode         *gpsprot.Mode // nil means no Mode property set
		setStatic    bool
		survey       gpsprot.Survey
		cur          *tmodeConfig
		expectFirst  *tmodeConfig
		expectSecond *tmodeConfig
		expectErr    bool
	}{
		// Test cases when cur is nil
		{
			name:         "nil current with no polling needed",
			cur:          nil,
			expectFirst:  nil,
			expectSecond: nil,
		},
		{
			name:      "nil current with polling needed",
			setStatic: true,
			cur:       nil,
			expectErr: true,
		},

		// Test cases with SetStatic but no Mode property
		{
			name:      "SetStatic with current disabled",
			setStatic: true,
			survey: gpsprot.Survey{
				MinDur:   300 * time.Second,
				AccLimit: 2 * gpsprot.Meter,
			},
			cur: &tmodeConfig{mode: tmodeDisabled},
			expectFirst: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   300,
				svinAccLimit: 2e4, // 2m in 0.1mm units
			},
		},
		{
			name:      "SetStatic with current fixed",
			setStatic: true,
			cur: &tmodeConfig{
				mode:        tmodeFixed,
				ecef:        [3]int32{100, 200, 300},
				fixedPosAcc: 1000,
			},
			expectFirst: nil,
		},
		{
			name:      "SetStatic with current survey, no SurveyAgain",
			setStatic: true,
			cur: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   120,
				svinAccLimit: 5000,
			},
			expectFirst: nil,
		},
		{
			name:      "SetStatic with current survey, with SurveyAgain",
			setStatic: true,
			survey: gpsprot.Survey{
				MinDur:   600 * time.Second,
				AccLimit: gpsprot.Meter,
				Flags:    gpsprot.SurveyAgain,
			},
			cur: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   120,
				svinAccLimit: 5000,
			},
			expectFirst: &tmodeConfig{mode: tmodeDisabled},
			expectSecond: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   600,
				svinAccLimit: 1e4, // 1m in 0.1mm units
			},
		},

		// Test cases with explicit Mode property
		{
			name: "Mode mobile (static=false)",
			mode: &gpsprot.Mode{Static: false},
			cur:  &tmodeConfig{mode: tmodeFixed},
			expectFirst: &tmodeConfig{mode: tmodeDisabled},
		},
		{
			name: "Mode survey (static=true, no position)",
			mode: &gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeNone},
			survey: gpsprot.Survey{
				MinDur:   180 * time.Second,
				AccLimit: 3 * gpsprot.Meter,
			},
			cur: &tmodeConfig{mode: tmodeDisabled},
			expectFirst: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   180,
				svinAccLimit: 3e4, // 3m in 0.1mm units
			},
		},
		{
			name: "Mode fixed (static=true, with position)",
			mode: &gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeECEF,
				FixedPosECEF: gpsprot.Point3D{
					4000 * gpsprot.Meter * 1000, // 4000km
					5000 * gpsprot.Meter * 1000, // 5000km
					6000 * gpsprot.Meter * 1000, // 6000km
				},
				FixedPosAcc: gpsprot.Centimeter,
			},
			cur: &tmodeConfig{mode: tmodeDisabled},
			expectFirst: &tmodeConfig{
				mode:        tmodeFixed,
				ecef:        [3]int32{4e8, 5e8, 6e8}, // 4000km, 5000km, 6000km in cm
				fixedPosAcc: 100,                     // 1cm in 0.1mm units
			},
		},
		{
			name: "Mode survey with SurveyAgain when already surveying",
			mode: &gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeNone},
			survey: gpsprot.Survey{
				MinDur:   240 * time.Second,
				AccLimit: gpsprot.Meter + 500*gpsprot.Millimeter, // 1.5m
				Flags:    gpsprot.SurveyAgain,
			},
			cur: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   120,
				svinAccLimit: 5000,
			},
			expectFirst: &tmodeConfig{mode: tmodeDisabled},
			expectSecond: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   240,
				svinAccLimit: 1.5e4, // 1.5m in 0.1mm units
			},
		},
		{
			name: "Mode survey with SurveyAgain when not currently surveying",
			mode: &gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeNone},
			survey: gpsprot.Survey{
				MinDur:   240 * time.Second,
				AccLimit: gpsprot.Meter + 500*gpsprot.Millimeter, // 1.5m
				Flags:    gpsprot.SurveyAgain,
			},
			cur: &tmodeConfig{mode: tmodeDisabled},
			expectFirst: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   240,
				svinAccLimit: 1.5e4, // 1.5m in 0.1mm units
			},
		},

		// Edge case: Mode.Static=false but SetStatic=true
		{
			name:      "Mode mobile but SetStatic true",
			mode:      &gpsprot.Mode{Static: false},
			setStatic: true,
			survey: gpsprot.Survey{
				MinDur:   300 * time.Second,
				AccLimit: 2 * gpsprot.Meter,
			},
			cur: &tmodeConfig{mode: tmodeDisabled},
			expectFirst: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   300,
				svinAccLimit: 2e4, // 2m in 0.1mm units
			},
		},

		// Test with LLH coordinates
		{
			name: "Mode fixed with LLH coordinates",
			mode: &gpsprot.Mode{
				Static:  true,
				PosType: gpsprot.PosTypeLLH,
				FixedPosLLH: [2]gpsprot.Angle{
					40*gpsprot.Degrees + 5*gpsprot.Nanodegrees,
					50*gpsprot.Degrees + 6*gpsprot.Nanodegrees,
				},
				Height:      1000*gpsprot.Meter + 7*gpsprot.Millimeter/10,
				FixedPosAcc: 5 * gpsprot.Millimeter,
			},
			cur: &tmodeConfig{mode: tmodeDisabled},
			expectFirst: &tmodeConfig{
				mode:         tmodeFixed,
				useLLH:       true,
				latLon:       [2]int32{4e8, 5e8}, // 40.0°, 50.0° in 1e-7 degrees
				height:       1e5,                 // 1000m in cm
				latLonHP:     [2]int8{5, 6},       // fractional parts in 1e-9 degrees
				heightHP:     7,                   // fractional part in 0.1mm
				fixedPosAcc:  50,                  // 5mm in 0.1mm units
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &gpsprot.ConfigTarget{
				Props: gpsprot.ConfigProps{},
				Opts: gpsprot.ConfigOptions{
					SetStatic: tt.setStatic,
					Survey:    tt.survey,
				},
			}

			if tt.mode != nil {
				target.Props.SetMode(*tt.mode)
			}

			gotFirst, gotSecond, err := tmodeConfigsTargetOld(target, tt.cur)

			if tt.expectErr {
				if err == nil {
					t.Errorf("tmodeConfigsTargetOld() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("tmodeConfigsTargetOld() unexpected error: %v", err)
				return
			}

			// Compare first result
			if !tmodeConfigEqual(gotFirst, tt.expectFirst) {
				t.Errorf("tmodeConfigsTargetOld() first result mismatch\ngot:  %+v\nwant: %+v", gotFirst, tt.expectFirst)
			}

			// Compare second result
			if !tmodeConfigEqual(gotSecond, tt.expectSecond) {
				t.Errorf("tmodeConfigsTargetOld() second result mismatch\ngot:  %+v\nwant: %+v", gotSecond, tt.expectSecond)
			}
		})
	}
}

// Helper function to compare tmodeConfig structs
func tmodeConfigEqual(a, b *tmodeConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}