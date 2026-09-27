package novmsg

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestRINEXSatNum(t *testing.T) {
	tests := []struct {
		name       string
		sys        SatSystem
		prn        uint16
		expectOEM7 uint8
		expectSino uint8
	}{
		{"GPS first", SatSystemGPS, 1, 1, 1},
		{"GPS last", SatSystemGPS, 32, 32, 32},
		{"GPS out of range", SatSystemGPS, 33, 0, 0},
		{"SBAS first", SatSystemSBAS, 120, 20, 20},
		{"SBAS last", SatSystemSBAS, 158, 58, 58},
		{"QZSS L1S", SatSystemSBAS, 183, 0, 0},
		{"GLONASS first", SatSystemGLONASS, 38, 1, 1},
		{"GLONASS last", SatSystemGLONASS, 61, 24, 24},
		{"GLONASS unknown slot", SatSystemGLONASS, 37, 0, 0},
		{"Galileo first", SatSystemGalileo, 1, 1, 1},
		{"Galileo last", SatSystemGalileo, 36, 36, 36},
		{"QZSS OEM7 first", SatSystemQZSS, 193, 1, 0},
		{"QZSS OEM7 last", SatSystemQZSS, 202, 10, 0},
		{"QZSS SinoGNSS first", SatSystemQZSS, 131, 0, 1},
		{"QZSS SinoGNSS last", SatSystemQZSS, 140, 0, 10},
		{"BDS OEM7 first", SatSystemBeiDou, 1, 1, 0},
		{"BDS OEM7 last", SatSystemBeiDou, 63, 63, 0},
		{"BDS SinoGNSS first", SatSystemBeiDou, 141, 0, 1},
		{"BDS SinoGNSS last", SatSystemBeiDou, 203, 0, 63},
		{"NavIC OEM7 first", SatSystemNavIC, 1, 1, 0},
		{"NavIC OEM7 last", SatSystemNavIC, 14, 14, 0},
		{"NavIC SinoGNSS first", SatSystemNavIC, 62, 0, 1},
		{"NavIC SinoGNSS last", SatSystemNavIC, 70, 0, 9},
		{"other", SatSystemOther, 1, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := [2]uint8{RangeMappingOEM7.RINEXSatNum(tc.sys, tc.prn), RangeMappingSinoGNSS.RINEXSatNum(tc.sys, tc.prn)}
			if expect := [2]uint8{tc.expectOEM7, tc.expectSino}; got != expect {
				t.Errorf("got  %v\nwant %v", got, expect)
			}
		})
	}
}

func TestRINEXObsSig(t *testing.T) {
	tests := []struct {
		name       string
		sys        SatSystem
		sigType    uint8
		expectOEM7 string
		expectSino string
	}{
		{"GPS L1C/A", SatSystemGPS, 0, "1C", "1C"},
		{"GPS SinoGNSS L5", SatSystemGPS, 2, "", "5Q"},
		{"GPS L2P", SatSystemGPS, 5, "2P", "2P"},
		{"GPS L2P(Y)", SatSystemGPS, 9, "2W", "2W"},
		{"GPS L5(Q)", SatSystemGPS, 14, "5Q", "5Q"},
		{"GPS L1C(P)", SatSystemGPS, 16, "1L", "1L"},
		{"GPS L2C(M)", SatSystemGPS, 17, "2S", "2S"},
		{"GPS unknown", SatSystemGPS, 1, "", ""},
		{"GLONASS L1C/A", SatSystemGLONASS, 0, "1C", "1C"},
		{"GLONASS L2C/A", SatSystemGLONASS, 1, "2C", "2C"},
		{"GLONASS L2P", SatSystemGLONASS, 5, "2P", "2P"},
		{"GLONASS L3(Q)", SatSystemGLONASS, 6, "3Q", "3Q"},
		{"Galileo E1(C)", SatSystemGalileo, 2, "1C", "1C"},
		{"Galileo E6B", SatSystemGalileo, 6, "6B", "6B"},
		{"Galileo E6C", SatSystemGalileo, 7, "6C", "6C"},
		{"Galileo E5a(Q)", SatSystemGalileo, 12, "5Q", "5Q"},
		{"Galileo E5b(Q)", SatSystemGalileo, 17, "7Q", "7Q"},
		{"Galileo E5AltBOC(Q)", SatSystemGalileo, 20, "8Q", "8Q"},
		{"Galileo type 1", SatSystemGalileo, 1, "", ""},
		{"BDS B1(I) D1", SatSystemBeiDou, 0, "2I", "2I"},
		{"BDS B2(I) D1", SatSystemBeiDou, 1, "7I", "7I"},
		{"BDS B3(I) D1", SatSystemBeiDou, 2, "6I", "6I"},
		{"BDS B1(I) D2", SatSystemBeiDou, 4, "2I", "2I"},
		{"BDS B2(I) D2", SatSystemBeiDou, 5, "7I", "7I"},
		{"BDS B3(I) D2", SatSystemBeiDou, 6, "6I", "6I"},
		{"BDS B1C(P)", SatSystemBeiDou, 7, "1P", "1P"},
		{"BDS SinoGNSS B1C", SatSystemBeiDou, 8, "", "1P"},
		{"BDS B2a(P)", SatSystemBeiDou, 9, "5P", "5P"},
		{"BDS B2b(I)", SatSystemBeiDou, 11, "7D", "7D"},
		{"BDS SinoGNSS B2a", SatSystemBeiDou, 12, "", "5P"},
		{"BDS SinoGNSS B2I", SatSystemBeiDou, 17, "", "7I"},
		{"BDS SinoGNSS B2b", SatSystemBeiDou, 19, "", "7D"},
		{"QZSS L1C/A", SatSystemQZSS, 0, "1C", "1C"},
		{"QZSS L5(Q)", SatSystemQZSS, 14, "5Q", "5Q"},
		{"QZSS L1C(P)", SatSystemQZSS, 16, "1L", "1L"},
		{"QZSS L2C(M)", SatSystemQZSS, 17, "2S", "2S"},
		{"QZSS L6P", SatSystemQZSS, 27, "6L", "6L"},
		{"SBAS L1C/A", SatSystemSBAS, 0, "1C", "1C"},
		{"SBAS L5(I)", SatSystemSBAS, 6, "5I", "5I"},
		{"NavIC L5 SPS", SatSystemNavIC, 0, "5A", "5A"},
		{"other", SatSystemOther, 0, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := [2]string{RangeMappingOEM7.RINEXObsSig(tc.sys, tc.sigType), RangeMappingSinoGNSS.RINEXObsSig(tc.sys, tc.sigType)}
			if expect := [2]string{tc.expectOEM7, tc.expectSino}; got != expect {
				t.Errorf("got  %q\nwant %q", got, expect)
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
					if RINEXSys(sys) == "" || RangeMappingSinoGNSS.RINEXSatNum(sys, o.PRN) == 0 || RangeMappingSinoGNSS.RINEXObsSig(sys, sig) == "" {
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
