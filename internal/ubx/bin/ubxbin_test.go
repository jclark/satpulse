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
	m := InfDebug(bytes)
	p2 := testMsgType1(t, m)
	if !slices.Equal(bytes, ([]byte)(*p2.(*InfDebug))) {
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
