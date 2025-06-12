package bin

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
	m := MonGnss{
		Version: 0,
		MonGnssV0: MonGnssV0{
			Supported:    MonGnssGPS | MonGnssGalileo,
			DefaultGnss:  MonGnssGPS,
			Enabled:      MonGnssGPS | MonGnssBeidou,
			Simultaneous: 4,
		},
	}
	p2 := testMsgType1(t, m)
	if !EqualMonGnss(&m, p2.(*MonGnss)) {
		t.Fatalf("msg mon-gnss v0 not roundtripped %v => %v", &m, p2)
	}
}

func TestMonGnssV1(t *testing.T) {
	m := MonGnss{
		Version: 1,
		Unknown: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
	}
	p2 := testMsgType1(t, m)
	if !EqualMonGnss(&m, p2.(*MonGnss)) {
		t.Fatalf("msg mon-gnss v1 not roundtripped %v => %v", &m, p2)
	}
}

func EqualMonGnss(p1, p2 *MonGnss) bool {
	if p1.Version != p2.Version {
		return false
	}
	if p1.Version == 0 {
		return p1.MonGnssV0 == p2.MonGnssV0
	}
	return slices.Equal(p1.Unknown, p2.Unknown)
}
