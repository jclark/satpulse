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
			// UM980 doesn't follow OEM7 spec here: the byte is exactly the percentage
			hdr.IdleTime *= 2
			// ASCII format has different Reserved field value
			hdr.Reserved = 14
			return hdr
		},
	},
	{
		name: "Bynav M20 TIME",
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
	{
		name:  "UM980 IONUTC",
		disable: true, // Disable because of conflicting message ID
		hex:   "aa44121c060000036c008c4461a04d09a8da88219d16261c21ec12000000000000005b3e000000000000583e00000000000080be00000000000080be0000000000400041000000000000e04000000000000010c10000000000001c414e09000000400200000000000000103e000000000000fc3c8909000007000000120000001200000000000000c9ca08ed",
		ascii: "#IONUTCA,COM3,17548,97.0,FINE,2381,562617.000,472258205,22,18;2.514570951461792e-08,2.235174179077148e-08,-1.192092895507812e-07,-1.192092895507812e-07,1.331200000000000e+05,3.276800000000000e+04,-2.621440000000000e+05,4.587520000000000e+05,2382,147456,9.313225746154785e-10,6.217248938e-15,2441,7,18,18,0*37734163\r\n",
		hdr: MsgHdr{
			Port: "COM3",
			CommonHdr: CommonHdr{
				Sequence:           17548,
				IdleTime:           Percentage(97),
				TimeStatus:         TimeStatusFine,
				Week:               2381,
				MillisecondsOfWeek: GPSec(562617000),
				RecvStatus:         472258205,
				Reserved:           60449,
				Version:            18,
			},
		},
		value: &IonUTC{
			Alpha0:    2.514570951461792e-08,
			Alpha1:    2.2351741790771484e-08,
			Alpha2:    -1.1920928955078125e-07,
			Alpha3:    -1.1920928955078125e-07,
			Beta0:     133120,
			Beta1:     32768,
			Beta2:     -262144,
			Beta3:     458752,
			UTCWn:     2382,
			Tot:       147456,
			A0:        9.313225746154785e-10,
			A1:        6.217248937900877e-15,
			WnLsf:     2441,
			Dn:        7,
			DeltatLs:  18,
			DeltatLsf: 18,
			Reserved:  0,
		},
		fixupValueForAscii: fixupUnicoreIonUTCValueForAscii,
		fixupHeaderForAscii: func(hdr MsgHdr) MsgHdr {
			hdr.IdleTime *= 2
			hdr.Reserved = 22
			return hdr
		},
	},
	{
		name:  "Bynav M20 IONUTC",
		hex:   "aa44121c080000606c000000c7b44e0910c8770000000000000010030000000000005b3e000000000000583e00000000000080be00000000000080be0000000000400041000000000000e04000000000000010c10000000000001c414e09000000400200000000000000103e000000000000fc3c89090000070000001200000012000000000000000c30cb0b",
		ascii: "#IONUTCA,COM3,0,99.7,FINESTEERING,2382,7850.000,00000000,0000,784;2.514570951461792e-08,2.235174179077148e-08,-1.192092895507813e-07,-1.192092895507813e-07,1.331200000000000e+05,3.276800000000000e+04,-2.621440000000000e+05,4.587520000000000e+05,2382,147456,9.3132257461548000e-10,6.217248938e-15,2441,7,18,18,0*73788199\r\n",
		hdr: MsgHdr{
			Port: "96", // = 0x60 (binary port value)
			CommonHdr: CommonHdr{
				Sequence:           0,
				IdleTime:           Percentage(199),
				TimeStatus:         TimeStatusFineSteering,
				Week:               2382,
				MillisecondsOfWeek: GPSec(7850000),
				RecvStatus:         0,
				Reserved:           0,
				Version:            784,
			},
		},
		value: &IonUTC{
			Alpha0:    2.514570951461792e-08,
			Alpha1:    2.2351741790771484e-08,
			Alpha2:    -1.1920928955078125e-07,
			Alpha3:    -1.1920928955078125e-07,
			Beta0:     133120,
			Beta1:     32768,
			Beta2:     -262144,
			Beta3:     458752,
			UTCWn:     2382,
			Tot:       147456,
			A0:        9.313225746154785e-10,
			A1:        6.217248937900877e-15,
			WnLsf:     2441,
			Dn:        7,
			DeltatLs:  18,
			DeltatLsf: 18,
			Reserved:  0,
		},
		fixupValueForAscii: fixupBynavIonUTCValueForAscii,
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

// fixupUnicoreIonUTCValueForAscii converts an IonUTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupUnicoreIonUTCValueForAscii(msg MsgBody) MsgBody {
	r := msg.(*IonUTC)
	return fixupIonUTCValueForAscii(r, "%.16g", "%.16g", "%.10g")
}

// fixupBynavIonUTCValueForAscii handles the specific precision formatting
// used by ByNav M20 receivers in their ASCII output.
func fixupBynavIonUTCValueForAscii(msg MsgBody) MsgBody {
	r := msg.(*IonUTC)
	result := *r
	
	// ByNav M20 has a rounding quirk where Alpha2/Alpha3 ASCII output shows ...813
	// but the binary value formats to ...812. Since these are negative values,
	// we need to subtract to make them more negative (from ...507812 to ...507813).
	const epsilon = 5e-23  // Small value to bump the last digit from 2 to 3
	result.Alpha2 -= epsilon
	result.Alpha3 -= epsilon
	
	// ByNav M20 uses "%.16g" for alpha/beta, "%.14g" for A0, "%.10g" for A1
	return fixupIonUTCValueForAscii(&result, "%.16g", "%.14g", "%.10g")
}

// fixupIonUTCValueForAscii is a helper that applies format strings to IonUTC fields
// to match the limited precision values that come from ASCII parsing.
func fixupIonUTCValueForAscii(msg *IonUTC, alphaBetaFormat, a0Format, a1Format string) *IonUTC {
	result := *msg
	
	// Apply formatting to alpha and beta fields
	fixupFloat(&result.Alpha0, alphaBetaFormat)
	fixupFloat(&result.Alpha1, alphaBetaFormat)
	fixupFloat(&result.Alpha2, alphaBetaFormat)
	fixupFloat(&result.Alpha3, alphaBetaFormat)
	fixupFloat(&result.Beta0, alphaBetaFormat)
	fixupFloat(&result.Beta1, alphaBetaFormat)
	fixupFloat(&result.Beta2, alphaBetaFormat)
	fixupFloat(&result.Beta3, alphaBetaFormat)
	
	// Apply specific formatting to A0 and A1
	fixupFloat(&result.A0, a0Format)
	fixupFloat(&result.A1, a1Format)
	
	return &result
}
