package asbin

import "testing"

func TestNavTime(t *testing.T) {
	// Example from spec: Get the GPS time
	runTests(t, []testCase{{
		name:   "spec_example",
		packet: "F1D9010510000007 2C79 FF553E16 1000 1200 0600 0000 925A",
		wantID: NavTimeID,
		wantMsg: &NavTime{
			NavSys:  NavTimeSysGPS,
			Flags:   NavTimeFlagWeekValid | NavTimeFlagSecondValid | NavTimeFlagLeapSecValid,
			Fractow: 31020,
			RefTow:  373183999,
			Week:    16,
			LeapSec: 18,
			TimeErr: 6,
		},
	}})
}

func TestNavSvin(t *testing.T) {
	runTests(t, []testCase{
		{
			name:   "windows_app",
			packet: "F1D901310E00 A0D08220 4B000000 07C90000 0101 6F03",
			wantID: NavSvinID,
			wantMsg: &NavSvin{
				NavITOW:    NavITOW{ITow: 545444000},
				PosUsed:    75,
				MeanStdDev: 51463,
				Valid:      1,
				Status:     1,
			},
		},
		{
			name:   "from_log",
			packet: "F1D901310E00 90DCB820 80090000 62FB0000 0101 6CC6",
			wantID: NavSvinID,
			wantMsg: &NavSvin{
				NavITOW:    NavITOW{ITow: 548986000},
				PosUsed:    2432,
				MeanStdDev: 64354,
				Valid:      1,
				Status:     1,
			},
		},
	})
}

func TestPollNavTime(t *testing.T) {
	// Test creating a poll message for NAV-TIME
	expected := "F1D9010501000007 1C"
	pollMsg := PollNavTime(NavTimeSysGPS)
	got := string(pollMsg)
	want := hexToBytes(t, expected)
	if got != string(want) {
		t.Errorf("PollNavTime(GPS):\ngot:  %X\nwant: %X", pollMsg, want)
	}
	if PacketMsgId(pollMsg) != NavTimeID {
		t.Errorf("message ID = %v, want %v", PacketMsgId(pollMsg), NavTimeID)
	}
}

func TestNavTimeUTC(t *testing.T) {
	// Captured from TAU1201: 2026-02-03 00:22:13 UTC
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d9012114005755610a0000000051560b00ea07020300160d273f69",
		wantID: NavTimeUtcID,
		wantMsg: &NavTimeUTC{
			NavITOW:   NavITOW{ITow: 174150999},
			TAcc:      0,
			Nano:      742993,
			Year:      2026,
			Month:     2,
			Day:       3,
			Hour:      0,
			Min:       22,
			Sec:       13,
			ValidFlag: 39, // 0x27: TOW valid, WKN valid, UTC valid, utcStandard=2 (USNO)
		},
	}})
}

func TestNavPosEcef(t *testing.T) {
	// Captured from TAU1201
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d90101140068d47c0ae2542df9ad1c4d24402ef7084d0200002a50",
		wantID: NavPosEcefID,
		wantMsg: &NavPosEcef{
			NavITOW: NavITOW{ITow: 175953000},
			EcefX:   -114469662, // cm, approx -1145 km
			EcefY:   609033389,  // cm, approx 6090 km
			EcefZ:   150416960,  // cm, approx 1504 km
			PAcc:    589,        // cm, 5.89 m
		},
	}})
}

func TestNavPosLlh(t *testing.T) {
	// Captured from TAU1201
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d901021c0068d47c0aac2afd3b7c4f2f08dc91ffff12f9ffff991200008f0d0000ac17",
		wantID: NavPosLlhID,
		wantMsg: &NavPosLlh{
			NavITOW: NavITOW{ITow: 175953000},
			Lon:     1006447276, // 1e-7 deg, 100.6447 E
			Lat:     137318268,  // 1e-7 deg, 13.7318 N (Thailand)
			Height:  -28196,     // mm, -28.2 m (below ellipsoid, normal for SE Asia geoid)
			HMSL:    -1774,      // mm, -1.77 m
			HAcc:    4761,       // mm, 4.76 m
			VAcc:    3471,       // mm, 3.47 m
		},
	}})
}

func TestNavDop(t *testing.T) {
	// Captured from TAU1201
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d90104120068d47c0a95007e0050006700470037002d004ed2",
		wantID: NavDopID,
		wantMsg: &NavDop{
			NavITOW: NavITOW{ITow: 175953000},
			GDOP:    149, // 0.01, 1.49
			PDOP:    126, // 0.01, 1.26
			TDOP:    80,  // 0.01, 0.80
			VDOP:    103, // 0.01, 1.03
			HDOP:    71,  // 0.01, 0.71
			NDOP:    55,  // 0.01, 0.55
			EDOP:    45,  // 0.01, 0.45
		},
	}})
}

func TestNavVelEcef(t *testing.T) {
	// Captured from TAU1201
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d90111140068d47c0a00000000000000000000000004000000eca5",
		wantID: NavVelEcefID,
		wantMsg: &NavVelEcef{
			NavITOW: NavITOW{ITow: 175953000},
			EcefVX:  0,
			EcefVY:  0,
			EcefVZ:  0,
			SAcc:    4, // cm/s
		},
	}})
}

func TestNavVelNed(t *testing.T) {
	// Captured from TAU1201
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d90112240068d47c0a0000000000000000000000000000000000000000653054000000000000000000e2b0",
		wantID: NavVelNedID,
		wantMsg: &NavVelNed{
			NavITOW: NavITOW{ITow: 175953000},
			VelN:    0,
			VelE:    0,
			VelD:    0,
			Speed:   0,
			GSpeed:  0,
			Heading: 5517413, // 1e-5 deg, 55.17 deg
			SAcc:    0,
			CAcc:    0,
		},
	}})
}

func TestNavAuto(t *testing.T) {
	// Captured from TAU1201: 2026-03-04 07:55:26 UTC, 3D fix, 25 SVs used
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d901c0200004ea07030407371afe2afd3b864e2f08388effff000000004d004d007300191aa438",
		wantID: NavAutoID,
		wantMsg: &NavAuto{
			FixState:  4, // 3D fix
			Year:      2026,
			Month:     3,
			Day:       4,
			Hour:      7,
			Min:       55,
			Sec:       26,
			Lon:       1006447358, // 1e-7 deg, 100.6447 E
			Lat:       137318022,  // 1e-7 deg, 13.7318 N
			Alt:       -29128,     // mm, -29.1 m (below ellipsoid)
			Speed:     0,          // cm/s, stationary
			Heading:   0,          // 1e-2 deg
			PDOP:      77,         // 0.01, 0.77
			HDOP:      77,         // 0.01, 0.77
			VDOP:      115,        // 0.01, 1.15
			SatInUse:  25,
			SatInView: 26,
		},
	}})
}

func TestNavSVInfo(t *testing.T) {
	// Captured from TAU1201: 26 satellites
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d90130780200b1620a1a000000e8000e0631509e00e5ffffff20995ac3f447bf33a2ed744110000e062e4fbcffe2ffffff3106a7c3a88a5f1eeafc724144000e062a4712001b000000642814c40e382f2120737241cb000e062d468f00a0000000f1bba1c3cdafd2614a21814104010e06343d87ff56ffffff9728a1c31333c6ec0b688141d5000e062d3767ff19000000f195a5c2a35378da547c8141c7000e0626377300090000008013a4c3aee16c80978a81414f000e062d33ccff82000000848505c3dd5d05a1404d7341ed000e061d33efff76000000594dd8c21d6966218ce275411a00020617331100702cfffff1bcde4265f3600175287441cf000e0628309a00a600000045a507c4275bd2df46bc814143000e062f2e880009ffffffc927d642349d51ba558b7341d1000e061e2decffceffffff999571c311accab56c188241d2000e062b2aac00f9ffffff112b03c4e3f7f2378afd8141cd000e0629289bffd4fffffffcada3c3fb2268a8b6218241f0000e0629288a0005000000996818c473110f291e21824103000e062727bcff3b0000002c8bf2c381ef5557f9e07441dc000e062526ddff0d00000008a546c452e591a04abc7641ee000e062c24b100f7ffffffaec622c2a711cd129f1382411b000e062c22a400a9ffffffaac85cc48174e162c362754120000e06241c7600270000007ceb2642f5119ae48ac77541f1000e0623179200a5ffffff881c68436b1af39782e9774150000e062c168dff89000000f50527c443c17ebb275a7541e4000e0626107d0055ffffffa65c0ec40ecf04a4ab27784108000e06230f58ff53000000149a70c4a47b4c725ce07641e3000e06230ea600d5ffffff92b218c49b5f9f28d84e7841b6e4",
		wantID: NavSvInfoID,
		wantMsg: &NavSVInfo{
			NavSVInfoFixed: NavSVInfoFixed{
				NavITOW: NavITOW{ITow: 174240000},
				NumCh:   26,
			},
			Sats: []NavSVInfoSat{
				{Svid: 232, Flags: 14, Quality: 6, Cno: 49, Elev: 80, Azim: 158, PrRes: -27, PseudorangeRate: -218.59814, Pseudorange: 2.194486723419948e+07},
				{Svid: 16, Flags: 14, Quality: 6, Cno: 46, Elev: 79, Azim: -68, PrRes: -30, PseudorangeRate: -334.04837, Pseudorange: 1.9910305898325592e+07},
				{Svid: 68, Flags: 14, Quality: 6, Cno: 42, Elev: 71, Azim: 18, PrRes: 27, PseudorangeRate: -592.6311, Pseudorange: 1.9345922074028067e+07},
				{Svid: 203, Flags: 14, Quality: 6, Cno: 45, Elev: 70, Azim: 143, PrRes: 160, PseudorangeRate: -323.4683, Pseudorange: 3.5924300227874376e+07},
				{Svid: 260, Flags: 14, Quality: 6, Cno: 52, Elev: 61, Azim: -121, PrRes: -170, PseudorangeRate: -322.3171, Pseudorange: 3.6503933596777104e+07},
				{Svid: 213, Flags: 14, Quality: 6, Cno: 45, Elev: 55, Azim: -153, PrRes: 25, PseudorangeRate: -82.792854, Pseudorange: 3.6670107308753274e+07},
				{Svid: 199, Flags: 14, Quality: 6, Cno: 38, Elev: 55, Azim: 115, PrRes: 9, PseudorangeRate: -328.15234, Pseudorange: 3.6786928053164825e+07},
				{Svid: 79, Flags: 14, Quality: 6, Cno: 45, Elev: 51, Azim: -52, PrRes: 130, PseudorangeRate: -133.52155, Pseudorange: 2.0239370063810218e+07},
				{Svid: 237, Flags: 14, Quality: 6, Cno: 29, Elev: 51, Azim: -17, PrRes: 118, PseudorangeRate: -108.15107, Pseudorange: 2.2948034087502588e+07},
				{Svid: 26, Flags: 2, Quality: 6, Cno: 23, Elev: 51, Azim: 17, PrRes: -54160, PseudorangeRate: 111.369026, Pseudorange: 2.113723208616962e+07},
				{Svid: 207, Flags: 14, Quality: 6, Cno: 40, Elev: 48, Azim: 154, PrRes: 166, PseudorangeRate: -542.58234, Pseudorange: 3.719394797771292e+07},
				{Svid: 67, Flags: 14, Quality: 6, Cno: 47, Elev: 46, Azim: 136, PrRes: -247, PseudorangeRate: 107.077705, Pseudorange: 2.049365964492531e+07},
				{Svid: 209, Flags: 14, Quality: 6, Cno: 30, Elev: 45, Azim: -20, PrRes: -50, PseudorangeRate: -241.58437, Pseudorange: 3.7948822723961e+07},
				{Svid: 210, Flags: 14, Quality: 6, Cno: 43, Elev: 42, Azim: 172, PrRes: -7, PseudorangeRate: -524.6729, Pseudorange: 3.772858299363687e+07},
				{Svid: 205, Flags: 14, Quality: 6, Cno: 41, Elev: 40, Azim: -101, PrRes: -44, PseudorangeRate: -327.35925, Pseudorange: 3.802491705084797e+07},
				{Svid: 240, Flags: 14, Quality: 6, Cno: 41, Elev: 40, Azim: 138, PrRes: 5, PseudorangeRate: -609.63434, Pseudorange: 3.80200371323575e+07},
				{Svid: 3, Flags: 14, Quality: 6, Cno: 39, Elev: 39, Azim: -68, PrRes: 59, PseudorangeRate: -485.08728, Pseudorange: 2.189301345848036e+07},
				{Svid: 220, Flags: 14, Quality: 6, Cno: 37, Elev: 38, Azim: -35, PrRes: 13, PseudorangeRate: -794.5786, Pseudorange: 2.3839914035619088e+07},
				{Svid: 238, Flags: 14, Quality: 6, Cno: 44, Elev: 36, Azim: 177, PrRes: -9, PseudorangeRate: -40.694023, Pseudorange: 3.7909474350131325e+07},
				{Svid: 27, Flags: 14, Quality: 6, Cno: 44, Elev: 34, Azim: 164, PrRes: -87, PseudorangeRate: -883.1354, Pseudorange: 2.2424630180042747e+07},
				{Svid: 32, Flags: 14, Quality: 6, Cno: 36, Elev: 28, Azim: 118, PrRes: 39, PseudorangeRate: 41.729965, Pseudorange: 2.283742228761478e+07},
				{Svid: 241, Flags: 14, Quality: 6, Cno: 35, Elev: 23, Azim: 146, PrRes: -91, PseudorangeRate: 232.11145, Pseudorange: 2.5073705496851366e+07},
				{Svid: 80, Flags: 14, Quality: 6, Cno: 44, Elev: 22, Azim: -115, PrRes: 137, PseudorangeRate: -668.0931, Pseudorange: 2.2389371718446027e+07},
				{Svid: 228, Flags: 14, Quality: 6, Cno: 38, Elev: 16, Azim: 125, PrRes: -171, PseudorangeRate: -569.44763, Pseudorange: 2.5328314251174025e+07},
				{Svid: 8, Flags: 14, Quality: 6, Cno: 35, Elev: 15, Azim: -168, PrRes: 83, PseudorangeRate: -962.4075, Pseudorange: 2.39876551436726e+07},
				{Svid: 227, Flags: 14, Quality: 6, Cno: 35, Elev: 14, Azim: 166, PrRes: -43, PseudorangeRate: -610.79016, Pseudorange: 2.5488770538909536e+07},
			},
		},
	}})
}
