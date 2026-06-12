package ntpshm

import (
	"testing"
	"time"
)

func TestPrecision(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want int8
	}{
		{"zero", 0, -128},
		{"negative", -time.Nanosecond, -128},
		{"1ns", time.Nanosecond, -29},
		{"5ns", 5 * time.Nanosecond, -27},
		{"100ns", 100 * time.Nanosecond, -23},
		{"1us", time.Microsecond, -19},
		{"1ms", time.Millisecond, -9},
		{"500ms", 500 * time.Millisecond, -1},
		{"1s", time.Second, 0},
		{"2s", 2 * time.Second, 1},
		{"just over 2s", 2*time.Second + time.Nanosecond, 2},
		{"60s", 60 * time.Second, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Precision(tt.d); got != tt.want {
				t.Fatalf("Precision(%v) = %d, want %d", tt.d, got, tt.want)
			}
		})
	}
}
