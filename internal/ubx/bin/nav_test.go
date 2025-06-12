package bin

import (
	"testing"

	"golang.org/x/exp/slices"
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
	m := NavTimeTrusted{
		Version: 1,
		NavTimeTrustedV1: NavTimeTrustedV1{
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
		},
	}
	p2 := testMsgType1(t, m)
	if !EqualNavTimeTrusted(&m, p2.(*NavTimeTrusted)) {
		t.Fatalf("msg nav-timetrusted v1 not roundtripped %v => %v", &m, p2)
	}
}

func TestNavTimeTrustedUnknown(t *testing.T) {
	m := NavTimeTrusted{
		Version: 2,
		Unknown: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
	}
	p2 := testMsgType1(t, m)
	if !EqualNavTimeTrusted(&m, p2.(*NavTimeTrusted)) {
		t.Fatalf("msg nav-timetrusted v2 not roundtripped %v => %v", &m, p2)
	}
}

func EqualNavTimeTrusted(p1, p2 *NavTimeTrusted) bool {
	if p1.Version != p2.Version {
		return false
	}
	if p1.Version == 1 {
		return p1.NavTimeTrustedV1 == p2.NavTimeTrustedV1
	}
	return slices.Equal(p1.Unknown, p2.Unknown)
}
