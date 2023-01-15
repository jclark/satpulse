package ubx

import "testing"

func TestNavTimeGPS(t *testing.T) {
	p := NavTimeGPSPayload{
		ITOW:  0x12345678,
		FTOW:  0,
		Week:  6523,
		LeapS: 37,
		Valid: 0,
		TAcc:  173,
	}
	m := NewMsg[NavTimeGPSPayload](NavTimeGPSId)
	m.payload = p
	b, err := m.Serialize()
	if err != nil {
		t.Fatalf("serialize err for nav-time-gps %v", err)
	}
	m2, err := ParseMsg(b)
	if err != nil {
		t.Fatalf("parse error for nav-time-gps %v", err)
	}
	if m2.ClsId() != NavTimeGPSId {
		t.Fatalf("clsid not roundtripped %v => %v", NavTimeGPSId, m2.ClsId())
	}
	p2 := m2.Payload().(*NavTimeGPSPayload)
	if *p2 != p {
		t.Fatalf("nav-time-gps not roundtripped %v => %v", &p, &p2)
	}
}
