package tui

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/app/session"
)

func newTestConfigView() *configView {
	snk := newSink()
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	sess := session.New(lg, snk, session.Options{})
	return newConfigView(sess, nil)
}

func TestConfigFieldErrors(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(v *configView)
		expect []string // substring of each expected error, in order
	}{
		{
			name:  "empty form is valid",
			setup: func(v *configView) {},
		},
		{
			name:  "mode-scoped fields are ignored outside their mode",
			setup: func(v *configView) { v.surveyTime.SetValue("0"); v.llhLat.SetValue("91") },
		},
		{
			name: "latitude range in llh mode",
			setup: func(v *configView) {
				v.timeMode = 3
				v.coordSystem = 1
				v.llhLat.SetValue("91")
				v.llhLon.SetValue("0")
				v.llhHeight.SetValue("0")
			},
			expect: []string{"latitude"},
		},
		{
			name:   "required ECEF fields flagged when empty",
			setup:  func(v *configView) { v.timeMode = 3; v.coordSystem = 0 },
			expect: []string{"ECEF X is required", "ECEF Y is required", "ECEF Z is required"},
		},
		{
			name: "ECEF triple must be on Earth",
			setup: func(v *configView) {
				v.timeMode = 3
				v.coordSystem = 0
				v.ecefX.SetValue("1")
				v.ecefY.SetValue("2")
				v.ecefZ.SetValue("3")
			},
			expect: []string{"not on Earth"},
		},
		{
			name:   "pulse width must be less than the period",
			setup:  func(v *configView) { v.ppsPeriod.SetValue("1"); v.ppsWidth.SetValue("1") },
			expect: []string{"pulse width must be less than the period"},
		},
		{
			name:   "survey time must be positive in survey mode",
			setup:  func(v *configView) { v.timeMode = 2; v.surveyTime.SetValue("0") },
			expect: []string{"survey time"},
		},
		{
			name:   "non-numeric entry",
			setup:  func(v *configView) { v.minElev.SetValue("abc") },
			expect: []string{"invalid min elevation"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestConfigView()
			tc.setup(v)
			errs, marks := v.fieldErrors()
			if len(errs) != len(tc.expect) {
				t.Fatalf("got %d errors %v, want %d", len(errs), errs, len(tc.expect))
			}
			for i, want := range tc.expect {
				if !strings.Contains(errs[i].Error(), want) {
					t.Errorf("error %d = %q, want substring %q", i, errs[i], want)
				}
			}
			if len(marks) != len(errs) {
				t.Errorf("marks %d, errors %d", len(marks), len(errs))
			}
		})
	}
}

// TestConfigBuildTargetRefusesInvalid checks that target construction
// shares the field rules: an invalid field aborts the whole apply.
func TestConfigBuildTargetRefusesInvalid(t *testing.T) {
	v := newTestConfigView()
	v.timePulseTouched = true
	v.ppsPeriod.SetValue("1")
	v.ppsWidth.SetValue("2")
	if _, err := v.buildTarget(); err == nil {
		t.Fatal("buildTarget accepted an invalid pulse width")
	}
}
