package ubxbin

import (
	"testing"

	"golang.org/x/exp/slices"
)

func TestMonVer(t *testing.T) {
	m := MonVer{
		MonVerFixed{[30]byte{1, 2, 3}, [10]byte{4, 5}},
		[][30]byte{{7, 8}, {9}},
	}
	p2 := testMsgType1(t, m)
	if !EqualMonVer(&m, p2.(*MonVer)) {
		t.Fatalf("msg mon-ver not roundtripped %v => %v", &m, p2)
	}
}

func EqualMonVer(p1, p2 *MonVer) bool {
	return p1.MonVerFixed == p2.MonVerFixed && slices.Equal(p1.Extension, p2.Extension)
}

func TestMonGnssV0(t *testing.T) {
	testMsgType(t, MonGnss{
		Version:      0,
		Supported:    MonGnssGPS | MonGnssGalileo,
		DefaultGnss:  MonGnssGPS,
		Enabled:      MonGnssGPS | MonGnssBeidou,
		Simultaneous: 4,
	})
}

func TestMonGnssUnknownVersion(t *testing.T) {
	// Test that unknown version gets parsed as UnknownMsg
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x0A, 0x28, // class=MON(0x0A), id=GNSS(0x28)
		0x08, 0x00, // length = 8 bytes
		// Payload starts here:
		0x01,                   // version = 1 (unknown)
		0x0F, 0x0E, 0x0D, 0x0C, // Some data
		0x04, 0x00, 0x00,       // More data
	}
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse MON-GNSS v1: %v", err)
	}

	// Should be parsed as UnknownMsg
	unknownMsg, ok := msg.(*UnknownMsg)
	if !ok {
		t.Fatalf("expected *UnknownMsg for unknown version, got %T", msg)
	}

	if unknownMsg.MsgID != MonGnssID {
		t.Errorf("expected MsgID=%v, got %v", MonGnssID, unknownMsg.MsgID)
	}

	expectedPayload := string([]byte{0x01, 0x0F, 0x0E, 0x0D, 0x0C, 0x04, 0x00, 0x00})
	if unknownMsg.Payload != expectedPayload {
		t.Errorf("expected payload=%x, got %x", expectedPayload, unknownMsg.Payload)
	}
}
