package rinex

import (
	"testing"
	"time"

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
	if _, err := DiffObservations(obs, nil, ObsTolerances{}, &testReporter{}, &er); err != nil {
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
	n, err := DiffObservations(a, b, ObsTolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005}, &r, nil)
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
			gotA, gotB := DiffSignal(tt.a, tt.b, ObsTolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005})
			if gotA != tt.wantA && (gotA == nil || tt.wantA == nil || *gotA != *tt.wantA) {
				t.Fatalf("aRet = %#v, want %#v", gotA, tt.wantA)
			}
			if gotB != tt.wantB && (gotB == nil || tt.wantB == nil || *gotB != *tt.wantB) {
				t.Fatalf("bRet = %#v, want %#v", gotB, tt.wantB)
			}
		})
	}
}

func TestDiffMetadata(t *testing.T) {
	d1 := time.Date(2026, time.May, 19, 13, 31, 24, 0, time.UTC)
	d2 := d1.Add(time.Second)
	a := Metadata{
		Version: Version304,
		Run: MetadataRun{
			Program: "prog-a",
			By:      "same",
			Date:    d1,
		},
		Comment:        Lines{"keep", "dup", "left", "dup"},
		Observer:       "obs-a",
		Agency:         "same-agency",
		Receiver:       Receiver{Number: "rx-a", Type: "rx-type", Version: "rx-vers"},
		Antenna:        Antenna{Number: "ant-num", Type: "ant-a"},
		ApproxPosition: testPtr([3]float64{1, 2, 3}),
		AntennaDelta:   testPtr([3]float64{0, 0, 0}),
		Interval:       testPtr(30.0),
		LeapSeconds:    testPtr(int16(18)),
	}
	a.Marker.Name = "mark-a"
	a.Marker.Number = "001"
	b := Metadata{
		Version: Version402,
		Run: MetadataRun{
			Program: "prog-b",
			By:      "same",
			Date:    d2,
		},
		Comment:        Lines{"dup", "keep", "right", "dup"},
		Observer:       "obs-b",
		Agency:         "same-agency",
		Receiver:       Receiver{Number: "rx-b", Type: "rx-type", Version: "rx-vers"},
		Antenna:        Antenna{Number: "ant-num", Type: "ant-b"},
		ApproxPosition: testPtr([3]float64{1.00004, 2, 3}),
		AntennaDelta:   testPtr([3]float64{0, 0.0002, 0}),
		Interval:       testPtr(15.0),
	}
	b.Marker.Name = "mark-b"
	b.Marker.Type = "GEODETIC"
	gotA, gotB := DiffMetadata(a, b, MetadataTolerances{ApproxPos: 0.00005, AntennaDelta: 0.00005})
	if gotA.Version != Version304 || gotB.Version != Version402 {
		t.Fatalf("versions = %q/%q", gotA.Version, gotB.Version)
	}
	if gotA.Run.Program != "prog-a" || gotB.Run.Program != "prog-b" || gotA.Run.By != "" || gotB.Run.By != "" || !gotA.Run.Date.Equal(d1) || !gotB.Run.Date.Equal(d2) {
		t.Fatalf("run diff = %#v %#v", gotA.Run, gotB.Run)
	}
	if len(gotA.Comment) != 1 || gotA.Comment[0] != "left" || len(gotB.Comment) != 1 || gotB.Comment[0] != "right" {
		t.Fatalf("comment diff = %#v %#v", gotA.Comment, gotB.Comment)
	}
	if gotA.Marker.Name != "mark-a" || gotB.Marker.Name != "mark-b" || gotA.Marker.Number != "001" || gotB.Marker.Type != "GEODETIC" {
		t.Fatalf("marker diff = %#v %#v", gotA.Marker, gotB.Marker)
	}
	if gotA.Observer != "obs-a" || gotB.Observer != "obs-b" || gotA.Agency != "" || gotB.Agency != "" {
		t.Fatalf("observer/agency diff = %#v %#v", gotA, gotB)
	}
	if gotA.Receiver.Number != "rx-a" || gotB.Receiver.Number != "rx-b" || gotA.Receiver.Type != "" || gotB.Receiver.Type != "" {
		t.Fatalf("receiver diff = %#v %#v", gotA.Receiver, gotB.Receiver)
	}
	if gotA.Antenna.Type != "ant-a" || gotB.Antenna.Type != "ant-b" || gotA.Antenna.Number != "" || gotB.Antenna.Number != "" {
		t.Fatalf("antenna diff = %#v %#v", gotA.Antenna, gotB.Antenna)
	}
	if gotA.ApproxPosition != nil || gotB.ApproxPosition != nil {
		t.Fatalf("approx position diff = %v %v", gotA.ApproxPosition, gotB.ApproxPosition)
	}
	if gotA.AntennaDelta == nil || gotB.AntennaDelta == nil || *gotA.AntennaDelta != [3]float64{0, 0, 0} || *gotB.AntennaDelta != [3]float64{0, 0.0002, 0} {
		t.Fatalf("antenna delta diff = %v %v", gotA.AntennaDelta, gotB.AntennaDelta)
	}
	if gotA.Interval == nil || gotB.Interval == nil || *gotA.Interval != 30 || *gotB.Interval != 15 {
		t.Fatalf("interval diff = %v %v", gotA.Interval, gotB.Interval)
	}
	if gotA.LeapSeconds == nil || *gotA.LeapSeconds != 18 || gotB.LeapSeconds != nil {
		t.Fatalf("leap seconds diff = %v %v", gotA.LeapSeconds, gotB.LeapSeconds)
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
