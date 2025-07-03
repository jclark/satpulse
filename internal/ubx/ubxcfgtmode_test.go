package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
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
				tc.toTmode3(&tmode3)

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
				tc.toTmode2(&tmode2)

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
				err = tc.toTmode(&tmode)
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
		})
	}
}