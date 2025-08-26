package novmsg

import (
	"testing"
)

var timeTestCases = []dataTestCase{
	{
		name:  "UM980 TIME",
		hex:   "aa44121c650000012c00c03461a04c09305d621afc4f8c212499120000000000a84d81508f5e10bf9ac298de427d433e00000000000032c0e907000008160239803e000001000000b3265817",
		ascii: "#TIMEA,COM1,13504,97.0,FINE,2380,442654.000,562843644,14,18;VALID,-6.244420740e-05,9.075413271e-09,-18.00000000000,2025,8,22,2,57,16000,VALID*70144497\r\n",
		hdr: MsgHdr{
			Port: "COM1", // Binary port 1 = "COM1"
			CommonHdr: CommonHdr{
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
		fixupHeaderForAscii: func(hdr MsgHdr) MsgHdr {
			// ASCII format has different Reserved field value
			hdr.Reserved = 14
			hdr.IdleTime *= 2 // UM980 doesn't follow OEM7 spec here: the byte is exactly the percentage
			return hdr
		},
	},
	{
		name: "ByNav M20 TIME",
		ascii: "#TIMEA,COM3,0,99.7,FINESTEERING,2380,524599.000,00000000,0000,784;VALID,-4.288684441e-04,0.000000000e+00,-18.00000000000,2025,8,23,1,43,1000,VALID*7d8d6d09\r\n",
		hex: "aa44121c650000602c000000c7b44c09d8be441f000000000000100300000000ca3c17f1371b3cbf000000000000000000000000000032c0e90700000817012be80300000100000032b1ed45",
		hdr: MsgHdr{
			Port: "96", // = 0x60 (this seems completely bogus)
			CommonHdr: CommonHdr{
				Sequence:           0,
				IdleTime:           Percentage(199), 
				TimeStatus:         TimeStatusFineSteering,
				Week:               2380,
				MillisecondsOfWeek: GPSec(524599000),
				RecvStatus:         0,
				Reserved:           0,
				Version:            784,
			},
		},
		value: &Time{
			ClockStatus: ClockStatusValid,
			Offset:      -0.00042886844411511567, // Use the precise binary value
			OffsetStd:   0.0,
			UTCOffset:   -18.0,
			UTCYear:     2025,
			UTCMonth:    8,
			UTCDay:      23,
			UTCHour:     1,
			UTCMin:      43,
			UTCMs:       1000,
			UTCStatus:   UTCStatusValid,
		},
		fixupValueForAscii: fixupTimeValueForAscii,
		fixupHeaderForAscii: func(hdr MsgHdr) MsgHdr {
			hdr.Port = "COM3"
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
func fixupTimeValueForAscii(msg MsgBody) MsgBody {
	r := msg.(*Time)
	result := *r

	const floatFormat = "%.10g"
	// Simulate receiver's ASCII formatting: binary -> printf("%.10g") -> parse back
	fixupFloat(&result.Offset, floatFormat)
	fixupFloat(&result.OffsetStd, floatFormat)
	return &result
}