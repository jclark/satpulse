package casbin

import (
	"reflect"
	"testing"
)

func TestTim2Parse(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want Msg
	}{
		{
			name: "TIM2-TPX",
			hex:  "bace180012004092d2001efbffff1f046f0001001a00a80704ef210000005f9971f0",
			want: &Tim2Tpx{
				TOW:      13800000,
				TOWSubms: -1250,
				Wn:       1055,
				PPSFlag:  Tim2PPSTOWValid | Tim2PPSWnValid | Tim2PPSLsValid | Tim2PPSTimeValid | 0x20 | Tim2PPSPulseValid,
				TBase:    Tim2TimeBaseGNSS,
				TSrc:     Tim2TSrcBDS,
				LeapSec:  4,
				QuanErr:  -17,
				TAcc:     33,
				Dt2Utc:   0,
			},
		},
		{
			name: "TIM2-TIMEGPS",
			hex:  "bace24001201f0c8d2003e81ffff6b0907037629ea56df8810410c3f93bf9cdf1d0112120f0001010000cd37a65d",
			want: &Tim2TimeGPS{Tim2TimeGNSS{
				TOW:      13814000,
				TOWSubms: -32450,
				Wn:       2411,
				TOWFlag:  Tim2PVT3D,
				WnFlag:   Tim2WnFromGNSSGood,
				TotSec:   1458186614,
				TAcc:     9.033416,
				Dt2Utc:   -1.1503615,
				DtLsfUtc: 18735004,
				Ls:       18,
				Lsf:      18,
				TFlag:    Tim2TimeFlagTOWValid | Tim2TimeFlagWnValid | Tim2TimeFlagLsValid | Tim2TimeFlagReliable,
				TSrc:     Tim2TSrcGPS,
				UtcFlag:  Tim2UtcUpdated,
				LsFlag:   Tim2LsEventNoEvent,
				LsYear:   0,
			}},
		},
		{
			name: "TIM2-TIMEBDS",
			hex:  "bace240012024092d200da52ffff1f040703684b0826e07b5540000000009cdf1d0104040f01010100004695756e",
			want: &Tim2TimeBDS{Tim2TimeGNSS{
				TOW:      13800000,
				TOWSubms: -44326,
				Wn:       1055,
				TOWFlag:  Tim2PVT3D,
				WnFlag:   Tim2WnFromGNSSGood,
				TotSec:   638077800,
				TAcc:     3.3356857,
				Dt2Utc:   0,
				DtLsfUtc: 18735004,
				Ls:       4,
				Lsf:      4,
				TFlag:    Tim2TimeFlagTOWValid | Tim2TimeFlagWnValid | Tim2TimeFlagLsValid | Tim2TimeFlagReliable,
				TSrc:     Tim2TSrcBDS,
				UtcFlag:  Tim2UtcUpdated,
				LsFlag:   Tim2LsEventNoEvent,
				LsYear:   0,
			}},
		},
		{
			name: "TIM2-TIMEGLN",
			hex:  "bace24001203a082d200da52ffff290604036494d9384e2c7a4400000000000000000000030200000000799c3e86",
			want: &Tim2TimeGLN{Tim2TimeGNSS{
				TOW:      13796000,
				TOWSubms: -44326,
				Wn:       1577,
				TOWFlag:  Tim2PVTDeadReckoning,
				WnFlag:   Tim2WnFromGNSSGood,
				TotSec:   953783396,
				TAcc:     1000.69226,
				Dt2Utc:   0,
				DtLsfUtc: 0,
				Ls:       0,
				Lsf:      0,
				TFlag:    Tim2TimeFlagTOWValid | Tim2TimeFlagWnValid,
				TSrc:     Tim2TSrcGLN,
				UtcFlag:  Tim2UtcNone,
				LsFlag:   Tim2LsEventNone,
				LsYear:   0,
			}},
		},
		{
			name: "TIM2-TIMEGAL",
			hex:  "bace24001204f0c8d200da52ffff6b050403762900324e2c7a44000000009cdf1d011212030303010000ce698382",
			want: &Tim2TimeGAL{Tim2TimeGNSS{
				TOW:      13814000,
				TOWSubms: -44326,
				Wn:       1387,
				TOWFlag:  Tim2PVTDeadReckoning,
				WnFlag:   Tim2WnFromGNSSGood,
				TotSec:   838871414,
				TAcc:     1000.69226,
				Dt2Utc:   0,
				DtLsfUtc: 18735004,
				Ls:       18,
				Lsf:      18,
				TFlag:    Tim2TimeFlagTOWValid | Tim2TimeFlagWnValid,
				TSrc:     Tim2TSrcGAL,
				UtcFlag:  Tim2UtcAuxiliary,
				LsFlag:   Tim2LsEventNoEvent,
				LsYear:   0,
			}},
		},
		{
			name: "TIM2-TIMEIRN",
			hex:  "bace24001205f0c8d200da52ffff6b050403762900324e2c7a44000000009cdf1d011212030403010000ce698384",
			want: &Tim2TimeIRN{Tim2TimeGNSS{
				TOW:      13814000,
				TOWSubms: -44326,
				Wn:       1387,
				TOWFlag:  Tim2PVTDeadReckoning,
				WnFlag:   Tim2WnFromGNSSGood,
				TotSec:   838871414,
				TAcc:     1000.69226,
				Dt2Utc:   0,
				DtLsfUtc: 18735004,
				Ls:       18,
				Lsf:      18,
				TFlag:    Tim2TimeFlagTOWValid | Tim2TimeFlagWnValid,
				TSrc:     Tim2TSrcIRN,
				UtcFlag:  Tim2UtcAuxiliary,
				LsFlag:   Tim2LsEventNoEvent,
				LsYear:   0,
			}},
		},
		{
			name: "TIM2-TIMEPOS",
			hex:  "bace40001206fba51dcf797731c12c7f2fe8953b57418aca6f29a8f3364100000000000000000000000000000000000000000000000000000000000000000f01010001000002b7978f2c",
			want: &Tim2TimePos{
				XTim:       -1.144697809046148e+06,
				YTim:       6.0903276278989725e+06,
				ZTim:       1.5041681618620479e+06,
				XFix:       0,
				YFix:       0,
				ZFix:       0,
				SurTimer:   0,
				SurPacc:    0,
				FixFlag:    Tim2PVTTimingFixed,
				TimFixMode: Tim2FixModeAutonomous,
				PRResiRms:  1,
				PosBias:    1,
				PDOP:       0,
			},
		},
		{
			name: "TIM2-LS",
			hex:  "bace10001207000000000000000000000001890712129907241a",
			want: &Tim2Ls{
				UtcExpPrnMask: 0,
				GNSSID:        GPS,
				SigID:         0,
				SVID:          0,
				RaimType:      Tim2RaimNoLsInfo,
				Wnlsf:         137,
				Dn:            7,
				Dtls:          18,
				Dtlsf:         18,
			},
		},
		{
			name: "TIM2-LY",
			hex:  "bace10001208000000000000000000000004000000001000120c",
			want: &Tim2Ly{
				WntExpPrnMask: 0,
				GNSSID:        GPS,
				SigID:         0,
				SVID:          0,
				RaimType:      Tim2RaimWnNormal,
				WnBias:        0,
			},
		},
		{
			name: "TIM2-TCXO",
			hex:  "bace1400120955a014b57109afb168fe15b5000000000700000049a8eb24",
			want: &Tim2Tcxo{
				DfuRatio:  -5.536761e-07,
				Dfu2Tcxo:  -5.0942437e-09,
				DfxRatio:  -5.5877035e-07,
				VcBias:    0,
				FrqFlag:   Tim2PVT3D,
				TcxoDoCnt: 0,
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
			if tc.want == nil {
				t.Logf("parsed %T: %+v", msg, msg)
				return
			}
			if !reflect.DeepEqual(msg, tc.want) {
				t.Errorf("got  %+v\nwant %+v", msg, tc.want)
			}
		})
	}
}
