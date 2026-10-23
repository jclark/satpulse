package gpsprot

import "testing"

func TestParseAccuracy(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expect    Length
		expectErr bool
	}{
		{name: "meters", input: "20", expect: 20 * Meter},
		{name: "sub-millimeter", input: "0.0001", expect: Millimeter / 10},
		{name: "rounds to nearest", input: "0.0000014", expect: Micrometer},
		{name: "too small to represent", input: "0.0000001", expect: Micrometer},
		{name: "exponent too small to represent", input: "1e-9", expect: Micrometer},
		{name: "too small for float64", input: "1e-400", expect: Micrometer},
		{name: "zero", input: "0", expect: 0},
		{name: "zero with exponent", input: "0.0e5", expect: 0},
		{name: "negative too small to represent", input: "-0.0000001", expect: -Micrometer},
		{name: "invalid", input: "abc", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAccuracy(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Errorf("got %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestAccuracyMeters(t *testing.T) {
	tests := []struct {
		name   string
		input  float64
		expect Length
	}{
		{name: "sub-millimeter", input: 0.0001, expect: Millimeter / 10},
		{name: "too small to represent", input: 1e-9, expect: Micrometer},
		{name: "zero", input: 0, expect: 0},
		{name: "negative too small to represent", input: -1e-9, expect: -Micrometer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AccuracyMeters(tc.input); got != tc.expect {
				t.Errorf("got %v, want %v", got, tc.expect)
			}
		})
	}
}
