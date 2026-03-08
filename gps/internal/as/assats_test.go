package as

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestAsSVID(t *testing.T) {
	// Test cases based on verified SVID mapping from packet capture analysis.
	// See plan/allystar-nav-svinfo-svid.md for correlation with NMEA GSV data.
	tests := []struct {
		name     string
		svid     uint16
		wantSVID gpsprot.SVID
	}{
		// GPS (1-32): Num = SVID
		// Verified: 1,2,3,4,8,9,31 from packet capture
		{"GPS 1", 1, gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}},
		{"GPS 8", 8, gpsprot.SVID{GNSS: gpsprot.GPS, Num: 8}},
		{"GPS 31", 31, gpsprot.SVID{GNSS: gpsprot.GPS, Num: 31}},
		{"GPS 32", 32, gpsprot.SVID{GNSS: gpsprot.GPS, Num: 32}},
		// SBAS NMEA-style (33-64): Num = SVID + 87 (gives SBAS PRN 120-151)
		// Unverified - no SBAS in capture.
		{"SBAS NMEA 33", 33, gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 120}},
		{"SBAS NMEA 64", 64, gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 151}},
		// SBAS native PRN (120-158): Num = SVID
		// Unverified - no SBAS in capture.
		{"SBAS native 120", 120, gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 120}},
		{"SBAS native 138", 138, gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 138}},
		{"SBAS native 158", 158, gpsprot.SVID{GNSS: gpsprot.SBAS, Num: 158}},
		// GLONASS (65-96): Num = SVID - 64 (gives slot 1-32)
		// Verified: 70->R06, 72->R08, 74->R10, 84->R20 from packet capture
		{"GLO 65", 65, gpsprot.SVID{GNSS: gpsprot.GLO, Num: 1}},
		{"GLO 70", 70, gpsprot.SVID{GNSS: gpsprot.GLO, Num: 6}},
		{"GLO 84", 84, gpsprot.SVID{GNSS: gpsprot.GLO, Num: 20}},
		{"GLO 96", 96, gpsprot.SVID{GNSS: gpsprot.GLO, Num: 32}},
		// QZSS (193-199): Num = SVID - 192 (gives PRN 1-7)
		// Verified: 199->J07 from packet capture
		{"QZSS 193", 193, gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 1}},
		{"QZSS 199", 199, gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 7}},
		// BeiDou (201-263): Num = SVID - 200 (gives PRN 1-63)
		// Verified: 203->C03, 207->C07, 210->C10, 227->C27, 240->C40, 260->C60, etc.
		{"BDS 201", 201, gpsprot.SVID{GNSS: gpsprot.BDS, Num: 1}},
		{"BDS 207", 207, gpsprot.SVID{GNSS: gpsprot.BDS, Num: 7}},
		{"BDS 240", 240, gpsprot.SVID{GNSS: gpsprot.BDS, Num: 40}},
		{"BDS 263", 263, gpsprot.SVID{GNSS: gpsprot.BDS, Num: 63}},
		// Galileo (301-336): Num = SVID - 300 (gives PRN 1-36)
		// Verified: 304->E04, 309->E09 from packet capture
		{"GAL 301", 301, gpsprot.SVID{GNSS: gpsprot.GAL, Num: 1}},
		{"GAL 304", 304, gpsprot.SVID{GNSS: gpsprot.GAL, Num: 4}},
		{"GAL 309", 309, gpsprot.SVID{GNSS: gpsprot.GAL, Num: 9}},
		{"GAL 336", 336, gpsprot.SVID{GNSS: gpsprot.GAL, Num: 36}},
		// NavIC NMEA-style (401-414): Num = SVID - 400 (gives PRN 1-14)
		// Unverified - no NavIC in capture.
		{"NavIC NMEA 401", 401, gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 1}},
		{"NavIC NMEA 414", 414, gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 14}},
		// NavIC Allystar extended (901-914): Num = SVID - 900 (gives PRN 1-14)
		// Unverified - no NavIC in capture.
		{"NavIC ext 901", 901, gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 1}},
		{"NavIC ext 914", 914, gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: 14}},
		// Invalid SVIDs (gaps in the SVID space)
		{"invalid 0", 0, gpsprot.SVID{}},
		{"invalid 100", 100, gpsprot.SVID{}},   // gap between GLO and SBAS native
		{"invalid 170", 170, gpsprot.SVID{}},   // gap between SBAS native and QZSS
		{"invalid 500", 500, gpsprot.SVID{}},   // gap between NavIC NMEA and extended
		{"invalid 1000", 1000, gpsprot.SVID{}}, // beyond NavIC extended
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := asSVID(tc.svid)
			if got != tc.wantSVID {
				t.Errorf("asSVID(%d) = %v, want %v", tc.svid, got, tc.wantSVID)
			}
		})
	}
}

func TestSatellitesNavSVInfo(t *testing.T) {
	tests := []struct {
		name   string
		input  asbin.NavSVInfo
		expect *gpsprot.SatellitesMsg
	}{
		{
			name: "empty input",
			input: asbin.NavSVInfo{
				NavSVInfoFixed: asbin.NavSVInfoFixed{NavITOW: asbin.NavITOW{ITow: 1000}, NumCh: 0},
				Sats:           nil,
			},
			expect: &gpsprot.SatellitesMsg{
				SVs:          []gpsprot.SVInfo{},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "filter low quality",
			input: asbin.NavSVInfo{
				NavSVInfoFixed: asbin.NavSVInfoFixed{NavITOW: asbin.NavITOW{ITow: 1000}, NumCh: 2},
				Sats: []asbin.NavSVInfoSat{
					{Svid: 1, Quality: asbin.NavSVInfoQualitySearching, Cno: 30},
					{Svid: 2, Quality: asbin.NavSVInfoQualityCodeLocked, Cno: 40},
				},
			},
			expect: &gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID:         gpsprot.SVID{GNSS: gpsprot.GPS, Num: 2},
						Signals:    []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 40}},
						LookAngles: &gpsprot.LookAngles{Azimuth: 0, Elevation: 0},
						Used:       false,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "used flag",
			input: asbin.NavSVInfo{
				NavSVInfoFixed: asbin.NavSVInfoFixed{NavITOW: asbin.NavITOW{ITow: 1000}, NumCh: 1},
				Sats: []asbin.NavSVInfoSat{
					{
						Svid:    5,
						Quality: asbin.NavSVInfoQualityCodeCarrierLocked,
						Flags:   asbin.NavSVInfoFlagUsed | asbin.NavSVInfoFlagEphemeris,
						Cno:     45,
						Elev:    60,
						Azim:    180,
					},
				},
			},
			expect: &gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID:         gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5},
						Signals:    []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 45, Used: true}},
						LookAngles: &gpsprot.LookAngles{Azimuth: 180, Elevation: 60},
						Used:       true,
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "multi-constellation from packet capture",
			// Real SVIDs from allystar.jsonl packet capture (2026-02-04T03:32:42Z)
			input: asbin.NavSVInfo{
				NavSVInfoFixed: asbin.NavSVInfoFixed{NavITOW: asbin.NavITOW{ITow: 271980000}, NumCh: 4},
				Sats: []asbin.NavSVInfoSat{
					{Svid: 8, Quality: asbin.NavSVInfoQualityCodeLocked, Cno: 36, Elev: 68, Azim: 24},    // GPS 8
					{Svid: 309, Quality: asbin.NavSVInfoQualityCodeLocked, Cno: 50, Elev: 73, Azim: -82}, // Galileo E09
					{Svid: 207, Quality: asbin.NavSVInfoQualityCodeLocked, Cno: 44, Elev: 86, Azim: 15},  // BeiDou C07
					{Svid: 84, Quality: asbin.NavSVInfoQualityCodeLocked, Cno: 41, Elev: 33, Azim: 157},  // GLONASS R20
				},
			},
			expect: &gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID:         gpsprot.SVID{GNSS: gpsprot.GPS, Num: 8},
						Signals:    []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 36}},
						LookAngles: &gpsprot.LookAngles{Azimuth: 24, Elevation: 68},
					},
					{
						ID:         gpsprot.SVID{GNSS: gpsprot.GAL, Num: 9},
						Signals:    []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1, CN0: 50}},
						LookAngles: &gpsprot.LookAngles{Azimuth: -82, Elevation: 73},
					},
					{
						ID:         gpsprot.SVID{GNSS: gpsprot.BDS, Num: 7},
						Signals:    []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 44}},
						LookAngles: &gpsprot.LookAngles{Azimuth: 15, Elevation: 86},
					},
					{
						ID:         gpsprot.SVID{GNSS: gpsprot.GLO, Num: 20},
						Signals:    []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 41}},
						LookAngles: &gpsprot.LookAngles{Azimuth: 157, Elevation: 33},
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
		{
			name: "filter invalid svid",
			input: asbin.NavSVInfo{
				NavSVInfoFixed: asbin.NavSVInfoFixed{NavITOW: asbin.NavITOW{ITow: 1000}, NumCh: 2},
				Sats: []asbin.NavSVInfoSat{
					{Svid: 100, Quality: asbin.NavSVInfoQualityCodeLocked, Cno: 30}, // invalid range
					{Svid: 5, Quality: asbin.NavSVInfoQualityCodeLocked, Cno: 40},   // valid GPS
				},
			},
			expect: &gpsprot.SatellitesMsg{
				SVs: []gpsprot.SVInfo{
					{
						ID:         gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5},
						Signals:    []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 40}},
						LookAngles: &gpsprot.LookAngles{Azimuth: 0, Elevation: 0},
					},
				},
				Tag:          Tag,
				NativeMsgID:  "NAV-SVINFO",
				UsedValidity: gpsprot.SatelliteUsedSignal,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := satellitesNavSVInfo(&tc.input)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("satellitesNavSVInfo() = %+v, want %+v", got, tc.expect)
			}
		})
	}
}
