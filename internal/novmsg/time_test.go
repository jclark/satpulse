package novmsg

import (
	"testing"
)

var timeTestCases = []dataTestCase{
	{
		name:  "UM980 TIME",
		hex:   "aa44121c650000012c00c03461a04c09305d621afc4f8c212499120000000000a84d81508f5e10bf9ac298de427d433e00000000000032c0e907000008160239803e000001000000b3265817",
		ascii: "#TIMEA,COM1,13504,97.0,FINE,2380,442654.000,562843644,14,18;VALID,-6.244420740e-05,9.075413271e-09,-18.00000000000,2025,8,22,2,57,16000,VALID*70144497\r\n",
		hdr: MessageHeader{
			Port: "COM1", // Binary port 1 = "COM1"
			CommonHeader: CommonHeader{
				Sequence:           13504,
				IdleTime:           Percentage(97),
				TimeStatus:         TimeStatusFine,
				Week:               2380,
				MillisecondsOfWeek: GPSec(442654000), // 442654.000 * 1000
				RecvStatus:         562843644,
				Reserved:           39204, // Binary format value (canonical)
				Version:            18,
			},
		},
		value: &Time{
			ClockStatus: ClockStatusValid, // VALID = 0
			Offset:      -6.244420740247078e-05, // Use the precise binary value
			OffsetStd:   9.075413270795956e-09,  // Use the precise binary value
			UTCOffset:   -18.0,
			UTCYear:     2025,
			UTCMonth:    8,
			UTCDay:      22,
			UTCHour:     2,
			UTCMin:      57,
			UTCMs:       16000,
			UTCStatus:   UTCStatusValid, // VALID = 1
		},
		// ASCII format has different floating-point precision and header field values
		fixupValueForAscii: fixupTimeValueForAscii,
		fixupHeaderForAscii: func(hdr MessageHeader) MessageHeader {
			// ASCII format has different Reserved field value
			hdr.CommonHeader.Reserved = 14
			return hdr
		},
	},
}

func TestTimeBinary(t *testing.T) {
	testDataBin(t, timeTestCases)
}

func TestTimeAscii(t *testing.T) {
	testDataAscii(t, timeTestCases)
}

// fixupTimeValueForAscii converts a Time with full binary precision
// to match the limited precision values that come from ASCII parsing.
// This simulates the receiver firmware's float-to-ASCII conversion using %.10g formatting.
func fixupTimeValueForAscii(msg Msg) Msg {
	r := msg.(*Time)
	result := *r // Copy the struct

	const floatFormat = "%.10g"
	// Simulate receiver's ASCII formatting: binary -> printf("%.10g") -> parse back
	fixupFloat(&result.Offset, floatFormat)
	fixupFloat(&result.OffsetStd, floatFormat)
	// UTCOffset typically stays exact since -18.0 is representable exactly

	return &result
}