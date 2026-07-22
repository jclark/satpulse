package casbin

import (
	"reflect"
	"testing"
)

func TestCfgTMode2Parse(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want Msg
	}{
		{
			name: "survey-in response",
			hex:  "bace1c0006160100000000000000000000000000000000000000d0070000204e00000d560616",
			want: &CfgTMode2{
				TimFixMode:  CfgTMode2Survey,
				BandMode:    CfgTMode2BandL1B1I,
				AntDetMode:  0,
				TSrcMode:    0,
				XFixed:      0,
				YFixed:      0,
				ZFixed:      0,
				FixedPacc:   0,
				SvinMinDur:  2000,
				SvinPaccLim: 20000,
			},
		},
		{
			name: "default survey config",
			hex:  "bace1c0006160100000000000000000000000000000040420f002c010000983a0000217e1516",
			want: &CfgTMode2{
				TimFixMode:  CfgTMode2Survey,
				BandMode:    CfgTMode2BandL1B1I,
				AntDetMode:  0,
				TSrcMode:    0,
				XFixed:      0,
				YFixed:      0,
				ZFixed:      0,
				FixedPacc:   1000000,
				SvinMinDur:  300,
				SvinPaccLim: 15000,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := parseHex(t, tc.hex)
			msg, err := ParseMsg(pkt)
			if err != nil {
				t.Fatalf("ParseMsg error: %v", err)
			}
			if !reflect.DeepEqual(msg, tc.want) {
				t.Errorf("got  %+v\nwant %+v", msg, tc.want)
			}
		})
	}
}

func TestCfgTMode2Roundtrip(t *testing.T) {
	testMsgType(t, CfgTMode2{
		TimFixMode:  CfgTMode2Survey,
		SvinMinDur:  2000,
		SvinPaccLim: 20000,
	})
}

func TestCfgParse(t *testing.T) {
	// Packets captured from the attached receivers (ATGM332D-F8N
	// V6.3.2.0, AT632-6T-30 V6.3.0.0, ATGM332D-5N71 V5.3.0.0).
	tests := []struct {
		name string
		hex  string
		want Msg
	}{
		{
			name: "CFG-PRT F8N port 0",
			hex:  "bace080006000033030000c2010008f50a00",
			want: &CfgPrt{PortID: 0, ProtoMask: 0x33, Mode: 0x0003, BaudRate: 115200},
		},
		{
			name: "CFG-PRT 5N71 port 0 at 9600",
			hex:  "bace0800060000ffc008802500008824c708",
			want: &CfgPrt{PortID: 0, ProtoMask: 0xFF, Mode: 0x08C0, BaudRate: 9600},
		},
		{
			name: "CFG-RATE F8N 1Hz",
			hex:  "bace04000604e8030101ec030705",
			want: &CfgRate{FixIntervalMs: 1000, FixRateHz: 1, Res: 1},
		},
		{
			name: "CFG-TP 5N71",
			hex:  "bace1000060340420f00a08601000300010500000000f3c81708",
			want: &CfgTP{Interval: 1000000, Width: 100000, PPSOutMode: 3,
				Polarity: 0, TBase: 1, TSrcMode: 5, UserDelay: 0},
		},
		{
			name: "CFG-NAVBAND F8N",
			hex:  "bace0c00060f01000000a10c8800adcd28005bdab60f",
			want: &CfgNavBand{SigBandAuto: 1, SigIDMaskFix: 0x00880CA1, SigIDMask: 0x0028CDAD},
		},
		{
			name: "CFG-NAVLIMIT F8N",
			hex:  "bace0800060a045008051200f4011e500211",
			want: &CfgNavLimit{MinSVs: 4, MaxSVs: 80, MinCNO: 8, MinElev: 5, Res: 0x01F40012},
		},
		{
			name: "CFG-NAVX 5N71",
			hex: "bace2c000607000000000030000008a201000007f40900000000000000000000000000" +
				"00000000000000000000000000000034d9fb10",
			want: &CfgNavx{Mask: 0, DynModel: 0, FixMode: 0x30, MinSVs: 0, MaxSVs: 0,
				MinCNO: 8, Res1: 0xA2, IniFix3D: 1, MinElev: 0, DrLimit: 0,
				NavSystem: NavSysGPS | NavSysBDS | NavSysGLN, WnRollOver: 2548},
		},
		{
			name: "CFG-TMODE 5N71 auto mode",
			hex: "bace2800060600000000000000000000000000000000000000000000000000000000" +
				"000000002c0100000000000054010606",
			want: &CfgTMode{Mode: TModeAuto, SvinMinDur: 300},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := parseHex(t, tc.hex)
			msg, err := ParseMsg(pkt)
			if err != nil {
				t.Fatalf("ParseMsg error: %v", err)
			}
			if !reflect.DeepEqual(msg, tc.want) {
				t.Errorf("got  %+v\nwant %+v", msg, tc.want)
			}
		})
	}
}

func TestCfgRoundtrip(t *testing.T) {
	testMsgType(t, CfgMsg{Target: MakeMsgID(0x0A, 0x04), Rate: PollRate})
	testMsgType(t, CfgPrt{PortID: PortCurrent, ProtoMask: PrtProtoBinaryIn | PrtProtoBinaryOut, Mode: 0x08C0, BaudRate: 115200})
	testMsgType(t, CfgRate{FixIntervalMs: 200, FixRateHz: 5})
	testMsgType(t, CfgCfg{Mask: CfgSectionNav | CfgSectionTP, OpMode: CfgOpSave})
	testMsgType(t, CfgRst{NavBbrMask: BbrEphemeris | BbrPosition, ResetMode: ResetHWImmediate, StartMode: StartCold})
	testMsgType(t, CfgTP{Interval: 1000000, Width: 100000, PPSOutMode: 5, TSrcMode: 9, UserDelay: 2.5e-8})
	testMsgType(t, CfgNavx{Mask: NavxNavSystem | NavxMinElev, MinElev: 15, NavSystem: NavSysGPS})
	testMsgType(t, CfgTMode{Mode: TModeFixed, EcefX: -1144700.25, EcefY: 6090345.5, EcefZ: 1504171.125, PosVar: 9, SvinMinDur: 2000, SvinVarLimit: 400})
	testMsgType(t, CfgNavBand{SigBandAuto: 0, SigIDMaskFix: 0x00880CA1, SigIDMask: 0x0028CDAD})
	testMsgType(t, CfgNmea{NmeaVer: 2, LatLonReso: 7, HeightReso: 3, GsaPlus: 4})
	testMsgType(t, CfgNavLimit{MinSVs: 4, MaxSVs: 40, MinCNO: 8, MinElev: -5})
	testMsgType(t, CfgRtcm{MsgEnable: RtcmEn1005 | RtcmEnGPSMSM | RtcmEnBDSMSM, MsmVer: 7})
}

func TestPollPayloadLen(t *testing.T) {
	if n := PollPayloadLen(CfgMsgID); n != 4 {
		t.Errorf("PollPayloadLen(CFG-MSG) = %d, want 4 (class + id + poll rate)", n)
	}
	for _, mid := range []MsgID{CfgPrtID, CfgTPID, CfgNavBandID} {
		if n := PollPayloadLen(mid); n != 0 {
			t.Errorf("PollPayloadLen(%v) = %d, want 0", mid, n)
		}
	}
}
