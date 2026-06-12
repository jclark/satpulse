package ntime

import (
	"testing"
	"time"
)

func TestRound(t *testing.T) {
	tests := []struct {
		name   string
		t      Time
		d      time.Duration
		expect Time
	}{
		{name: "down", t: Time(1_400_000_000), d: time.Second, expect: Time(1_000_000_000)},
		{name: "up", t: Time(1_500_000_000), d: time.Second, expect: Time(2_000_000_000)},
		{name: "exact", t: Time(3_000_000_000), d: time.Second, expect: Time(3_000_000_000)},
		{name: "negative", t: Time(-1_400_000_000), d: time.Second, expect: Time(-1_000_000_000)},
		{name: "millisecond", t: Time(1_000_499_999), d: time.Millisecond, expect: Time(1_000_000_000)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.Round(tc.d); got != tc.expect {
				t.Errorf("got %d, want %d", got, tc.expect)
			}
		})
	}
}

func TestConversions(t *testing.T) {
	sys := time.Date(2026, 4, 19, 12, 0, 0, 123, time.UTC)
	if got := Sys(sys); got != Time(sys.UnixNano()) {
		t.Errorf("Sys: got %d, want %d", got, sys.UnixNano())
	}
	if got := Sys(sys).SysTime(); !got.Equal(sys) {
		t.Errorf("SysTime round trip: got %v, want %v", got, sys)
	}
}

func TestArithmetic(t *testing.T) {
	a := Time(5_000_000_000)
	b := a.Add(250 * time.Millisecond)
	if b != Time(5_250_000_000) {
		t.Errorf("Add: got %d", b)
	}
	if d := b.Sub(a); d != 250*time.Millisecond {
		t.Errorf("Sub: got %v", d)
	}
	if !a.Before(b) || b.Before(a) {
		t.Errorf("Before: got %v, %v", a.Before(b), b.Before(a))
	}
}
