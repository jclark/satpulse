package as

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

func TestTimeMode(t *testing.T) {
	surveyed := asbin.CfgFixedECEF{X: -114469630, Y: 609033320, Z: 150417057}
	surveyedMode := gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeECEF,
		FixedPosECEF: gpsprot.Point3D{
			gpsprot.Meters(-1144696.30), gpsprot.Meters(6090333.20), gpsprot.Meters(1504170.57)}}
	tests := []struct {
		name       string
		mode       *gpsprot.Mode
		setStatic  bool
		surveyOpts gpsprot.Survey
		asFoundPos asbin.CfgFixedECEF
		asFoundSvy asbin.CfgSurvey
		expectPos  asbin.CfgFixedECEF
		expectSvy  asbin.CfgSurvey
		expectMode gpsprot.Mode
	}{
		{
			name:       "mobile_from_fixed",
			mode:       &gpsprot.Mode{Static: false},
			asFoundPos: surveyed,
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectMode: gpsprot.Mode{Static: false},
		},
		{
			name:       "fixed_pos_ecef",
			mode:       &surveyedMode,
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectPos:  surveyed,
			expectMode: surveyedMode,
		},
		{
			// idle unit, explicit static-without-position with survey
			// parameters: a fresh survey starts
			name:       "survey_in_idle",
			mode:       &gpsprot.Mode{Static: true},
			surveyOpts: gpsprot.Survey{MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			// re-applying static-without-position over a completed survey
			// (fixed position present) must not discard it: no SurveyAgain,
			// so the fixed position and survey registers are left untouched
			name:       "static_none_preserves_fixed",
			mode:       &gpsprot.Mode{Static: true},
			surveyOpts: gpsprot.Survey{MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			asFoundPos: surveyed,
			expectPos:  surveyed,
			expectMode: surveyedMode,
		},
		{
			// re-applying static-without-position while a survey is running
			// must not restart it: the parameters are left untouched
			name:       "static_none_preserves_running_survey",
			mode:       &gpsprot.Mode{Static: true},
			surveyOpts: gpsprot.Survey{MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectSvy:  asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			// SurveyAgain over a running survey restarts it with the new
			// parameters
			name:       "static_none_survey_again_restarts",
			mode:       &gpsprot.Mode{Static: true},
			surveyOpts: gpsprot.Survey{Flags: gpsprot.SurveyAgain, MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			// SurveyAgain over a COMPLETED survey (fixed position present)
			// must restart it: the fixed position is zeroed and the new
			// survey parameters written. Without SurveyAgain that position
			// is preserved (static_none_preserves_fixed above).
			name:       "static_none_survey_again_over_completed",
			mode:       &gpsprot.Mode{Static: true},
			surveyOpts: gpsprot.Survey{Flags: gpsprot.SurveyAgain, MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			asFoundPos: surveyed,
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			name:       "setstatic_preserves_fixed",
			setStatic:  true,
			asFoundPos: surveyed,
			expectPos:  surveyed,
			expectMode: surveyedMode,
		},
		{
			name:       "setstatic_preserves_running_survey",
			setStatic:  true,
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectSvy:  asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			name:       "setstatic_idle_starts_survey",
			setStatic:  true,
			surveyOpts: gpsprot.Survey{MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			name:       "setstatic_survey_again_restarts",
			setStatic:  true,
			surveyOpts: gpsprot.Survey{Flags: gpsprot.SurveyAgain, MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pos, svy := tc.asFoundPos, tc.asFoundSvy
			rcvr := &testReceiver{monVer: tau1201Ver(), fixedEcef: &pos, survey: &svy}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			if tc.mode != nil {
				target.Props.SetMode(*tc.mode)
			}
			target.Opts.SetStatic = tc.setStatic
			target.Opts.Survey = tc.surveyOpts
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(*rcvr.fixedEcef, tc.expectPos) {
				t.Errorf("FIXEDECEF\ngot  %+v\nwant %+v", *rcvr.fixedEcef, tc.expectPos)
			}
			if !reflect.DeepEqual(*rcvr.survey, tc.expectSvy) {
				t.Errorf("SURVEY\ngot  %+v\nwant %+v", *rcvr.survey, tc.expectSvy)
			}
			got, ok := cfg.ConfigProps().GetMode()
			if !ok || !reflect.DeepEqual(got, tc.expectMode) {
				t.Errorf("mode = %+v/%v, want %+v", got, ok, tc.expectMode)
			}
		})
	}
}

func TestTimeModeAbsent(t *testing.T) {
	// A receiver without the registers is silent to both polls: the
	// mode property shows as absence, nothing is written, no error.
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMode(gpsprot.Mode{Static: true})
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if _, ok := cfg.ConfigProps().GetMode(); ok {
		t.Error("mode reported for a receiver without the registers")
	}
}
