package novmsg

import "testing"

var byCheckTests = []dataTestCase[Port]{
	{
		name:  "Bynav M10 BYCHECK",
		ascii: "#BYCHECKA,COM1,0,99.9,FINESTEERING,2437,429495.000,00000000,0000,782;7843,2437,429495.000,1,1,1,1,1,1,1,1,1,1,1,1*8c0596c3\r\n",
		hex:   "aa44121c20a500203c000000c7b48509d89299190000000000000e03a31e000085090000e0b6d148010000000100000001000000010000000100000001000000010000000100000001000000010000000100000001000000689c42f5",
		hdr: MsgHdr[Port]{
			Port: COM1,
			CommonHdr: CommonHdr{
				IdleTime:           Percentage(199),
				TimeStatus:         TimeStatusFineSteering,
				Week:               2437,
				MillisecondsOfWeek: GPSec(429495000),
				Version:            782,
			},
		},
		value: &ByCheck{
			Runtime:             7843,
			Week:                2437,
			Sow:                 429495,
			DualFrequency:       ByCheckTrue,
			AntennaVoltage:      ByCheckTrue,
			GloFrequencyDiff:    ByCheckTrue,
			WorkFrequency:       ByCheckTrue,
			BaseStationPosition: ByCheckTrue,
			BaseAntennaBlock:    ByCheckTrue,
			DiffLink:            ByCheckTrue,
			DualBase:            ByCheckTrue,
			BoardTemperature:    ByCheckTrue,
			RoverAntennaBlock:   ByCheckTrue,
			DifferentialData:    ByCheckTrue,
			BasePositionError:   ByCheckTrue,
		},
		// The M10 gives the idle time as 99.9 in ASCII but 199 (99.5) in binary.
		fixupHeaderForAscii: func(hdr MsgHdr[Port]) MsgHdr[Port] {
			hdr.IdleTime = 200
			return hdr
		},
	},
}

func TestByCheckBinary(t *testing.T) {
	testDataBin(t, byCheckTests, map[MsgID]func() MsgBody{ByCheckID: func() MsgBody { return &ByCheck{} }})
}

func TestByCheckAscii(t *testing.T) {
	testDataAscii[Port, AsciiHdr](t, byCheckTests, map[string]func() MsgBody{"BYCHECKA": func() MsgBody { return &ByCheck{} }})
}
