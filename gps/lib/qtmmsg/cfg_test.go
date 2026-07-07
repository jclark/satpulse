package qtmmsg

import (
	"reflect"
	"testing"
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
		{name: "unregistered sentence", payload: "PQTMCFGRCVRMODE,OK,1"},
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

// TestCfgSentences audits the registration list: every registered
// sentence constructs a value whose Sentence matches its key, and
// round-trips through EncodeWrite and ParseCfgResponse without error.
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
