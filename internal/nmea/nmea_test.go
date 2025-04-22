package nmea

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

func TestSplit(t *testing.T) {
	f := Split("$GPGLL,5057.970,N,00146.110,E,142451,A*27\r\n")
	df := f.Fields
	if f.TalkerID != "GP" || f.Format != "GLL" || !f.ChecksumOK() || len(df) != 6 ||
		df[0] != "5057.970" || df[1] != "N" || df[2] != "00146.110" ||
		df[3] != "E" || df[4] != "142451" || df[5] != "A" {
		t.Fatalf("NMEASplit failed")
	}
	f = Split("$GPTXT,1,Hello^21,3*FF\r\n")
	df = f.Fields
	if len(df) != 3 || df[1] != "Hello!" {
		t.Fatalf("NMEASplit failed on caret")
	}
}

func TestUnescape(t *testing.T) {
	if unescape("abc^0D^0Ade^A0f") != "abc\r\nde\u00A0f" {
		t.Fatalf("NMEAUnescape failed")
	}
}

func TestRMC(t *testing.T) {

	testTime(t, "$GPRMC,210230,A,3855.4487,N,09446.0071,W,0.0,076.2,130495,003.8,E*69\r\n", ptime.UTC(1995, 4, 13, 21, 2, 30, 0))
	// This is in the UBX docs, but checksum is 0x57 not 0x2D
	testTime(t, "$GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V*2D\r\n", ptime.UTC(2002, 12, 9, 8, 35, 59, 0))
	testTime(t, "$GPRMC,141632.00,A,5550.6150,N,03732.2523,E,000.00000,243.5,300518,,,A*56\r\n", ptime.UTC(2018, 5, 30, 14, 16, 32, 0))
	testTime(t, "$GNRMC,153632.00,A,5550.602949,N,03732.239610,E,000.00000,000.0,310518,,,R,V*28\r\n", ptime.UTC(2018, 5, 31, 15, 36, 32, 0))
}

func TestZDA(t *testing.T) {
	testTime(t, "$GPZDA,082710.00,16,09,2002,00,00*64\r\n", ptime.UTC(2002, 9, 16, 8, 27, 10, 0))
	testTime(t, "$GPZDA,234500,09,06,1995,-12,45*6C\r\n", ptime.UTC(1995, 6, 9, 23, 45, 0, 0))
}

func testTime(t *testing.T, s string, expectUTC ptime.UTCTime) {
	sen, e := Parse(s)
	if e != nil {
		t.Fatalf("nmea.Parse failed: %v: %s", e, s)
	}
	var h timeHandler
	var zt time.Time
	pp := NewPacketProcessor()
	handled, e := pp.Dispatch(sen, zt, &h)
	if e != nil || !handled {
		t.Fatalf("nmea.Dispatch failed: %v: %s", e, s)
	}
	utc := h.utc
	if utc == nil {
		t.Fatalf("nmea.UTCTime failed: %s", s)
	}
	if *utc != expectUTC {
		t.Fatalf("nmea.UTCTime wrong time: %s: got %v, want %v", s, utc, expectUTC)
	}
}

type timeHandler struct {
	gpsprot.DefaultHandler
	utc *ptime.UTCTime
}

func (h *timeHandler) Time(msg *gpsprot.TimeMsg, _ time.Time) {
	h.utc = msg.UTCTime
}

func TestScanTime(t *testing.T) {
	var hour, min, sec uint8
	var nanos int32
	f := func(s string, n int32) {
		if !scanTime("142451"+s, &hour, &min, &sec, &nanos) ||
			hour != 14 || min != 24 || sec != 51 || nanos != n {
			t.Fatalf("NMEAScanTime %s failed", s)
		}
	}
	f("", 0)
	f(".", 0)
	f(".0", 0)
	f(".00", 0)
	f(".0000", 0)
	f(".000000000", 0)
	f(".000000001", 1)
	f(".5", 5e8)

	b := func(s string) {
		if scanTime(s, &hour, &min, &sec, &nanos) {
			t.Fatalf("NMEAScanTime %s succeeded", s)
		}
	}
	b("14245")
	b("14245 ")
	b(" 142451")
	b("142451. ")
	b("14245X")
	b("142451.0000000001")
}

func TestGSVParse(t *testing.T) {
	tests := []struct {
		sen   string
		svs   []gpsprot.SVInfo
		sigID uint64
		final bool
	}{
		// From ublox docs (unfortunately with null fields)
		{
			sen:   "$GPGSV,3,1,09,09,,,17,10,,,40,12,,,49,13,,,35,1*6F\r\n",
			sigID: 1,
		},
		{
			sen:   "$GPGSV,3,2,09,15,,,44,17,,,45,19,,,44,24,,,50,1*64\r\n",
			sigID: 1,
		},
		{
			sen:   "$GPGSV,3,3,09,25,,,40,1*6E\r\n",
			sigID: 1,
			final: true,
		},
		{
			sen:   "$GPGSV,1,1,03,12,,,42,24,,,47,32,,,37,5*66\r\n",
			sigID: 5,
			final: true,
		},
		{
			sen:   "$GAGSV,1,1,00,2*76\r\n",
			sigID: 2,
			final: true,
		},
		// From Allystar docs
		{
			sen: "$GPGSV,3,2,12,53,38,212,46,50,35,139,42,41,32,226,42,28,25,173,44*77\r\n",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.SBAS, PRN: 53 + 87}, Elevation: 38, Azimuth: 212, CNO: 46},
				{SVID: gpsprot.SVID{GNSS: gpsprot.SBAS, PRN: 50 + 87}, Elevation: 35, Azimuth: 139, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.SBAS, PRN: 41 + 87}, Elevation: 32, Azimuth: 226, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 28}, Elevation: 25, Azimuth: 173, CNO: 44},
			},
		},
		{
			sen: "$GPGSV,3,3,12,2,22,264,42,12,21,318,43,23,17,93,42,9,12,126,37*43\r\n",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 2}, Elevation: 22, Azimuth: 264, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 12}, Elevation: 21, Azimuth: 318, CNO: 43},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 23}, Elevation: 17, Azimuth: 93, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 9}, Elevation: 12, Azimuth: 126, CNO: 37},
			},
			final: true,
		},
		{
			sen: "$BDGSV,3,1,12,216,79,57,44,237,67,249,44,220,53,301,44,870,53,301,44*57\r\n",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 216}, Elevation: 79, Azimuth: 57, CNO: 44},
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 237}, Elevation: 67, Azimuth: 249, CNO: 44},
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 220}, Elevation: 53, Azimuth: 301, CNO: 44},
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 870}, Elevation: 53, Azimuth: 301, CNO: 44},
			},
		},
		{
			sen: "$GLGSV,2,2,08,79,24,299,45,78,22,254,49,81,18,303,45,66,10,181,44*6F\r\n",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 79 - 64}, Elevation: 24, Azimuth: 299, CNO: 45},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 78 - 64}, Elevation: 22, Azimuth: 254, CNO: 49},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 81 - 64}, Elevation: 18, Azimuth: 303, CNO: 45},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 66 - 64}, Elevation: 10, Azimuth: 181, CNO: 44},
			},
			final: true,
		},
		{
			sen: "$GAGSV,2,1,05,12,69,355,46,19,42,115,42,24,30,246,45,11,27,290,40*60\r\n",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 12}, Elevation: 69, Azimuth: 355, CNO: 46},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 19}, Elevation: 42, Azimuth: 115, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 24}, Elevation: 30, Azimuth: 246, CNO: 45},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 11}, Elevation: 27, Azimuth: 290, CNO: 40},
			},
		},
		{
			sen: "$GIGSV,2,1,06,904,67,205,47,907,45,158,45,903,34,227,44,909,20,257,40*63\r\n",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 904}, Elevation: 67, Azimuth: 205, CNO: 47},
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 907}, Elevation: 45, Azimuth: 158, CNO: 45},
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 903}, Elevation: 34, Azimuth: 227, CNO: 44},
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 909}, Elevation: 20, Azimuth: 257, CNO: 40},
			},
		},
		{
			sen: "$GPGSV,3,2,11,19,32,147,42,41,32,226,42,12,27,254,43,25,19,296,39,1*66",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 19}, Elevation: 32, Azimuth: 147, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.SBAS, PRN: 41 + 87}, Elevation: 32, Azimuth: 226, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 12}, Elevation: 27, Azimuth: 254, CNO: 43},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 25}, Elevation: 19, Azimuth: 296, CNO: 39},
			},
			sigID: 1,
		},
		{
			sen: "$GPGSV,3,4,10,25,17,310,40,8*5C",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 25}, Elevation: 17, Azimuth: 310, CNO: 40},
			},
			sigID: 8,
		},
		{
			sen: "$BDGSV,4,4,16,10,18,213,35,1*4C",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 10}, Elevation: 18, Azimuth: 213, CNO: 35},
			},
			sigID: 1,
			final: true,
		},
		{
			sen: "$BDGSV,4,5,16,29,83,343,45,20,76,109,45,30,38,124,42,4*40",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 29}, Elevation: 83, Azimuth: 343, CNO: 45},
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 20}, Elevation: 76, Azimuth: 109, CNO: 45},
				{SVID: gpsprot.SVID{GNSS: gpsprot.BDS, PRN: 30}, Elevation: 38, Azimuth: 124, CNO: 42},
			},
			sigID: 4,
		},
		{
			sen: "$GLGSV,2,1,06,81,48,335,48,88,61,73,43,66,53,182,38,65,52,44,37,1*73",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 81 - 64}, Elevation: 48, Azimuth: 335, CNO: 48},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 88 - 64}, Elevation: 61, Azimuth: 73, CNO: 43},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 66 - 64}, Elevation: 53, Azimuth: 182, CNO: 38},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 65 - 64}, Elevation: 52, Azimuth: 44, CNO: 37},
			},
			sigID: 1,
		},
		{
			sen: "$GAGSV,2,1,06,15,78,354,48,8,33,201,42,13,28,311,41,5,31,47,27,6*40",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 15}, Elevation: 78, Azimuth: 354, CNO: 48},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 8}, Elevation: 33, Azimuth: 201, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 13}, Elevation: 28, Azimuth: 311, CNO: 41},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 5}, Elevation: 31, Azimuth: 47, CNO: 27},
			},
			sigID: 6,
		},
		{
			sen: "$GAGSV,2,2,06,15,78,354,46,13,28,311,41,2*75",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 15}, Elevation: 78, Azimuth: 354, CNO: 46},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GAL, PRN: 13}, Elevation: 28, Azimuth: 311, CNO: 41},
			},
			sigID: 2,
			final: true,
		},
		{
			sen: "$GIGSV,2,1,07,5,75,208,46,7,39,160,43,3,30,225,42,9,14,254,39,1*7D",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 5}, Elevation: 75, Azimuth: 208, CNO: 46},
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 7}, Elevation: 39, Azimuth: 160, CNO: 43},
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 3}, Elevation: 30, Azimuth: 225, CNO: 42},
				{SVID: gpsprot.SVID{GNSS: gpsprot.NAVIC, PRN: 9}, Elevation: 14, Azimuth: 254, CNO: 39},
			},
			sigID: 1,
		},
		// From Quectel docs
		{
			sen: "$GPGSV,2,1,05,10,77,300,36,12,40,082,31,23,58,153,35,25,46,137,33,1*67",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 10}, Elevation: 77, Azimuth: 300, CNO: 36},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 12}, Elevation: 40, Azimuth: 82, CNO: 31},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 23}, Elevation: 58, Azimuth: 153, CNO: 35},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 25}, Elevation: 46, Azimuth: 137, CNO: 33},
			},
			sigID: 1,
		},
		{
			sen: "$GPGSV,2,2,05,32,45,316,34,1*52",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 32}, Elevation: 45, Azimuth: 316, CNO: 34},
			},
			sigID: 1,
			final: true,
		},
		{
			sen: "$GPGSV,2,1,05,10,77,300,31,12,40,082,25,23,58,153,29,25,46,137,28,6*65",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 10}, Elevation: 77, Azimuth: 300, CNO: 31},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 12}, Elevation: 40, Azimuth: 82, CNO: 25},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 23}, Elevation: 58, Azimuth: 153, CNO: 29},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 25}, Elevation: 46, Azimuth: 137, CNO: 28},
			},
			sigID: 6,
		},
		{
			sen: "$GPGSV,2,2,05,32,45,316,25,6*55",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 32}, Elevation: 45, Azimuth: 316, CNO: 25},
			},
			sigID: 6,
			final: true,
		},
		{
			sen: "$GPGSV,1,1,04,10,77,300,32,23,58,153,30,25,46,137,30,32,45,316,26,8*61",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 10}, Elevation: 77, Azimuth: 300, CNO: 32},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 23}, Elevation: 58, Azimuth: 153, CNO: 30},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 25}, Elevation: 46, Azimuth: 137, CNO: 30},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 32}, Elevation: 45, Azimuth: 316, CNO: 26},
			},
			sigID: 8,
			final: true,
		},
		{
			sen: "$GLGSV,1,1,03,67,57,036,37,68,30,328,34,78,53,184,27,1*4B",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 67 - 64}, Elevation: 57, Azimuth: 36, CNO: 37},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 68 - 64}, Elevation: 30, Azimuth: 328, CNO: 34},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 78 - 64}, Elevation: 53, Azimuth: 184, CNO: 27},
			},
			sigID: 1,
			final: true,
		},
		{
			sen: "$GLGSV,1,1,03,67,57,036,31,68,30,328,27,78,53,184,31,3*4A",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 67 - 64}, Elevation: 57, Azimuth: 36, CNO: 31},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 68 - 64}, Elevation: 30, Azimuth: 328, CNO: 27},
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: 78 - 64}, Elevation: 53, Azimuth: 184, CNO: 31},
			},
			sigID: 3,
			final: true,
		},
		{
			sen: "$GLGSV,1,1,01,,65,190,31",
			svs: []gpsprot.SVInfo{
				{SVID: gpsprot.SVID{GNSS: gpsprot.GLO, PRN: gpsprot.GLOUnknown}, Elevation: 65, Azimuth: 190, CNO: 31},
			},
			final: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(Trim(test.sen), func(t *testing.T) {
			sen := Split(addTrailer(test.sen))
			if !sen.ChecksumOK() {
				t.Fatalf("invalid checksum failed: computed %2X, in message %2X", sen.ComputedChecksum, sen.ChecksumField)
			}
			svs, sigID, final, err := parseGSV(sen)
			if err != nil {
				t.Fatalf("unexpected GSV parsing error: %v", err)
			}

			if len(svs) != len(test.svs) {
				t.Fatalf("GSV SV count mismatch: got %d, want %d", len(svs), len(test.svs))
			}
			for i, sv := range svs {
				if sv.SVID != test.svs[i].SVID {
					t.Fatalf("GSV SVID mismatch at index %d: got %v, want %v", i, sv.SVID, test.svs[i].SVID)
				}
				if sv.Elevation != test.svs[i].Elevation {
					t.Fatalf("GSV Elevation mismatch at index %d: got %d, want %d", i, sv.Elevation, test.svs[i].Elevation)
				}
				if sv.Azimuth != test.svs[i].Azimuth {
					t.Fatalf("GSV Azimuth mismatch at index %d: got %d, want %d", i, sv.Azimuth, test.svs[i].Azimuth)
				}
				if sv.CNO != test.svs[i].CNO {
					t.Fatalf("GSV CNO mismatch at index %d: got %d, want %d", i, sv.CNO, test.svs[i].CNO)
				}
			}
			if final != test.final {
				t.Fatalf("GSV final flag mismatch: got %v, want %v", final, test.final)
			}
			if sigID != test.sigID {
				t.Fatalf("GSV sigID mismatch: got %d, want %d", sigID, test.sigID)
			}
		})
	}
}

func addTrailer(s string) string {
	if !strings.Contains(s, "*") {
		s += fmt.Sprintf("*%02X\r\n", Checksum(s[1:]))
	}
	return s
}

type gsvTestMsgHandler struct {
	gpsprot.DefaultHandler
	n int
}

func (h *gsvTestMsgHandler) Satellites(msg *gpsprot.SatellitesMsg, _ time.Time) {
	h.n = len(msg.Info)
}

func TestGSVCombine(t *testing.T) {
	sens := []string{
		"$GPGSV,1,1,04,10,77,300,32,23,58,153,30,25,46,137,30,32,45,316,26,8*61", // 4
		"$GLGSV,1,1,03,67,57,036,31,68,30,328,27,78,53,184,31,3*4A",              // 3
		"$GAGSV,2,1,06,15,78,354,48,8,33,201,42,13,28,311,41,5,31,47,27,6*40",    // 4
		"$GAGSV,2,2,06,15,78,354,46,13,28,311,41,2*75",                           // 2
		"$GPRMC,210230,A,3855.4487,N,09446.0071,W,0.0,076.2,130495,003.8,E*69\r\n",
	}
	tests := []struct {
		order []int
		expect []struct { i, count int }
	}{
		{
			[]int{1, 2, 3, -1, 0, 1, 2, 3, 0, 1, 2, 3},
			[]struct{ i, count int }{ {3, 9}, {7, 13}, {11, 13}, },
		},
		{
			[]int{3, 4, 0, 1, 2, 3, 4, 0, 1, 2, 3, 4},
			[]struct{ i, count int }{ {1, 2},{5, 13}, {10, 13}, },
		},
	}
	for ti, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%d", ti), func(t *testing.T) {
			order := test.order
			expect := test.expect
			gs := newGSVState()
			k := 0
			for i, j := range order {
				h := gsvTestMsgHandler{n: -1}
				if j >= 0 {
					sen := Split(sens[j])
					if !sen.ChecksumOK() {
						t.Fatalf("invalid checksum failed: computed %2X, in message %2X", sen.ComputedChecksum, sen.ChecksumField)
					}
					gs.process(sen, time.Time{}, &h)
				} else {
					gs.flush(&h)
				}
				if k < len(expect) && i == expect[k].i {
					if h.n != expect[k].count {
						t.Fatalf("GSV combine failed at %d: got %d, want %d", i, h.n, expect[k].count)
					}
					k++
				} else if h.n != -1 {
					t.Fatalf("GSV combine failed: got %d, want none", h.n)
				}
			}
		})
	}
}
