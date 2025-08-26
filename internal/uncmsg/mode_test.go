package uncmsg

import "testing"

func TestModeData(t *testing.T) {
	tests := []dataTestCase{
		{
			name: "MODE BASE TIME",
			// No binPacket since MODE is ASCII-only
			asciiPacket: "#MODE,97,GPS,FINE,2379,562467000,0,0,18,271;MODE BASE 1234 TIME 60 2.5 3.5,*53",
			msg: &Msg{
				Hdr: MsgHdr{
					CPUIdlePercent: 97,
					TimingHdr: TimingHdr{
						TimeRef:            TimeRefGPS,
						TimeStatus:         TimeStatusFine,
						Week:               2379,
						MillisecondsOfWeek: 562467000,
						Reserved:           0,
						Version:            0,
						LeapSec:            18,
						DelayMs:            271,
					},
				},
				Body: &Mode{
					Mode:        "MODE BASE 1234 TIME 60 2.5 3.5",
					HeadingMode: "", // Empty for single-antenna UM980
				},
			},
		},
	}

	// Only test ASCII since MODE is ASCII-only (no binary format)
	testDataAscii(t, tests)
}