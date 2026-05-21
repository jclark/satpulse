package rinex

import (
	"testing"

	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestDiffObservationsReportsInputProblems(t *testing.T) {
	t1 := mustTime(t, "2026-05-19T13:31:24.0000000")
	t2 := mustTime(t, "2026-05-19T13:31:25.0000000")
	obs := []SignalObservation{
		{T: t1, Sat: "G02", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
		{T: t1, Sat: "G01", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
		{T: t2, Sat: "G01", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
		{T: t1, Sat: "G01", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(2.0)}},
	}
	var er testErrorReporter
	if _, err := DiffObservations(obs, nil, Tolerances{}, &testReporter{}, &er); err != nil {
		t.Fatalf("DiffObservations: %v", err)
	}
	if len(er.unordered) != 1 || er.unordered[0] != (testUnordered{side: 0, index: 3}) {
		t.Fatalf("unordered = %#v", er.unordered)
	}
	if len(er.duplicates) != 1 || er.duplicates[0] != (testDuplicate{side: 0, index: 3, prevIndex: 1}) {
		t.Fatalf("duplicates = %#v", er.duplicates)
	}
}

func TestDiffObservationsReportsNilMissingSide(t *testing.T) {
	tm := mustTime(t, "2026-05-19T13:31:24.0000000")
	a := []SignalObservation{
		{T: tm, Sat: "G01", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
	}
	b := []SignalObservation{
		{T: tm, Sat: "G02", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(2.0)}},
	}
	var r testReporter
	n, err := DiffObservations(a, b, Tolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005}, &r, nil)
	if err != nil {
		t.Fatalf("DiffObservations: %v", err)
	}
	if n != 2 || len(r.reports) != 2 {
		t.Fatalf("report count = %d/%d, want 2", n, len(r.reports))
	}
	if r.reports[0].a == nil || r.reports[0].b != nil {
		t.Fatalf("first report sides = %#v %#v, want a present and b nil", r.reports[0].a, r.reports[0].b)
	}
	if r.reports[1].a != nil || r.reports[1].b == nil {
		t.Fatalf("second report sides = %#v %#v, want a nil and b present", r.reports[1].a, r.reports[1].b)
	}
}

func TestDiffSignal(t *testing.T) {
	a := SignalValues{PR: opt.Make(1.0), CP: opt.Make(2.0), LLI: opt.Make(LLILostLock)}
	b := SignalValues{PR: opt.Make(1.001), CP: opt.Make(2.0), LLI: opt.Make(LLIHalfCycleAmbiguity)}
	for _, tt := range []struct {
		name  string
		a     *SignalValues
		b     *SignalValues
		wantA *SignalValues
		wantB *SignalValues
	}{
		{
			name:  "both missing",
			wantA: nil,
			wantB: nil,
		},
		{
			name:  "a missing",
			b:     &b,
			wantA: nil,
			wantB: &b,
		},
		{
			name:  "b missing",
			a:     &a,
			wantA: &a,
			wantB: nil,
		},
		{
			name:  "same",
			a:     &a,
			b:     &a,
			wantA: &SignalValues{},
			wantB: &SignalValues{},
		},
		{
			name:  "different values",
			a:     &a,
			b:     &b,
			wantA: &SignalValues{PR: opt.Make(1.0), LLI: opt.Make(LLILostLock)},
			wantB: &SignalValues{PR: opt.Make(1.001), LLI: opt.Make(LLIHalfCycleAmbiguity)},
		},
		{
			name:  "missing field",
			a:     &SignalValues{PR: opt.Make(1.0)},
			b:     &SignalValues{},
			wantA: &SignalValues{PR: opt.Make(1.0)},
			wantB: &SignalValues{},
		},
		{
			name:  "within tolerance",
			a:     &SignalValues{PR: opt.Make(1.0), CN0: opt.Make(float32(45.0))},
			b:     &SignalValues{PR: opt.Make(1.0001), CN0: opt.Make(float32(45.0001))},
			wantA: &SignalValues{},
			wantB: &SignalValues{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotA, gotB := DiffSignal(tt.a, tt.b, Tolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005})
			if gotA != tt.wantA && (gotA == nil || tt.wantA == nil || *gotA != *tt.wantA) {
				t.Fatalf("aRet = %#v, want %#v", gotA, tt.wantA)
			}
			if gotB != tt.wantB && (gotB == nil || tt.wantB == nil || *gotB != *tt.wantB) {
				t.Fatalf("bRet = %#v, want %#v", gotB, tt.wantB)
			}
		})
	}
}

type testReporter struct {
	reports []testReport
}

type testReport struct {
	t   Time
	sat SatelliteID
	sig SignalID
	a   *SignalValues
	b   *SignalValues
}

func (r *testReporter) Diff(t Time, sat SatelliteID, sig SignalID, a, b *SignalValues) error {
	r.reports = append(r.reports, testReport{t: t, sat: sat, sig: sig, a: a, b: b})
	return nil
}

type testErrorReporter struct {
	duplicates []testDuplicate
	unordered  []testUnordered
}

type testDuplicate struct {
	side      int
	index     int
	prevIndex int
}

type testUnordered struct {
	side  int
	index int
}

func (r *testErrorReporter) Duplicate(side int, index, prevIndex int) error {
	r.duplicates = append(r.duplicates, testDuplicate{side: side, index: index, prevIndex: prevIndex})
	return nil
}

func (r *testErrorReporter) Unordered(side int, index int) error {
	r.unordered = append(r.unordered, testUnordered{side: side, index: index})
	return nil
}
