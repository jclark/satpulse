package casbin

import (
	"encoding/binary"
	"testing"
)

func TestAckParse(t *testing.T) {
	tests := []struct {
		name      string
		hex       string
		wantID    MsgID
		wantClsID uint8
		wantMsgID uint8
	}{
		{
			name:      "ACK-NAK for MON-VER",
			hex:       "bace040005000a0400000e040500",
			wantID:    AckNakID,
			wantClsID: 0x0A,
			wantMsgID: 0x04,
		},
		{
			name:      "ACK-ACK for CFG-RATE",
			hex:       "bace04000501060400000a040501",
			wantID:    AckAckID,
			wantClsID: 0x06,
			wantMsgID: 0x04,
		},
		{
			name:      "ACK-ACK for CFG-TP",
			hex:       "bace04000501060300000a030501",
			wantID:    AckAckID,
			wantClsID: 0x06,
			wantMsgID: 0x03,
		},
		{
			name:      "ACK-ACK for CFG-NAVX",
			hex:       "bace04000501060700000a070501",
			wantID:    AckAckID,
			wantClsID: 0x06,
			wantMsgID: 0x07,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := parseHex(t, tc.hex)
			// Verify checksum
			wantCk := binary.LittleEndian.Uint32([]byte(pkt[len(pkt)-4:]))
			gotCk := Checksum([]byte(pkt[:len(pkt)-4]))
			if gotCk != wantCk {
				t.Fatalf("checksum mismatch: got 0x%08x, want 0x%08x", gotCk, wantCk)
			}
			msg, err := ParseMsg(pkt)
			if err != nil {
				t.Fatalf("ParseMsg error: %v", err)
			}
			if msg.ID() != tc.wantID {
				t.Fatalf("ID() = %v, want %v", msg.ID(), tc.wantID)
			}
			var payload *AckPayload
			switch m := msg.(type) {
			case *AckNak:
				payload = &m.AckPayload
			case *AckAck:
				payload = &m.AckPayload
			default:
				t.Fatalf("unexpected type %T", msg)
			}
			if payload.ClsID != tc.wantClsID {
				t.Errorf("ClsID = 0x%02x, want 0x%02x", payload.ClsID, tc.wantClsID)
			}
			if payload.MsgID != tc.wantMsgID {
				t.Errorf("MsgID = 0x%02x, want 0x%02x", payload.MsgID, tc.wantMsgID)
			}
		})
	}
}

func TestAckRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Msg
	}{
		{"AckNak", &AckNak{AckPayload{ClsID: 0x06, MsgID: 0x07}}},
		{"AckAck", &AckAck{AckPayload{ClsID: 0x06, MsgID: 0x07}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Serialize(tc.msg)
			if err != nil {
				t.Fatalf("Serialize error: %v", err)
			}
			msg2, err := ParseMsg(string(b))
			if err != nil {
				t.Fatalf("ParseMsg error: %v", err)
			}
			if msg2.ID() != tc.msg.ID() {
				t.Fatalf("ID mismatch: got %v, want %v", msg2.ID(), tc.msg.ID())
			}
		})
	}
}
