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
