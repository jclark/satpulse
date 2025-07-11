package bin

import (
	"testing"
)

func TestNavTimeGPS(t *testing.T) {
	testMsgType(t, NavTimeGPS{
		ITOW:  0x12345678,
		FTOW:  0,
		Week:  6523,
		LeapS: 37,
		Valid: 0,
		TAcc:  173,
	})
}

func TestNavTimeTrustedV1(t *testing.T) {
	testMsgType(t, NavTimeTrusted{
		Version:  1,
		RefSys:   NavTimeTrustedRefSysGPS,
		Valid:    NavTimeTrustedValidTrustedTime | NavTimeTrustedValidDeltaTime,
		ITOW:     0x12345678,
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

func TestNavTimeTrustedUnknownVersion(t *testing.T) {
	// Test that unknown version gets parsed as UnknownMsg
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x01, 0x64, // class=NAV(0x01), id=TIMETRUSTED(0x64)
		0x28, 0x00, // length = 40 bytes
		// Payload starts here:
		0x02,       // version = 2 (unknown)
		0x01,       // RefSys = GPS
		0x03,       // Valid
		0x00,       // reserved
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
