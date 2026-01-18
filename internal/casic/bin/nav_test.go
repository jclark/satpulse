package bin

import (
	"testing"
)

// Test data from casic-messages.jsonl

func TestNavSolParse(t *testing.T) {
	// NAV-SOL packet from test data
	// {"t": "2026-01-18T03:51:55.397511Z", "tag": "CSIP", "msg": "NAV-SOL", ...}
	pkt := parseHex(t, "bace48000102418cc804070700031909100072006209e60800008036cb40bae82a46797731c1bf42b680973b5741053d8e86a7f3364107c0c93f00000000000000000000000088ea6b39a4c2893fe958f59d")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	sol, ok := msg.(*NavSol)
	if !ok {
		t.Fatalf("expected *NavSol, got %T", msg)
	}
	// Verify parsed fields
	if sol.RunTime != 0x04c88c41 {
		t.Errorf("RunTime = 0x%x, want 0x%x", sol.RunTime, 0x04c88c41)
	}
	if sol.PosValid != NavPos3D {
		t.Errorf("PosValid = %d, want %d", sol.PosValid, NavPos3D)
	}
	if sol.VelValid != NavVel3D {
		t.Errorf("VelValid = %d, want %d", sol.VelValid, NavVel3D)
	}
	if sol.TimeSrc != NavTimeSrcGPS {
		t.Errorf("TimeSrc = %d, want %d", sol.TimeSrc, NavTimeSrcGPS)
	}
	if sol.System != (NavSystemGPS | NavSystemBDS) {
		t.Errorf("System = 0x%x, want 0x%x", sol.System, NavSystemGPS|NavSystemBDS)
	}
	if sol.NumSV != 25 {
		t.Errorf("NumSV = %d, want 25", sol.NumSV)
	}
	if sol.NumSVGPS != 9 {
		t.Errorf("NumSVGPS = %d, want 9", sol.NumSVGPS)
	}
	if sol.NumSVBDS != 16 {
		t.Errorf("NumSVBDS = %d, want 16", sol.NumSVBDS)
	}
	if sol.NumSVGLN != 0 {
		t.Errorf("NumSVGLN = %d, want 0", sol.NumSVGLN)
	}
	if sol.Week != 2402 {
		t.Errorf("Week = %d, want 2402", sol.Week)
	}
	// TOW should be around 13934 seconds (3:51:55 into GPS week on Sunday)
	if sol.TOW < 13900 || sol.TOW > 14000 {
		t.Errorf("TOW = %f, expected ~13934", sol.TOW)
	}
}

func TestNavSolRoundtrip(t *testing.T) {
	m := NavSol{
		NavRunTime: NavRunTime{RunTime: 12345},
		PosValid:   NavPos3D,
		VelValid:   NavVel3D,
		TimeSrc:    NavTimeSrcGPS,
		System:     NavSystemGPS | NavSystemBDS,
		NumSV:      10,
		NumSVGPS:   6,
		NumSVBDS:   4,
		NumSVGLN:   0,
		Week:       2402,
		TOW:        123456.789,
		ECEFX:      -2694892.5,
		ECEFY:      4297123.5,
		ECEFZ:      -3854123.5,
		PAcc:       25.0,
		ECEFVX:     0.1,
		ECEFVY:     -0.2,
		ECEFVZ:     0.05,
		SAcc:       0.01,
		PDOP:       1.5,
	}
	testMsgType(t, m)
}

func TestNavTimeUTCParse(t *testing.T) {
	// NAV-TIMEUTC packet from test data
	// {"t": "2026-01-18T03:51:56.507218Z", "tag": "CSIP", "msg": "NAV-TIMEUTC", ...}
	pkt := parseHex(t, "bace180001102890c804ee3d8940004959b70000ea0701120333380700036730994a")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	utc, ok := msg.(*NavTimeUTC)
	if !ok {
		t.Fatalf("expected *NavTimeUTC, got %T", msg)
	}
	// Verify parsed fields
	if utc.RunTime != 0x04c89028 {
		t.Errorf("RunTime = 0x%x, want 0x%x", utc.RunTime, 0x04c89028)
	}
	if utc.Year != 2026 {
		t.Errorf("Year = %d, want 2026", utc.Year)
	}
	if utc.Month != 1 {
		t.Errorf("Month = %d, want 1", utc.Month)
	}
	if utc.Day != 18 {
		t.Errorf("Day = %d, want 18", utc.Day)
	}
	if utc.Hour != 3 {
		t.Errorf("Hour = %d, want 3", utc.Hour)
	}
	if utc.Min != 51 {
		t.Errorf("Min = %d, want 51", utc.Min)
	}
	if utc.Sec != 56 {
		t.Errorf("Sec = %d, want 56", utc.Sec)
	}
	if utc.Valid != 7 { // all valid bits set
		t.Errorf("Valid = %d, want 7", utc.Valid)
	}
	if utc.TimeSrc != NavTimeSrcGPS {
		t.Errorf("TimeSrc = %d, want %d", utc.TimeSrc, NavTimeSrcGPS)
	}
	if utc.DateValid != NavDateMultipleSats {
		t.Errorf("DateValid = %d, want %d", utc.DateValid, NavDateMultipleSats)
	}
}

func TestNavTimeUTCRoundtrip(t *testing.T) {
	m := NavTimeUTC{
		NavRunTime: NavRunTime{RunTime: 12345},
		TAcc:       0.000001,
		MsErr:      0.0001,
		Year:       2026,
		Month:      1,
		Day:        18,
		Hour:       12,
		Min:        30,
		Sec:        45,
		Valid:      NavTimeUTCTOWValid | NavTimeUTCWeekValid | NavTimeUTCLeapValid,
		TimeSrc:    NavTimeSrcGPS,
		DateValid:  NavDateMultipleSats,
	}
	testMsgType(t, m)
}

func TestNavClockParse(t *testing.T) {
	// NAV-CLOCK packet from test data
	pkt := parseHex(t, "bace400001112890c8045a8de8c3ee3d8940ca2e8f38fee8ffffb5936a41902608316209120707000000e08c6a41000000b116040407000000000000000000000000000000031cc8bdc8")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	clk, ok := msg.(*NavClock)
	if !ok {
		t.Fatalf("expected *NavClock, got %T", msg)
	}
	if clk.RunTime != 0x04c89028 {
		t.Errorf("RunTime = 0x%x, want 0x%x", clk.RunTime, 0x04c89028)
	}
	// Check GPS system data (index 0)
	if clk.Systems[0].Wn != 2402 {
		t.Errorf("GPS Week = %d, want 2402", clk.Systems[0].Wn)
	}
	if clk.Systems[0].Leaps != 18 {
		t.Errorf("GPS Leaps = %d, want 18", clk.Systems[0].Leaps)
	}
}

func TestNavClockRoundtrip(t *testing.T) {
	m := NavClock{
		NavRunTime: NavRunTime{RunTime: 12345},
		FreqBias:   0.0001,
		TAcc:       0.00001,
		FAcc:       0.000001,
		Systems: [3]NavClockSys{
			{TOW: 123456789.0, DtUTC: 0.001, Wn: 2402, Leaps: 18, Valid: 7},
			{TOW: 123456789.0, DtUTC: 0.002, Wn: 850, Leaps: 4, Valid: 7},
			{},
		},
	}
	testMsgType(t, m)
}

func TestNavGPSInfoParse(t *testing.T) {
	// NAV-GPSINFO packet from test data (13 satellites)
	pkt := parseHex(t, "bacea40001202890c8040d0900001001c0201508bb00000000001c02c1611c15a0009d6e4f400503c1632514d5007b9220c01204c1632e4d1f018e1d103f1f07c1611d0a3c0155ad61410c08c1632e5293005220d33e1409c163272b3a01f74591bf0f10c020141a1d00000000000e1bc1611f38310078d6ea400a1fc1211c155c00e62ff7c50bc2c02016293000000000001dc3c1611d187600e5278ec006c700632600000000000000d5613570")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	info, ok := msg.(*NavGPSInfo)
	if !ok {
		t.Fatalf("expected *NavGPSInfo, got %T", msg)
	}
	if info.RunTime != 0x04c89028 {
		t.Errorf("RunTime = 0x%x, want 0x%x", info.RunTime, 0x04c89028)
	}
	if info.NumViewSV != 13 {
		t.Errorf("NumViewSV = %d, want 13", info.NumViewSV)
	}
	if info.NumFixSV != 9 {
		t.Errorf("NumFixSV = %d, want 9", info.NumFixSV)
	}
	if info.System != NavInfoSysGPS {
		t.Errorf("System = %d, want %d", info.System, NavInfoSysGPS)
	}
	if len(info.SVs) != 13 {
		t.Fatalf("len(SVs) = %d, want 13", len(info.SVs))
	}
	// Check first satellite
	sv := info.SVs[0]
	if sv.Chn != 16 {
		t.Errorf("SV[0].Chn = %d, want 16", sv.Chn)
	}
	if sv.SVID != 1 {
		t.Errorf("SV[0].SVID = %d, want 1", sv.SVID)
	}
}

func TestNavGPSInfoRoundtrip(t *testing.T) {
	m := NavGPSInfo{
		NavGPSInfoFixed: NavGPSInfoFixed{
			NavRunTime: NavRunTime{RunTime: 12345},
			NumViewSV:  3,
			NumFixSV:   2,
			System:     NavInfoSysGPS,
		},
		SVs: []NavSVInfo{
			{Chn: 1, SVID: 5, Flags: NavSVUsed, Quality: NavSVQualityPRValid, CNO: 45, Elev: 30, Azim: 120, PRRes: 1.5},
			{Chn: 2, SVID: 10, Flags: NavSVUsed, Quality: NavSVQualityPRValid, CNO: 40, Elev: 45, Azim: 200, PRRes: -0.5},
			{Chn: 3, SVID: 15, Flags: 0, Quality: 0, CNO: 30, Elev: 15, Azim: 300, PRRes: 2.0},
		},
	}
	p2 := testMsgType1[NavGPSInfo, *NavGPSInfo](t, m)
	m2 := p2.(*NavGPSInfo)
	if m2.NumViewSV != m.NumViewSV {
		t.Errorf("NumViewSV = %d, want %d", m2.NumViewSV, m.NumViewSV)
	}
	if len(m2.SVs) != len(m.SVs) {
		t.Fatalf("len(SVs) = %d, want %d", len(m2.SVs), len(m.SVs))
	}
	for i, sv := range m.SVs {
		if m2.SVs[i] != sv {
			t.Errorf("SV[%d] = %+v, want %+v", i, m2.SVs[i], sv)
		}
	}
}

func TestNavBDSInfoParse(t *testing.T) {
	// NAV-BDSINFO packet from test data (23 satellites)
	pkt := parseHex(t, "bace1c0101212890c804171001001302c1632e41e700148bf33e1503c1632c45920032bd21bd0205c16328280001ca991bbf0906c1611930190098141f410307c1632d518f0042baaebe1908c1611910bf0009dcb6c0ff09c000002d0100000000001a0ac1632a47c400dab9103eff0bc0000008660000000000040dc1632315c900a259c4be1b10c121112f1c00490ded3fff13c0000006a100000000001615c1632e2b1601635b963e0816c163332bc80094ac0240011dc020132b560000000000ff1ec00000082f00000000002023c1632b27a900a52b17bfff24c0000014370000000000ff26c000000cb000000000001827c121112f2e00030140c11728c163314a69006487c8be072dc12116270300df7b6340113bc1211f2a6c00abc8f63dfdee347b")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	info, ok := msg.(*NavBDSInfo)
	if !ok {
		t.Fatalf("expected *NavBDSInfo, got %T", msg)
	}
	if info.NumViewSV != 23 {
		t.Errorf("NumViewSV = %d, want 23", info.NumViewSV)
	}
	if info.NumFixSV != 16 {
		t.Errorf("NumFixSV = %d, want 16", info.NumFixSV)
	}
	if info.System != NavInfoSysBDS {
		t.Errorf("System = %d, want %d", info.System, NavInfoSysBDS)
	}
	if len(info.SVs) != 23 {
		t.Fatalf("len(SVs) = %d, want 23", len(info.SVs))
	}
}

func TestNavGLNInfoParse(t *testing.T) {
	// NAV-GLNINFO packet from test data (empty, 0 satellites)
	pkt := parseHex(t, "bace080001222890c804000002003090cb26")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	info, ok := msg.(*NavGLNInfo)
	if !ok {
		t.Fatalf("expected *NavGLNInfo, got %T", msg)
	}
	if info.NumViewSV != 0 {
		t.Errorf("NumViewSV = %d, want 0", info.NumViewSV)
	}
	if info.NumFixSV != 0 {
		t.Errorf("NumFixSV = %d, want 0", info.NumFixSV)
	}
	if info.System != NavInfoSysGLN {
		t.Errorf("System = %d, want %d", info.System, NavInfoSysGLN)
	}
	if len(info.SVs) != 0 {
		t.Fatalf("len(SVs) = %d, want 0", len(info.SVs))
	}
}
