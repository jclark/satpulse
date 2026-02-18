package uncmsg

import (
	"testing"
)

var staDOPTests = []dataTestCase{
	{
		name:        "STADOP with 28 PRNs",
		binPacket:   mustHexDecode("aa44b561ba03620000a06609881a84108c44000000121600a016841029f602406d20db3f867b8f3fe5c9c53f86a33c3f526e033ff850073f0000a040000000001c00010007000900110013001e0027003d004e0050005300560069006b00a200a300a500a600a700a900aa00b000bb00be00c700c800db00dc00fba1fa58"),
		asciiPacket: "#STADOPA,97,GPS,FINE,2406,277093000,17548,0,18,22;277092000,2.0463,1.7119,1.1210,1.5452,0.7369,0.5134,0.5286,5.0,0.0,28,1,7,9,17,19,30,39,61,78,80,83,86,105,107,162,163,165,166,167,169,170,176,187,190,199,200,219,220*78d1bb8a\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2406,
					MillisecondsOfWeek: 277093000,
					Reserved:           17548,
					Version:            0,
					LeapSec:            18,
					DelayMs:            22,
				},
			},
			Body: &StaDOP{
				StaDOPFixed: StaDOPFixed{
					Reserved:  277092000,
					GDOP:      2.0462744,
					PDOP:      1.711927,
					TDOP:      1.1209571,
					VDOP:      1.5452238,
					HDOP:      0.73687017,
					NDOP:      0.5134021,
					EDOP:      0.52857924,
					Cutoff:    5.0,
					Reserved2: 0.0,
					PRNCount:  28,
				},
				PRNs: []uint16{
					1, 7, 9, 17, 19, 30, 39, 61,
					78, 80, 83, 86, 105, 107,
					162, 163, 165, 166, 167, 169, 170, 176,
					187, 190, 199, 200, 219, 220,
				},
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			return fixupStaDOPForAscii(msg)
		},
	},
}

func fixupStaDOPForAscii(msg *Msg) *Msg {
	newMsg := *msg
	m := msg.Body.(*StaDOP)
	r := *m
	r.StaDOPFixed = m.StaDOPFixed
	fixupFloat32(&r.GDOP, "%.4f")
	fixupFloat32(&r.PDOP, "%.4f")
	fixupFloat32(&r.TDOP, "%.4f")
	fixupFloat32(&r.VDOP, "%.4f")
	fixupFloat32(&r.HDOP, "%.4f")
	fixupFloat32(&r.NDOP, "%.4f")
	fixupFloat32(&r.EDOP, "%.4f")
	fixupFloat32(&r.Cutoff, "%.1f")
	fixupFloat32(&r.Reserved2, "%.1f")
	newMsg.Body = &r
	return &newMsg
}

func TestStaDOP(t *testing.T) {
	t.Run("bin", func(t *testing.T) {
		testDataBin(t, staDOPTests)
	})
	t.Run("ascii", func(t *testing.T) {
		testDataAscii(t, staDOPTests)
	})
}
