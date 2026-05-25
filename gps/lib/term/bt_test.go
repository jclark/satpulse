package term

import (
	"testing"
)

func TestParseBDAddr(t *testing.T) {
	tests := []struct {
		in   string
		want BDAddr
	}{
		{"00:00:00:00:00:00", BDAddr{0, 0, 0, 0, 0, 0}},
		{"AA:BB:CC:DD:EE:FF", BDAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}},
		{"aa:bb:cc:dd:ee:ff", BDAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}},
		{"01:23:45:67:89:AB", BDAddr{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB}},
	}
	for _, tt := range tests {
		got, err := ParseBDAddr(tt.in)
		if err != nil {
			t.Errorf("ParseBDAddr(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseBDAddr(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseBDAddrBad(t *testing.T) {
	tests := []string{
		"",
		"AA:BB:CC:DD:EE",
		"AA:BB:CC:DD:EE:FF:00",
		"AA-BB-CC-DD-EE-FF",
		"AA:BB:CC:DD:EE:GG",
		"AABBCCDDEEFF",
		"A:BB:CC:DD:EE:FF",
	}
	for _, s := range tests {
		_, err := ParseBDAddr(s)
		if err == nil {
			t.Errorf("ParseBDAddr(%q) succeeded, want error", s)
		}
	}
}

func TestBDAddrString(t *testing.T) {
	a := BDAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if got, want := a.String(), "AA:BB:CC:DD:EE:FF"; got != want {
		t.Errorf("BDAddr.String() = %q, want %q", got, want)
	}
}

func TestBDAddrRoundTrip(t *testing.T) {
	in := "12:34:56:78:9A:BC"
	a, err := ParseBDAddr(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.String(); got != in {
		t.Errorf("round-trip %q -> %v -> %q", in, a, got)
	}
}
