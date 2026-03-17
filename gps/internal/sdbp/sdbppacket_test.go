package sdbp

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// validPackets contains complete packets from the SDBP protocol doc.
var validPackets = []struct {
	name  string
	hex   string
	msgID string
}{
	// QUE-VER from protocol doc
	{"QUE-VER", "233e 0501 0000 0617", "QUE-VER"},
}

func TestChecksum(t *testing.T) {
	for _, tc := range validPackets {
		t.Run(tc.name, func(t *testing.T) {
			pkt, err := hex.DecodeString(stripSpaces(tc.hex))
			if err != nil {
				t.Fatalf("bad hex: %v", err)
			}
			computed := PacketFormat.ComputeChecksum(pkt)
			extracted := PacketFormat.ExtractChecksum(pkt)
			if !bytes.Equal(computed, extracted) {
				t.Errorf("checksum mismatch: computed %x, extracted %x", computed, extracted)
			}
		})
	}
}

func TestStateMachine(t *testing.T) {
	for _, tc := range validPackets {
		t.Run(tc.name, func(t *testing.T) {
			pkt, err := hex.DecodeString(stripSpaces(tc.hex))
			if err != nil {
				t.Fatalf("bad hex: %v", err)
			}
			if !gpsprot.IsValidPacket(PacketFormat, pkt) {
				t.Errorf("packet not recognized as valid")
			}
		})
	}
}

func TestMsgID(t *testing.T) {
	for _, tc := range validPackets {
		t.Run(tc.name, func(t *testing.T) {
			pkt, _ := hex.DecodeString(stripSpaces(tc.hex))
			got := PacketFormat.MsgID(pkt)
			if got != tc.msgID {
				t.Errorf("MsgID = %q, want %q", got, tc.msgID)
			}
		})
	}
}

func TestTag(t *testing.T) {
	if PacketFormat.Tag() != Tag {
		t.Errorf("Tag() = %q, want %q", PacketFormat.Tag(), Tag)
	}
	if Tag != "SDBP" {
		t.Errorf("Tag = %q, want SDBP", Tag)
	}
}

func TestInvalidPackets(t *testing.T) {
	tests := []struct {
		name string
		pkt  []byte
	}{
		{"wrong sync1", []byte{0xB5, 0x3E, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{"wrong sync2", []byte{0x23, 0x62, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{"too short", []byte{0x23, 0x3E, 0x02, 0x01}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if gpsprot.IsValidPacket(PacketFormat, tc.pkt) {
				t.Errorf("expected packet to be invalid")
			}
		})
	}
}

func stripSpaces(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			result = append(result, s[i])
		}
	}
	return string(result)
}
