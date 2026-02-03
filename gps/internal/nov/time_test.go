package nov

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/ptime"
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

func TestUTCConversionParamsFromIonUTC(t *testing.T) {
	//asciiPacket := "#IONUTCA,COM3,17548,97.0,FINE,2381,562617.000,472258205,22,18;2.514570951461792e-08,2.235174179077148e-08,-1.192092895507812e-07,-1.192092895507812e-07,1.331200000000000e+05,3.276800000000000e+04,-2.621440000000000e+05,4.587520000000000e+05,2382,147456,9.313225746154785e-10,6.217248938e-15,2441,7,18,18,0*37734163\r\n"
	asciiPacket := "#IONUTCA,COM3,0,99.7,FINESTEERING,2382,7850.000,00000000,0000,784;2.514570951461792e-08,2.235174179077148e-08,-1.192092895507813e-07,-1.192092895507813e-07,1.331200000000000e+05,3.276800000000000e+04,-2.621440000000000e+05,4.587520000000000e+05,2382,147456,9.3132257461548000e-10,6.217248938e-15,2441,7,18,18,0*73788199\r\n"

	msg, err := novmsg.ParseAsciiMessage([]byte(asciiPacket))
	if err != nil {
		t.Fatalf("Failed to parse ASCII packet: %v", err)
	}

	ionutc, ok := msg.Body.(*novmsg.IonUTC)
	if !ok {
		t.Fatalf("Parsed message is not IonUTC, got %T", msg.Body)
	}

	_, now := msgHdrTime(&msg.Hdr)
	conversion, err := utcConversionParamsFromIonUTC(ionutc, now)
	if err != nil {
		t.Fatalf("utcConversionParamsFromIonUTC failed: %v", err)
	}

	gnss, headerTime := msgHdrTime(&msg.Hdr)
	if gnss == 0 {
		t.Fatal("Expected valid GNSS time from header")
	}

	correction, valid := conversion.Correction.Correction(headerTime)

	// Calculate the floating-point correction directly (matching ptime implementation)
	timeDiff := headerTime.Sub(conversion.Correction.Ref)
	fpCorrection := conversion.Correction.Bias + float64(timeDiff)*conversion.Correction.Drift

	t.Logf("Reference time: %v", conversion.Correction.Ref)
	t.Logf("Header time: %v", headerTime)
	t.Logf("Time difference: %v", headerTime.Sub(conversion.Correction.Ref))
	t.Logf("Time difference ns: %v", float64(timeDiff))
	t.Logf("Bias: %v ns", conversion.Correction.Bias)
	t.Logf("Drift: %v", conversion.Correction.Drift)
	t.Logf("FP correction: %v ns", fpCorrection)
	t.Logf("Duration correction: %v", correction)
	t.Logf("Correction valid: %v", valid)

	if !valid {
		t.Errorf("Correction should be valid")
	}

	if fpCorrection == 0 {
		t.Errorf("Floating-point correction should be non-zero for this test case, got %v", fpCorrection)
	}

	// Verify the LeapSecond matches LeapSecond2016
	expectedLS := ptime.LeapSecond2016()
	if conversion.LeapSecond != expectedLS {
		t.Errorf("LeapSecond = %v, want %v", conversion.LeapSecond, expectedLS)
	}
}
