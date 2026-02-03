package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubxbin"
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

				var tmode3 ubxbin.CfgTmode3
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

				var tmode2 ubxbin.CfgTmode2
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

				var tmode ubxbin.CfgTmode
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
					tc.addItems(&items, false)

					// Convert items to CfgVals
					vals := MakeCfgVals()
					vals.AddItems(items)

					// Convert CfgVals back to tmodeConfig
					var tc2 tmodeConfig
					ok := tc2.fromCfgVals(&vals, tmodeInfoRelevant)
					if !ok {
						t.Fatalf("fromCfgVals failed with tmodeInfoRelevant")
					}

					if tc2 != *tc {
						t.Errorf("cfgvals roundtrip (tmodeInfoRelevant) failed: got %+v, want %+v", tc2, *tc)
					}
				})

				// Test with all=true (all items)
				t.Run("all_items", func(t *testing.T) {
					// Convert tmodeConfig to items
					var items []ucv.Item
					tc.addItems(&items, true)

					// Convert items to CfgVals
					vals := MakeCfgVals()
					vals.AddItems(items)

					// Convert CfgVals back to tmodeConfig
					var tc2 tmodeConfig
					ok := tc2.fromCfgVals(&vals, tmodeInfoAll)
					if !ok {
						t.Fatalf("fromCfgVals failed with tmodeInfoAll")
					}

					if tc2 != *tc {
						t.Errorf("cfgvals roundtrip (tmodeInfoAll) failed: got %+v, want %+v", tc2, *tc)
					}
				})
			})
		})
	}
}

func TestCreateTmodeConfigs(t *testing.T) {
	tests := []struct {
		name           string
		mode           *gpsprot.Mode // nil means no Mode property set
		setStatic      bool
		survey         gpsprot.Survey
		cur            *tmodeConfig
		resurveyMethod resurveyMethod
		expectFirst    *tmodeConfig
		expectSecond   *tmodeConfig
		expectErr      bool
	}{
		// Test cases when cur is nil
		{
			name:           "nil current with no polling needed",
			cur:            nil,
			resurveyMethod: resurveyDisable,
			expectFirst:    nil,
			expectSecond:   nil,
		},

		// Test cases with SetStatic but no Mode property
		{
			name:      "SetStatic with current disabled",
			setStatic: true,
			survey: gpsprot.Survey{
				MinDur:   300 * time.Second,
				AccLimit: 2 * gpsprot.Meter,
			},
			cur:            &tmodeConfig{mode: tmodeDisabled},
			resurveyMethod: resurveyDisable,
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
			resurveyMethod: resurveyDisable,
			expectFirst:    nil,
		},
		{
			name:      "SetStatic with current survey, no SurveyAgain",
			setStatic: true,
			cur: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   120,
				svinAccLimit: 5000,
			},
			resurveyMethod: resurveyDisable,
			expectFirst:    nil,
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
			resurveyMethod: resurveyDisable,
			expectFirst:    &tmodeConfig{mode: tmodeDisabled},
			expectSecond: &tmodeConfig{
				mode:         tmodeSurveyIn,
				svinMinDur:   600,
				svinAccLimit: 1e4, // 1m in 0.1mm units
			},
		},

		// Test cases with explicit Mode property
		{
			name:           "Mode mobile (static=false)",
			mode:           &gpsprot.Mode{Static: false},
			cur:            &tmodeConfig{mode: tmodeFixed},
			resurveyMethod: resurveyDisable,
			expectFirst:    &tmodeConfig{mode: tmodeDisabled},
		},
		{
			name: "Mode survey (static=true, no position)",
			mode: &gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeNone},
			survey: gpsprot.Survey{
				MinDur:   180 * time.Second,
				AccLimit: 3 * gpsprot.Meter,
			},
			cur:            &tmodeConfig{mode: tmodeDisabled},
			resurveyMethod: resurveyDisable,
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
			cur:            &tmodeConfig{mode: tmodeDisabled},
			resurveyMethod: resurveyDisable,
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
			resurveyMethod: resurveyDisable,
			expectFirst:    &tmodeConfig{mode: tmodeDisabled},
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
			cur:            &tmodeConfig{mode: tmodeDisabled},
			resurveyMethod: resurveyDisable,
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
			cur:            &tmodeConfig{mode: tmodeDisabled},
			resurveyMethod: resurveyDisable,
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
			cur:            &tmodeConfig{mode: tmodeDisabled},
			resurveyMethod: resurveyDisable,
			expectFirst: &tmodeConfig{
				mode:        tmodeFixed,
				useLLH:      true,
				latLon:      [2]int32{4e8, 5e8}, // 40.0°, 50.0° in 1e-7 degrees
				height:      1e5,                // 1000m in cm
				latLonHP:    [2]int8{5, 6},      // fractional parts in 1e-9 degrees
				heightHP:    7,                  // fractional part in 0.1mm
				fixedPosAcc: 50,                 // 5mm in 0.1mm units
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

			gotFirst, gotSecond, err := createTmodeConfigs(target, tt.cur, tt.resurveyMethod)

			if tt.expectErr {
				if err == nil {
					t.Errorf("createTmodeConfigs() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("createTmodeConfigs() unexpected error: %v", err)
				return
			}

			// Compare first result
			if !tmodeConfigEqual(gotFirst, tt.expectFirst) {
				t.Errorf("createTmodeConfigs() first result mismatch\ngot:  %+v\nwant: %+v", gotFirst, tt.expectFirst)
			}

			// Compare second result
			if !tmodeConfigEqual(gotSecond, tt.expectSecond) {
				t.Errorf("createTmodeConfigs() second result mismatch\ngot:  %+v\nwant: %+v", gotSecond, tt.expectSecond)
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

func TestTmodeRequiredInfo(t *testing.T) {
	tests := []struct {
		name     string
		target   *gpsprot.ConfigTarget
		method   resurveyMethod
		expected tmodeInfo
	}{
		{
			name:     "no mode and no setStatic",
			target:   gpsprot.NewConfigTarget(),
			method:   resurveyChange,
			expected: tmodeInfoNone,
		},
		{
			name: "survey mode without resurvey doesn't need survey info",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				return target
			}(),
			method:   resurveyChange,
			expected: tmodeInfoNone,
		},
		{
			name: "survey again needs survey info",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey.Flags |= gpsprot.SurveyAgain
				return target
			}(),
			method:   resurveyChange,
			expected: tmodeInfoSurvey,
		},
		{
			name: "fixed position ECEF should need no info",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{
					Static:  true,
					PosType: gpsprot.PosTypeECEF,
					FixedPosECEF: [3]gpsprot.Length{
						4194304 * gpsprot.Meter,
						837860 * gpsprot.Meter,
						4581200 * gpsprot.Meter,
					},
					FixedPosAcc: 10 * gpsprot.Millimeter,
				})
				return target
			}(),
			method:   resurveyChange,
			expected: tmodeInfoNone,
		},
		{
			name: "disable mode doesn't need info",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: false})
				return target
			}(),
			method:   resurveyChange,
			expected: tmodeInfoNone,
		},
		{
			name: "setStatic without mode and without resurvey needs mode",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Opts.SetStatic = true
				return target
			}(),
			method:   resurveyChange,
			expected: tmodeInfoMode,
		},
		{
			name: "setStatic without mode and with resurvey needs mode and survey",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Opts.SetStatic = true
				target.Opts.Survey.Flags |= gpsprot.SurveyAgain
				return target
			}(),
			method:   resurveyChange,
			expected: tmodeInfoMode | tmodeInfoSurvey,
		},
		{
			name: "non-static mode with setStatic and resurvey needs survey info",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.SetStatic = true
				target.Opts.Survey.Flags |= gpsprot.SurveyAgain
				return target
			}(),
			method:   resurveyChange,
			expected: tmodeInfoSurvey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.target.Opts.Survey.MinDur = 300 * time.Second
			tt.target.Opts.Survey.AccLimit = 5 * gpsprot.Meter

			result := tmodeRequiredInfo(tt.target, tt.method)
			if result != tt.expected {
				t.Errorf("tmodeRequiredInfo() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestSplitLength(t *testing.T) {
	const mm10 = gpsprot.Micrometer * 100

	testCases := []struct {
		length       gpsprot.Length
		expectedCm   int32
		expectedMm10 int8
	}{
		{105 * mm10, 1, 5},
		{250 * mm10, 3, -50},
		{-105 * mm10, -1, -5},
		{-250 * mm10, -3, 50},
		{10475 * gpsprot.Micrometer, 1, 5},
	}

	for _, tc := range testCases {
		cm, mm10, err := splitLength(tc.length)
		if err != nil {
			t.Errorf("splitLength returned error: %v", err)
		} else if mm10 < -99 || mm10 > 99 {
			t.Errorf("splitLength(%v) = (%v, %v), want mm10 in [-99, 99]", tc.length, cm, mm10)
		} else if cm != tc.expectedCm || mm10 != tc.expectedMm10 {
			t.Errorf("splitLength(%v) = (%v, %v), want (%v, %v)", tc.length, cm, mm10, tc.expectedCm, tc.expectedMm10)
		}
	}
}

func TestSplitAngle(t *testing.T) {
	testCases := []struct {
		angle         gpsprot.Angle
		expectedDegE7 int32
		expectedDegE9 int8
	}{
		{105 * gpsprot.Nanodegrees, 1, 5},
		{250 * gpsprot.Nanodegrees, 3, -50},
		{-105 * gpsprot.Nanodegrees, -1, -5},
		{-250 * gpsprot.Nanodegrees, -3, 50},
		{12345 * gpsprot.Nanodegrees, 123, 45},
		{45 * gpsprot.Degrees, 450000000, 0},
		{45123456789 * gpsprot.Nanodegrees, 451234568, -11},
	}

	for _, tc := range testCases {
		degreesE7, degreesE9, err := splitAngle(tc.angle)
		if err != nil {
			t.Errorf("splitAngle returned error: %v", err)
		} else if degreesE9 < -99 || degreesE9 > 99 {
			t.Errorf("splitAngle(%v) = (%v, %v), want degreesE9 in [-99, 99]", tc.angle, degreesE7, degreesE9)
		} else if degreesE7 != tc.expectedDegE7 || degreesE9 != tc.expectedDegE9 {
			t.Errorf("splitAngle(%v) = (%v, %v), want (%v, %v)", tc.angle, degreesE7, degreesE9, tc.expectedDegE7, tc.expectedDegE9)
		}
	}
}

func TestDivModRound(t *testing.T) {
	testCases := []struct {
		x, y, q int64
	}{
		{500, 100, 5},
		{550, 100, 6},
		{449, 100, 4},
		{-500, 100, -5},
		{-550, 100, -6},
		{-449, 100, -4},
		{17, 10, 2},
		{-17, 10, -2},
		{17, 1000, 0},
		{-17, 1000, 0},
		{1005, 1000, 1},
		{-1005, 1000, -1},
		{13, 10, 1},
		{15, 10, 2},
		{18, 10, 2},
		{-13, 10, -1},
		{-15, 10, -2},
		{-18, 10, -2},
	}

	for _, tc := range testCases {
		q, r := divModRound(tc.x, tc.y)
		if q != tc.q {
			t.Errorf("divModRound(%d, %d) = (%d, %d), want quotient %d",
				tc.x, tc.y, q, r, tc.q)
		}
		if tc.x != q*tc.y+r {
			t.Errorf("divModRound(%d, %d) = (%d, %d), does not satisfy x = quotient*y + remainder",
				tc.x, tc.y, q, r)
		}
	}
}
