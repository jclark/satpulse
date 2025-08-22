package scan

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nov"
)

func TestNovAtelPacketScanning(t *testing.T) {
	// Test data from the log - combining bin and ascii chunks between UNCA packets
	// This represents a NovAtel RECTIMEA packet (message ID 101)
	packet1Hex := "aa44121c650000012c00c03461a04c09b019e3167b0c0d1e24" + "99120000000000" +
		"ce4ce7adff50febe19b56918152b443e00000000000032c0e907000008150a27" +
		"c05d0000010000009ef77717"
	
	loggedPacket, err := hex.DecodeString(packet1Hex)
	if err != nil {
		t.Fatalf("Failed to decode test packet: %v", err)
	}
	
	// Create a reader and scanner
	r := strings.NewReader(string(loggedPacket))
	s := New(r, 1024)
	
	// Scan the packet
	pkt, err := s.Scan()
	if err != nil {
		t.Fatalf("Error scanning packet: %v", err)
	}
	
	if !pkt.HasTag(nov.BinPacketFormat.Tag()) {
		if pkt.Format == nil {
			t.Fatalf("Packet not recognized as any format (nil)")
		} else {
			t.Fatalf("Packet recognized as %s instead of %s", pkt.Format.Tag(), nov.BinPacketFormat.Tag())
		}
	}
	
	if pkt.Data != string(loggedPacket) {
		t.Fatalf("Packet data mismatch: expected length %d, got length %d", len(loggedPacket), len(pkt.Data))
	}
	
	if !pkt.ChecksumValid {
		t.Fatalf("Packet checksum not valid")
	}
	
	// Check message ID
	msgID := nov.BinPacketFormat.MsgID([]byte(pkt.Data))
	if msgID != "101" {
		t.Errorf("Expected message ID 101, got %s", msgID)
	}
}

func TestNovAtelNextFunction(t *testing.T) {
	// Test what happens when we call Next() directly on the NovAtel format
	packet1Hex := "aa44121c650000012c00c03461a04c09b019e3167b0c0d1e24" + "99120000000000" +
		"ce4ce7adff50febe19b56918152b443e00000000000032c0e907000008150a27" +
		"c05d0000010000009ef77717"
	
	loggedPacket, err := hex.DecodeString(packet1Hex)
	if err != nil {
		t.Fatalf("Failed to decode test packet: %v", err)
	}
	
	// Test first byte (0xAA)
	state := nov.BinPacketFormat.Next(0, loggedPacket, 0, 0) // stateSync=0, packetLen=0
	if state == 0 {
		t.Errorf("NovAtel.Next() returned stateSync for first byte 0xAA, should have advanced")
	} else {
		t.Logf("NovAtel.Next() correctly returned state %d for first byte 0xAA", state)
	}
	
	// Test second byte (0x44)  
	state = nov.BinPacketFormat.Next(state, loggedPacket, 1, 1)
	if state == 0 {
		t.Errorf("NovAtel.Next() returned stateSync for second byte 0x44, should have advanced")
	} else {
		t.Logf("NovAtel.Next() correctly returned state %d for second byte 0x44", state)
	}
	
	// Test third byte (0x12)
	state = nov.BinPacketFormat.Next(state, loggedPacket, 2, 2)
	if state == 0 {
		t.Errorf("NovAtel.Next() returned stateSync for third byte 0x12, should have advanced")
	} else {
		t.Logf("NovAtel.Next() correctly returned state %d for third byte 0x12", state)
	}
}

func TestNovAtelWithJunkData(t *testing.T) {
	// Test that NovAtel packets are properly detected with junk data before them
	packet1Hex := "aa44121c650000012c00c03461a04c09b019e3167b0c0d1e24" + "99120000000000" +
		"ce4ce7adff50febe19b56918152b443e00000000000032c0e907000008150a27" +
		"c05d0000010000009ef77717"
	
	loggedPacket, err := hex.DecodeString(packet1Hex)
	if err != nil {
		t.Fatalf("Failed to decode test packet: %v", err)
	}
	
	// Add some junk before the packet to test scanning
	junkData := "some random junk data"
	fullData := junkData + string(loggedPacket)
	
	t.Run("NovAtelOnly", func(t *testing.T) {
		// Create a scanner with ONLY NovAtel format
		r := strings.NewReader(fullData)
		s := &Scanner{
			pktFormats: []gpsprot.PacketFormat{nov.BinPacketFormat},
			r:          r,
			buf:        make([]byte, 0, 1024),
		}
		
		// First scan should get the junk
		pkt, err := s.Scan()
		if err != nil {
			t.Fatalf("Error scanning junk: %v", err)
		}
		if pkt.Format != nil {
			t.Fatalf("Junk data incorrectly recognized as %s", pkt.Format.Tag())
		}
		if pkt.Data != junkData {
			t.Fatalf("Junk data mismatch: expected %q, got %q", junkData, pkt.Data)
		}
		
		// Second scan should get the NovAtel packet
		pkt, err = s.Scan()
		if err != nil {
			t.Fatalf("Error scanning NovAtel packet with NovAtel-only scanner: %v", err)
		}
		
		if !pkt.HasTag(nov.BinPacketFormat.Tag()) {
			if pkt.Format == nil {
				t.Fatalf("NovAtel packet not recognized as any format (nil)")
			} else {
				t.Fatalf("NovAtel packet recognized as %s instead of %s", pkt.Format.Tag(), nov.BinPacketFormat.Tag())
			}
		}
	})
	
	t.Run("WithAllFormats", func(t *testing.T) {
		// Test with full scanner including all formats
		r := strings.NewReader(fullData)
		s := New(r, 1024)
		
		// First scan should get the junk
		pkt, err := s.Scan()
		if err != nil {
			t.Fatalf("Error scanning junk: %v", err)
		}
		if pkt.Format != nil {
			t.Fatalf("Junk data incorrectly recognized as %s", pkt.Format.Tag())
		}
		if pkt.Data != junkData {
			t.Fatalf("Junk data mismatch: expected %q, got %q", junkData, pkt.Data)
		}
		
		// Second scan should get the NovAtel packet
		pkt, err = s.Scan()
		if err != nil {
			t.Fatalf("Error scanning NovAtel packet with full scanner: %v", err)
		}
		
		if !pkt.HasTag(nov.BinPacketFormat.Tag()) {
			if pkt.Format == nil {
				t.Fatalf("NovAtel packet not recognized as any format (nil)")
			} else {
				t.Fatalf("NovAtel packet recognized as %s instead of %s", pkt.Format.Tag(), nov.BinPacketFormat.Tag())
			}
		}
	})
}