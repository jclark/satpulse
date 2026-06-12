package casbin

import (
	"math"
	"testing"
)

// Test data captured from ATGM332D-6N74 (V6 firmware), 2026-03-04

func TestNav2DopParse(t *testing.T) {
	pkt := parseHex(t, "bace18001101c048653f0d07023f6fd93c3f7583ad3e88b4c13e4e80093f9fe12d7b")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	dop, ok := msg.(*Nav2Dop)
	if !ok {
		t.Fatalf("expected *Nav2Dop, got %T", msg)
	}
	if math.Abs(float64(dop.PDOP)-0.896) > 0.01 {
		t.Errorf("PDOP = %f, want ~0.896", dop.PDOP)
	}
	if math.Abs(float64(dop.HDOP)-0.508) > 0.01 {
		t.Errorf("HDOP = %f, want ~0.508", dop.HDOP)
	}
	if math.Abs(float64(dop.VDOP)-0.738) > 0.01 {
		t.Errorf("VDOP = %f, want ~0.738", dop.VDOP)
	}
}

func TestNav2DopRoundtrip(t *testing.T) {
	m := Nav2Dop{
		PDOP: 1.5, HDOP: 0.8, VDOP: 1.2,
		NDOP: 0.5, EDOP: 0.6, TDOP: 0.9,
	}
	testMsgType(t, m)
}

func TestNav2SolParse(t *testing.T) {
	pkt := parseHex(t, "bace48001102b8008d0f680900000707001f2a091506040200000000000075785bdb787731c116c7eb439b3b574114d3d3ffa6f33641d0624b40e65b323ce974b63cac9d383ccc91443ec048653fcc81a40c")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	sol, ok := msg.(*Nav2Sol)
	if !ok {
		t.Fatalf("expected *Nav2Sol, got %T", msg)
	}
	if sol.TOW != 0x0f8d00b8 {
		t.Errorf("TOW = 0x%x, want 0x%x", sol.TOW, 0x0f8d00b8)
	}
	if sol.Wn != 2408 {
		t.Errorf("Wn = %d, want 2408", sol.Wn)
	}
	if sol.FixFlags != PVT3D {
		t.Errorf("FixFlags = %d, want %d", sol.FixFlags, PVT3D)
	}
	if sol.VelFlags != Nav2Vel3D {
		t.Errorf("VelFlags = %d, want %d", sol.VelFlags, Nav2Vel3D)
	}
	if sol.GnssMask != Nav2GnssGPS|Nav2GnssBDS|Nav2GnssGLN|Nav2GnssGAL|Nav2GnssQZSS {
		t.Errorf("GnssMask = 0x%x, want 0x%x", sol.GnssMask, Nav2GnssGPS|Nav2GnssBDS|Nav2GnssGLN|Nav2GnssGAL|Nav2GnssQZSS)
	}
	if sol.NumFixTot != 42 {
		t.Errorf("NumFixTot = %d, want 42", sol.NumFixTot)
	}
	if sol.NumFixGPS != 9 {
		t.Errorf("NumFixGPS = %d, want 9", sol.NumFixGPS)
	}
	if sol.NumFixBDS != 21 {
		t.Errorf("NumFixBDS = %d, want 21", sol.NumFixBDS)
	}
	if sol.NumFixGLN != 6 {
		t.Errorf("NumFixGLN = %d, want 6", sol.NumFixGLN)
	}
	if sol.NumFixGAL != 4 {
		t.Errorf("NumFixGAL = %d, want 4", sol.NumFixGAL)
	}
	if sol.NumFixQZSS != 2 {
		t.Errorf("NumFixQZSS = %d, want 2", sol.NumFixQZSS)
	}
	if math.Abs(float64(sol.PDOP)-0.896) > 0.01 {
		t.Errorf("PDOP = %f, want ~0.896", sol.PDOP)
	}
}

func TestNav2SolRoundtrip(t *testing.T) {
	m := Nav2Sol{
		Nav2TOW:   Nav2TOW{TOW: 260881000},
		Wn:        2408,
		FixFlags:  PVT3D,
		VelFlags:  Nav2Vel3D,
		GnssMask:  Nav2GnssGPS | Nav2GnssBDS,
		NumFixTot: 20,
		NumFixGPS: 8,
		NumFixBDS: 12,
		X:         -1168000.5,
		Y:         5673000.5,
		Z:         1497000.5,
		PAcc:      3.2,
		VX:        0.01,
		VY:        -0.02,
		VZ:        0.005,
		SAcc:      0.19,
		PDOP:      0.9,
	}
	testMsgType(t, m)
}

func TestNav2PvhParse(t *testing.T) {
	pkt := parseHex(t, "bace58001103b8008d0f680900000707001f2a0915060402000068090000bcfdc9d342295940943261ccaa762b40d1c064c11626f4c121b272bc920ecc3be419b43c3114df3c8da2833cd0798e436ab41140bfe60d40cc91443e69ba4541bbcf49cd")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	pvh, ok := msg.(*Nav2Pvh)
	if !ok {
		t.Fatalf("expected *Nav2Pvh, got %T", msg)
	}
	if pvh.TOW != 0x0f8d00b8 {
		t.Errorf("TOW = 0x%x, want 0x%x", pvh.TOW, 0x0f8d00b8)
	}
	if pvh.FixFlags != PVT3D {
		t.Errorf("FixFlags = %d, want %d", pvh.FixFlags, PVT3D)
	}
	if pvh.NumFixTot != 42 {
		t.Errorf("NumFixTot = %d, want 42", pvh.NumFixTot)
	}
	// Lon ~100.644, Lat ~13.732 (Thailand)
	if math.Abs(pvh.Lon-100.644) > 0.01 {
		t.Errorf("Lon = %f, expected ~100.644", pvh.Lon)
	}
	if math.Abs(pvh.Lat-13.732) > 0.01 {
		t.Errorf("Lat = %f, expected ~13.732", pvh.Lat)
	}
}

func TestNav2PvhRoundtrip(t *testing.T) {
	m := Nav2Pvh{
		Nav2TOW:   Nav2TOW{TOW: 260881000},
		Wn:        2408,
		FixFlags:  PVT3D,
		VelFlags:  Nav2Vel3D,
		GnssMask:  Nav2GnssGPS | Nav2GnssBDS | Nav2GnssGLN | Nav2GnssGAL,
		NumFixTot: 30,
		NumFixGPS: 9,
		NumFixBDS: 15,
		NumFixGLN: 6,
		Lon:       100.644,
		Lat:       13.732,
		Height:    16.2,
		SepGeoid:  -30.5,
		VelE:      0.01,
		VelN:      -0.02,
		VelU:      0.005,
		Speed3D:   0.02,
		Speed2D:   0.01,
		Heading:   123.45,
		HAcc:      2.5,
		VAcc:      4.0,
		SAcc:      0.19,
		CAcc:      50.0,
	}
	testMsgType(t, m)
}

func TestNav2TimeUTCParse(t *testing.T) {
	pkt := parseHex(t, "bace14001105a4211b3f74bdffff0000ea070304001c010f010430f2166c")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	utc, ok := msg.(*Nav2TimeUTC)
	if !ok {
		t.Fatalf("expected *Nav2TimeUTC, got %T", msg)
	}
	if utc.Year != 2026 {
		t.Errorf("Year = %d, want 2026", utc.Year)
	}
	if utc.Month != 3 {
		t.Errorf("Month = %d, want 3", utc.Month)
	}
	if utc.Day != 4 {
		t.Errorf("Day = %d, want 4", utc.Day)
	}
	if utc.Hour != 0 {
		t.Errorf("Hour = %d, want 0", utc.Hour)
	}
	if utc.Min != 28 {
		t.Errorf("Min = %d, want 28", utc.Min)
	}
	if utc.Sec != 1 {
		t.Errorf("Sec = %d, want 1", utc.Sec)
	}
	if utc.TFlags&Nav2TimeTOWValid == 0 {
		t.Error("expected Nav2TimeTOWValid set")
	}
	if utc.TFlags&Nav2TimeReliable == 0 {
		t.Error("expected Nav2TimeReliable set")
	}
	if utc.TimeSrc != Nav2TimeSrcBDS {
		t.Errorf("TimeSrc = %d, want BDS", utc.TimeSrc)
	}
}

func TestNav2TimeUTCRoundtrip(t *testing.T) {
	m := Nav2TimeUTC{
		TAcc:    50.5,
		Subms:   12345,
		Subcs:   -2,
		Cs:      45,
		Year:    2026,
		Month:   3,
		Day:     4,
		Hour:    12,
		Min:     30,
		Sec:     45,
		TFlags:  Nav2TimeTOWValid | Nav2TimeWNValid | Nav2TimeLeapValid | Nav2TimeReliable,
		TimeSrc: Nav2TimeSrcGPS,
		LeapSec: 18,
	}
	testMsgType(t, m)
}

func TestNav2SigParse(t *testing.T) {
	// NAV2-SIG with 44 signals + 4 trailing bytes
	pkt := parseHex(t, "bacecc021106b8008d0f002c270000010001e4ff21f150750105c100680000020002f0ff21f150752011a60062000003000391ff29f150751018dc0047000004000401002af9507513453e012a0000080008f2ff31f550750d48aa002a0000090009f2ff23f150751a2340013a000010001033001af95075151f17004600001b001b7eff23f15075233f41002e00001f001fd3ff18f150750a1851005b0004031303000023f150702b147800640004071307fdff26f150752d387300320003070707a2ff2dfd507516507200290003080708000024f95070191b8c005100030e070e10002afd50150c000000000003150715f1ff28fd50751b2202013900031b071bedff2bf5507500394f012d0001020a02e6ff2ff150750841e8002a0001030a03c4ff2cf15075124691002b0001050a05d4ff26f1507522280101350001070b07e2ff2df15075174b99002a0001080b08fbff1ef150751115c0005200010a0b0aa2ff2df150751d43ba002b00010d0b0d0d0026f150750e1aca00480001100b10ffff1af150751e2c1b003800011b0b1b050029f150750413ba005600011b0e1bf2ff25f350752e13ba005600011c0b1c86ff2cf15075272990003c0001200b20f1ff33f950751837f8002d0001250b2536ff26f15075022342013b0001250e25020024f150750b2342013b0001260b26ffff14e15075240fb200610001260e26000015f15070060fb200610001280b28c9ff30f95075264575002c0001280e2889ff2ff95075284575002c0001290b29eaff2ff15075141daf00480001290e29f3ff2df15075031daf004800013b0a3b350021f950151f2a6a000000013c0a3c000032f150702a3dec002c0002010509eeff2ffd50750f21b200420002020504fbff35f150750544fd002900020c0507fcff21f15075071512005600020d0506000020f150702c1d460141000216050534001cf150751c154f0063000217050b8fff28f15075211b7b005400aa762b404f76bf5b")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	sig, ok := msg.(*Nav2Sig)
	if !ok {
		t.Fatalf("expected *Nav2Sig, got %T", msg)
	}
	if sig.NumTrkTot != 44 {
		t.Errorf("NumTrkTot = %d, want 44", sig.NumTrkTot)
	}
	if sig.NumFixTot != 39 {
		t.Errorf("NumFixTot = %d, want 39", sig.NumFixTot)
	}
	if len(sig.Sigs) != 44 {
		t.Fatalf("len(Sigs) = %d, want 44", len(sig.Sigs))
	}
	// First signal: GPS PRN 1 L1CA
	s := sig.Sigs[0]
	if s.GNSSID != 0 || s.SVID != 1 || s.SigID != SigGPSL1CA {
		t.Errorf("Sigs[0] = GNSS=%d SV=%d Sig=%d, want GPS PRN 1 L1CA", s.GNSSID, s.SVID, s.SigID)
	}
	if s.CNO != 33 {
		t.Errorf("Sigs[0].CNO = %d, want 33", s.CNO)
	}
	if s.Elev != 5 || s.Azim != 193 {
		t.Errorf("Sigs[0] elev=%d azim=%d, want 5/193", s.Elev, s.Azim)
	}
	// Last signal: GLONASS slot 23 L1
	s = sig.Sigs[43]
	if s.GNSSID != 2 || s.SVID != 23 || s.SigID != SigGLOL1 {
		t.Errorf("Sigs[43] = GNSS=%d SV=%d Sig=%d, want GLO slot 23 L1", s.GNSSID, s.SVID, s.SigID)
	}
}

func TestNav2SigParseNoTrailing(t *testing.T) {
	// Verify that a NAV2-SIG with no trailing bytes also parses correctly
	m := Nav2Sig{
		Nav2SigFixed: Nav2SigFixed{TOW: 12345, NumTrkTot: 2, NumFixTot: 1},
		Sigs: []Nav2SigInfo{
			{GNSSID: 0, SVID: 1, SigID: SigGPSL1CA, CNO: 40, Elev: 30, Azim: 120},
			{GNSSID: 1, SVID: 5, SigID: SigBDSB1IMEO, CNO: 35, Elev: 45, Azim: 200},
		},
	}
	b, err := Serialize(&m)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	msg, err := ParseMsg(string(b))
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	sig := msg.(*Nav2Sig)
	if len(sig.Sigs) != 2 {
		t.Fatalf("len(Sigs) = %d, want 2", len(sig.Sigs))
	}
	if sig.Sigs[0].SVID != 1 {
		t.Errorf("Sigs[0].SVID = %d, want 1", sig.Sigs[0].SVID)
	}
}

func TestNav2SigTrailingBytes(t *testing.T) {
	// Build a valid Nav2Sig packet, then append trailing bytes of various lengths
	m := Nav2Sig{
		Nav2SigFixed: Nav2SigFixed{TOW: 12345, NumTrkTot: 1, NumFixTot: 1},
		Sigs: []Nav2SigInfo{
			{GNSSID: 0, SVID: 1, SigID: SigGPSL1CA, CNO: 40, Elev: 30, Azim: 120},
		},
	}
	b, err := Serialize(&m)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	// b is a valid packet: sync(2) + len(2) + cls(1) + id(1) + payload + cksum(4)
	// Extract just the payload
	payload := b[6 : len(b)-4]
	for _, extraLen := range []int{1, 4, 7, 16, 100} {
		extra := make([]byte, extraLen)
		newPayload := append(payload, extra...)
		pkt, err := PackMsg(Nav2SigID, newPayload)
		if err != nil {
			t.Fatalf("PackMsg with %d trailing: %v", extraLen, err)
		}
		msg, err := ParseMsg(string(pkt))
		if err != nil {
			t.Fatalf("ParseMsg with %d trailing bytes: %v", extraLen, err)
		}
		sig := msg.(*Nav2Sig)
		if len(sig.Sigs) != 1 {
			t.Errorf("with %d trailing: len(Sigs) = %d, want 1", extraLen, len(sig.Sigs))
		}
	}
}

func TestNav2SigNavEpoch(t *testing.T) {
	m := Nav2Sig{Nav2SigFixed: Nav2SigFixed{TOW: 260881000}}
	var nm NavMsg = &m
	if nm.NavEpoch() != 260881000 {
		t.Errorf("NavEpoch() = %d, want 260881000", nm.NavEpoch())
	}
}
