package unc

import (
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/uncmsg"
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

func TestTimeRecTime(t *testing.T) {
	// Parse the ASCII packet to get the message
	asciiPacket := "#RECTIMEA,97,GPS,FINE,2379,441279000,0,0,18,15;VALID,-4.997042211e-04,7.786425700e-09,-18.00000000000,2025,8,15,2,34,21000,VALID*a15fd8d6\r\n"
	header, msg, err := uncmsg.ParseAsciiMessage([]byte(asciiPacket))
	if err != nil {
		t.Fatalf("Failed to parse ASCII packet: %v", err)
	}
	
	recTime, ok := msg.(*uncmsg.RecTime)
	if !ok {
		t.Fatalf("Parsed message is not RecTime, got %T", msg)
	}
	
	// Convert to TimeMsg
	timeMsg, err := timeRecTime(header, recTime, TagAscii)
	if err != nil {
		t.Fatalf("timeRecTime failed: %v", err)
	}
	
	if timeMsg == nil {
		t.Fatal("timeRecTime returned nil")
	}
	
	// Check the values
	if timeMsg.Tag != TagAscii {
		t.Errorf("Tag = %v, want %v", timeMsg.Tag, TagAscii)
	}
	
	if timeMsg.NativeMsgID != "RECTIME" {
		t.Errorf("NativeMsgID = %q, want %q", timeMsg.NativeMsgID, "RECTIME")
	}
	
	if timeMsg.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want %v", timeMsg.GNSS, gpsprot.GPS)
	}
	
	// Check UTC time (2025-08-15 02:34:21.000)
	if timeMsg.UTCTime == nil {
		t.Fatal("UTCTime is nil")
	}
	
	utc := *timeMsg.UTCTime
	expectedUTC := ptime.UTC(2025, 8, 15, 2, 34, 21, 0)
	if utc != expectedUTC {
		t.Errorf("UTCTime = %v, want %v", utc, expectedUTC)
	}
	
	// Check TAI time - convert the UTC to TAI using leap second info
	// Use the 2016 leap second (UTC offset = 37)
	ls := ptime.LeapSecond2016()
	expectedTAI := ls.UTCtoTime(expectedUTC)
	if timeMsg.TAITime != expectedTAI {
		t.Errorf("TAITime = %v, want %v", timeMsg.TAITime, expectedTAI)
	}
	
	// Check UTC offset (GPS-UTC = -18, so TAI-UTC = 37)
	if timeMsg.UTCOffset != 37 {
		t.Errorf("UTCOffset = %d, want %d", timeMsg.UTCOffset, 37)
	}
	
	// Check accuracy (based on OffsetStd = 7.786425700e-09 seconds)
	// convertAccuracy rounds up to avoid underestimating
	expectedAccuracy := time.Duration(8) // 7.786425700 nanoseconds rounds up to 8
	if timeMsg.Accuracy != expectedAccuracy {
		t.Errorf("Accuracy = %v, want %v", timeMsg.Accuracy, expectedAccuracy)
	}
}