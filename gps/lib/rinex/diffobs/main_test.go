package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
)

func TestJSONReporter(t *testing.T) {
	t1 := testTime(t, "2026-05-19T13:31:24.0000000")
	t2 := testTime(t, "2026-05-19T13:31:25.0000000")
	a := []rinex.SignalObservation{
		{T: t1, Sat: "G01", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), CP: opt.Make(2.0)}},
		{T: t1, Sat: "G02", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(5.0)}},
		{T: t2, Sat: "G01", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(10.0)}},
	}
	b := []rinex.SignalObservation{
		{T: t1, Sat: "G01", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(1.001), CP: opt.Make(2.0)}},
		{T: t1, Sat: "G03", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(6.0)}},
		{T: t2, Sat: "G01", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(10.0001)}},
	}
	var buf bytes.Buffer
	n, err := rinex.DiffObservations(a, b, rinex.ObsTolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005}, jsonReporter{enc: json.NewEncoder(&buf)}, nil)
	if err != nil {
		t.Fatalf("DiffObservations: %v", err)
	}
	if n != 3 {
		t.Fatalf("diff count = %d, want 3\n%s", n, buf.String())
	}
	want := strings.Join([]string{
		`{"t":"2026-05-19T13:31:24.0000000","sat":"G01","sig":"1C","a":{"pr":1},"b":{"pr":1.001}}`,
		`{"t":"2026-05-19T13:31:24.0000000","sat":"G02","sig":"1C","a":{"pr":5}}`,
		`{"t":"2026-05-19T13:31:24.0000000","sat":"G03","sig":"1C","b":{"pr":6}}`,
		``,
	}, "\n")
	if got := buf.String(); got != want {
		t.Fatalf("diff output:\n%s\nwant:\n%s", got, want)
	}
}

func TestJSONReporterMetadata(t *testing.T) {
	a := rinex.Metadata{Comment: rinex.Lines{"left"}}
	var b rinex.Metadata
	b.Marker.Name = "right"
	var buf bytes.Buffer
	if err := (jsonReporter{enc: json.NewEncoder(&buf)}).Metadata(a, b); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	want := `{"a":{"comment":["left"]},"b":{"marker":{"name":"right"}}}` + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("metadata diff output:\n%s\nwant:\n%s", got, want)
	}
}

func testTime(t *testing.T, s string) rinex.Time {
	t.Helper()
	var tm rinex.Time
	if err := tm.UnmarshalText([]byte(s)); err != nil {
		t.Fatalf("UnmarshalText(%q): %v", s, err)
	}
	return tm
}
