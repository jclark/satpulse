package asbin

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

const degToRad = math.Pi / 180

func TestCfgElev(t *testing.T) {
	// Captured from TAU1201: trkMask=1deg, naviMask=5deg
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d9060b080035fa8e3cc2b8b23d7be0",
		wantID: CfgElevID,
		wantMsg: &CfgElev{
			TrkMask:  float32(1 * degToRad),
			NaviMask: float32(5 * degToRad),
		},
	}})
}

func TestCfgNmeaVer(t *testing.T) {
	// Captured from TAU1201: NMEA V4.10
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d906430100034d30",
		wantID: CfgNmeaVerID,
		wantMsg: &CfgNmeaVer{
			Version: CfgNmeaVerV410,
		},
	}})
}

func TestCfgPps(t *testing.T) {
	// Example from spec: 1PPS with 500us pulse, positive polarity on GPIO13
	runTests(t, []testCase{{
		name:   "spec_example",
		packet: "F1D906070F00 40420F00 00000000 10270000 010D01 F386",
		wantID: CfgPpsID,
		wantMsg: &CfgPps{
			Period:    1000000, // 1s in µs
			Offset:    0,
			DutyCycle: 10000, // 10%
			Polarity:  CfgPpsPolarityRisingEdge,
			GPIO:      13,
			Sync:      CfgPpsSyncAlways,
		},
	}})
}

func TestCfgCfg(t *testing.T) {
	runTests(t, []testCase{
		{
			name:   "baudrate_nmea",
			packet: "F1D9060908000000000003000000 1A07",
			wantID: CfgCfgID,
			wantMsg: &CfgCfg{
				Action: CfgCfgActionSave,
				Mask:   CfgCfgMaskBaudrate | CfgCfgMaskNmeaMsgRate,
			},
		},
		{
			name:   "nav_settings",
			packet: "F1D9060908000000000004000000 1B0B",
			wantID: CfgCfgID,
			wantMsg: &CfgCfg{
				Action: CfgCfgActionSave,
				Mask:   CfgCfgMaskNavSettings,
			},
		},
		{
			name:   "factory_reset",
			packet: "F1D906090800 02000000 FFFFFFFF 1501",
			wantID: CfgCfgID,
			wantMsg: &CfgCfg{
				Action: CfgCfgActionClear,
				Mask:   CfgCfgMaskFactoryReset,
			},
		},
	})
}

func TestCfgPrt(t *testing.T) {
	runTests(t, []testCase{
		{
			name:   "uart1_9600",
			packet: "F1D9060008000100000080250000 B40F",
			wantID: CfgPrtID,
			wantMsg: &CfgPrt{
				PortID:   1,
				Res:      [3]byte{0, 0, 0},
				Baudrate: 9600,
			},
		},
		{
			name:   "uart0_115200",
			packet: "F1D90600080000000000 00C20100 D1E0",
			wantID: CfgPrtID,
			wantMsg: &CfgPrt{
				PortID:   0,
				Res:      [3]byte{0, 0, 0},
				Baudrate: 115200,
			},
		},
	})
}

func TestCfgMsg(t *testing.T) {
	runTests(t, []testCase{
		{
			name:   "gsv_rate_2",
			packet: "F1D9060103 00F00402 0019",
			wantID: CfgMsgID,
			wantMsg: &CfgMsg{
				CfgMsgFixed: CfgMsgFixed{MsgClass: 0xF0, MsgID: 0x04, Rate: 2}, // NmeaGsvID
			},
		},
		{
			name:   "gll_rate_5",
			packet: "F1D9060103 00F00105 0016",
			wantID: CfgMsgID,
			wantMsg: &CfgMsg{
				CfgMsgFixed: CfgMsgFixed{MsgClass: 0xF0, MsgID: 0x01, Rate: 5}, // NmeaGllID
			},
		},
		{
			name:   "vtg_disabled",
			packet: "F1D9060103 00F00600 001B",
			wantID: CfgMsgID,
			wantMsg: &CfgMsg{
				CfgMsgFixed: CfgMsgFixed{MsgClass: 0xF0, MsgID: 0x06, Rate: 0}, // NmeaVtgID
			},
		},
		{
			// 4-byte readback form: RMC at rate 5 with PortMask 0xFF, as a
			// 5Hz-native unit (TAU951M-P200, D10P) answers a CFG-MSG poll.
			name:   "rmc_rate_5_portmask",
			packet: "F1D9060104 00F00505FF 0427",
			wantID: CfgMsgID,
			wantMsg: &CfgMsg{
				CfgMsgFixed: CfgMsgFixed{MsgClass: 0xF0, MsgID: 0x05, Rate: 5},
				PortMask:    []byte{0xFF},
			},
		},
	})
}

// TestCfgMsgRoundTrip checks that a set serializes as the 3-byte form and
// that both the 3- and 4-byte readback forms survive a parse/serialize
// round trip.
func TestCfgMsgRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  *CfgMsg
		want string // expected serialized hex
	}{
		{
			name: "set_3byte",
			msg:  &CfgMsg{CfgMsgFixed: CfgMsgFixed{MsgClass: 0xF0, MsgID: 0x05, Rate: 1}},
			want: "f1d9060103 00f00501 001a",
		},
		{
			name: "readback_4byte",
			msg:  &CfgMsg{CfgMsgFixed: CfgMsgFixed{MsgClass: 0xF0, MsgID: 0x05, Rate: 5}, PortMask: []byte{0xFF}},
			want: "f1d9060104 00f00505ff 0427",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt, err := Serialize(tc.msg)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if got := string(pkt); got != string(hexToBytes(t, tc.want)) {
				t.Errorf("Serialize:\ngot  %X\nwant %s", pkt, strings.ReplaceAll(tc.want, " ", ""))
			}
			back, err := ParseMsg(string(pkt))
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			if !reflect.DeepEqual(back, tc.msg) {
				t.Errorf("round trip:\ngot  %+v\nwant %+v", back, tc.msg)
			}
		})
	}
}

// TestCfgMsgBadLen checks that a CFG-MSG payload that is neither 3 nor 4
// bytes is rejected.
func TestCfgMsgBadLen(t *testing.T) {
	// 5-byte payload (class,id,rate + two trailing bytes)
	_, err := ParseMsg(string(hexToBytes(t, "F1D9060105 00F00505FFFF 0431")))
	if err == nil {
		t.Error("ParseMsg accepted a 5-byte CFG-MSG payload")
	}
}

func TestCfgNavSat(t *testing.T) {
	// Example from spec: GPS L1, BEIDOU B1, GPS L5, BEIDOU B2A
	runTests(t, []testCase{{
		name:   "spec_example",
		packet: "F1D9060C0400 05820000 9D36",
		wantID: CfgNavSatID,
		wantMsg: &CfgNavSat{
			EnableMask: CfgNavSatMaskGPSL1 | CfgNavSatMaskBEIDOUB1 | CfgNavSatMaskGPSL5 | CfgNavSatMaskBEIDOUB2A,
		},
	}})
}

func TestCfgSurvey(t *testing.T) {
	// Example from spec: survey time 5s, accuracy 100mm
	runTests(t, []testCase{{
		name:   "spec_example",
		packet: "F1D9061208 0005000000 64000000 8916",
		wantID: CfgSurveyID,
		wantMsg: &CfgSurvey{
			MinDur:   5,
			AccLimit: 100,
		},
	}})
}

func TestCfgFixedECEF(t *testing.T) {
	// Position a TAU951M survey-in wrote into CFG-FIXEDECEF
	runTests(t, []testCase{{
		name:   "surveyed",
		packet: "F1D906140C00 02552DF9 681C4D24 A12EF708 6600",
		wantID: CfgFixedECEFID,
		wantMsg: &CfgFixedECEF{
			X: -114469630,
			Y: 609033320,
			Z: 150417057,
		},
	}})
}

func TestCfgFixedLLA(t *testing.T) {
	// Bangkok test-bench position, altitude -2.26 m
	runTests(t, []testCase{{
		name:   "bangkok",
		packet: "F1D906130C00 E04F2F08 8F2CFD3B 1EFFFFFF 995B",
		wantID: CfgFixedLLAID,
		wantMsg: &CfgFixedLLA{
			Lat: 137318368,  // 13.7318368 deg
			Lon: 1006447759, // 100.6447759 deg
			Alt: -226,
		},
	}})
}

func TestCfgSimpleRst(t *testing.T) {
	// Example from spec: warm start
	runTests(t, []testCase{{
		name:   "warm_start",
		packet: "F1D9064001 0002 4923",
		wantID: CfgSimpleRstID,
		wantMsg: &CfgSimpleRst{
			Mode: CfgSimpleRstModeWarmStart,
		},
	}})
}

func TestPollPrt(t *testing.T) {
	// Poll UART0 configuration
	expected := "F1D9060001000007 21"
	pollMsg := PollPrt(0)
	got := string(pollMsg)
	want := hexToBytes(t, expected)
	if got != string(want) {
		t.Errorf("PollPrt(0):\ngot:  %X\nwant: %X", pollMsg, want)
	}
	if PacketMsgId(pollMsg) != CfgPrtID {
		t.Errorf("message ID = %v, want %v", PacketMsgId(pollMsg), CfgPrtID)
	}
}

func TestPollCfgMsg(t *testing.T) {
	// Poll NMEA GGA message rate
	expected := "F1D9060102 00F000 F911"
	pollMsg := PollCfgMsg(NmeaGgaID)
	got := string(pollMsg)
	want := hexToBytes(t, expected)
	if got != string(want) {
		t.Errorf("PollCfgMsg(NmeaGgaID):\ngot:  %X\nwant: %X", pollMsg, want)
	}
	if PacketMsgId(pollMsg) != CfgMsgID {
		t.Errorf("message ID = %v, want %v", PacketMsgId(pollMsg), CfgMsgID)
	}
}
