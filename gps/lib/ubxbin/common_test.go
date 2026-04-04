package ubxbin

import (
	"strings"
	"testing"
)

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
	b, _ := PackMsg(a.ID(), []byte{clsCfg, 0x01,
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
	if len(buf) != PacketMinLen {
		t.Fatalf("unexpected output length %d (expected %d)", len(buf), PacketMinLen)
	}
	if buf[2] != clsMon {
		t.Fatalf("invalid cls byte")
	}
}
