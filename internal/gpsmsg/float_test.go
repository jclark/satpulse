package gpsmsg

import (
	"math"
	"testing"
)

const floatZero float64 = 0

func TestSafeFloat64ToInt64_Normal(t *testing.T) {
	// Array of float64 values that are exactly representable as int64
	inputs := []float64{
		0,
		-floatZero,
		1,
		-1,
		1024,             // A simple power of 2
		-1024,            // Negative power of 2
		123456,           // A typical positive integer
		-123456,          // A typical negative integer
		1 << 50,          // A large power of 2, well within the exactly representable range
		-(1 << 50),       // Negative large power of 2, well within the exactly representable range
		1 << 53,          // At the limit of exact representation
		-(1 << 53),       // Negative value at the limit of exact representation
		(1 << 53) - 1,    // Just below the limit of exact representation
		-((1 << 53) - 1), // Negative value just below the limit of exact representation
	}

	for _, input := range inputs {
		expected := int64(input)
		result, err := float64ToInt64(input)
		if err != nil {
			t.Errorf("safeFloat64ToInt64(%v) returned an unexpected error: %v", input, err)
		}
		if result != expected {
			t.Errorf("safeFloat64ToInt64(%v) = %v, want %v", input, result, expected)
		}
	}
}

func TestSafeFloat64ToInt64_Large(t *testing.T) {
	for f := float64(math.MaxInt64 - 1024); f < float64(math.MaxInt64+1024); f = math.Nextafter(f, math.Inf(1)) {
		testSafeFloat64ToInt64Val(t, f)
	}
	for f := float64(math.MinInt64 + 1024); f > float64(math.MinInt64-1024); f = math.Nextafter(f, math.Inf(-1)) {
		testSafeFloat64ToInt64Val(t, f)
	}
}

func testSafeFloat64ToInt64Val(t *testing.T, f float64) {
	result, err := float64ToInt64(f)
	if float64(math.MinInt64) <= f && f <= float64(math.MaxInt64) {
		if err != nil {
			t.Errorf("safeFloat64ToInt64(%v) returned an unexpected error: %v", f, err)
		} else if float64(result) != f {
			t.Errorf("safeFloat64ToInt64(%v) returned int64 %v, which converted back to %v", f, result, float64(result))
		}
	} else {
		if err == nil {
			t.Errorf("safeFloat64ToInt64(%v) should have returned an error", f)
		}
	}
}

func TestSafeFloat64ToInt64_NaN(t *testing.T) {
	input := math.NaN()

	_, err := float64ToInt64(input)
	if err == nil {
		t.Errorf("safeFloat64ToInt64(%v) should have returned an error", input)
	}
}

func TestSafeFloat64ToInt64_Infinity(t *testing.T) {
	inputs := []float64{math.Inf(1), math.Inf(-1)}

	for _, input := range inputs {
		_, err := float64ToInt64(input)
		if err == nil {
			t.Errorf("safeFloat64ToInt64(%v) should have returned an error", input)
		}
	}
}
