package bin

import (
	"strings"
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

func TestCfgCfg(t *testing.T) {
	m := CfgCfg{
		CfgCfgFixed{
			ClearMask: CfgCfgIOPort,
			SaveMask:  CfgCfgRXMConf,
			LoadMask:  CfgCfgNavConf,
		},
		[]CfgCfgDeviceMask{CfgCfgDevFlash},
	}
	p2 := testMsgType1(t, m)
	if !EqualCfgCfg(&m, p2.(*CfgCfg)) {
		t.Fatalf("msg cfg-cfg not roundtripped %v => %v", &m, p2)
	}
	m = CfgCfg{
		CfgCfgFixed{
			ClearMask: CfgCfgIOPort,
			SaveMask:  CfgCfgRXMConf,
			LoadMask:  CfgCfgNavConf,
		},
		[]CfgCfgDeviceMask{},
	}
	p2 = testMsgType1(t, m)
	if !EqualCfgCfg(&m, p2.(*CfgCfg)) {
		t.Fatalf("msg cfg-cfg not roundtripped %v => %v", &m, p2)
	}
}

func EqualCfgCfg(p1, p2 *CfgCfg) bool {
	return p1.CfgCfgFixed == p2.CfgCfgFixed && slices.Equal(p1.DeviceMask, p2.DeviceMask)
}

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

func TestCfgValget(t *testing.T) {
	m := CfgValget{
		CfgValgetFixed{
			Version:  CfgValgetVersionResponse,
			Layer:    CfgValgetLayerRAM,
			Position: 0,
		},
		[]byte{1, 2, 3, 4, 5},
	}
	p2 := testMsgType1(t, m)
	if !EqualCfgValget(&m, p2.(*CfgValget)) {
		t.Fatalf("msg cfg-valget not roundtripped %v => %v", &m, p2)
	}
}

func EqualCfgValget(p1, p2 *CfgValget) bool {
	return p1.CfgValgetFixed == p2.CfgValgetFixed && slices.Equal(p1.CfgData, p2.CfgData)
}

func TestInf(t *testing.T) {
	bytes := ([]byte)("hello")
	m := InfDebug{InfText(bytes)}
	p2 := testMsgType1(t, m)
	if !slices.Equal(bytes, ([]byte)(p2.(*InfDebug).InfText)) {
		t.Fatalf("msg inf not roundtripped %v => %v", &m, p2)
	}
}

func testMsgType[M comparable, PM interface {
	ID() MsgID
	*M
}](t *testing.T, m M) {
	p2 := testMsgType1[M, PM](t, m)
	m2 := *p2.(PM)
	if m2 != m {
		p := PM(&m)
		mid := p.ID()
		t.Fatalf("msg %v not roundtripped %v => %v", mid, &p, &p2)
	}
}

func testMsgType1[M any, PM interface {
	ID() MsgID
	*M
}](t *testing.T, m M) Msg {
	p := PM(&m)
	mid := p.ID()
	b, err := Serialize(p)
	if err != nil {
		t.Fatalf("serialize err for %v: %v", mid, err)
	}
	p2, err := ParseMsg(string(b))
	if err != nil {
		t.Fatalf("parse error for %v: %v", mid, err)
	}
	if p2.ID() != p.ID() {
		t.Fatalf("msgid not roundtripped %v => %v", mid, p2.ID())
	}
	return p2
}

func TestTrailingBytes(t *testing.T) {
	a := AckAck{}
	b, _ := packMsg(a.ID(), []byte{clsCfg, 0x01,
		// extra byte
		0x42})
	s := string(b)
	_, err := ParseMsg(s)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("failed to give an error for trailing bytes")
	}
}

func TestPoll(t *testing.T) {
	buf := Poll(MonVerID)
	if len(buf) != packetMinLength {
		t.Fatalf("unexpected output length %d (expected %d)", len(buf), packetMinLength)
	}
	if buf[2] != clsMon {
		t.Fatalf("invalid cls byte")
	}
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
