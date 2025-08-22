package nov

import (
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

func TestConvertAccuracy(t *testing.T) {
	tests := []struct {
		input    float64
		expected time.Duration
	}{
		// Normal cases
		{1.0, time.Second},
		{0.001, time.Millisecond},
		{0.000001, time.Microsecond},
		{0.000000001, time.Nanosecond},
		
		// Fractional nanoseconds - should round up
		{0.0000000015, 2 * time.Nanosecond},  // 1.5ns -> 2ns
		{0.0000000011, 2 * time.Nanosecond},  // 1.1ns -> 2ns
		{0.0000000019, 2 * time.Nanosecond},  // 1.9ns -> 2ns
		{7.786425700e-09, 8 * time.Nanosecond}, // 7.786ns -> 8ns (from actual test case)
		
		// Large values (still within range)
		{3600.0, time.Hour},
		{86400.0, 24 * time.Hour},
		{31536000.0, 365 * 24 * time.Hour}, // 1 year
		
		// Edge cases - zero and negative
		{0.0, 0},
		{-1.0, 0},
		{-0.001, 0},
		{-1000.0, 0},
		
		// Special float values
		{math.Inf(1), 0},  // Positive infinity
		{math.Inf(-1), 0}, // Negative infinity
		{math.NaN(), 0},   // NaN
		
		// Out of range (time.Duration max is ~292 years)
		{1e20, 0}, // Way beyond int64 range
		{math.MaxFloat64, 0},
	}

	for _, tt := range tests {
		result := convertAccuracy(tt.input)
		if result != tt.expected {
			t.Errorf("convertAccuracy(%g) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestConvertUTCOffset(t *testing.T) {
	tests := []struct {
		input    float64
		expected uint8
	}{
		// Normal cases
		{-18.0, 37}, // GPS-UTC = -18 (typical 2024), TAI-UTC = 19 - (-18) = 37
		{-17.0, 36}, // TAI-UTC = 19 - (-17) = 36
		{-19.0, 38}, // TAI-UTC = 19 - (-19) = 38
		{0.0, 19},   // TAI-UTC = 19 - 0 = 19
		
		// Edge cases that should work
		{-236.0, 255}, // TAI-UTC = 19 - (-236) = 255 (max valid uint8)
		
		// Error cases - fractional values
		{-18.5, 0},
		{-17.1, 0},
		{0.5, 0},
		
		// Error cases - out of range (would wrap)
		{-237.0, 0},  // TAI-UTC = 19 - (-237) = 256, wraps to 0, but check fails
		{20.0, 0},    // TAI-UTC = 19 - 20 = -1, wraps to 255, but check fails
		{-1000.0, 0}, // Way out of range
		{1000.0, 0},  // Way out of range
		
		// Special float values
		{math.Inf(1), 0},  // Positive infinity
		{math.Inf(-1), 0}, // Negative infinity
		{math.NaN(), 0},    // NaN
	}

	for _, tt := range tests {
		result := convertUTCOffset(tt.input)
		if result != tt.expected {
			// Calculate what the intermediate value would be for debugging
			floatOff := float64(ptime.TAIMinusGPS) - tt.input
			t.Errorf("convertUTCOffset(%f) = %d, want %d (intermediate: %f)", 
				tt.input, result, tt.expected, floatOff)
		}
	}
}
