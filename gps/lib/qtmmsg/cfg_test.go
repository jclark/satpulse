package qtmmsg

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/opt"
)

// Response payloads are verbatim captures from an LG290P R02A01S.
func TestParseCfgResponse(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		expect  CfgMsg
	}{
		{
			name:    "CfgUART",
			payload: "PQTMCFGUART,OK,1,460800,8,0,1,0",
			expect:  &CfgUART{Index: 1, BaudRate: 460800, DataBit: 8, Parity: 0, StopBit: 1, FlowCtrl: 0},
		},
		{
			name:    "CfgPPS",
			payload: "PQTMCFGPPS,OK,1,1,100,1,1,0",
			expect:  &CfgPPS{Index: 1, Enable: 1, Duration: 100, Mode: 1, Polarity: 1, Reserved: 0},
		},
		{
			name:    "CfgPPS2",
			payload: "PQTMCFGPPS2,OK,1,1,100,1,1,0,1000,0,1,0,0,0",
			expect: &CfgPPS2{
				Index: 1, Enable: 1, Duration: 100, Mode: 1, Polarity: 1,
				Period: 1000, Userdelay: 0, Reserved2: 1,
			},
		},
		{
			name:    "CfgPPS2 userdelay",
			payload: "PQTMCFGPPS2,OK,1,1,200,1,1,0,1000,250,1,0,0,0",
			expect: &CfgPPS2{
				Index: 1, Enable: 1, Duration: 200, Mode: 1, Polarity: 1,
				Period: 1000, Userdelay: 250, Reserved2: 1,
			},
		},
		{
			name:    "CfgFixRate",
			payload: "PQTMCFGFIXRATE,OK,1000",
			expect:  &CfgFixRate{FixInterval: 1000},
		},
		{
			name:    "CfgEleThd",
			payload: "PQTMCFGELETHD,OK,5.0",
			expect:  &CfgEleThd{Ele: 5.0},
		},
		{
			name:    "CfgSignal",
			payload: "PQTMCFGSIGNAL,OK,07,03,0F,3F,0F,01",
			expect:  &CfgSignal{GPSSig: 0x07, GLOSig: 0x03, GALSig: 0x0F, BDSSig: 0x3F, QZSSig: 0x0F, NACSig: 0x01},
		},
		{
			name:    "CfgCnst",
			payload: "PQTMCFGCNST,OK,1,1,1,1,1,1",
			expect:  &CfgCnst{GPS: 1, GLONASS: 1, Galileo: 1, BDS: 1, QZSS: 1, NavIC: 1},
		},
		{
			name:    "CfgSvin disabled",
			payload: "PQTMCFGSVIN,OK,0,0,0.0,0.0000,0.0000,0.0000,0.0",
			expect:  &CfgSvin{},
		},
		{
			name:    "CfgSvin survey",
			payload: "PQTMCFGSVIN,OK,1,60,100.0,0.0000,0.0000,0.0000,0.0",
			expect:  &CfgSvin{Mode: 1, CfgCnt: 60, AccLimit3D: 100.0},
		},
		{
			name:    "CfgRcvrMode",
			payload: "PQTMCFGRCVRMODE,OK,2",
			expect:  &CfgRcvrMode{Mode: 2},
		},
		{
			name:    "CfgMsgRate no version",
			payload: "PQTMCFGMSGRATE,OK,GGA,1",
			expect:  &CfgMsgRate{MsgName: "GGA", Rate: 1},
		},
		{
			name:    "CfgMsgRate zero rate",
			payload: "PQTMCFGMSGRATE,OK,ZDA,0",
			expect:  &CfgMsgRate{MsgName: "ZDA", Rate: 0},
		},
		{
			name:    "CfgProt",
			payload: "PQTMCFGPROT,OK,1,1,00000007,00000006",
			expect:  &CfgProt{PortType: 1, PortID: 1, InputProt: 0x7, OutputProt: 0x6},
		},
		{
			name:    "CfgRtcm",
			payload: "PQTMCFGRTCM,OK,4,0,-90.0,07,06,1,0",
			expect:  &CfgRtcm{MSMType: 4, MSMElevThd: -90.0, Reserved0: "07", Reserved1: "06", EPHMode: 1},
		},
		{
			name:    "CfgMsgRate with version",
			payload: "PQTMCFGMSGRATE,OK,PQTMPVT,0,1",
			expect:  &CfgMsgRate{MsgName: "PQTMPVT", Rate: 0, MsgVer: opt.Make[uint8](1)},
		},
		{
			name:    "CfgRSID",
			payload: "PQTMCFGRSID,OK,1024",
			expect:  &CfgRSID{ID: 1024},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCfgResponse(tc.payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestParseCfgResponseNonCfg(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "set ack", payload: "PQTMCFGPPS,OK"},
		{name: "error", payload: "PQTMCFGPPS,ERROR,1"},
		{name: "unregistered sentence", payload: "PQTMCFGNAVMODE,OK,11"},
		{name: "not PQTM", payload: "GNGGA,034418.00,1343.91295,N"},
		{name: "verno data", payload: "PQTMVERNO,LG290P03AANR02A01S,2025/12/12,11:21:01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCfgResponse(tc.payload)
			if got != nil || err != nil {
				t.Errorf("got (%+v, %v), want (nil, nil)", got, err)
			}
		})
	}
}

// Expected W payloads marked "hw" were accepted by an LG290P R02A01S.
func TestEncodeWrite(t *testing.T) {
	tests := []struct {
		name   string
		msg    CfgMsg
		expect string
	}{
		{
			name:   "CfgUART current-port baud only", // hw
			msg:    &CfgUART{BaudRate: 115200},
			expect: "PQTMCFGUART,W,115200",
		},
		{
			name:   "CfgUART indexed full tuple",
			msg:    &CfgUART{Index: 1, BaudRate: 460800, DataBit: 8, Parity: 0, StopBit: 1, FlowCtrl: 0},
			expect: "PQTMCFGUART,W,1,460800,8,0,1,0",
		},
		{
			name:   "CfgPPS enable", // hw
			msg:    &CfgPPS{Index: 1, Enable: 1, Duration: 100, Mode: 1, Polarity: 1},
			expect: "PQTMCFGPPS,W,1,1,100,1,1,0",
		},
		{
			name:   "CfgPPS disable truncates", // hw
			msg:    &CfgPPS{Index: 1, Enable: 0, Duration: 100, Mode: 1, Polarity: 1},
			expect: "PQTMCFGPPS,W,1,0",
		},
		{
			name: "CfgPPS2 enable", // hw
			msg: &CfgPPS2{
				Index: 1, Enable: 1, Duration: 200, Mode: 1, Polarity: 1,
				Period: 1000, Userdelay: 250, Reserved2: 1,
			},
			expect: "PQTMCFGPPS2,W,1,1,200,1,1,0,1000,250,1,0,0,0",
		},
		{
			name:   "CfgPPS2 disable truncates",
			msg:    &CfgPPS2{Index: 1, Enable: 0, Duration: 100, Period: 1000},
			expect: "PQTMCFGPPS2,W,1,0",
		},
		{
			name:   "CfgFixRate", // hw
			msg:    &CfgFixRate{FixInterval: 200},
			expect: "PQTMCFGFIXRATE,W,200",
		},
		{
			name:   "CfgEleThd fixed decimal", // hw
			msg:    &CfgEleThd{Ele: 45},
			expect: "PQTMCFGELETHD,W,45.0",
		},
		{
			name:   "CfgSignal", // hw
			msg:    &CfgSignal{GPSSig: 0x07, GLOSig: 0x00, GALSig: 0x0F, BDSSig: 0x3F, QZSSig: 0x0F, NACSig: 0x01},
			expect: "PQTMCFGSIGNAL,W,07,00,0F,3F,0F,01",
		},
		{
			name:   "CfgCnst", // hw
			msg:    &CfgCnst{GPS: 1, GLONASS: 0, Galileo: 1, BDS: 1, QZSS: 1, NavIC: 1},
			expect: "PQTMCFGCNST,W,1,0,1,1,1,1",
		},
		{
			name:   "CfgSvin survey omits zero Distance",
			msg:    &CfgSvin{Mode: 1, CfgCnt: 60, AccLimit3D: 100.0},
			expect: "PQTMCFGSVIN,W,1,60,100.0,0.0000,0.0000,0.0000",
		},
		{
			name:   "CfgSvin fixed position",
			msg:    &CfgSvin{Mode: 2, ECEFX: -1144698.0455, ECEFY: 6090335.4099, ECEFZ: 1504171.3914},
			expect: "PQTMCFGSVIN,W,2,0,0.0,-1144698.0455,6090335.4099,1504171.3914",
		},
		{
			name:   "CfgRcvrMode", // hw
			msg:    &CfgRcvrMode{Mode: 2},
			expect: "PQTMCFGRCVRMODE,W,2",
		},
		{
			name:   "CfgMsgRate no version", // hw
			msg:    &CfgMsgRate{MsgName: "GLL", Rate: 0},
			expect: "PQTMCFGMSGRATE,W,GLL,0",
		},
		{
			name:   "CfgMsgRate with version", // hw
			msg:    &CfgMsgRate{MsgName: "PQTMSVINSTATUS", Rate: 1, MsgVer: opt.Make[uint8](1)},
			expect: "PQTMCFGMSGRATE,W,PQTMSVINSTATUS,1,1",
		},
		{
			name:   "CfgProt", // hw
			msg:    &CfgProt{PortType: 1, PortID: 1, InputProt: 0x7, OutputProt: 0x6},
			expect: "PQTMCFGPROT,W,1,1,00000007,00000006",
		},
		{
			name:   "CfgRSID",
			msg:    &CfgRSID{ID: 1024},
			expect: "PQTMCFGRSID,W,1024",
		},
		{
			name:   "CfgRtcm",
			msg:    &CfgRtcm{MSMType: 4, MSMElevThd: -90.0, Reserved0: "07", Reserved1: "06", EPHMode: 1},
			expect: "PQTMCFGRTCM,W,4,0,-90.0,07,06,1,0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EncodeWrite(tc.msg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Errorf("got  %q\nwant %q", got, tc.expect)
			}
		})
	}
}

// Payloads are verbatim R02A01S captures.
func TestParseVerno(t *testing.T) {
	got, err := ParseVerno("PQTMVERNO,LG290P03AANR02A01S,2025/12/12,11:21:01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expect := &Verno{VerStr: "LG290P03AANR02A01S", BuildDate: "2025/12/12", BuildTime: "11:21:01"}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %+v\nwant %+v", got, expect)
	}
	if got, err := ParseVerno("PQTMCFGPPS,OK,1,1,100,1,1,0"); got != nil || err != nil {
		t.Errorf("non-VERNO: got (%+v, %v), want (nil, nil)", got, err)
	}
	if got, err := ParseVerno("PQTMVERNO,ERROR,2"); got != nil || err != nil {
		t.Errorf("ERROR form: got (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestParseUniqID(t *testing.T) {
	got, err := ParseUniqID("PQTMUNIQID,OK,8,0000183B31B3C252")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expect := &UniqID{Length: 8, ID: "0000183B31B3C252"}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %+v\nwant %+v", got, expect)
	}
	if got, err := ParseUniqID("PQTMUNIQID,OK"); got != nil || err != nil {
		t.Errorf("bare OK: got (%+v, %v), want (nil, nil)", got, err)
	}
}

// Payloads are verbatim R02A01S captures from rover and base mode.
func TestParseLstMsg(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		expect    *LstMsgLine
		expectEnd bool
	}{
		{
			name:    "nmea entry",
			payload: "PQTMLSTMSG,OK,1,1,GGA,1",
			expect:  &LstMsgLine{PortType: 1, PortID: 1, MsgName: "GGA", Rate: 1},
		},
		{
			name:    "pqtm entry with version",
			payload: "PQTMLSTMSG,OK,1,1,PQTMTXT,1,1",
			expect:  &LstMsgLine{PortType: 1, PortID: 1, MsgName: "PQTMTXT", Rate: 1, MsgVer: opt.Make[uint8](1)},
		},
		{
			name:    "rtcm entry with offset",
			payload: "PQTMLSTMSG,OK,1,1,RTCM3-107X,1,0",
			expect:  &LstMsgLine{PortType: 1, PortID: 1, MsgName: "RTCM3-107X", Rate: 1, MsgVer: opt.Make[uint8](0)},
		},
		{
			name:      "end line",
			payload:   "PQTMLSTMSG,OK,End",
			expectEnd: true,
		},
		{
			name:    "not lstmsg",
			payload: "PQTMCFGPPS,OK,1,1,100,1,1,0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line, end, err := ParseLstMsg(tc.payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if end != tc.expectEnd {
				t.Errorf("end = %v, want %v", end, tc.expectEnd)
			}
			if !reflect.DeepEqual(line, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", line, tc.expect)
			}
		})
	}
}

// TestCfgSentences audits the registration list: every registered
// sentence constructs a value whose Sentence matches its key.
func TestCfgSentences(t *testing.T) {
	if len(cfgMap) == 0 {
		t.Fatal("no CFG messages registered")
	}
	for name, ctor := range cfgMap {
		m := ctor()
		if m.Sentence() != name {
			t.Errorf("%s: Sentence() = %q", name, m.Sentence())
		}
	}
}
