package nmea

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/as"
	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestGGAParse(t *testing.T) {
	tests := []struct {
		sen   string
		numSV int
	}{
		{
			sen:   "$GNGGA,071113.000,3957.7995312,N,11619.0286230,E,4,16,0.99,103.965,M,-8.408,M,1.0,4042*40",
			numSV: 16,
		},
		{
			sen:   "$GNGGA,025159.000,3149.29993210,N,11706.91264104,E,1,16,1.26,97.250,M,-4.945,M,,*5A",
			numSV: 16,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(Trim(test.sen), func(t *testing.T) {
			sen := Parse(addTrailer(test.sen))
			gga, err := parseGGA(sen)
			if err != nil {
				t.Fatalf("unexpected GGA parsing error: %v", err)
			}
			if gga.numSV != test.numSV {
				t.Fatalf("GGA SV count mismatch: got %d, want %d", gga.numSV, test.numSV)
			}
		})
	}
}

func TestGSAParse(t *testing.T) {
	tests := []struct {
		sen   string
		svids []gpsprot.SVID
	}{
		{
			sen: "$GNGSA,A,3,10,12,23,25,32,,,,,,,,2.38,1.26,2.01,1*0B",
			svids: []gpsprot.SVID{
				{GNSS: gpsprot.GPS, PRN: 10},
				{GNSS: gpsprot.GPS, PRN: 12},
				{GNSS: gpsprot.GPS, PRN: 23},
				{GNSS: gpsprot.GPS, PRN: 25},
				{GNSS: gpsprot.GPS, PRN: 32},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(Trim(test.sen), func(t *testing.T) {
			sen := Parse(addTrailer(test.sen))
			gsa, err := parseGSA(sen)
			if err != nil {
				t.Fatalf("unexpected GGA parsing error: %v", err)
			}
			if len(gsa.svids) != len(test.svids) {
				t.Fatalf("unexpected number of SVIDs, expected %d, got %d", len(test.svids), len(gsa.svids))
			}
		})
	}
}

func TestGSVParse(t *testing.T) {
	tests := []struct {
		sen   string
		svs   []svInfo
		sigID int
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
			svs: []svInfo{
				{id: 53, elev: 38, azim: 212, cn0: 46},
				{id: 50, elev: 35, azim: 139, cn0: 42},
				{id: 41, elev: 32, azim: 226, cn0: 42},
				{id: 28, elev: 25, azim: 173, cn0: 44},
			},
		},
		{
			sen: "$GPGSV,3,3,12,2,22,264,42,12,21,318,43,23,17,93,42,9,12,126,37*43\r\n",
			svs: []svInfo{
				{id: 2, elev: 22, azim: 264, cn0: 42},
				{id: 12, elev: 21, azim: 318, cn0: 43},
				{id: 23, elev: 17, azim: 93, cn0: 42},
				{id: 9, elev: 12, azim: 126, cn0: 37},
			},
			final: true,
		},
		{
			sen: "$BDGSV,3,1,12,216,79,57,44,237,67,249,44,220,53,301,44,870,53,301,44*57\r\n",
			svs: []svInfo{
				{id: 216, elev: 79, azim: 57, cn0: 44},
				{id: 237, elev: 67, azim: 249, cn0: 44},
				{id: 220, elev: 53, azim: 301, cn0: 44},
				{id: 870, elev: 53, azim: 301, cn0: 44},
			},
		},
		{
			sen: "$GLGSV,2,2,08,79,24,299,45,78,22,254,49,81,18,303,45,66,10,181,44*6F\r\n",
			svs: []svInfo{
				{id: 79, elev: 24, azim: 299, cn0: 45},
				{id: 78, elev: 22, azim: 254, cn0: 49},
				{id: 81, elev: 18, azim: 303, cn0: 45},
				{id: 66, elev: 10, azim: 181, cn0: 44},
			},
			final: true,
		},
		{
			sen: "$GAGSV,2,1,05,12,69,355,46,19,42,115,42,24,30,246,45,11,27,290,40*60\r\n",
			svs: []svInfo{
				{id: 12, elev: 69, azim: 355, cn0: 46},
				{id: 19, elev: 42, azim: 115, cn0: 42},
				{id: 24, elev: 30, azim: 246, cn0: 45},
				{id: 11, elev: 27, azim: 290, cn0: 40},
			},
		},
		{
			sen: "$GIGSV,2,1,06,904,67,205,47,907,45,158,45,903,34,227,44,909,20,257,40*63\r\n",
			svs: []svInfo{
				{id: 904, elev: 67, azim: 205, cn0: 47},
				{id: 907, elev: 45, azim: 158, cn0: 45},
				{id: 903, elev: 34, azim: 227, cn0: 44},
				{id: 909, elev: 20, azim: 257, cn0: 40},
			},
		},
		{
			sen: "$GPGSV,3,2,11,19,32,147,42,41,32,226,42,12,27,254,43,25,19,296,39,1*66",
			svs: []svInfo{
				{id: 19, elev: 32, azim: 147, cn0: 42},
				{id: 41, elev: 32, azim: 226, cn0: 42},
				{id: 12, elev: 27, azim: 254, cn0: 43},
				{id: 25, elev: 19, azim: 296, cn0: 39},
			},
			sigID: 1,
		},
		{
			sen: "$GPGSV,3,4,10,25,17,310,40,8*5C",
			svs: []svInfo{
				{id: 25, elev: 17, azim: 310, cn0: 40},
			},
			sigID: 8,
		},
		{
			sen: "$BDGSV,4,4,16,10,18,213,35,1*4C",
			svs: []svInfo{
				{id: 10, elev: 18, azim: 213, cn0: 35},
			},
			sigID: 1,
			final: true,
		},
		{
			sen: "$BDGSV,4,5,16,29,83,343,45,20,76,109,45,30,38,124,42,4*40",
			svs: []svInfo{
				{id: 29, elev: 83, azim: 343, cn0: 45},
				{id: 20, elev: 76, azim: 109, cn0: 45},
				{id: 30, elev: 38, azim: 124, cn0: 42},
			},
			sigID: 4,
		},
		{
			sen: "$GLGSV,2,1,06,81,48,335,48,88,61,73,43,66,53,182,38,65,52,44,37,1*73",
			svs: []svInfo{
				{id: 81, elev: 48, azim: 335, cn0: 48},
				{id: 88, elev: 61, azim: 73, cn0: 43},
				{id: 66, elev: 53, azim: 182, cn0: 38},
				{id: 65, elev: 52, azim: 44, cn0: 37},
			},
			sigID: 1,
		},
		{
			sen: "$GAGSV,2,1,06,15,78,354,48,8,33,201,42,13,28,311,41,5,31,47,27,6*40",
			svs: []svInfo{
				{id: 15, elev: 78, azim: 354, cn0: 48},
				{id: 8, elev: 33, azim: 201, cn0: 42},
				{id: 13, elev: 28, azim: 311, cn0: 41},
				{id: 5, elev: 31, azim: 47, cn0: 27},
			},
			sigID: 6,
		},
		{
			sen: "$GAGSV,2,2,06,15,78,354,46,13,28,311,41,2*75",
			svs: []svInfo{
				{id: 15, elev: 78, azim: 354, cn0: 46},
				{id: 13, elev: 28, azim: 311, cn0: 41},
			},
			sigID: 2,
			final: true,
		},
		{
			sen: "$GIGSV,2,1,07,5,75,208,46,7,39,160,43,3,30,225,42,9,14,254,39,1*7D",
			svs: []svInfo{
				{id: 5, elev: 75, azim: 208, cn0: 46},
				{id: 7, elev: 39, azim: 160, cn0: 43},
				{id: 3, elev: 30, azim: 225, cn0: 42},
				{id: 9, elev: 14, azim: 254, cn0: 39},
			},
			sigID: 1,
		},
		// From Quectel docs
		{
			sen: "$GPGSV,2,1,05,10,77,300,36,12,40,082,31,23,58,153,35,25,46,137,33,1*67",
			svs: []svInfo{
				{id: 10, elev: 77, azim: 300, cn0: 36},
				{id: 12, elev: 40, azim: 82, cn0: 31},
				{id: 23, elev: 58, azim: 153, cn0: 35},
				{id: 25, elev: 46, azim: 137, cn0: 33},
			},
			sigID: 1,
		},
		{
			sen: "$GPGSV,2,2,05,32,45,316,34,1*52",
			svs: []svInfo{
				{id: 32, elev: 45, azim: 316, cn0: 34},
			},
			sigID: 1,
			final: true,
		},
		{
			sen: "$GPGSV,2,1,05,10,77,300,31,12,40,082,25,23,58,153,29,25,46,137,28,6*65",
			svs: []svInfo{
				{id: 10, elev: 77, azim: 300, cn0: 31},
				{id: 12, elev: 40, azim: 82, cn0: 25},
				{id: 23, elev: 58, azim: 153, cn0: 29},
				{id: 25, elev: 46, azim: 137, cn0: 28},
			},
			sigID: 6,
		},
		{
			sen: "$GPGSV,2,2,05,32,45,316,25,6*55",
			svs: []svInfo{
				{id: 32, elev: 45, azim: 316, cn0: 25},
			},
			sigID: 6,
			final: true,
		},
		{
			sen: "$GPGSV,1,1,04,10,77,300,32,23,58,153,30,25,46,137,30,32,45,316,26,8*61",
			svs: []svInfo{
				{id: 10, elev: 77, azim: 300, cn0: 32},
				{id: 23, elev: 58, azim: 153, cn0: 30},
				{id: 25, elev: 46, azim: 137, cn0: 30},
				{id: 32, elev: 45, azim: 316, cn0: 26},
			},
			sigID: 8,
			final: true,
		},
		{
			sen: "$GLGSV,1,1,03,67,57,036,37,68,30,328,34,78,53,184,27,1*4B",
			svs: []svInfo{
				{id: 67, elev: 57, azim: 36, cn0: 37},
				{id: 68, elev: 30, azim: 328, cn0: 34},
				{id: 78, elev: 53, azim: 184, cn0: 27},
			},
			sigID: 1,
			final: true,
		},
		{
			sen: "$GLGSV,1,1,03,67,57,036,31,68,30,328,27,78,53,184,31,3*4A",
			svs: []svInfo{
				{id: 67, elev: 57, azim: 36, cn0: 31},
				{id: 68, elev: 30, azim: 328, cn0: 27},
				{id: 78, elev: 53, azim: 184, cn0: 31},
			},
			sigID: 3,
			final: true,
		},
		{
			sen:   "$GPGSV,3,3,08,,,,,,,,,,,,,,,,*71",
			svs:   []svInfo{},
			final: true,
		},
		{
			sen: "$GLGSV,1,1,01,,65,190,31",
			svs: []svInfo{
				{id: 0, elev: 65, azim: 190, cn0: 31},
			},
			final: true,
		},
		{
			sen: "$GPGSV,4,3,15,20,43,009,,23,-1,231,28,24,03,206,,25,26,283,38*61\r\n",
			svs: []svInfo{
				{id: 23, elev: -1, azim: 231, cn0: 28},
				{id: 25, elev: 26, azim: 283, cn0: 38},
			},
		},
		{
			sen: "$GPGSV,1,1,01,21,65,360,31",
			svs: []svInfo{
				// LBE-1321 generates azimuth 360
				{id: 21, elev: 65, azim: 360, cn0: 31},
			},
			final: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(Trim(test.sen), func(t *testing.T) {
			sen := Parse(addTrailer(test.sen))
			gsv, err := parseGSV(sen)
			if err != nil {
				t.Fatalf("unexpected GSV parsing error: %v", err)
			}
			svs := gsv.svs
			if len(svs) != len(test.svs) {
				t.Fatalf("GSV SV count mismatch: got %d, want %d", len(svs), len(test.svs))
			}
			for i, sv := range svs {
				if sv.id != test.svs[i].id {
					t.Fatalf("GSV SV ID mismatch at index %d: got %v, want %v", i, sv.id, test.svs[i].id)
				}
				if sv.elev != test.svs[i].elev {
					t.Fatalf("GSV Elevation mismatch at index %d: got %d, want %d", i, sv.elev, test.svs[i].elev)
				}
				if sv.azim != test.svs[i].azim {
					t.Fatalf("GSV Azimuth mismatch at index %d: got %d, want %d", i, sv.azim, test.svs[i].azim)
				}
				if sv.cn0 != test.svs[i].cn0 {
					t.Fatalf("GSV CN0 mismatch at index %d: got %d, want %d", i, sv.cn0, test.svs[i].cn0)
				}
			}
			final := gsv.numMsg == gsv.msgNum
			if final != test.final {
				t.Fatalf("GSV final flag mismatch: got %v, want %v", final, test.final)
			}
			if gsv.sigID != test.sigID {
				t.Fatalf("GSV sigID mismatch: got %d, want %d", gsv.sigID, test.sigID)
			}
		})
	}
}

func addTrailer(s string) string {
	if !strings.Contains(s, "*") {
		s += fmt.Sprintf("*%02X\r\n", Checksum(([]byte)(s[1:])))
	}
	return s
}

type testSatellitesBufferMsgHandler struct {
	gpsprot.DefaultHandler
	nSV   int
	nUsed int
}

func (h *testSatellitesBufferMsgHandler) Satellites(msg *gpsprot.SatellitesMsg, _ time.Time) {
	h.nSV = len(msg.SVs)
	h.nUsed = 0
	for _, sv := range msg.SVs {
		if sv.Used {
			h.nUsed++
		}
	}
}

func TestSatellitesBuffer(t *testing.T) {
	sens := []string{
		"$GPGSV,1,1,04,10,77,300,32,23,58,153,30,25,46,137,30,32,45,316,26,8*61", // 4
		"$GLGSV,1,1,03,67,57,036,31,68,30,328,27,78,53,184,31,3*4A",              // 3
		"$GAGSV,2,1,06,15,78,354,48,8,33,201,42,13,28,311,41,5,31,47,27,6*40",    //  6 signals, but 2 repeated
		"$GAGSV,2,2,06,15,78,354,46,13,28,311,41,2*75",                           //  so 4 in all
		"$GPRMC,210230,A,3855.4487,N,09446.0071,W,0.0,076.2,130495,003.8,E*69\r\n",
		"$GPGSV,3,3,08,,,,,,,,,,,,,,,,*71", // 0
		"$GNGSA,A,3,8,5,,,,,,,,,,,2.38,1.26,2.01,3",
	}
	type result struct {
		i, nSV, nUsed int
	}
	tests := []struct {
		order  []int
		expect []result
	}{
		{
			order: []int{1, 2, 3, -1, 0, 1, 2, 3, 0, 1, 2, 3},
			expect: []result{
				{i: 3, nSV: 7},
				{i: 7, nSV: 11},
				{i: 11, nSV: 11},
			},
		},
		{
			order: []int{3, 4, 0, 1, 2, 3, 4, 0, 1, 2, 3, 4},
			expect: []result{
				{i: 1, nSV: 2},
				{i: 5, nSV: 11},
				{i: 10, nSV: 11},
			},
		},
		{
			order: []int{5, -1, 5},
			expect: []result{
				{i: 1, nSV: 0},
				{i: 2, nSV: 0},
			},
		},
		{
			order: []int{2, 3, 6, -1},
			expect: []result{
				{i: 3, nSV: 4, nUsed: 2},
			},
		},
		{
			order: []int{2, 3, -1, 6, 2, 3},
			expect: []result{
				{i: 2, nSV: 4, nUsed: 0},
				{i: 5, nSV: 4, nUsed: 2},
			},
		},
	}
	for ti, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%d", ti), func(t *testing.T) {
			order := test.order
			expect := test.expect
			sb := newSatellitesBuffer()
			k := 0
			for i, j := range order {
				h := testSatellitesBufferMsgHandler{nSV: -1}
				if j >= 0 {
					sen := Parse(addTrailer(sens[j]))
					sb.process(sen, time.Time{}, &h)
				} else {
					sb.idle(&h)
				}
				if k < len(expect) && i == expect[k].i {
					if h.nSV != expect[k].nSV {
						t.Fatalf("wrong SV count at %d: got %d, want %d", i, h.nSV, expect[k].nSV)
					}
					if h.nUsed != expect[k].nUsed {
						t.Fatalf("wrong used count at %d: got %d, want %d", i, h.nUsed, expect[k].nUsed)
					}
					k++
				} else if h.nSV != -1 {
					t.Fatalf("handling failed at %d: got %d, want none", i, h.nSV)
				}
			}
		})
	}
}

func TestSVID(t *testing.T) {
	dflt := dfltSVNumbering
	allystar := as.NewNMEASVNumbering()
	tests := []struct {
		gnss      gpsprot.GNSS
		svid      int
		sigid     int
		svname    string
		signame   string
		numbering []gpsprot.NMEASVNumberingRange
	}{
		{gnss: gpsprot.GPS, svid: 15, sigid: 0, svname: "G15", signame: "", numbering: dflt},
		{gnss: gpsprot.GPS, svid: 1, sigid: 1, svname: "G01", signame: "L1 C/A", numbering: dflt},
		{gnss: gpsprot.GPS, svid: 35, sigid: 0, svname: "S122", signame: "", numbering: dflt},
		{gnss: gpsprot.BDS, svid: 861, sigid: 0, svname: "C11", signame: "B2A", numbering: allystar},
		{gnss: gpsprot.GPS, svid: 861, sigid: 0, numbering: allystar}, // fail because inconsistent GNSS
		{gnss: gpsprot.GLO, svid: 0, sigid: 0, svname: "R?", signame: "", numbering: dflt},
		{gnss: gpsprot.GLO, svid: 65, sigid: 0, svname: "R01", signame: "", numbering: dflt},
		{gnss: gpsprot.GLO, svid: 88, sigid: 0, svname: "R24", signame: "", numbering: dflt},
		{gnss: gpsprot.GLO, svid: 89, sigid: 0, numbering: dflt}, // fail because out of range for valid SVID
	}
	for ti, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%d", ti), func(t *testing.T) {
			sb := newSatellitesBuffer()
			sb.setNumbering(test.numbering)
			svid, signame := sb.convertSVID(test.gnss, test.svid, test.sigid)
			if svid.IsZero() {
				if test.svname != "" {
					t.Fatalf("conversion to SVID failed, but expected %s", test.svname)
				}
				return
			}
			svname := svid.String()
			if test.svname == "" {
				t.Fatalf("conversion to SVID got %s, but expected to fail", svname)
			}
			if svname != test.svname {
				t.Fatalf("SVID name mismatch: got %s, want %s", svname, test.svname)
			}
			if signame != test.signame {
				t.Fatalf("Signal name mismatch: got %s, want %s", signame, test.signame)
			}
		})
	}
}
