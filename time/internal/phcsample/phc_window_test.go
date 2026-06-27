package phcsample

import (
	"testing"
	"time"
)

func TestFirstDiscontinuity(t *testing.T) {
	const sec = time.Second
	const ms = time.Millisecond
	const discThresh = 1 * ms

	tests := []struct {
		name       string
		intervals  []time.Duration
		expectIdx  int
		expectOK   bool
	}{
		{
			name:      "all clean",
			intervals: []time.Duration{sec, sec, sec, sec},
			expectOK:  false,
		},
		{
			name:      "sub-threshold jitter ignored",
			intervals: []time.Duration{sec, sec + 500*time.Microsecond, sec, sec},
			expectOK:  false,
		},
		{
			name:      "forward missing-pulse gap",
			intervals: []time.Duration{sec, sec, 2 * sec, sec, sec},
			expectIdx: 2,
			expectOK:  true,
		},
		{
			name:      "forward PHC step above threshold",
			intervals: []time.Duration{sec, sec, sec + 5*ms, sec, sec},
			expectIdx: 2,
			expectOK:  true,
		},
		{
			name:      "backward PHC step above threshold",
			intervals: []time.Duration{sec, sec, sec - 5*ms, sec, sec},
			expectIdx: 2,
			expectOK:  true,
		},
		{
			name:      "negative interval from backward step",
			intervals: []time.Duration{sec, sec, -500 * ms, sec, sec},
			expectIdx: 2,
			expectOK:  true,
		},
		{
			name:      "interior glitched edge - both neighbour intervals distorted",
			intervals: []time.Duration{sec, sec + 5*ms, sec - 5*ms, sec, sec},
			expectOK:  false,
		},
		{
			name:      "trailing glitched edge triggers discontinuity",
			intervals: []time.Duration{sec, sec, sec, sec + 5*ms},
			expectIdx: 3,
			expectOK:  true,
		},
		{
			name:      "leading glitched edge triggers discontinuity",
			intervals: []time.Duration{sec + 5*ms, sec, sec, sec},
			expectIdx: 0,
			expectOK:  true,
		},
		{
			name:      "burst of jitter - multiple adjacent large deviations",
			intervals: []time.Duration{sec, sec + 5*ms, sec - 5*ms, sec + 5*ms, sec},
			expectOK:  false,
		},
		{
			name:      "first of two separate discontinuities",
			intervals: []time.Duration{sec, 2 * sec, sec, sec, 2 * sec, sec},
			expectIdx: 1,
			expectOK:  true,
		},
		{
			name:      "at-threshold deviation counts",
			intervals: []time.Duration{sec, sec, sec + discThresh, sec, sec},
			expectIdx: 2,
			expectOK:  true,
		},
		{
			name:      "just-below-threshold deviation does not",
			intervals: []time.Duration{sec, sec, sec + discThresh - 1, sec, sec},
			expectOK:  false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx, ok := firstDiscontinuity(tc.intervals, sec, discThresh)
			if ok != tc.expectOK || (ok && idx != tc.expectIdx) {
				t.Errorf("got (%d, %v), want (%d, %v)", idx, ok, tc.expectIdx, tc.expectOK)
			}
		})
	}
}
