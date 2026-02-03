package casbin

import (
	"encoding/hex"
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
		t.Fatalf("msg %v not roundtripped %v => %v", mid, &m, &m2)
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

func parseHex(t *testing.T, hexStr string) string {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	return string(b)
}

func TestTrailingBytes(t *testing.T) {
	// Create a NavSol-like packet with extra bytes
	b, _ := PackMsg(NavSolID, make([]byte, 73)) // 72 + 1 extra byte
	_, err := ParseMsg(string(b))
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("failed to give an error for trailing bytes")
	}
}

func TestPoll(t *testing.T) {
	buf := Poll(NavSolID)
	if len(buf) != packetMinLength {
		t.Fatalf("unexpected output length %d (expected %d)", len(buf), packetMinLength)
	}
	if buf[4] != clsNav {
		t.Fatalf("invalid cls byte: got 0x%02x, want 0x%02x", buf[4], clsNav)
	}
}

func TestPacketMsgID(t *testing.T) {
	pkt := parseHex(t, "bace48000102418cc804070700031909100072006209e60800008036cb40bae82a46797731c1bf42b680973b5741053d8e86a7f3364107c0c93f00000000000000000000000088ea6b39a4c2893fe958f59d")
	mid := PacketMsgID(pkt)
	if mid != NavSolID {
		t.Fatalf("PacketMsgID got %v, want %v", mid, NavSolID)
	}
}

func TestMsgIDString(t *testing.T) {
	tests := []struct {
		mid  MsgID
		want string
	}{
		{NavSolID, "NAV-SOL"},
		{NavTimeUTCID, "NAV-TIMEUTC"},
		{TimTPID, "TIM-TP"},
		{MsgGPSUTCID, "MSG-GPSUTC"},
		{MakeMsgID(0xFF, 0x42), "0xFF-0x42"},
	}
	for _, tc := range tests {
		got := tc.mid.String()
		if got != tc.want {
			t.Errorf("MsgID(%04x).String() = %q, want %q", uint16(tc.mid), got, tc.want)
		}
	}
}

func TestChecksum(t *testing.T) {
	// Test with an actual packet from the receiver (NAV-SOL)
	fullPkt := parseHex(t, "bace48000102418cc804070700031909100072006209e60800008036cb40bae82a46797731c1bf42b680973b5741053d8e86a7f3364107c0c93f00000000000000000000000088ea6b39a4c2893fe958f59d")
	wantCk := uint32(fullPkt[len(fullPkt)-4]) | uint32(fullPkt[len(fullPkt)-3])<<8 |
		uint32(fullPkt[len(fullPkt)-2])<<16 | uint32(fullPkt[len(fullPkt)-1])<<24
	gotCk := Checksum([]byte(fullPkt[:len(fullPkt)-4]))
	if gotCk != wantCk {
		t.Errorf("Checksum = 0x%08x, want 0x%08x", gotCk, wantCk)
	}
}
