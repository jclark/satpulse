package nmea

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/internal/as"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/internal/sino"
)


func TestGSAParse(t *testing.T) {
	tests := []struct {
		sen   string
		svids []gpsprot.SVID
	}{
		{
			sen: "$GNGSA,A,3,10,12,23,25,32,,,,,,,,2.38,1.26,2.01,1*0B",
			svids: []gpsprot.SVID{
				{GNSS: gpsprot.GPS, Num: 10},
				{GNSS: gpsprot.GPS, Num: 12},
				{GNSS: gpsprot.GPS, Num: 23},
				{GNSS: gpsprot.GPS, Num: 25},
				{GNSS: gpsprot.GPS, Num: 32},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(Trim(test.sen), func(t *testing.T) {
			sen := parseApprovedSentence(addTrailer(test.sen))
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
		{
			sen:   "$GPGSV,1,1,00,", // sigID can be blank
			final: true,
		},
		{
			sen:   "$GPGSV,1,1,00*79", // no antenna - zero satellites, no sigID field
			final: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(Trim(test.sen), func(t *testing.T) {
			sen := parseApprovedSentence(addTrailer(test.sen))
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
		s += fmt.Sprintf("*%02X\r\n", nmeamsg.Checksum(([]byte)(s[1:])))
	}
	if !strings.HasSuffix(s, "\r\n") {
		s += "\r\n"
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
			// Realistic scenario: partial burst to establish boundary, then complete burst
			order: []int{3, -1, 0, 1, 2, 3, 6, -1},
			expect: []result{
				// i=0: GAGSV 2/2 (partial), i=1: idle establishes boundary
				{i: 7, nSV: 11, nUsed: 2}, // idle flushes GPS(4) + GLO(3) + GAL(4) with GSA info
			},
		},
		{
			// Multiple bursts with idle between
			order: []int{0, 1, 2, 3, -1, 0, 1, 2, 3, -1},
			expect: []result{
				// First idle establishes boundary, clears buffer
				{i: 9, nSV: 11, nUsed: 0}, // Second idle flushes all
			},
		},
		{
			// NMEA 4.10: Multiple signal IDs for same GNSS
			order: []int{0, 1, -1, 0, 5, 1, -1},
			expect: []result{
				// First idle establishes boundary, clears buffer
				// After boundary: GPS sigID=8, GLO sigID=3
				{i: 6, nSV: 7, nUsed: 0}, // idle flushes GPS(4) + GLO(3) + 0 from GPGSV 3/3
			},
		},
		{
			// Protection against missing idle: continuous stream with no idle
			// Simulates receiver sending bursts back-to-back
			order: []int{4, 0, 1, 2, 3, 6, 4, 0, 1, 2, 3, 6, 4, 0},
			expect: []result{
				// i=0: GPRMC, i=1: GPGSV - format change establishes boundary
				// i=1-5: First burst accumulates
				// i=6: GPRMC - format change, no flush
				// i=7: GPGSV - repeats!
				{i: 7, nSV: 11, nUsed: 2}, // GPS repeats, flush first burst with GSA
				// i=8-11: Second burst accumulates
				// i=12: GPRMC - format change
				// i=13: GPGSV - repeats!
				{i: 13, nSV: 11, nUsed: 2}, // GPS repeats, flush second burst
			},
		},
		{
			// Edge case: Only GSV messages, no idle, no format changes
			// Start with partial burst, then repeated complete bursts
			order: []int{2, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3},
			expect: []result{
				// i=0: GAGSV 1/2 (partial burst)
				// i=1: GPGSV - format change, possibleBoundary clears buffer
				// i=2-4: First complete burst (GPS+GLO+GAL)
				// i=5: GPGSV - GPS repeats!
				{i: 5, nSV: 11}, // GPS repeats, flush first burst
				// i=6-8: Second complete burst accumulates
				// i=9: GPGSV - GPS repeats again!
				{i: 9, nSV: 11}, // GPS repeats, flush second burst
				// i=10-12: Third burst accumulates
				// No more flushes (would need another repeat)
			},
		},
		// Legacy tests from previous GNSS-tracking buffering approach
		// These mostly test edge cases where the new approach doesn't flush
		{
			order: []int{1, 2, 3, -1, 0, 1, 2, 3, 0, 1, 2, 3},
			expect: []result{
				// First idle clears buffer (no flush) to establish boundary
				{i: 8, nSV: 11}, // GPS repeats: flush all (GPS+GLO+GAL)
				// After flush at i=8, buffer has only GPS(4)
				// i=9: GLO added (first time), no flush
				// i=10,11: GAL parts added, no flush yet
			},
		},
		{
			order: []int{3, 4, 0, 1, 2, 3, 4, 0, 1, 2, 3, 4},
			expect: []result{
				// i=0: GAGSV 2/2 sigID=2 (incomplete)
				// i=1: GPRMC - first format change, possibleBoundary clears buffer
				// i=2: GPS sigID=8, i=3: GLO sigID=3, i=4-5: GAL sigID=6,2
				// i=6: GPRMC - no flush
				// i=7: GPS sigID=8 repeats
				{i: 7, nSV: 11}, // GPS repeats: flush all
				// Buffer cleared, gsvKeys reset
				// i=8: GLO sigID=3 (first time in new buffer)
				// i=9-10: GAL sigID=6,2 (first time in new buffer)
				// i=11: GPRMC - no flush
			},
		},
		{
			order:  []int{5, -1, 5},
			expect: []result{
				// i=0: GPGSV 3/3 (incomplete, has no satellites)
				// i=1: idle - first idle clears buffer (no flush)
				// i=2: GPGSV 3/3 again (still no satellites)
				// No flushes expected
			},
		},
		{
			order:  []int{2, 3, 6, -1},
			expect: []result{
				// i=0-1: GAGSV parts (4 satellites)
				// i=2: GNGSA (provides used info)
				// i=3: idle - first idle clears buffer (no flush)
				// No flush expected with new design
			},
		},
		{
			order:  []int{2, 3, -1, 6, 2, 3},
			expect: []result{
				// i=0-1: GAGSV parts (4 satellites)
				// i=2: idle - first idle clears buffer (no flush)
				// i=3: GNGSA
				// i=4: GAGSV 1/2 - format change GSA->GSV, possibleBoundary does nothing
				// i=5: GAGSV 2/2 - completes GAL sigID=2
				// No flush expected - need idle or repeated key
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
					sen := parseApprovedSentence(addTrailer(sens[j]))
					sb.process(sen, time.Time{}, &h, nil)
				} else {
					sb.idle(&h, nil)
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
	sinognss := sino.NewNMEASVNumbering()
	tests := []struct {
		gnss      gpsprot.GNSS
		svid      int
		sigid     int
		svname    string
		signame   gpsprot.SignalID
		numbering []gpsprot.NMEASVNumberingRange
	}{
		{gnss: gpsprot.GPS, svid: 15, sigid: 0, svname: "G15", signame: "", numbering: dflt},
		{gnss: gpsprot.GPS, svid: 1, sigid: 1, svname: "G01", signame: "L1 C/A", numbering: dflt},
		{gnss: gpsprot.GPS, svid: 35, sigid: 0, svname: "S22", signame: "", numbering: dflt},
		{gnss: gpsprot.BDS, svid: 861, sigid: 0, svname: "C11", signame: "B2a", numbering: allystar},
		{gnss: gpsprot.GPS, svid: 861, sigid: 0, numbering: allystar}, // fail because inconsistent GNSS
		{gnss: gpsprot.GLO, svid: 0, sigid: 0, svname: "R?", signame: "", numbering: dflt},
		{gnss: gpsprot.GLO, svid: 65, sigid: 0, svname: "R01", signame: "", numbering: dflt},
		{gnss: gpsprot.GLO, svid: 88, sigid: 0, svname: "R24", signame: "", numbering: dflt},
		{gnss: gpsprot.GLO, svid: 89, sigid: 0, svname: "R25", signame: "", numbering: dflt}, // slot 25 is valid (spare)
		{gnss: gpsprot.GLO, svid: 96, sigid: 0, svname: "R32", signame: "", numbering: dflt}, // slot 32 is valid (spare)
		{gnss: gpsprot.GLO, svid: 97, sigid: 0, numbering: dflt},                             // fail: slot 33 out of range
		{gnss: gpsprot.GLO, svid: 38, sigid: 0, svname: "R01", signame: "", numbering: sinognss},    // vendor 38 -> slot 1
		{gnss: gpsprot.GLO, svid: 61, sigid: 0, svname: "R24", signame: "", numbering: sinognss},    // vendor 61 -> slot 24
		{gnss: gpsprot.GLO, svid: 63, sigid: 0, numbering: sinognss},                                // vendor 63 -> slot 26, fails (beyond 24 operational)
		{gnss: gpsprot.NAVIC, svid: 63, sigid: 0, svname: "I02", signame: "", numbering: sinognss},  // vendor 63 -> NAVIC sat 2 (overlapping range OK)
		{gnss: gpsprot.GPS, svid: 133, sigid: 0, svname: "J03", signame: "", numbering: sinognss},   // QZSS in GPS sentence: 133 -> J03
		{gnss: gpsprot.GPS, svid: 137, sigid: 0, svname: "J07", signame: "", numbering: sinognss},   // QZSS in GPS sentence: 137 -> J07
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

type testPacketHandler struct {
	gpsprot.DefaultHandler
	satellites []*gpsprot.SatellitesMsg
}

func (h *testPacketHandler) Satellites(msg *gpsprot.SatellitesMsg, _ time.Time) {
	h.satellites = append(h.satellites, msg)
}

func TestPacketProcessorSatellites(t *testing.T) {
	tests := []struct {
		name         string
		nmeaMessages []string
		expectedSVs  []gpsprot.SVInfo
		numbering    []gpsprot.NMEASVNumberingRange
	}{
		{
			name: "simple GPS GSV only",
			nmeaMessages: []string{
				"$GNRMC,011451.00,A,1343.91062,N,10038.68405,E,0.000,,240725,,,D,V*1B\r\n",
				"$GPGSV,3,1,11,05,53,234,47,11,39,354,26,12,42,301,42,17,24,109,27,1*63\r\n",
				"$GPGSV,3,2,11,19,41,088,23,20,72,306,41,21,82,339,44,22,23,161,38,1*62\r\n",
				"$GPGSV,3,3,11,41,64,233,51,42,56,116,47,50,56,116,40,1*51\r\n",
				"$GNGLL,1343.91062,N,10038.68405,E,011451.00,A,D*7E\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 234, Elevation: 53}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 47}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 11}, LookAngles: &gpsprot.LookAngles{Azimuth: 354, Elevation: 39}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 26}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 301, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 42}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 17}, LookAngles: &gpsprot.LookAngles{Azimuth: 109, Elevation: 24}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 27}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 88, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 23}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 20}, LookAngles: &gpsprot.LookAngles{Azimuth: 306, Elevation: 72}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 41}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 21}, LookAngles: &gpsprot.LookAngles{Azimuth: 339, Elevation: 82}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 44}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 22}, LookAngles: &gpsprot.LookAngles{Azimuth: 161, Elevation: 23}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 38}}, Used: false},
				// SBAS satellites (GPS IDs 41, 42, 50 -> SBAS 28, 29, 37)
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 28}, LookAngles: &gpsprot.LookAngles{Azimuth: 233, Elevation: 64}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 51}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 47}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 37}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 40}}, Used: false},
			},
		},
		{
			name: "GPS GSV messages with multiple signals",
			nmeaMessages: []string{
				"$GNRMC,011451.00,A,1343.91062,N,10038.68405,E,0.000,,240725,,,D,V*1B\r\n",
				"$GNGSA,M,3,12,11,21,22,20,05,,,,,,,99.99,99.99,99.99,1*3A\r\n", // GPS: 12,11,21,22,20,05 used
				"$GPGSV,3,1,11,05,53,234,47,11,39,354,26,12,42,301,42,17,24,109,27,1*63\r\n",
				"$GPGSV,3,2,11,19,41,088,23,20,72,306,41,21,82,339,44,22,23,161,38,1*62\r\n",
				"$GPGSV,3,3,11,41,64,233,51,42,56,116,47,50,56,116,40,1*51\r\n",
				"$GPGSV,2,1,06,05,53,234,43,06,24,035,15,11,39,354,25,12,42,301,39,6*67\r\n",
				"$GPGSV,2,2,06,17,24,109,18,21,82,339,49,6*69\r\n",
				"$GPGSV,2,1,07,09,10,057,,13,16,176,,14,06,155,,15,03,203,,0*6A\r\n",
				"$GPGSV,2,2,07,24,04,235,,25,13,314,,40,35,255,,0*57\r\n",
				"$GNGLL,1343.91062,N,10038.68405,E,011451.00,A,D*7E\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				// GPS satellites with signal combinations from sigID 1 and 6 - GSA indicates 12,11,21,22,20,05 are used
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 234, Elevation: 53}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 47}, {ID: "L2C-L", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 35, Elevation: 24}, Signals: []gpsprot.SignalInfo{{ID: "L2C-L", CN0: 15}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 11}, LookAngles: &gpsprot.LookAngles{Azimuth: 354, Elevation: 39}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 26}, {ID: "L2C-L", CN0: 25}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 301, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 42}, {ID: "L2C-L", CN0: 39}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 17}, LookAngles: &gpsprot.LookAngles{Azimuth: 109, Elevation: 24}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 27}, {ID: "L2C-L", CN0: 18}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 88, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 23}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 20}, LookAngles: &gpsprot.LookAngles{Azimuth: 306, Elevation: 72}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 41}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 21}, LookAngles: &gpsprot.LookAngles{Azimuth: 339, Elevation: 82}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 44}, {ID: "L2C-L", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 22}, LookAngles: &gpsprot.LookAngles{Azimuth: 161, Elevation: 23}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 38}}, Used: true},
				// SBAS satellites (GPS IDs 41, 42, 50 -> SBAS 28, 29, 37)
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 28}, LookAngles: &gpsprot.LookAngles{Azimuth: 233, Elevation: 64}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 51}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 47}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 37}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 40}}, Used: false},
			},
		},
		{
			name: "GLONASS GSV messages",
			nmeaMessages: []string{
				"$GNRMC,011451.00,A,1343.91062,N,10038.68405,E,0.000,,240725,,,D,V*1B\r\n",
				"$GNGSA,M,3,80,69,73,,,,,,,,,,99.99,99.99,99.99,2*3D\r\n", // GLO: 80->R16, 69->R05, 73->R09 used
				"$GLGSV,2,1,05,68,45,067,19,69,47,148,49,73,37,258,49,80,22,195,37,1*77\r\n",
				"$GLGSV,2,2,05,83,26,016,28,1*4F\r\n",
				"$GLGSV,2,1,05,68,45,067,30,69,47,148,48,73,37,258,47,80,22,195,37,3*71\r\n",
				"$GLGSV,2,2,05,83,26,016,16,3*40\r\n",
				"$GLGSV,2,1,05,67,07,033,,70,00,195,,74,19,306,,82,10,065,,0*75\r\n",
				"$GLGSV,2,2,05,84,15,320,,0*45\r\n",
				"$GNGLL,1343.91062,N,10038.68405,E,011451.00,A,D*7E\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				// GLONASS satellites (NMEA IDs 68,69,73,80,83 -> GLO nums 4,5,9,16,19) - GSA indicates 80->R16, 69->R05, 73->R09 are used
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 4}, LookAngles: &gpsprot.LookAngles{Azimuth: 67, Elevation: 45}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 19}, {ID: "L2", CN0: 30}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 148, Elevation: 47}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 49}, {ID: "L2", CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 258, Elevation: 37}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 49}, {ID: "L2", CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 195, Elevation: 22}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 37}, {ID: "L2", CN0: 37}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 16, Elevation: 26}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 28}, {ID: "L2", CN0: 16}}, Used: false},
			},
		},
		{
			name: "Galileo GSV messages",
			nmeaMessages: []string{
				"$GNRMC,011451.00,A,1343.91062,N,10038.68405,E,0.000,,240725,,,D,V*1B\r\n",
				"$GNGSA,M,3,24,09,04,31,25,11,16,36,06,,,,99.99,99.99,99.99,3*35\r\n", // GAL: 24,09,04,31,25,11,16,36,06 used
				"$GAGSV,3,1,10,04,31,114,31,06,31,144,45,09,22,169,45,10,35,012,22,2*73\r\n",
				"$GAGSV,3,2,10,11,41,337,33,16,64,273,49,24,65,202,48,25,45,304,49,2*7F\r\n",
				"$GAGSV,3,3,10,31,20,158,40,36,22,282,41,2*77\r\n",
				"$GAGSV,3,1,10,04,31,114,28,06,31,144,38,09,22,169,38,10,35,012,25,7*79\r\n",
				"$GAGSV,3,2,10,11,41,337,28,16,64,273,48,24,65,202,47,25,45,304,43,7*74\r\n",
				"$GAGSV,3,3,10,31,20,158,38,36,22,282,36,7*7D\r\n",
				"$GAGSV,1,1,02,12,22,035,,19,08,061,,0*74\r\n",
				"$GNGLL,1343.91062,N,10038.68405,E,011451.00,A,D*7E\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				// Galileo satellites (E5b sigID=2, E1 sigID=7) - GSA indicates 24,09,04,31,25,11,16,36,06 are used
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 4}, LookAngles: &gpsprot.LookAngles{Azimuth: 114, Elevation: 31}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 28}, {ID: "E5b", CN0: 31}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 144, Elevation: 31}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 38}, {ID: "E5b", CN0: 45}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 169, Elevation: 22}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 38}, {ID: "E5b", CN0: 45}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 12, Elevation: 35}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 25}, {ID: "E5b", CN0: 22}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 11}, LookAngles: &gpsprot.LookAngles{Azimuth: 337, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 28}, {ID: "E5b", CN0: 33}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 273, Elevation: 64}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 48}, {ID: "E5b", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 24}, LookAngles: &gpsprot.LookAngles{Azimuth: 202, Elevation: 65}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 47}, {ID: "E5b", CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 25}, LookAngles: &gpsprot.LookAngles{Azimuth: 304, Elevation: 45}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 43}, {ID: "E5b", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 31}, LookAngles: &gpsprot.LookAngles{Azimuth: 158, Elevation: 20}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 38}, {ID: "E5b", CN0: 40}}, Used: true},
			},
		},
		{
			name: "BeiDou GSV messages",
			nmeaMessages: []string{
				"$GNRMC,011451.00,A,1343.91062,N,10038.68405,E,0.000,,240725,,,D,V*1B\r\n",
				"$GNGSA,M,3,19,45,06,36,16,38,10,09,07,13,40,08,99.99,99.99,99.99,4*3F\r\n", // BDS: 19,45,06,36,16,38,10,09,07,13,40,08 used
				"$GBGSV,4,1,15,06,14,179,26,07,71,063,39,08,79,316,43,09,17,193,34,1*77\r\n",
				"$GBGSV,4,2,15,10,71,016,41,13,76,269,49,19,66,141,50,20,41,055,26,1*78\r\n",
				"$GBGSV,4,3,15,22,24,198,44,29,42,034,27,35,24,324,35,36,19,181,42,1*7C\r\n",
				"$GBGSV,4,4,15,38,69,006,44,40,58,071,31,45,33,234,47,1*4A\r\n",
				"$GBGSV,2,1,07,06,14,179,36,07,71,063,40,08,79,316,46,09,17,193,41,B*09\r\n",
				"$GBGSV,2,2,07,10,71,016,43,13,76,269,47,16,13,177,30,B*3F\r\n",
				"$GBGSV,3,1,09,01,38,103,,02,63,228,,03,70,144,,04,22,096,,0*75\r\n",
				"$GBGSV,3,2,09,05,39,252,,26,13,289,,30,16,086,,39,10,165,,0*73\r\n",
				"$GBGSV,3,3,09,47,33,059,,0*41\r\n",
				"$GNGLL,1343.91062,N,10038.68405,E,011451.00,A,D*7E\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				// BeiDou satellites - GSA lists 19,45,06,36,16,38,10,09,07,13,40,08 as used, but only those with valid CN0 appear in output
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 179, Elevation: 14}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 26}, {ID: "B2I", CN0: 36}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 63, Elevation: 71}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 39}, {ID: "B2I", CN0: 40}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 8}, LookAngles: &gpsprot.LookAngles{Azimuth: 316, Elevation: 79}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 43}, {ID: "B2I", CN0: 46}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 193, Elevation: 17}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 34}, {ID: "B2I", CN0: 41}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 16, Elevation: 71}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 41}, {ID: "B2I", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 269, Elevation: 76}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 49}, {ID: "B2I", CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 177, Elevation: 13}, Signals: []gpsprot.SignalInfo{{ID: "B2I", CN0: 30}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 141, Elevation: 66}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 50}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 20}, LookAngles: &gpsprot.LookAngles{Azimuth: 55, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 26}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 22}, LookAngles: &gpsprot.LookAngles{Azimuth: 198, Elevation: 24}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 44}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 34, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 27}}, Used: false},
			},
		},
		{
			name: "QZSS GSV messages",
			nmeaMessages: []string{
				"$GNRMC,011451.00,A,1343.91062,N,10038.68405,E,0.000,,240725,,,D,V*1B\r\n",
				"$GNGSA,M,3,04,02,,,,,,,,,,,99.99,99.99,99.99,5*3F\r\n", // QZSS: 04,02 used
				"$GQGSV,1,1,03,02,42,137,40,04,49,093,22,07,56,116,37,1*56\r\n",
				"$GQGSV,1,1,03,02,42,137,43,04,49,093,30,07,56,116,44,6*55\r\n",
				"$GQGSV,1,1,01,03,42,047,,0*53\r\n",
				"$GNGLL,1343.91062,N,10038.68405,E,011451.00,A,D*7E\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				// QZSS satellites - GSA lists 04,02 as used
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 137, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 40}, {ID: "L2C-L", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 4}, LookAngles: &gpsprot.LookAngles{Azimuth: 93, Elevation: 49}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 22}, {ID: "L2C-L", CN0: 30}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 37}, {ID: "L2C-L", CN0: 44}}, Used: false},
			},
		},
		{
			name: "complete NMEA message set",
			nmeaMessages: []string{
				"$GNRMC,011451.00,A,1343.91062,N,10038.68405,E,0.000,,240725,,,D,V*1B\r\n",
				"$GNGSA,M,3,12,11,21,22,20,05,,,,,,,99.99,99.99,99.99,1*3A\r\n",             // GPS: 12,11,21,22,20,05
				"$GNGSA,M,3,80,69,73,,,,,,,,,,99.99,99.99,99.99,2*3D\r\n",                   // GLO: 80->15, 69->4, 73->8
				"$GNGSA,M,3,24,09,04,31,25,11,16,36,06,,,,99.99,99.99,99.99,3*35\r\n",       // GAL: 24,09,04,31,25,11,16,36,06
				"$GNGSA,M,3,19,45,06,36,16,38,10,09,07,13,40,08,99.99,99.99,99.99,4*3F\r\n", // BDS: 19,45,06,36,16,38,10,09,07,13,40,08
				"$GNGSA,M,3,04,02,,,,,,,,,,,99.99,99.99,99.99,5*3F\r\n",                     // QZSS: 04,02
				"$GPGSV,3,1,11,05,53,234,47,11,39,354,26,12,42,301,42,17,24,109,27,1*63\r\n",
				"$GPGSV,3,2,11,19,41,088,23,20,72,306,41,21,82,339,44,22,23,161,38,1*62\r\n",
				"$GPGSV,3,3,11,41,64,233,51,42,56,116,47,50,56,116,40,1*51\r\n",
				"$GPGSV,2,1,06,05,53,234,43,06,24,035,15,11,39,354,25,12,42,301,39,6*67\r\n",
				"$GPGSV,2,2,06,17,24,109,18,21,82,339,49,6*69\r\n",
				"$GPGSV,2,1,07,09,10,057,,13,16,176,,14,06,155,,15,03,203,,0*6A\r\n",
				"$GPGSV,2,2,07,24,04,235,,25,13,314,,40,35,255,,0*57\r\n",
				"$GLGSV,2,1,05,68,45,067,19,69,47,148,49,73,37,258,49,80,22,195,37,1*77\r\n",
				"$GLGSV,2,2,05,83,26,016,28,1*4F\r\n",
				"$GLGSV,2,1,05,68,45,067,30,69,47,148,48,73,37,258,47,80,22,195,37,3*71\r\n",
				"$GLGSV,2,2,05,83,26,016,16,3*40\r\n",
				"$GLGSV,2,1,05,67,07,033,,70,00,195,,74,19,306,,82,10,065,,0*75\r\n",
				"$GLGSV,2,2,05,84,15,320,,0*45\r\n",
				"$GAGSV,3,1,10,04,31,114,31,06,31,144,45,09,22,169,45,10,35,012,22,2*73\r\n",
				"$GAGSV,3,2,10,11,41,337,33,16,64,273,49,24,65,202,48,25,45,304,49,2*7F\r\n",
				"$GAGSV,3,3,10,31,20,158,40,36,22,282,41,2*77\r\n",
				"$GAGSV,3,1,10,04,31,114,28,06,31,144,38,09,22,169,38,10,35,012,25,7*79\r\n",
				"$GAGSV,3,2,10,11,41,337,28,16,64,273,48,24,65,202,47,25,45,304,43,7*74\r\n",
				"$GAGSV,3,3,10,31,20,158,38,36,22,282,36,7*7D\r\n",
				"$GAGSV,1,1,02,12,22,035,,19,08,061,,0*74\r\n",
				"$GBGSV,4,1,15,06,14,179,26,07,71,063,39,08,79,316,43,09,17,193,34,1*77\r\n",
				"$GBGSV,4,2,15,10,71,016,41,13,76,269,49,19,66,141,50,20,41,055,26,1*78\r\n",
				"$GBGSV,4,3,15,22,24,198,44,29,42,034,27,35,24,324,35,36,19,181,42,1*7C\r\n",
				"$GBGSV,4,4,15,38,69,006,44,40,58,071,31,45,33,234,47,1*4A\r\n",
				"$GBGSV,2,1,07,06,14,179,36,07,71,063,40,08,79,316,46,09,17,193,41,B*09\r\n",
				"$GBGSV,2,2,07,10,71,016,43,13,76,269,47,16,13,177,30,B*3F\r\n",
				"$GBGSV,3,1,09,01,38,103,,02,63,228,,03,70,144,,04,22,096,,0*75\r\n",
				"$GBGSV,3,2,09,05,39,252,,26,13,289,,30,16,086,,39,10,165,,0*73\r\n",
				"$GBGSV,3,3,09,47,33,059,,0*41\r\n",
				"$GQGSV,1,1,03,02,42,137,40,04,49,093,22,07,56,116,37,1*56\r\n",
				"$GQGSV,1,1,03,02,42,137,43,04,49,093,30,07,56,116,44,6*55\r\n",
				"$GQGSV,1,1,01,03,42,047,,0*53\r\n",
				"$GNGLL,1343.91062,N,10038.68405,E,011451.00,A,D*7E\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				// GPS satellites with correct Used flags (from GPS test)
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 234, Elevation: 53}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 47}, {ID: "L2C-L", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 35, Elevation: 24}, Signals: []gpsprot.SignalInfo{{ID: "L2C-L", CN0: 15}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 11}, LookAngles: &gpsprot.LookAngles{Azimuth: 354, Elevation: 39}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 26}, {ID: "L2C-L", CN0: 25}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 301, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 42}, {ID: "L2C-L", CN0: 39}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 17}, LookAngles: &gpsprot.LookAngles{Azimuth: 109, Elevation: 24}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 27}, {ID: "L2C-L", CN0: 18}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 88, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 23}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 20}, LookAngles: &gpsprot.LookAngles{Azimuth: 306, Elevation: 72}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 41}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 21}, LookAngles: &gpsprot.LookAngles{Azimuth: 339, Elevation: 82}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 44}, {ID: "L2C-L", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 22}, LookAngles: &gpsprot.LookAngles{Azimuth: 161, Elevation: 23}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 38}}, Used: true},

				// SBAS satellites (from GPS test)
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 28}, LookAngles: &gpsprot.LookAngles{Azimuth: 233, Elevation: 64}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 51}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 47}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 37}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 40}}, Used: false},

				// GLONASS satellites with correct Used flags (from GLONASS test)
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 4}, LookAngles: &gpsprot.LookAngles{Azimuth: 67, Elevation: 45}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 19}, {ID: "L2", CN0: 30}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 148, Elevation: 47}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 49}, {ID: "L2", CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 258, Elevation: 37}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 49}, {ID: "L2", CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 195, Elevation: 22}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 37}, {ID: "L2", CN0: 37}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 16, Elevation: 26}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 28}, {ID: "L2", CN0: 16}}, Used: false},

				// Galileo satellites with correct Used flags (from Galileo test)
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 4}, LookAngles: &gpsprot.LookAngles{Azimuth: 114, Elevation: 31}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 28}, {ID: "E5b", CN0: 31}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 144, Elevation: 31}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 38}, {ID: "E5b", CN0: 45}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 169, Elevation: 22}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 38}, {ID: "E5b", CN0: 45}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 12, Elevation: 35}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 25}, {ID: "E5b", CN0: 22}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 11}, LookAngles: &gpsprot.LookAngles{Azimuth: 337, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 28}, {ID: "E5b", CN0: 33}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 273, Elevation: 64}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 48}, {ID: "E5b", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 24}, LookAngles: &gpsprot.LookAngles{Azimuth: 202, Elevation: 65}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 47}, {ID: "E5b", CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 25}, LookAngles: &gpsprot.LookAngles{Azimuth: 304, Elevation: 45}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 43}, {ID: "E5b", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 31}, LookAngles: &gpsprot.LookAngles{Azimuth: 158, Elevation: 20}, Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 38}, {ID: "E5b", CN0: 40}}, Used: true},

				// BeiDou satellites with correct Used flags (from BeiDou test)
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 179, Elevation: 14}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 26}, {ID: "B2I", CN0: 36}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 63, Elevation: 71}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 39}, {ID: "B2I", CN0: 40}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 8}, LookAngles: &gpsprot.LookAngles{Azimuth: 316, Elevation: 79}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 43}, {ID: "B2I", CN0: 46}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 193, Elevation: 17}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 34}, {ID: "B2I", CN0: 41}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 16, Elevation: 71}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 41}, {ID: "B2I", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 269, Elevation: 76}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 49}, {ID: "B2I", CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 177, Elevation: 13}, Signals: []gpsprot.SignalInfo{{ID: "B2I", CN0: 30}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 141, Elevation: 66}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 50}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 20}, LookAngles: &gpsprot.LookAngles{Azimuth: 55, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 26}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 22}, LookAngles: &gpsprot.LookAngles{Azimuth: 198, Elevation: 24}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 44}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 34, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 27}}, Used: false},

				// QZSS satellites with correct Used flags (from QZSS test)
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 137, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 40}, {ID: "L2C-L", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 4}, LookAngles: &gpsprot.LookAngles{Azimuth: 93, Elevation: 49}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 22}, {ID: "L2C-L", CN0: 30}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 37}, {ID: "L2C-L", CN0: 44}}, Used: false},
			},
		},
		{
			name: "SinoGNSS vendor-specific numbering",
			nmeaMessages: []string{
				"$GPRMC,072001.00,A,1343.9084990,N,10038.6847542,E,000.007,053.6,041025,0.0,W,A*22\r\n",
				"$GNGSA,M,3,26,31,16,02,10,,,,,,,,1.0,0.5,0.8,1*27\r\n",                             // GPS: system ID 1
				"$GNGSA,M,3,133,137,,,,,,,,,,,1.0,0.5,0.8,5*24\r\n",                                 // QZSS: system ID 5
				"$GNGSA,M,3,143,142,150,147,145,149,153,148,200,159,176,162,1.0,0.5,0.8,4*20\r\n",   // BDS: system ID 4
				"$GNGSA,M,3,185,166,180,199,161,178,,,,,,,1.0,0.5,0.8,4*2C\r\n",                     // BDS: system ID 4
				"$GNGSA,M,3,38,45,49,50,,,,,,,,,1.0,0.5,0.8,2*21\r\n",                               // GLO: system ID 2
				"$GNGSA,M,3,93,99,89,103,82,91,,,,,,,1.0,0.5,0.8,3*14\r\n",                          // GAL: system ID 3
				"$GPGSV,2,1,07,26,86,199,49,31,46,340,31,16,43,202,47,133,15,146,31*49\r\n",
				"$GPGSV,2,2,07,137,56,115,36,02,11,250,37,10,35,159,44*73\r\n",
				"$BDGSV,5,1,20,143,71,143,46,142,67,238,46,150,31,173,42,147,34,161,42*67\r\n",
				"$BDGSV,5,2,20,145,40,259,42,149,54,322,40,146,51,347,22,153,76,203,48*67\r\n",
				"$BDGSV,5,3,20,148,66,174,47,196,79,219,42,200,62,240,49,159,28,127,43*65\r\n",
				"$BDGSV,5,4,20,176,25,032,27,162,71,062,45,185,65,344,48,166,36,245,48*65\r\n",
				"$BDGSV,5,5,20,180,26,147,40,199,43,105,34,161,36,335,33,178,52,166,45*6B\r\n",
				"$GLGSV,2,1,05,38,58,058,32,45,23,122,39,63,17,126,39,49,66,344,51*64\r\n",
				"$GLGSV,2,2,05,50,49,236,47*5C\r\n",
				"$GAGSV,2,1,06,93,51,194,47,99,39,063,25,89,41,357,19,103,24,211,43*58\r\n",
				"$GAGSV,2,2,06,82,20,268,28,91,65,069,39*6E\r\n",
				"$GIGSV,1,1,02,63,25,226,44,70,40,297,44*69\r\n",
			},
			numbering: sino.NewNMEASVNumbering(),
			expectedSVs: []gpsprot.SVInfo{
				// GPS satellites (GSA: 26,31,16,02,10 used)
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 26}, LookAngles: &gpsprot.LookAngles{Azimuth: 199, Elevation: 86}, Signals: []gpsprot.SignalInfo{{CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 31}, LookAngles: &gpsprot.LookAngles{Azimuth: 340, Elevation: 46}, Signals: []gpsprot.SignalInfo{{CN0: 31}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 202, Elevation: 43}, Signals: []gpsprot.SignalInfo{{CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 250, Elevation: 11}, Signals: []gpsprot.SignalInfo{{CN0: 37}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 159, Elevation: 35}, Signals: []gpsprot.SignalInfo{{CN0: 44}}, Used: true},
				// QZSS satellites (GSA: 133->3, 137->7 used)
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 3}, LookAngles: &gpsprot.LookAngles{Azimuth: 146, Elevation: 15}, Signals: []gpsprot.SignalInfo{{CN0: 31}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 115, Elevation: 56}, Signals: []gpsprot.SignalInfo{{CN0: 36}}, Used: true},
				// BeiDou satellites (GSA: 143,142,150,147,145,149,153,148,200,159,176,162,185,166,180,199,161,178 used)
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 3}, LookAngles: &gpsprot.LookAngles{Azimuth: 143, Elevation: 71}, Signals: []gpsprot.SignalInfo{{CN0: 46}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 238, Elevation: 67}, Signals: []gpsprot.SignalInfo{{CN0: 46}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 173, Elevation: 31}, Signals: []gpsprot.SignalInfo{{CN0: 42}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 161, Elevation: 34}, Signals: []gpsprot.SignalInfo{{CN0: 42}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 259, Elevation: 40}, Signals: []gpsprot.SignalInfo{{CN0: 42}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 322, Elevation: 54}, Signals: []gpsprot.SignalInfo{{CN0: 40}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 347, Elevation: 51}, Signals: []gpsprot.SignalInfo{{CN0: 22}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 203, Elevation: 76}, Signals: []gpsprot.SignalInfo{{CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 8}, LookAngles: &gpsprot.LookAngles{Azimuth: 174, Elevation: 66}, Signals: []gpsprot.SignalInfo{{CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 56}, LookAngles: &gpsprot.LookAngles{Azimuth: 219, Elevation: 79}, Signals: []gpsprot.SignalInfo{{CN0: 42}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 60}, LookAngles: &gpsprot.LookAngles{Azimuth: 240, Elevation: 62}, Signals: []gpsprot.SignalInfo{{CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 127, Elevation: 28}, Signals: []gpsprot.SignalInfo{{CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 36}, LookAngles: &gpsprot.LookAngles{Azimuth: 32, Elevation: 25}, Signals: []gpsprot.SignalInfo{{CN0: 27}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 22}, LookAngles: &gpsprot.LookAngles{Azimuth: 62, Elevation: 71}, Signals: []gpsprot.SignalInfo{{CN0: 45}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 45}, LookAngles: &gpsprot.LookAngles{Azimuth: 344, Elevation: 65}, Signals: []gpsprot.SignalInfo{{CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 26}, LookAngles: &gpsprot.LookAngles{Azimuth: 245, Elevation: 36}, Signals: []gpsprot.SignalInfo{{CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 40}, LookAngles: &gpsprot.LookAngles{Azimuth: 147, Elevation: 26}, Signals: []gpsprot.SignalInfo{{CN0: 40}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 59}, LookAngles: &gpsprot.LookAngles{Azimuth: 105, Elevation: 43}, Signals: []gpsprot.SignalInfo{{CN0: 34}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 21}, LookAngles: &gpsprot.LookAngles{Azimuth: 335, Elevation: 36}, Signals: []gpsprot.SignalInfo{{CN0: 33}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 38}, LookAngles: &gpsprot.LookAngles{Azimuth: 166, Elevation: 52}, Signals: []gpsprot.SignalInfo{{CN0: 45}}, Used: true},
				// GLONASS satellites (GSA: 38,45,49,50 → 1,8,12,13 used)
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 1}, LookAngles: &gpsprot.LookAngles{Azimuth: 58, Elevation: 58}, Signals: []gpsprot.SignalInfo{{CN0: 32}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 8}, LookAngles: &gpsprot.LookAngles{Azimuth: 122, Elevation: 23}, Signals: []gpsprot.SignalInfo{{CN0: 39}}, Used: true},
				// Vendor ID 63 -> slot 26 is rejected (beyond 24 operational slots, spare satellite)
				// {ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 26}, LookAngles: &gpsprot.LookAngles{Azimuth: 126, Elevation: 17}, Signals: []gpsprot.SignalInfo{{CN0: 39}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 344, Elevation: 66}, Signals: []gpsprot.SignalInfo{{CN0: 51}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 236, Elevation: 49}, Signals: []gpsprot.SignalInfo{{CN0: 47}}, Used: true},
				// Galileo satellites (GSA: 93,99,89,103,82,91 → 23,29,19,33,12,21 used)
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 23}, LookAngles: &gpsprot.LookAngles{Azimuth: 194, Elevation: 51}, Signals: []gpsprot.SignalInfo{{CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 63, Elevation: 39}, Signals: []gpsprot.SignalInfo{{CN0: 25}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 357, Elevation: 41}, Signals: []gpsprot.SignalInfo{{CN0: 19}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 33}, LookAngles: &gpsprot.LookAngles{Azimuth: 211, Elevation: 24}, Signals: []gpsprot.SignalInfo{{CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 268, Elevation: 20}, Signals: []gpsprot.SignalInfo{{CN0: 28}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 21}, LookAngles: &gpsprot.LookAngles{Azimuth: 69, Elevation: 65}, Signals: []gpsprot.SignalInfo{{CN0: 39}}, Used: true},
				// NAVIC satellites (63->2, 70->9)
				{ID: gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 226, Elevation: 25}, Signals: []gpsprot.SignalInfo{{CN0: 44}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 297, Elevation: 40}, Signals: []gpsprot.SignalInfo{{CN0: 44}}, Used: false},
			},
		},
		{
			name: "NMEA 4.11 with GLONASS spare slot 26",
			nmeaMessages: []string{
				"$GNRMC,071958.00,A,1343.9085094,N,10038.6847601,E,000.004,076.8,041025,0.0,W,A,S*47\r\n",
				"$GNGSA,M,3,26,31,16,02,10,,,,,,,,1.0,0.5,0.8,1*33\r\n",       // GPS
				"$GNGSA,M,3,65,72,76,77,,,,,,,,,1.0,0.5,0.8,2*35\r\n",         // GLO: slots 1,8,12,13
				"$GNGSA,M,3,23,29,19,33,12,21,,,,,,,1.0,0.5,0.8,3*31\r\n",     // GAL
				"$GNGSA,M,3,03,02,10,07,05,09,13,08,60,19,36,22,1.0,0.5,0.8,4*3E\r\n",  // BDS
				"$GNGSA,M,3,45,26,40,59,21,38,,,,,,,1.0,0.5,0.8,4*31\r\n",     // BDS
				"$GNGSA,M,3,03,07,,,,,,,,,,,1.0,0.5,0.8,5*31\r\n",             // QZSS
				"$GPGSV,2,1,05,26,86,199,49,31,46,340,31,16,43,202,47,02,11,250,37,1*63\r\n",
				"$GPGSV,2,2,05,10,35,159,44,1*5B\r\n",
				"$GPGSV,2,1,05,26,86,199,52,31,46,340,24,16,43,202,37,02,11,250,23,4*6A\r\n",
				"$GPGSV,2,2,05,10,35,159,36,4*5B\r\n",
				"$GPGSV,1,1,02,26,86,199,50,10,35,159,40,8*6F\r\n",
				"$GLGSV,2,1,05,65,58,058,33,72,23,122,39,90,17,126,38,76,66,344,51,1*75\r\n",  // 90 is slot 26 (spare)
				"$GLGSV,2,2,05,77,49,236,47,1*44\r\n",
				"$GLGSV,1,1,04,66,30,351,32,72,23,122,43,90,17,126,45,76,66,344,48,3*7C\r\n",
				"$GBGSV,5,1,20,03,71,143,46,02,67,238,47,10,31,173,42,07,34,161,43,1*79\r\n",
				"$GBGSV,5,2,20,05,40,259,43,09,54,322,40,06,51,347,22,13,76,203,48,1*78\r\n",
				"$GBGSV,5,3,20,08,66,174,47,56,79,219,42,60,62,240,49,19,28,127,43,1*72\r\n",
				"$GBGSV,5,4,20,36,25,032,27,22,71,062,45,45,65,344,49,26,36,245,48,1*72\r\n",
				"$GBGSV,5,5,20,40,26,147,41,59,43,105,34,21,36,335,34,38,52,166,45,1*73\r\n",
				"$GBGSV,6,1,22,03,71,143,49,02,67,238,50,10,31,173,39,07,34,161,45,8*72\r\n",
				"$GBGSV,6,2,22,05,40,259,43,09,54,322,38,01,37,104,31,16,47,359,26,8*7E\r\n",
				"$GBGSV,6,3,22,06,51,347,38,13,76,203,49,08,66,174,49,56,79,219,45,8*7D\r\n",
				"$GBGSV,6,4,22,60,62,240,53,19,28,127,44,36,25,032,27,22,71,062,43,8*7A\r\n",
				"$GBGSV,6,5,22,45,65,344,49,26,36,245,50,40,26,147,44,59,43,105,37,8*7E\r\n",
				"$GBGSV,6,6,22,21,36,335,36,38,52,166,51,8*70\r\n",
				"$GBGSV,3,1,11,03,71,143,50,02,67,238,49,10,31,173,46,07,34,161,46,B*06\r\n",
				"$GBGSV,3,2,11,05,40,259,43,09,54,322,43,01,37,104,33,16,47,359,27,B*0E\r\n",
				"$GBGSV,3,3,11,06,51,347,37,13,76,203,49,08,66,174,49,B*3B\r\n",
				"$GBGSV,2,1,07,19,28,127,41,22,71,062,44,45,65,344,49,26,36,245,47,3*7C\r\n",
				"$GBGSV,2,2,07,40,26,147,40,21,36,335,31,38,52,166,46,3*4B\r\n",
				"$GBGSV,2,1,07,19,28,127,40,22,71,062,40,45,65,344,45,26,36,245,44,5*70\r\n",
				"$GBGSV,2,2,07,40,26,147,41,21,36,335,35,38,52,166,43,5*4D\r\n",
				"$GBGSV,3,1,09,60,62,240,51,19,28,127,42,22,71,062,42,45,65,344,47,6*7C\r\n",
				"$GBGSV,3,2,09,26,36,245,49,40,26,147,43,59,43,105,36,21,36,335,40,6*7E\r\n",
				"$GBGSV,3,3,09,38,52,166,46,6*47\r\n",
				"$GAGSV,1,1,04,23,51,194,46,33,24,211,42,12,20,268,35,21,65,069,37,1*78\r\n",
				"$GAGSV,2,1,06,23,51,194,51,29,39,062,20,19,41,357,26,33,24,211,44,2*75\r\n",
				"$GAGSV,2,2,06,12,20,268,39,21,65,069,41,2*7D\r\n",
				"$GAGSV,2,1,06,23,51,194,48,29,39,062,25,19,41,357,19,33,24,211,43,7*76\r\n",
				"$GAGSV,2,2,06,12,20,268,28,21,65,069,39,7*77\r\n",
				"$GAGSV,2,1,05,23,51,194,52,29,39,062,30,33,24,211,43,12,20,268,38,5*7A\r\n",
				"$GAGSV,2,2,05,21,65,069,41,5*4E\r\n",
				"$GAGSV,1,1,04,23,51,194,52,33,24,211,46,12,20,268,41,21,65,069,43,3*7B\r\n",
				"$GQGSV,1,1,02,03,15,146,30,07,56,115,36,1*64\r\n",
				"$GQGSV,1,1,02,03,15,146,42,07,56,115,51,6*67\r\n",
				"$GQGSV,1,1,02,03,15,146,37,07,56,115,47,8*6C\r\n",
				"$GIGSV,1,1,02,02,25,226,44,09,40,297,44,1*7D\r\n",
			},
			numbering: nil, // Use default NMEA 4.11 numbering
			expectedSVs: []gpsprot.SVInfo{
				// GPS satellites
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 26}, LookAngles: &gpsprot.LookAngles{Azimuth: 199, Elevation: 86}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 49}, {ID: "L2P", CN0: 52}, {ID: "L5-Q", CN0: 50}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 31}, LookAngles: &gpsprot.LookAngles{Azimuth: 340, Elevation: 46}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 31}, {ID: "L2P", CN0: 24}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 202, Elevation: 43}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 47}, {ID: "L2P", CN0: 37}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 250, Elevation: 11}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 37}, {ID: "L2P", CN0: 23}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 159, Elevation: 35}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 44}, {ID: "L2P", CN0: 36}, {ID: "L5-Q", CN0: 40}}, Used: true},
				// GLONASS satellites - slot 26 (ID 90) is NOW ACCEPTED after msg.go fix
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 1}, LookAngles: &gpsprot.LookAngles{Azimuth: 58, Elevation: 58}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 33}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 8}, LookAngles: &gpsprot.LookAngles{Azimuth: 122, Elevation: 23}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 39}, {ID: "L2", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 26}, LookAngles: &gpsprot.LookAngles{Azimuth: 126, Elevation: 17}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 38}, {ID: "L2", CN0: 45}}, Used: false}, // Spare slot 26 accepted!
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 344, Elevation: 66}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 51}, {ID: "L2", CN0: 48}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 236, Elevation: 49}, Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 351, Elevation: 30}, Signals: []gpsprot.SignalInfo{{ID: "L2", CN0: 32}}, Used: false},
				// BeiDou satellites
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 3}, LookAngles: &gpsprot.LookAngles{Azimuth: 143, Elevation: 71}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 46}, {ID: "B3I", CN0: 49}, {ID: "B2I", CN0: 50}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 238, Elevation: 67}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 47}, {ID: "B3I", CN0: 50}, {ID: "B2I", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 173, Elevation: 31}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 42}, {ID: "B3I", CN0: 39}, {ID: "B2I", CN0: 46}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 161, Elevation: 34}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 43}, {ID: "B3I", CN0: 45}, {ID: "B2I", CN0: 46}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 259, Elevation: 40}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 43}, {ID: "B3I", CN0: 43}, {ID: "B2I", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 322, Elevation: 54}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 40}, {ID: "B3I", CN0: 38}, {ID: "B2I", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 347, Elevation: 51}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 22}, {ID: "B3I", CN0: 38}, {ID: "B2I", CN0: 37}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 203, Elevation: 76}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 48}, {ID: "B3I", CN0: 49}, {ID: "B2I", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 8}, LookAngles: &gpsprot.LookAngles{Azimuth: 174, Elevation: 66}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 47}, {ID: "B3I", CN0: 49}, {ID: "B2I", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 127, Elevation: 28}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 43}, {ID: "B3I", CN0: 44}, {ID: "B1C", CN0: 41}, {ID: "B2a", CN0: 40}, {ID: "B2b", CN0: 42}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 22}, LookAngles: &gpsprot.LookAngles{Azimuth: 62, Elevation: 71}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 45}, {ID: "B3I", CN0: 43}, {ID: "B1C", CN0: 44}, {ID: "B2a", CN0: 40}, {ID: "B2b", CN0: 42}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 26}, LookAngles: &gpsprot.LookAngles{Azimuth: 245, Elevation: 36}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 48}, {ID: "B3I", CN0: 50}, {ID: "B1C", CN0: 47}, {ID: "B2a", CN0: 44}, {ID: "B2b", CN0: 49}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 21}, LookAngles: &gpsprot.LookAngles{Azimuth: 335, Elevation: 36}, Signals: []gpsprot.SignalInfo{{ID: "B1I", CN0: 34}, {ID: "B3I", CN0: 36}, {ID: "B1C", CN0: 31}, {ID: "B2a", CN0: 35}, {ID: "B2b", CN0: 40}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 1}, LookAngles: &gpsprot.LookAngles{Azimuth: 104, Elevation: 37}, Signals: []gpsprot.SignalInfo{{ID: "B3I", CN0: 31}, {ID: "B2I", CN0: 33}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 359, Elevation: 47}, Signals: []gpsprot.SignalInfo{{ID: "B3I", CN0: 26}, {ID: "B2I", CN0: 27}}, Used: false},
				// Galileo satellites
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 23}, LookAngles: &gpsprot.LookAngles{Azimuth: 194, Elevation: 51}, Signals: []gpsprot.SignalInfo{{ID: "E5a", CN0: 46}, {ID: "E5b", CN0: 51}, {ID: "E1", CN0: 48}, {ID: "E6", CN0: 52}, {ID: "E5", CN0: 52}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 268, Elevation: 20}, Signals: []gpsprot.SignalInfo{{ID: "E5a", CN0: 35}, {ID: "E5b", CN0: 39}, {ID: "E1", CN0: 28}, {ID: "E6", CN0: 38}, {ID: "E5", CN0: 41}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 21}, LookAngles: &gpsprot.LookAngles{Azimuth: 69, Elevation: 65}, Signals: []gpsprot.SignalInfo{{ID: "E5a", CN0: 37}, {ID: "E5b", CN0: 41}, {ID: "E1", CN0: 39}, {ID: "E6", CN0: 41}, {ID: "E5", CN0: 43}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 62, Elevation: 39}, Signals: []gpsprot.SignalInfo{{ID: "E5b", CN0: 20}, {ID: "E1", CN0: 25}, {ID: "E6", CN0: 30}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 357, Elevation: 41}, Signals: []gpsprot.SignalInfo{{ID: "E5b", CN0: 26}, {ID: "E1", CN0: 19}}, Used: true},
				// QZSS satellites
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 3}, LookAngles: &gpsprot.LookAngles{Azimuth: 146, Elevation: 15}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 30}, {ID: "L2C-L", CN0: 42}, {ID: "L5-Q", CN0: 37}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 115, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 36}, {ID: "L2C-L", CN0: 51}, {ID: "L5-Q", CN0: 47}}, Used: true},
				// NAVIC satellites
				{ID: gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 226, Elevation: 25}, Signals: []gpsprot.SignalInfo{{ID: "L5", CN0: 44}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 297, Elevation: 40}, Signals: []gpsprot.SignalInfo{{ID: "L5", CN0: 44}}, Used: false},
			},
		},
		{
			name: "GSV mixed signal IDs in single series (Allystar style)",
			nmeaMessages: []string{
				// Allystar TAU1201 mixes signal IDs within a series: msgs 1-2 have sigID=1, msgs 3-4 have sigID=8
				// msgNum continues (1,2,3,4) instead of restarting at 1 for each sigID
				"$GNRMC,082450.000,A,1343.90828,N,10038.68398,E,0.001,189.06,310126,,,A,S*3C\r\n",
				"$GNGSA,A,3,06,199,30,195,19,03,07,,,,,,1.15,0.60,0.98,1*06\r\n",
				"$GPGSV,4,1,13,14,61,23,29,6,57,238,51,199,55,115,39,30,49,198,49,1*5F\r\n",
				"$GPGSV,4,2,13,195,46,54,22,19,34,315,42,3,26,74,18,7,26,169,44,1*50\r\n",
				"$GPGSV,4,3,13,6,57,238,47,199,55,115,47,30,49,198,50,195,46,54,17,8*62\r\n",
				"$GPGSV,4,4,13,194,44,74,16,8*57\r\n",
				"$GNGLL,1343.90828,N,10038.68398,E,082450.000,A,A*65\r\n",
			},
			expectedSVs: []gpsprot.SVInfo{
				// Satellites parsed from all 4 GSV messages with combined signals
				// SVIDs 199, 195, 194 are beyond valid GPS range and get filtered out
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 14}, LookAngles: &gpsprot.LookAngles{Azimuth: 23, Elevation: 61}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 29}}, Used: false},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 238, Elevation: 57}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 51}, {ID: "L5-Q", CN0: 47}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 30}, LookAngles: &gpsprot.LookAngles{Azimuth: 198, Elevation: 49}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 49}, {ID: "L5-Q", CN0: 50}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 315, Elevation: 34}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 42}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 3}, LookAngles: &gpsprot.LookAngles{Azimuth: 74, Elevation: 26}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 18}}, Used: true},
				{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 169, Elevation: 26}, Signals: []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 44}}, Used: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processor := NewPacketProcessor(gpsprot.NewNavEpochManager())
			if test.numbering != nil {
				processor.SetSVNumbering(test.numbering)
			}
			handler := &testPacketHandler{}
			processor.SetMsgHandler(handler)

			// Process all messages
			testTime := time.Date(2025, 7, 24, 1, 14, 51, 0, time.UTC)
			for i, msg := range test.nmeaMessages {
				msgID, err := processor.ProcessPacket(msg, testTime.Add(time.Duration(i)*time.Millisecond))
				if err != nil {
					t.Fatalf("error processing message %d (%s): %v", i, msgID, err)
				}
			}

			// Call Idle to flush any remaining data
			processor.Idle(testTime.Add(time.Duration(len(test.nmeaMessages)) * time.Millisecond))

			// Verify we got exactly one SatellitesMsg
			if len(handler.satellites) != 1 {
				t.Fatalf("expected exactly 1 SatellitesMsg, got %d", len(handler.satellites))
			}

			actualMsg := handler.satellites[0]
			expectedMsg := &gpsprot.SatellitesMsg{
				Tag:          Tag,
				NativeMsgID:  "GNGSV",
				UsedValidity: gpsprot.SatelliteUsedSV,
				SVs:          test.expectedSVs,
			}

			testCompareSatellitesMsgs(t, actualMsg, expectedMsg)
		})
	}
}

// testCompareSatellitesMsgs compares two SatellitesMsg in an order-insensitive way
func testCompareSatellitesMsgs(t *testing.T, actual, expected *gpsprot.SatellitesMsg) {
	if actual.Tag != expected.Tag {
		t.Errorf("Tag mismatch: got %s, expected %s", actual.Tag, expected.Tag)
	}
	if actual.NativeMsgID != expected.NativeMsgID {
		t.Errorf("NativeMsgID mismatch: got %s, expected %s", actual.NativeMsgID, expected.NativeMsgID)
	}
	if actual.UsedValidity != expected.UsedValidity {
		t.Errorf("UsedValidity mismatch: got %v, expected %v", actual.UsedValidity, expected.UsedValidity)
	}

	if len(actual.SVs) != len(expected.SVs) {
		t.Errorf("Different number of satellites: got %d, expected %d", len(actual.SVs), len(expected.SVs))
		return
	}

	// Create maps for order-insensitive comparison
	actualMap := make(map[gpsprot.SVID]gpsprot.SVInfo)
	expectedMap := make(map[gpsprot.SVID]gpsprot.SVInfo)

	for _, sv := range actual.SVs {
		actualMap[sv.ID] = sv
	}
	for _, sv := range expected.SVs {
		expectedMap[sv.ID] = sv
	}

	// Check each expected satellite exists in actual
	for svid, expectedSV := range expectedMap {
		actualSV, exists := actualMap[svid]
		if !exists {
			t.Errorf("Missing satellite %s in actual results", svid)
			continue
		}

		// Compare satellite fields
		if actualSV.Used != expectedSV.Used {
			t.Errorf("Satellite %s Used mismatch: got %v, expected %v", svid, actualSV.Used, expectedSV.Used)
		}

		// Compare look angles
		if (actualSV.LookAngles == nil) != (expectedSV.LookAngles == nil) {
			t.Errorf("Satellite %s LookAngles nil mismatch: got %v, expected %v", svid, actualSV.LookAngles == nil, expectedSV.LookAngles == nil)
		} else if actualSV.LookAngles != nil {
			if actualSV.LookAngles.Azimuth != expectedSV.LookAngles.Azimuth {
				t.Errorf("Satellite %s Azimuth mismatch: got %d, expected %d", svid, actualSV.LookAngles.Azimuth, expectedSV.LookAngles.Azimuth)
			}
			if actualSV.LookAngles.Elevation != expectedSV.LookAngles.Elevation {
				t.Errorf("Satellite %s Elevation mismatch: got %d, expected %d", svid, actualSV.LookAngles.Elevation, expectedSV.LookAngles.Elevation)
			}
		}

		// Compare signals order-insensitively
		if len(actualSV.Signals) != len(expectedSV.Signals) {
			t.Errorf("Satellite %s signal count mismatch: got %d, expected %d", svid, len(actualSV.Signals), len(expectedSV.Signals))
			continue
		}

		actualSignalMap := make(map[gpsprot.SignalID]gpsprot.SignalInfo)
		expectedSignalMap := make(map[gpsprot.SignalID]gpsprot.SignalInfo)

		for _, sig := range actualSV.Signals {
			actualSignalMap[sig.ID] = sig
		}
		for _, sig := range expectedSV.Signals {
			expectedSignalMap[sig.ID] = sig
		}

		for sigID, expectedSig := range expectedSignalMap {
			actualSig, exists := actualSignalMap[sigID]
			if !exists {
				t.Errorf("Satellite %s missing signal %s", svid, sigID)
				continue
			}
			if actualSig.CN0 != expectedSig.CN0 {
				t.Errorf("Satellite %s signal %s CN0 mismatch: got %d, expected %d", svid, sigID, actualSig.CN0, expectedSig.CN0)
			}
			if actualSig.Used != expectedSig.Used {
				t.Errorf("Satellite %s signal %s Used mismatch: got %v, expected %v", svid, sigID, actualSig.Used, expectedSig.Used)
			}
		}

		// Check for extra signals in actual
		for sigID := range actualSignalMap {
			if _, exists := expectedSignalMap[sigID]; !exists {
				t.Errorf("Satellite %s has unexpected signal %s", svid, sigID)
			}
		}
	}

	// Check for extra satellites in actual
	for svid := range actualMap {
		if _, exists := expectedMap[svid]; !exists {
			t.Errorf("Unexpected satellite %s in actual results", svid)
		}
	}
}
