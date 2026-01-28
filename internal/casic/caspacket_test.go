package casic

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
		// MON-VER poll (empty payload)
		{"MON-VER poll", "bace00000a0400000a04"},
		// ACK-NAK response
		{"ACK-NAK", "bace040005000a0400000e040500"},
		// ACK-ACK response
		{"ACK-ACK", "bace04000501060700000a070501"},
		// CFG-NAVX response (44 byte payload)
		{"CFG-NAVX", "bace2c0006070000000000300000081001000003f409000000000000000000000000000000000000000000000000000000003443fb10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt, err := hex.DecodeString(tc.hex)
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
		{"MON-VER poll", "bace00000a0400000a04"},
		{"ACK-NAK", "bace040005000a0400000e040500"},
		{"CFG-NAVX", "bace2c0006070000000000300000081001000003f409000000000000000000000000000000000000000000000000000000003443fb10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt, err := hex.DecodeString(tc.hex)
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
		{"bace00000a0400000a04", "MON-0x04"},
		{"bace040005000a0400000e040500", "ACK-0x00"},
		{"bace04000501060700000a070501", "ACK-0x01"},
		{"bace2c0006070000000000300000081001000003f409000000000000000000000000000000000000000000000000000000003443fb10", "CFG-0x07"},
	}
	for _, tc := range tests {
		t.Run(tc.expect, func(t *testing.T) {
			pkt, _ := hex.DecodeString(tc.hex)
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
	if Tag != "CASBIN" {
		t.Errorf("Tag = %q, want CASBIN", Tag)
	}
}

func TestInvalidPackets(t *testing.T) {
	tests := []struct {
		name string
		pkt  []byte
	}{
		{"wrong sync1", []byte{0xB5, 0xCE, 0x00, 0x00, 0x0A, 0x04, 0x00, 0x00, 0x0A, 0x04}},
		{"wrong sync2", []byte{0xBA, 0x62, 0x00, 0x00, 0x0A, 0x04, 0x00, 0x00, 0x0A, 0x04}},
		{"too short", []byte{0xBA, 0xCE, 0x00, 0x00}},
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
