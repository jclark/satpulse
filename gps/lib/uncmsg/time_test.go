package uncmsg

import (
	"testing"

	"github.com/jclark/satpulse/gps/lib/novmsg"
)

var timeTests = []dataTestCase{
	{
		name:        "PPSSTATUS with standard timing data",
		binPacket:   mustHexDecode("aa44b55d28233c0000a0480968e334200000000000121d000300000048090000" + "80df3420fcffffffa0b259fe2000e803150000000000000069666600000000" + "2bbcd2100100000000acecb02c000000000000000091a800a5"),
		asciiPacket: "#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 93,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2376,
					MillisecondsOfWeek: 540337000,
					Reserved:           0,
					Version:            0,
					LeapSec:            18,
					DelayMs:            29,
				},
			},
			Body: &PPSStatus{
				Status:       3,
				Week:         2376,
				MsSecInWeek:  540336000,
				PPSPulseErr:  -4,
				OffsetTime:   -27676000,
				ConfigInfo:   0x03E80020,
				Register:     0x00000015,
				TimeEstErr:   0,
				InnerQuality: 0x00666669,
				PPSInnerSta:  0x2B000000,
				PPSCfgSta:    0x0110D2BC,
				PPSStage:     0x00000000,
				Reserved1:    0x2CB0ECAC,
				Reserved2:    0,
				Reserved3:    0,
			},
		},
	},
	{
		name:        "RECTIME with timing and UTC data",
		binPacket:   mustHexDecode("aa44b56166002c0000a04b0988a70a16000000000012120000000000b39fb0a7e62a3fbfafeea64e93f94b3e00000000000032c0e9070000080e062a78e6000001000000b3998b61"),
		asciiPacket: "#RECTIMEA,97,GPS,FINE,2379,369797000,0,0,18,18;VALID,-4.755795596e-04,1.302682980e-08,-18.00000000000,2025,8,14,6,42,59000,VALID*9f81d9db\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2379,
					MillisecondsOfWeek: 369797000,
					Reserved:           0,
					Version:            0,
					LeapSec:            18,
					DelayMs:            18,
				},
			},
			Body: &RecTime{
				novmsg.Time{
					ClockStatus: novmsg.ClockStatusValid,
					Offset:      -0.00047557955957921587,
					OffsetStd:   1.3026829799647186e-08,
					UTCOffset:   -18.0,
					UTCYear:     2025,
					UTCMonth:    8,
					UTCDay:      14,
					UTCHour:     6,
					UTCMin:      42,
					UTCMs:       59000,
					UTCStatus:   novmsg.UTCStatusValid,
				},
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			// Create a copy and apply the fixup to the body
			newMsg := *msg
			newMsg.Body = fixupRecTimeValueForAscii(msg.Body.(*RecTime))
			return &newMsg
		},
	},
	{
		name:        "GPSUTC with leap second information",
		binPacket:   mustHexDecode("aa44b5611300300000a04b098065311600000000001214004b0900000000090000000000000010be000000000000e0bc89090000070000001200000012000000000000000000000054c1801f"),
		asciiPacket: "#GPSUTCA,97,GPS,FINE,2379,372336000,0,0,18,20;2379,589824,-9.313225746154785e-10,-1.776356839e-15,2441,7,18,18,0,0*8301ddef\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2379,
					MillisecondsOfWeek: 372336000,
					Reserved:           0,
					Version:            0,
					LeapSec:            18,
					DelayMs:            20,
				},
			},
			Body: &GPSUTC{
				UTCWn:     2379,
				Tot:       589824,
				A0:        -9.313225746154785e-10,
				A1:        -1.7763568394002505e-15,
				WnLsf:     2441,
				Dn:        7,
				DeltatLs:  18,
				DeltatLsf: 18,
				DeltatUTC: 0,
				Reserved:  0,
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			newMsg := *msg
			newMsg.Body = fixupGPSUTCValueForAscii(msg.Body.(*GPSUTC))
			return &newMsg
		},
	},
	{
		name:        "GALUTC with Galileo UTC parameters",
		binPacket:   mustHexDecode("aa44b5611400400000a04b09c04e3a160000000000121500000000000000103e000000000000000012000000600000004b0500008905000007000000120000000000000000001a3e000000000000c0bc004605000b000000d168e2d8"),
		asciiPacket: "#GALUTCA,97,GPS,FINE,2379,372920000,0,0,18,21;9.313225746154785e-10,0.000000000000000e+00,18,96,1355,1417,7,18,1.513399183750153e-09,-4.440892098500626e-16,345600,11*3729930d\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2379,
					MillisecondsOfWeek: 372920000,
					Reserved:           0,
					Version:            0,
					LeapSec:            18,
					DelayMs:            21,
				},
			},
			Body: &GALUTC{
				A0:        9.313225746154785e-10,
				A1:        0.0,
				DeltatLs:  18,
				Tot:       96,
				UTCWn:     1355,
				WnLsf:     1417,
				Dn:        7,
				DeltatLsf: 18,
				DA0g:      1.5133991837501526e-09,
				DA1g:      -4.440892098500626e-16,
				UlT0g:     345600,
				UlWN0g:    11,
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			newMsg := *msg
			newMsg.Body = fixupGALUTCValueForAscii(msg.Body.(*GALUTC))
			return &newMsg
		},
	},
	{
		name:        "BDSUTC with BDS UTC parameters",
		binPacket:   mustHexDecode("aa44b561dc07300000a04b09a0773b1600000000001214000000000000000000000000000000203e00000000000006bd3d0400000600000004000000040000000000000000000000d67c80c1"),
		asciiPacket: "#BDSUTCA,97,GPS,FINE,2379,372996000,0,0,18,20;0,0,1.862645149230957e-09,-9.769962617e-15,1085,6,4,4,0,0*2bd1f58c\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2379,
					MillisecondsOfWeek: 372996000,
					Reserved:           0,
					Version:            0,
					LeapSec:            18,
					DelayMs:            20,
				},
			},
			Body: &BDSUTC{
				Reserved1: 0,
				Reserved2: 0,
				A0:        1.862645149230957e-09,
				A1:        -9.769962616701378e-15,
				WnLsf:     1085,
				Dn:        6,
				DeltatLs:  4,
				DeltatLsf: 4,
				Reserved3: 0,
				Reserved4: 0,
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			newMsg := *msg
			newMsg.Body = fixupBDSUTCValueForAscii(msg.Body.(*BDSUTC))
			return &newMsg
		},
	},
	{
		name:        "BD3UTC with BDS-3 UTC parameters",
		binPacket:   mustHexDecode("aa44b5611600380000a04b09f04b4e160000000000121500ff030000500000000000000000801b3e00000000000007bd00000000000000003d000000060000000400000004000000000000000000000050994c1a"),
		asciiPacket: "#BD3UTCA,97,GPS,FINE,2379,374230000,0,0,18,21;1023,80,1.600710675120354e-09,-1.021405183e-14,0.000000000000000e+00,61,6,4,4,0,0*4b99b719\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2379,
					MillisecondsOfWeek: 374230000,
					Reserved:           0,
					Version:            0,
					LeapSec:            18,
					DelayMs:            21,
				},
			},
			Body: &BD3UTC{
				UTCWn:     1023,
				Tot:       80,
				A0:        1.6007106751203537e-09,
				A1:        -1.021405182655144e-14,
				A2:        0.0,
				WnLsf:     61,
				Dn:        6,
				DeltatLs:  4,
				DeltatLsf: 4,
				Reserved1: 0,
				Reserved2: 0,
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			newMsg := *msg
			newMsg.Body = fixupBD3UTCValueForAscii(msg.Body.(*BD3UTC))
			return &newMsg
		},
	},
}

func TestTime(t *testing.T) {
	t.Run("bin", func(t *testing.T) {
		testDataBin(t, timeTests)
	})
	t.Run("ascii", func(t *testing.T) {
		testDataAscii(t, timeTests)
	})
}

// fixupRecTimeValueForAscii converts a RecTime with full binary precision
// to match the limited precision values that come from ASCII parsing.
// This simulates the receiver firmware's float-to-ASCII conversion using %.10g formatting.
func fixupRecTimeValueForAscii(msg MsgBody) MsgBody {
	r := msg.(*RecTime)
	result := *r // Copy the struct

	const floatFormat = "%.10g"
	// Simulate receiver's ASCII formatting: binary -> printf("%.10g") -> parse back
	fixupFloat(&result.Offset, floatFormat)
	fixupFloat(&result.OffsetStd, floatFormat)
	// UTCOffset typically stays exact since -18.0 is representable exactly

	return &result
}

// fixupGPSUTCValueForAscii converts a GPSUTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupGPSUTCValueForAscii(msg MsgBody) MsgBody {
	g := msg.(*GPSUTC)
	result := *g // Copy the struct

	const floatFormat = "%.10g"

	// Only A1 needs fixup - A0 maintains full precision in ASCII
	fixupFloat(&result.A1, floatFormat)

	return &result
}

// fixupGALUTCValueForAscii converts a GALUTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupGALUTCValueForAscii(msg MsgBody) MsgBody {
	g := msg.(*GALUTC)
	result := *g // Copy the struct

	const floatFormat = "%.16g"
	fixupFloat(&result.DA0g, floatFormat)

	return &result
}

// fixupBDSUTCValueForAscii converts a BDSUTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupBDSUTCValueForAscii(msg MsgBody) MsgBody {
	b := msg.(*BDSUTC)
	result := *b // Copy the struct

	const floatFormat = "%.10g"
	// Fix up floating point precision differences - ASCII has limited precision
	fixupFloat(&result.A1, floatFormat)
	// A0 maintains precision in ASCII format

	return &result
}

// fixupBD3UTCValueForAscii converts a BD3UTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupBD3UTCValueForAscii(msg MsgBody) MsgBody {
	b := msg.(*BD3UTC)
	result := *b // Copy the struct

	// Different precision for different fields
	fixupFloat(&result.A0, "%.16g") // A0 matches with 16g
	fixupFloat(&result.A1, "%.10g") // A1 needs less precision
	// A2 maintains exact precision (0.0)

	return &result
}
