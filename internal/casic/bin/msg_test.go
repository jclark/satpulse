package bin

import (
	"testing"
)

func TestMsgBDSUTCRoundtrip(t *testing.T) {
	m := MsgBDSUTC{
		A0UTC: 12345678,
		A1UTC: -9876,
		Dtls:  4,
		Dtlsf: 4,
		Wnlsf: 100,
		Dn:    7,
		Valid: MsgUTCValidOK,
	}
	testMsgType(t, m)
}

func TestMsgGPSUTCRoundtrip(t *testing.T) {
	m := MsgGPSUTC{
		Tot:   147456, // 36 * 4096
		A0UTC: 12345678,
		A1UTC: -9876,
		Dtls:  18,
		Dtlsf: 18,
		Wnt:   150,
		Wnlsf: 200,
		Dn:    7,
		Valid: MsgUTCValidOK,
	}
	testMsgType(t, m)
}

func TestMsgUTCPayloadSize(t *testing.T) {
	// Both MSG-BDSUTC and MSG-GPSUTC should have 20-byte payloads
	m1 := MsgBDSUTC{}
	b1, err := Serialize(&m1)
	if err != nil {
		t.Fatalf("Serialize MsgBDSUTC error: %v", err)
	}
	// Packet: sync(2) + len(2) + class(1) + id(1) + payload(20) + checksum(4) = 30
	if len(b1) != 30 {
		t.Errorf("MsgBDSUTC packet length = %d, want 30", len(b1))
	}

	m2 := MsgGPSUTC{}
	b2, err := Serialize(&m2)
	if err != nil {
		t.Fatalf("Serialize MsgGPSUTC error: %v", err)
	}
	if len(b2) != 30 {
		t.Errorf("MsgGPSUTC packet length = %d, want 30", len(b2))
	}
}

func TestMsgUTCValidConstants(t *testing.T) {
	// Verify the validity constants match the spec
	if MsgUTCInvalid != 0 {
		t.Errorf("MsgUTCInvalid = %d, want 0", MsgUTCInvalid)
	}
	if MsgUTCUnhealthy != 1 {
		t.Errorf("MsgUTCUnhealthy = %d, want 1", MsgUTCUnhealthy)
	}
	if MsgUTCExpired != 2 {
		t.Errorf("MsgUTCExpired = %d, want 2", MsgUTCExpired)
	}
	if MsgUTCValidOK != 3 {
		t.Errorf("MsgUTCValidOK = %d, want 3", MsgUTCValidOK)
	}
}
