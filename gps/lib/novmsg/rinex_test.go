package novmsg

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type rinexResult struct {
	sat, sig   string
	phaseShift float64
}

func TestRangeRINEX(t *testing.T) {
	// SinoGNSS numbers QZSS, BDS and NavIC differently, so each case has a
	// PRN for RangeRINEX and one for SinoRangeRINEX.
	tests := []struct {
		name       string
		sys        SatSystem
		prn        uint16
		sinoPRN    uint16
		sigType    uint8
		expect     rinexResult
		expectSino rinexResult
	}{
		{"GPS first", SatSystemGPS, 1, 1, 0, rinexResult{"G01", "1C", 0}, rinexResult{"G01", "1C", 0}},
		{"GPS last", SatSystemGPS, 32, 32, 0, rinexResult{"G32", "1C", 0}, rinexResult{"G32", "1C", 0}},
		{"GPS out of range", SatSystemGPS, 33, 33, 0, rinexResult{}, rinexResult{}},
		{"GPS L2P", SatSystemGPS, 7, 7, 5, rinexResult{"G07", "2P", 0}, rinexResult{"G07", "2P", 0}},
		{"GPS L2P(Y)", SatSystemGPS, 7, 7, 9, rinexResult{"G07", "2W", 0}, rinexResult{"G07", "2W", 0}},
		{"GPS L5(Q)", SatSystemGPS, 7, 7, 14, rinexResult{"G07", "5Q", 0}, rinexResult{"G07", "5Q", 0}},
		{"GPS SinoGNSS L5", SatSystemGPS, 7, 7, 2, rinexResult{}, rinexResult{"G07", "5Q", 0}},
		{"GPS L1C(P)", SatSystemGPS, 7, 7, 16, rinexResult{"G07", "1L", 0}, rinexResult{"G07", "1L", 0.25}},
		{"GPS L2C", SatSystemGPS, 7, 7, 17, rinexResult{"G07", "2S", 0}, rinexResult{"G07", "2X", 0}},
		{"GPS unknown signal", SatSystemGPS, 7, 7, 1, rinexResult{}, rinexResult{}},
		{"SBAS first", SatSystemSBAS, 120, 120, 0, rinexResult{"S20", "1C", 0}, rinexResult{"S20", "1C", 0}},
		{"SBAS last L5", SatSystemSBAS, 158, 158, 6, rinexResult{"S58", "5I", 0}, rinexResult{"S58", "5I", 0}},
		{"QZSS L1S", SatSystemSBAS, 183, 183, 0, rinexResult{}, rinexResult{}},
		{"GLONASS first", SatSystemGLONASS, 38, 38, 0, rinexResult{"R01", "1C", 0}, rinexResult{"R01", "1C", 0}},
		{"GLONASS last", SatSystemGLONASS, 61, 61, 1, rinexResult{"R24", "2C", 0}, rinexResult{"R24", "2C", 0}},
		{"GLONASS L2P", SatSystemGLONASS, 40, 40, 5, rinexResult{"R03", "2P", 0}, rinexResult{"R03", "2P", 0}},
		{"GLONASS L3", SatSystemGLONASS, 40, 40, 6, rinexResult{"R03", "3Q", 0}, rinexResult{"R03", "3Q", 0}},
		{"GLONASS unknown slot", SatSystemGLONASS, 37, 37, 0, rinexResult{}, rinexResult{}},
		{"Galileo E1(C)", SatSystemGalileo, 1, 1, 2, rinexResult{"E01", "1C", 0}, rinexResult{"E01", "1C", 0}},
		{"Galileo E6B", SatSystemGalileo, 36, 36, 6, rinexResult{"E36", "6B", 0}, rinexResult{"E36", "6B", 0}},
		{"Galileo E6C", SatSystemGalileo, 4, 4, 7, rinexResult{"E04", "6C", 0}, rinexResult{"E04", "6C", 0}},
		{"Galileo E5a(Q)", SatSystemGalileo, 4, 4, 12, rinexResult{"E04", "5Q", 0}, rinexResult{"E04", "5Q", 0}},
		{"Galileo E5b(Q)", SatSystemGalileo, 4, 4, 17, rinexResult{"E04", "7Q", 0}, rinexResult{"E04", "7Q", 0}},
		{"Galileo E5AltBOC(Q)", SatSystemGalileo, 4, 4, 20, rinexResult{"E04", "8Q", 0}, rinexResult{"E04", "8Q", 0}},
		{"Galileo type 1", SatSystemGalileo, 4, 4, 1, rinexResult{}, rinexResult{}},
		{"BDS first B1(I) D1", SatSystemBeiDou, 1, 141, 0, rinexResult{"C01", "2I", 0}, rinexResult{"C01", "2I", 0}},
		{"BDS last B2(I) D1", SatSystemBeiDou, 63, 203, 1, rinexResult{"C63", "7I", 0}, rinexResult{"C63", "7I", 0}},
		{"BDS B3(I) D1", SatSystemBeiDou, 5, 145, 2, rinexResult{"C05", "6I", 0}, rinexResult{"C05", "6I", 0}},
		{"BDS B1(I) D2", SatSystemBeiDou, 5, 145, 4, rinexResult{"C05", "2I", 0}, rinexResult{"C05", "2I", 0}},
		{"BDS B2(I) D2", SatSystemBeiDou, 5, 145, 5, rinexResult{"C05", "7I", 0}, rinexResult{"C05", "7I", 0}},
		{"BDS B3(I) D2", SatSystemBeiDou, 5, 145, 6, rinexResult{"C05", "6I", 0}, rinexResult{"C05", "6I", 0}},
		{"BDS B1C(P)", SatSystemBeiDou, 25, 165, 7, rinexResult{"C25", "1P", 0}, rinexResult{"C25", "1P", 0}},
		{"BDS B2a(P)", SatSystemBeiDou, 25, 165, 9, rinexResult{"C25", "5P", 0}, rinexResult{"C25", "5P", 0}},
		{"BDS B2b(I)", SatSystemBeiDou, 25, 165, 11, rinexResult{"C25", "7D", 0}, rinexResult{"C25", "7D", 0}},
		{"BDS SinoGNSS B1C", SatSystemBeiDou, 25, 165, 8, rinexResult{}, rinexResult{"C25", "1P", 0}},
		{"BDS SinoGNSS B2a", SatSystemBeiDou, 25, 165, 12, rinexResult{}, rinexResult{"C25", "5P", 0}},
		{"BDS SinoGNSS B2I", SatSystemBeiDou, 5, 145, 17, rinexResult{}, rinexResult{"C05", "7I", 0}},
		{"BDS SinoGNSS B2b", SatSystemBeiDou, 25, 165, 19, rinexResult{}, rinexResult{"C25", "7D", 0}},
		{"BDS SinoGNSS PRN with OEM7", SatSystemBeiDou, 141, 1, 0, rinexResult{}, rinexResult{}},
		{"QZSS first L1C/A", SatSystemQZSS, 193, 131, 0, rinexResult{"J01", "1C", 0}, rinexResult{"J01", "1C", 0}},
		{"QZSS last L5(Q)", SatSystemQZSS, 202, 140, 14, rinexResult{"J10", "5Q", 0}, rinexResult{"J10", "5Q", 0}},
		{"QZSS L1C(P)", SatSystemQZSS, 194, 132, 16, rinexResult{"J02", "1L", 0}, rinexResult{"J02", "1L", 0.25}},
		{"QZSS L2C", SatSystemQZSS, 194, 132, 17, rinexResult{"J02", "2S", 0}, rinexResult{"J02", "2X", 0}},
		{"QZSS L6P", SatSystemQZSS, 194, 132, 27, rinexResult{"J02", "6L", 0}, rinexResult{"J02", "6L", 0}},
		{"NavIC first", SatSystemNavIC, 1, 62, 0, rinexResult{"I01", "5A", 0}, rinexResult{"I01", "5A", 0}},
		{"NavIC last", SatSystemNavIC, 14, 70, 0, rinexResult{"I14", "5A", 0}, rinexResult{"I09", "5A", 0}},
		{"other", SatSystemOther, 1, 1, 0, rinexResult{}, rinexResult{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got, gotSino rinexResult
			got.sat, got.sig, got.phaseShift = RangeRINEX(tc.sys, tc.prn, tc.sigType)
			gotSino.sat, gotSino.sig, gotSino.phaseShift = SinoRangeRINEX(tc.sys, tc.sinoPRN, tc.sigType)
			if got != tc.expect {
				t.Errorf("RangeRINEX got  %+v\nwant %+v", got, tc.expect)
			}
			if gotSino != tc.expectSino {
				t.Errorf("SinoRangeRINEX got  %+v\nwant %+v", gotSino, tc.expectSino)
			}
		})
	}
}

// TestRINEXSinoCorpus checks that the SinoGNSS mapping maps every record
// of every binary RANGE in the SinoGNSS captures, so that a gap in the
// mapping for a signal the receivers output fails.
func TestRINEXSinoCorpus(t *testing.T) {
	files := []string{
		"K901/raw-obs.jsonl",
		"K901/raw-cross.jsonl",
		"K901/raw-obs-ascii.jsonl",
		"K803/raw-obs.jsonl",
		"K803/raw-cross.jsonl",
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			n := 0
			for _, m := range readRangeCapture(t, "../../testdata/packets/sinognss/"+name) {
				for _, o := range m.Obs {
					sys, sig := o.Status.SatSystem(), o.Status.SignalType()
					if sat, _, _ := SinoRangeRINEX(sys, o.PRN, sig); sat == "" {
						t.Errorf("unmapped record: system %d, PRN %d, signal type %d", sys, o.PRN, sig)
					}
					n++
				}
			}
			if n == 0 {
				t.Fatalf("no RANGE records")
			}
		})
	}
}

// readRangeCapture returns the binary RANGE logs in a packet log.
func readRangeCapture(t *testing.T, path string) []*Range {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var ms []*Range
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r struct {
			Msg string `json:"msg"`
			Bin string `json:"bin"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Msg != "RANGE" || r.Bin == "" {
			continue
		}
		b, err := hex.DecodeString(r.Bin)
		if err != nil {
			t.Fatal(err)
		}
		msg, err := ParseBinMsg(b)
		if err != nil {
			t.Fatal(err)
		}
		ms = append(ms, msg.Body.(*Range))
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ms
}
