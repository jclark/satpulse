package as

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestChecksum(t *testing.T) {
	tests := []struct {
		name string
		hex  string
	}{
		// NAV-TIME - 24 bytes total
		{"NAV-TIME", "f1d9010510000007 2c79ff553e161000 12000600000092 5a"},
		// CFG-PPS - 23 bytes total
		{"CFG-PPS", "f1d906070f0040420f00000000001027000001 0d01f386"},
		// ACK-NAK - 10 bytes total
		{"ACK-NAK", "f1d90500020006010e33"},
		// CFG-CFG - 16 bytes total
		{"CFG-CFG", "f1d9060908000000000003000000 1a07"},
	}
	for _, tc := range tests {
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
	tests := []struct {
		name string
		hex  string
	}{
		{"NAV-TIME", "f1d901051000000 72c79ff553e161000 12000600000092 5a"},
		{"CFG-PPS", "f1d906070f0040420f00000000001027000001 0d01f386"},
		{"ACK-NAK", "f1d90500020006010e33"},
		{"CFG-CFG", "f1d9060908000000000003000000 1a07"},
	}
	for _, tc := range tests {
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
	tests := []struct {
		hex    string
		expect string
	}{
		{"f1d901051000000 72c79ff553e161000 12000600000092 5a", "NAV-TIME"},
		{"f1d906070f0040420f00000000001027000001 0d01f386", "CFG-PPS"},
		{"f1d90500020006010e33", "ACK-NAK"},
		{"f1d9060908000000000003000000 1a07", "CFG-CFG"},
	}
	for _, tc := range tests {
		t.Run(tc.expect, func(t *testing.T) {
			pkt, _ := hex.DecodeString(stripSpaces(tc.hex))
			got := PacketFormat.MsgID(pkt)
			if got != tc.expect {
				t.Errorf("MsgID = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestTag(t *testing.T) {
	if PacketFormat.Tag() != Tag {
		t.Errorf("Tag() = %q, want %q", PacketFormat.Tag(), Tag)
	}
	if Tag != "ASBIN" {
		t.Errorf("Tag = %q, want ASBIN", Tag)
	}
}

func TestInvalidPackets(t *testing.T) {
	tests := []struct {
		name string
		pkt  []byte
	}{
		{"wrong sync1", []byte{0xB5, 0xD9, 0x01, 0x05, 0x00, 0x00, 0x00, 0x00}},
		{"wrong sync2", []byte{0xF1, 0x62, 0x01, 0x05, 0x00, 0x00, 0x00, 0x00}},
		{"too short", []byte{0xF1, 0xD9, 0x01, 0x05}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if gpsprot.IsValidPacket(PacketFormat, tc.pkt) {
				t.Errorf("expected packet to be invalid")
			}
		})
	}
}

func TestRescanOnBadChecksum(t *testing.T) {
	if PacketFormat.RescanOnBadChecksum(true, nil) {
		t.Errorf("RescanOnBadChecksum should return false")
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
