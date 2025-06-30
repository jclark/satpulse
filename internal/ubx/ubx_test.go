package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/ubx/bin"
)

func TestProcessPacketWithParseError(t *testing.T) {
	p := NewPacketProcessor()
	
	// Create a valid UBX packet with empty payload for CFG-NAV5 (which requires 36 bytes)
	// This will pass PacketFormat validation but fail during ParseMsg
	invalidPacket, err := bin.PackMsg(bin.CfgNav5ID, []byte{})
	if err != nil {
		t.Fatalf("failed to create test packet: %v", err)
	}

	var tZero time.Time // Use zero time for simplicity
	msgID, err := p.ProcessPacket(string(invalidPacket), tZero)
	
	// Should return error from ParseMsg
	if err == nil {
		t.Fatal("expected error when ParseMsg fails, got nil")
	}
	
	// msgID should be from PacketFormat.MsgID when m is nil
	expectedMsgID := bin.CfgNav5ID.String()
	if msgID != expectedMsgID {
		t.Fatalf("expected msgID %q when ParseMsg fails, got %q", expectedMsgID, msgID)
	}
}