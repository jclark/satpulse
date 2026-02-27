package ubxbin

import (
	"encoding/hex"
	"testing"
)

func TestNavEOE(t *testing.T) {
	testMsgType(t, NavEOE{
		NavITOW: NavITOW{ITOW: 0x12345678},
	})
}

func TestNavTimeGPS(t *testing.T) {
	testMsgType(t, NavTimeGPS{
		NavITOW: NavITOW{ITOW: 0x12345678},
		FTOW:    0,
		Week:    6523,
		LeapS:   37,
		Valid:   0,
		TAcc:    173,
	})
}

func TestNavTimeTrustedV1(t *testing.T) {
	testMsgType(t, NavTimeTrusted{
		Version: 1,
		RefSys:  NavTimeTrustedRefSysGPS,
		Valid:   NavTimeTrustedValidTrustedTime | NavTimeTrustedValidDeltaTime,
		NavITOW: NavITOW{ITOW: 0x12345678},
		IniWno:   2100,
		PropWno:  2101,
		IniTow:   123456000,
		PropTow:  123457000,
		IniTAcc:  50,
		PropTAcc: 75,
		DeltaS:   -2,
		DeltaMs:  -250,
	})
}

func TestNavSig(t *testing.T) {
	testMsgType1(t, NavSig{
		NavSigFixed: NavSigFixed{
			NavITOW: NavITOW{ITOW: 0x12345678},
			Version: 0,
			NumSigs: 2,
		},
		Signals: []NavSigSignal{
			{
				GNSSID:     GPS,
				SVID:       1,
				SigID:      0,
				FreqID:     0,
				PRRes:      -123,
				CNO:        45,
				QualityInd: NavSigQualityCodeLocked,
				CorrSource: NavSigCorrSourceRTCM3SSR,
				IonoModel:  NavSigIonoModelKlobucharGPS,
				SigFlags:   NavSigHealthHealthy | NavSigPrUsed | NavSigCrUsed,
			},
			{
				GNSSID:     GAL,
				SVID:       11,
				SigID:      1,
				FreqID:     0,
				PRRes:      456,
				CNO:        38,
				QualityInd: NavSigQualityCodeCarrierLocked5,
				CorrSource: NavSigCorrSourceSPARTN,
				IonoModel:  NavSigIonoModelDualFreq,
				SigFlags:   NavSigHealthUnhealthy | NavSigPrSmoothed | NavSigDoUsed,
			},
		},
	})
}

func TestNavHPPosECEF(t *testing.T) {
	testMsgType(t, NavHPPosECEF{
		Version: 0,
		NavITOW: NavITOW{ITOW: 0x12345678},
		ECEF:    [3]int32{-265927895, -443413736, 384610019},
		ECEFHp:  [3]int8{-42, 17, -8},
		Flags:   0,
		PAcc:    1234,
	})
}

func TestNavHPPosECEFUnknownVersion(t *testing.T) {
	packet := []byte{
		0xB5, 0x62,
		0x01, 0x13,
		0x1C, 0x00, // length = 28
		0x01,                   // version = 1 (unknown)
		0x00, 0x00, 0x00,      // reserved
		0x78, 0x56, 0x34, 0x12, // ITOW
		0x00, 0x00, 0x00, 0x00, // ECEF X
		0x00, 0x00, 0x00, 0x00, // ECEF Y
		0x00, 0x00, 0x00, 0x00, // ECEF Z
		0x00, 0x00, 0x00, // ECEFHp
		0x00,             // Flags
		0x00, 0x00, 0x00, 0x00, // PAcc
	}
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := msg.(*UnknownMsg); !ok {
		t.Fatalf("expected *UnknownMsg for unknown version, got %T", msg)
	}
}

func TestNavHPPosLLH(t *testing.T) {
	testMsgType(t, NavHPPosLLH{
		Version:  0,
		Flags:    0,
		NavITOW:  NavITOW{ITOW: 0x12345678},
		Lon:      -1172345678,
		Lat:      337654321,
		Height:   12345,
		HMSL:     -5678,
		LonHp:    -42,
		LatHp:    17,
		HeightHp: -8,
		HMSLHp:   5,
		HAcc:     567,
		VAcc:     890,
	})
}

func TestNavHPPosLLHUnknownVersion(t *testing.T) {
	packet := []byte{
		0xB5, 0x62,
		0x01, 0x14,
		0x24, 0x00, // length = 36
		0x01,       // version = 1 (unknown)
		0x00, 0x00, // reserved
		0x00,       // flags
		0x78, 0x56, 0x34, 0x12, // ITOW
		0x00, 0x00, 0x00, 0x00, // Lon
		0x00, 0x00, 0x00, 0x00, // Lat
		0x00, 0x00, 0x00, 0x00, // Height
		0x00, 0x00, 0x00, 0x00, // HMSL
		0x00, 0x00, 0x00, 0x00, // LonHp, LatHp, HeightHp, HMSLHp
		0x00, 0x00, 0x00, 0x00, // HAcc
		0x00, 0x00, 0x00, 0x00, // VAcc
	}
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := msg.(*UnknownMsg); !ok {
		t.Fatalf("expected *UnknownMsg for unknown version, got %T", msg)
	}
}

func TestNavHPPosECEFReal(t *testing.T) {
	// Real packet captured from ZED-F9P
	b, _ := hex.DecodeString("b56201131c0000000000d819331caf73b617cf14f0ff25ad9d1d1d300c00b0370000fdd0")
	msg, err := ParseMsg(string(b))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m, ok := msg.(*NavHPPosECEF)
	if !ok {
		t.Fatalf("expected *NavHPPosECEF, got %T", msg)
	}
	if m.Version != 0 {
		t.Errorf("version=%d, want 0", m.Version)
	}
	if m.ITOW != 473111000 {
		t.Errorf("itow=%d", m.ITOW)
	}
	// ECEF X ~3978 km (plausible for Earth surface)
	if m.ECEF[0] < 300000000 || m.ECEF[0] > 500000000 {
		t.Errorf("ECEF X=%d out of plausible range", m.ECEF[0])
	}
	if m.PAcc == 0 {
		t.Error("PAcc is zero")
	}
}

func TestNavHPPosLLHReal(t *testing.T) {
	// Real packet captured from ZED-F9P
	b, _ := hex.DecodeString("b5620114240000000000d819331cf112e9ffe8e9b21ebc7901007bc70000ef0a0201891f0000e62d00003412")
	msg, err := ParseMsg(string(b))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m, ok := msg.(*NavHPPosLLH)
	if !ok {
		t.Fatalf("expected *NavHPPosLLH, got %T", msg)
	}
	if m.Version != 0 {
		t.Errorf("version=%d, want 0", m.Version)
	}
	if m.ITOW != 473111000 {
		t.Errorf("itow=%d", m.ITOW)
	}
	// Lat ~51.5 deg (London area)
	if m.Lat < 500000000 || m.Lat > 520000000 {
		t.Errorf("Lat=%d out of plausible range", m.Lat)
	}
	// Lon should be small negative (west of Greenwich)
	if m.Lon > 0 || m.Lon < -10000000 {
		t.Errorf("Lon=%d out of plausible range", m.Lon)
	}
	if m.HAcc == 0 {
		t.Error("HAcc is zero")
	}
	if m.VAcc == 0 {
		t.Error("VAcc is zero")
	}
}

func TestNavTimeTrustedUnknownVersion(t *testing.T) {
	// Test that unknown version gets parsed as UnknownMsg
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x01, 0x64, // class=NAV(0x01), id=TIMETRUSTED(0x64)
		0x28, 0x00, // length = 40 bytes
		// Payload starts here:
		0x02,                   // version = 2 (unknown)
		0x01,                   // RefSys = GPS
		0x03,                   // Valid
		0x00,                   // reserved
		0x78, 0x56, 0x34, 0x12, // ITOW
		0x34, 0x08, // IniWno
		0x35, 0x08, // PropWno
		0x00, 0x5B, 0x5A, 0x07, // IniTow
		0x00, 0x6F, 0x5A, 0x07, // PropTow
		0x32, 0x00, 0x00, 0x00, // IniTAcc
		0x4B, 0x00, 0x00, 0x00, // PropTAcc
		0xFE, 0xFF, 0xFF, 0xFF, // DeltaS
		0x06, 0xFF, 0xFF, 0xFF, // DeltaMs
		0x00, 0x00, 0x00, 0x00, // reserved
	}
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse NAV-TIMETRUSTED v2: %v", err)
	}

	// Should be parsed as UnknownMsg
	unknownMsg, ok := msg.(*UnknownMsg)
	if !ok {
		t.Fatalf("expected *UnknownMsg for unknown version, got %T", msg)
	}

	if unknownMsg.MsgID != NavTimeTrustedID {
		t.Errorf("expected MsgID=%v, got %v", NavTimeTrustedID, unknownMsg.MsgID)
	}
}
