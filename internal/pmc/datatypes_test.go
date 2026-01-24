package pmc

import (
	"math"
	"testing"
	"time"
)

func TestDurationToClockAccuracy(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     ClockAccuracy
	}{
		{"Zero", 0, 0},
		{"Negative", -1 * time.Nanosecond, 0},
		{"Over10s", 11 * time.Second, 0},

		// Test all accuracy levels
		{"1ns", 1 * time.Nanosecond, ClockAccuracyWithin1ns},
		{"2ns", 2 * time.Nanosecond, ClockAccuracyWithin2point5ns},
		{"3ns", 3 * time.Nanosecond, ClockAccuracyWithin10ns},
		{"10ns", 10 * time.Nanosecond, ClockAccuracyWithin10ns},
		{"24ns", 24 * time.Nanosecond, ClockAccuracyWithin25ns},
		{"25ns", 25 * time.Nanosecond, ClockAccuracyWithin25ns},
		{"99ns", 99 * time.Nanosecond, ClockAccuracyWithin100ns},
		{"100ns", 100 * time.Nanosecond, ClockAccuracyWithin100ns},
		{"249ns", 249 * time.Nanosecond, ClockAccuracyWithin250ns},
		{"250ns", 250 * time.Nanosecond, ClockAccuracyWithin250ns},

		{"1µs", 1 * time.Microsecond, ClockAccuracyWithin1us},
		{"2µs", 2 * time.Microsecond, ClockAccuracyWithin2point5us},
		{"2.5µs", 2500 * time.Nanosecond, ClockAccuracyWithin2point5us},
		{"3µs", 3 * time.Microsecond, ClockAccuracyWithin10us},
		{"10µs", 10 * time.Microsecond, ClockAccuracyWithin10us},
		{"24µs", 24 * time.Microsecond, ClockAccuracyWithin25us},
		{"25µs", 25 * time.Microsecond, ClockAccuracyWithin25us},
		{"99µs", 99 * time.Microsecond, ClockAccuracyWithin100us},
		{"100µs", 100 * time.Microsecond, ClockAccuracyWithin100us},
		{"249µs", 249 * time.Microsecond, ClockAccuracyWithin250us},
		{"250µs", 250 * time.Microsecond, ClockAccuracyWithin250us},

		{"1ms", 1 * time.Millisecond, ClockAccuracyWithin1ms},
		{"2ms", 2 * time.Millisecond, ClockAccuracyWithin2point5ms},
		{"2.5ms", 2500 * time.Microsecond, ClockAccuracyWithin2point5ms},
		{"3ms", 3 * time.Millisecond, ClockAccuracyWithin10ms},
		{"10ms", 10 * time.Millisecond, ClockAccuracyWithin10ms},
		{"24ms", 24 * time.Millisecond, ClockAccuracyWithin25ms},
		{"25ms", 25 * time.Millisecond, ClockAccuracyWithin25ms},
		{"99ms", 99 * time.Millisecond, ClockAccuracyWithin100ms},
		{"100ms", 100 * time.Millisecond, ClockAccuracyWithin100ms},
		{"249ms", 249 * time.Millisecond, ClockAccuracyWithin250ms},
		{"250ms", 250 * time.Millisecond, ClockAccuracyWithin250ms},

		{"1s", 1 * time.Second, ClockAccuracyWithin1s},
		{"9s", 9 * time.Second, ClockAccuracyWithin10s},
		{"10s", 10 * time.Second, ClockAccuracyWithin10s},

		// Edge cases
		{"Just under 10s", 9999 * time.Millisecond, ClockAccuracyWithin10s},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DurationToClockAccuracy(tt.duration)
			if got != tt.want {
				t.Errorf("DurationToClockAccuracy(%v) = %v (%s), want %v (%s)",
					tt.duration, got, got.Description(), tt.want, tt.want.Description())
			}
		})
	}
}

func TestClockAccuracyDescription(t *testing.T) {
	for _, tc := range []struct {
		ca ClockAccuracy
		s  string
	}{
		{ClockAccuracyWithin1ps, "<=1ps"},
		{ClockAccuracyWithin2point5ps, "<=2.5ps"},
		{ClockAccuracyWithin10ps, "<=10ps"},
		{ClockAccuracyWithin25ps, "<=25ps"},
		{ClockAccuracyWithin100ps, "<=100ps"},
		{ClockAccuracyWithin250ps, "<=250ps"},
		{ClockAccuracyWithin1ns, "<=1ns"},
		{ClockAccuracyWithin2point5ns, "<=2.5ns"},
		{ClockAccuracyWithin10ns, "<=10ns"},
		{ClockAccuracyWithin25ns, "<=25ns"},
		{ClockAccuracyWithin100ns, "<=100ns"},
		{ClockAccuracyWithin250ns, "<=250ns"},
		{ClockAccuracyWithin1us, "<=1us"},
		{ClockAccuracyWithin2point5us, "<=2.5us"},
		{ClockAccuracyWithin10us, "<=10us"},
		{ClockAccuracyWithin25us, "<=25us"},
		{ClockAccuracyWithin100us, "<=100us"},
		{ClockAccuracyWithin250us, "<=250us"},
		{ClockAccuracyWithin1ms, "<=1ms"},
		{ClockAccuracyWithin2point5ms, "<=2.5ms"},
		{ClockAccuracyWithin10ms, "<=10ms"},
		{ClockAccuracyWithin25ms, "<=25ms"},
		{ClockAccuracyWithin100ms, "<=100ms"},
		{ClockAccuracyWithin250ms, "<=250ms"},
		{ClockAccuracyWithin1s, "<=1s"},
		{ClockAccuracyWithin10s, "<=10s"},
		{ClockAccuracyGreater10s, ">10s"},
		{ClockAccuracyUnknown, "unknown"},
		{ClockAccuracy(0), "reserved"},
	} {
		desc := tc.ca.Description()
		if desc != tc.s {
			t.Errorf("ClockAccuracy.Description() = %q, want %q", desc, tc.s)
		}
	}

}

func TestAdevToOffsetScaledLogVariance(t *testing.T) {
	specVariance := 1.414 * math.Pow(2, -73)

	tests := []struct {
		name      string
		adev      float64
		tau       float64
		want      uint16
		wantPanic bool
	}{
		{
			name: "SpecExample",
			adev: math.Sqrt(3 * specVariance),
			tau:  1,
			want: 0x3780,
		},
		{
			name: "MinimumRepresentable",
			adev: math.Pow(2, -80),
			tau:  1,
			want: 0x0000,
		},
		{
			name: "UnknownForNonPositiveAdev",
			adev: 0,
			tau:  1,
			want: OffsetScaledLogVarianceUnknown,
		},
		{
			name: "OverflowReturnsFFFF",
			adev: math.Pow(2, 90),
			tau:  1,
			want: 0xFFFF,
		},
		{
			name:      "InvalidTauPanics",
			adev:      1e-9,
			tau:       0,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.wantPanic {
						t.Fatalf("unexpected panic: %v", r)
					}
				} else if tt.wantPanic {
					t.Fatal("expected panic but function returned")
				}
			}()

			if tt.wantPanic {
				AdevToOffsetScaledLogVariance(tt.adev, tt.tau)
				return
			}

			got := AdevToOffsetScaledLogVariance(tt.adev, tt.tau)
			if got != tt.want {
				t.Fatalf("AdevToOffsetScaledLogVariance(%g, %g) = 0x%04X, want 0x%04X", tt.adev, tt.tau, got, tt.want)
			}
		})
	}
}
