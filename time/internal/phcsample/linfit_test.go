package phcsample

import (
	"testing"
	"time"
)

func TestFitLineFloat(t *testing.T) {
	tests := []struct {
		name            string
		xs, ys          []float64
		expectSlope     float64
		expectIntercept float64
		expectOK        bool
	}{
		{
			name:            "exact line",
			xs:              []float64{0, 1, 2, 3},
			ys:              []float64{0.5, 2.5, 4.5, 6.5},
			expectSlope:     2,
			expectIntercept: 0.5,
			expectOK:        true,
		},
		{
			name:            "negative slope",
			xs:              []float64{0, 1, 2},
			ys:              []float64{3, 2, 1},
			expectSlope:     -1,
			expectIntercept: 3,
			expectOK:        true,
		},
		{
			// Residuals +0.5,-1.5,+1.5,-0.5 around y = x + 0.5; the
			// sums work out to recover the line exactly.
			name:            "noise around a line",
			xs:              []float64{0, 1, 2, 3},
			ys:              []float64{1, 0, 4, 3},
			expectSlope:     1,
			expectIntercept: 0.5,
			expectOK:        true,
		},
		{
			name:     "empty",
			expectOK: false,
		},
		{
			name:     "single point",
			xs:       []float64{1},
			ys:       []float64{1},
			expectOK: false,
		},
		{
			name:     "no x spread",
			xs:       []float64{2, 2, 2},
			ys:       []float64{1, 2, 3},
			expectOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, ok := fitLine(tc.xs, tc.ys)
			if ok != tc.expectOK {
				t.Fatalf("ok = %v, want %v", ok, tc.expectOK)
			}
			if !ok {
				return
			}
			if l.slope != tc.expectSlope || l.intercept != tc.expectIntercept {
				t.Errorf("got slope %v intercept %v, want %v %v", l.slope, l.intercept, tc.expectSlope, tc.expectIntercept)
			}
		})
	}
}

func TestFitLineDuration(t *testing.T) {
	// y = 2x + 250ms over Duration coordinates; recovery is exact.
	xs := []time.Duration{0, time.Second, 2 * time.Second}
	ys := []time.Duration{250 * time.Millisecond, 2250 * time.Millisecond, 4250 * time.Millisecond}
	l, ok := fitLine(xs, ys)
	if !ok || l.slope != 2 || l.intercept != 250*time.Millisecond {
		t.Fatalf("got %+v ok=%v, want slope 2 intercept 250ms", l, ok)
	}
	if got := lineAt(l, 500*time.Millisecond); got != 1250*time.Millisecond {
		t.Errorf("lineAt = %v, want 1.25s", got)
	}
	if got := lineResidual(l, time.Second, 2*time.Second); got != -250*time.Millisecond {
		t.Errorf("lineResidual = %v, want -250ms", got)
	}
}

func TestFitLineMixedTypes(t *testing.T) {
	// Duration x against float64 y: the fit carries fractional y exactly.
	xs := []time.Duration{0, 2}
	ys := []float64{0.5, 1.5}
	l, ok := fitLine(xs, ys)
	if !ok || l.slope != 0.5 || l.intercept != 0.5 {
		t.Fatalf("got %+v ok=%v, want slope 0.5 intercept 0.5", l, ok)
	}
	if got := lineAt(l, time.Duration(3)); got != 2.0 {
		t.Errorf("lineAt = %v, want 2.0", got)
	}
}

func TestFitLineIntTruncation(t *testing.T) {
	// For an int64-based Y the fractional intercept truncates at fit
	// time, and lineAt truncates the slope term.
	xs := []time.Duration{1, 3}
	ys := []time.Duration{1, 2} // slope 0.5, intercept 0.5 -> truncates to 0
	l, ok := fitLine(xs, ys)
	if !ok || l.slope != 0.5 || l.intercept != 0 {
		t.Fatalf("got %+v ok=%v, want slope 0.5 intercept 0", l, ok)
	}
	if got := lineAt(l, time.Duration(3)); got != 1 {
		t.Errorf("lineAt = %v, want 1 (1.5 truncated)", got)
	}
}
