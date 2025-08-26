package nov

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/novmsg"
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

func TestTimeMsgFromTime(t *testing.T) {
	tests := []struct {
		name      string
		packet    string
		expect    *gpsprot.TimeMsg
		expectErr bool
	}{
		{
			name: "valid TIMEA packet",
			packet: "#TIMEA,COM3,17548,97.0,FINE,2381,207960.000,117601205,13,18;VALID,3.107194079e-04,1.298132705e-08,-18.00000000000,2025,8,26,9,45,42000,VALID*14216034\r\n",
			expect: &gpsprot.TimeMsg{
				Tag:         TagAscii,
				NativeMsgID: "TIME",
				GNSS:        0, // NovAtel doesn't specify reference GNSS
				TAITime:     ptime.GPS(2381, 207960*time.Second),
				UTCTime: func() *ptime.UTCTime {
					utc := ptime.UTC(2025, 8, 26, 9, 45, 42, 0)
					return &utc
				}(),
				UTCOffset: 37, // TAI-UTC = 19 - (-18) = 37
				Accuracy:  13 * time.Nanosecond, // 1.298132705e-08 seconds rounded up
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := novmsg.ParseAsciiMessage([]byte(tt.packet))
			if err != nil {
				t.Fatalf("Failed to parse ASCII packet: %v", err)
			}

			timeMsg, ok := msg.Body.(*novmsg.Time)
			if !ok {
				t.Fatalf("Parsed message is not Time, got %T", msg.Body)
			}

			got, err := timeMsgFromTime(&msg.Hdr, timeMsg, TagAscii)
			
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error, but got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(got, tt.expect) {
				t.Errorf("timeMsgFromTime() result mismatch")
				t.Errorf("Got:      %+v", got)
				t.Errorf("Expected: %+v", tt.expect)
			}
		})
	}
}
