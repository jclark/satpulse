package sdbp

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

func TestSatsDatSAT(t *testing.T) {
	tests := []struct {
		name  string
		input *sdbpbin.DatSAT
		want  *gpsprot.SatellitesMsg
	}{
		{
			name: "mixed-constellations",
			input: &sdbpbin.DatSAT{
				DatSATFixed: sdbpbin.DatSATFixed{SatCount: 5},
				Sats: []sdbpbin.DatSATEntry{
					{GNSSID: sdbpbin.GPS, SatID: 15, SignalID: sdbpbin.SigGPSL1CA, CN0: 34, Elev: 53, Azim: 11, Flags: 0x00a7},
					{GNSSID: sdbpbin.BDS, SatID: 5, SignalID: sdbpbin.SigBDSB1I, CN0: 41, Elev: 40, Azim: 256, Flags: 0x00a7},
					{GNSSID: sdbpbin.GAL, SatID: 9, SignalID: sdbpbin.SigGALE1, CN0: 47, Elev: 70, Azim: 353, Flags: 0x00a5},
					{GNSSID: sdbpbin.BDS, SatID: 1, SignalID: 7, CN0: 0, Elev: 38, Azim: 102, Flags: 0x000a}, // unknown, skipped
					{GNSSID: sdbpbin.GLO, SatID: 4, OrbitID: 6, SignalID: sdbpbin.SigGLOG1, CN0: 47, Elev: 41, Azim: 333, Flags: 0x00a5},
				},
			},
			want: &gpsprot.SatellitesMsg{
				NativeMsgID:  "DAT-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 15}, LookAngles: opt.Make(gpsprot.LookAngles{Azimuth: 11, Elevation: 53}), Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 34, Used: true}}, Used: true},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 5}, LookAngles: opt.Make(gpsprot.LookAngles{Azimuth: 256, Elevation: 40}), Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 41, Used: true}}, Used: true},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 9}, LookAngles: opt.Make(gpsprot.LookAngles{Azimuth: 353, Elevation: 70}), Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1, CN0: 47, Used: true}}, Used: true},
					{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 4}, LookAngles: opt.Make(gpsprot.LookAngles{Azimuth: 333, Elevation: 41}), Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 47, Used: true}}, Used: true},
				},
			},
		},
		{
			name: "not-used",
			input: &sdbpbin.DatSAT{
				DatSATFixed: sdbpbin.DatSATFixed{SatCount: 1},
				Sats: []sdbpbin.DatSATEntry{
					{GNSSID: sdbpbin.GPS, SatID: 20, SignalID: sdbpbin.SigGPSL1CA, CN0: 18, Elev: 14, Azim: 35, Flags: 0x0027},
				},
			},
			want: &gpsprot.SatellitesMsg{
				NativeMsgID:  "DAT-SAT",
				UsedValidity: gpsprot.SatelliteUsedSignal,
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 20}, LookAngles: opt.Make(gpsprot.LookAngles{Azimuth: 35, Elevation: 14}), Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 18}}},
				},
			},
		},
		{
			name: "empty",
			input: &sdbpbin.DatSAT{
				DatSATFixed: sdbpbin.DatSATFixed{SatCount: 0},
			},
			want: nil,
		},
		{
			name: "all-unknown-signals",
			input: &sdbpbin.DatSAT{
				DatSATFixed: sdbpbin.DatSATFixed{SatCount: 2},
				Sats: []sdbpbin.DatSATEntry{
					{GNSSID: sdbpbin.BDS, SatID: 1, SignalID: 7, CN0: 0, Flags: 0x000a},
					{GNSSID: sdbpbin.GPS, SatID: 13, SignalID: 7, CN0: 0, Flags: 0x0005},
				},
			},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := satsDatSAT(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("satsDatSAT:\n  got  %+v\n  want %+v", got, tc.want)
			}
		})
	}
}

// TestSatsDatSATCaptured parses the real captured DAT-SAT packet through
// the full pipeline (parse -> convert) and checks the SV count.
func TestSatsDatSATCaptured(t *testing.T) {
	m := parsePacket(t, captureSATHex).(*sdbpbin.DatSAT)
	msg := satsDatSAT(m)
	if msg == nil {
		t.Fatal("expected non-nil SatellitesMsg")
	}
	// Count entries with known signals
	var wantSVs int
	for _, s := range m.Sats {
		if sigMap, ok := signalIDMap[s.GNSSID]; ok {
			if _, ok := sigMap[s.SignalID]; ok {
				wantSVs++
			}
		}
	}
	if len(msg.SVs) != wantSVs {
		t.Errorf("len(SVs) = %d, want %d", len(msg.SVs), wantSVs)
	}
}

const captureSATHex = "233e0630d504888e590d580001000700266600000000000a00000200070043ed000000000007000003000700478c00000000000a000004000700165f00000000000a000005000029280001a7000000a70000060000232a9e00d5000000a700000700002b28ba00b7ffffffa7000008000700220600000000000700000900002623a900b1ffffffa700000a0000282dca007cffffffa700000b00002b3e3101c8ffffffa700000c00002438a500d4010000a700000d000021246101ab010000a700000e00002a232201e4000000a70000100007002d910000000000070000150007001b1a00000000000700001800002510af00dcfbffffa700001800032610af003ffcffffa700001a0003161a7f001a010000a700001f000700480801000000000a00002100001e120c01e3fdffffa7000021000320120c01f4030000a700002200002849de00ec000000a700002200032749de00e2000000a700002600031a23190044010000a7000027000700338900000000000700002800002c21b00031ffffffa700002800032d21b00010ffffffa700002a0000332c47012dfdffffa700002a00032f2c47011cfdffffa700002b00002a21390100010000a700002b000321213901c2000000a700002c00002a1c9c0050feffffa700002c00032a1c9c0042feffffa700002d000700084900000000000a000031000018094101000000004a000032000028290200000000004a000032000325290200000000004a00003800002b300400000000004a00003b0007002b6600000000000a00003c00002e3ff300d7000000a700003d000700488b00000000000a00003e000700156200000000000a00010500001a0d48001a0d0000a700010a00001d092a01cb040000a700010a00041d092a011e040000a700010c0000140595009aadc8002700010d000700082300000000000500010f000020211800cd040000a7000112000024365901c3fdffffa700011200042d36590116fdffffa70001140000120e230098080000270001170000271c490133fdffffa50001170004231c4901e1fbffffa50001180000113a6900020b00002500011900001508ba00b8c82cff2700011d00002c2acc0094000000a700012000002911ed00ecfdffffa700012000042511ed0031feffffa700020300070033690000000000050002040000162f2e00000000002a00020400041d2f2e00000000002a000207000014387400000000002a00020800001346d900000000002a00020800042946d900000000002a0004040000101720000000000025000404000119172000c10d0000250004050000292ded0030feffffa50004050001292ded00c7fdffffa500040600001f2f1700cafcffffa50004060001202f1700e1fbffffa500040900002f46610119ffffffa500040900012d466101fbfeffffa500040a00001a2d6a0011030000a500040a00011c2d6a007e050000a500040b0000193f4000ed020000a500041000001308ad00000000002500041200001ea50000000000004000041900070007da000000000005000424000029a5000000000000400005030500150f15000f0300002500050406002f294d010e010000a50005050100271f0d019a000000a500050dfe0700004200000000000a00051104002a1a9900c3010000a5000512fd001b4c9100c0000000a500051303002f2d500106000000a5000500000017000000000000000000c417"
