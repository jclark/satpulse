package as

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

func TestSurveyNavSvin(t *testing.T) {
	tests := []struct {
		name   string
		input  asbin.NavSvin
		expect gpsprot.SurveyMsg
	}{
		{
			name: "in progress",
			input: asbin.NavSvin{
				NavITOW:    asbin.NavITOW{ITow: 100000},
				PosUsed:    60,
				MeanStdDev: 50000, // 5000mm = 5m
				Valid:      0,
				Status:     0, // not finished
			},
			expect: gpsprot.SurveyMsg{
				Accuracy:   5 * gpsprot.Meter,
				Valid:      false,
				InProgress: true,
				ObsCount:   60,
				ObsTime:    60 * gpsprot.Second,
			},
		},
		{
			name: "completed valid",
			input: asbin.NavSvin{
				NavITOW:    asbin.NavITOW{ITow: 200000},
				PosUsed:    300,
				MeanStdDev: 100, // 10mm
				Valid:      1,
				Status:     1, // finished
			},
			expect: gpsprot.SurveyMsg{
				Accuracy:   10 * gpsprot.Millimeter,
				Valid:      true,
				InProgress: false,
				ObsCount:   300,
				ObsTime:    300 * gpsprot.Second,
			},
		},
		{
			name: "completed not valid",
			input: asbin.NavSvin{
				NavITOW:    asbin.NavITOW{ITow: 300000},
				PosUsed:    120,
				MeanStdDev: 100000, // 10000mm = 10m (did not meet accuracy)
				Valid:      0,
				Status:     1, // finished but not valid
			},
			expect: gpsprot.SurveyMsg{
				Accuracy:   10 * gpsprot.Meter,
				Valid:      false,
				InProgress: false,
				ObsCount:   120,
				ObsTime:    120 * gpsprot.Second,
			},
		},
		{
			name: "high precision accuracy",
			input: asbin.NavSvin{
				NavITOW:    asbin.NavITOW{ITow: 400000},
				PosUsed:    3600,
				MeanStdDev: 25, // 2.5mm
				Valid:      1,
				Status:     1,
			},
			expect: gpsprot.SurveyMsg{
				Accuracy:   gpsprot.Millimeter * 5 / 2, // 2.5mm
				Valid:      true,
				InProgress: false,
				ObsCount:   3600,
				ObsTime:    3600 * gpsprot.Second,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := surveyNavSvin(&tc.input)
			if !reflect.DeepEqual(*got, tc.expect) {
				t.Errorf("surveyNavSvin() = %+v, want %+v", *got, tc.expect)
			}
		})
	}
}
