package ubx

import "testing"

func TestNavTimeGPS(t *testing.T) {
	p := NavTimeGPS{
		ITOW:  0x12345678,
		FTOW:  0,
		Week:  6523,
		LeapS: 37,
		Valid: 0,
		TAcc:  173,
	}
	var m Msg = &p
	b, err := Serialize(m)
	if err != nil {
		t.Fatalf("serialize err for nav-time-gps %v", err)
	}
	m2, err := ParseMsg(b)
	if err != nil {
		t.Fatalf("parse error for nav-time-gps %v", err)
	}
	if m2.ID() != p.ID() {
		t.Fatalf("clsid not roundtripped %v => %v", p.ID(), m2.ID())
	}
	p2 := m2.(*NavTimeGPS)
	if *p2 != p {
		t.Fatalf("nav-time-gps not roundtripped %v => %v", &p, &p2)
	}
}
