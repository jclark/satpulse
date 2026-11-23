package scan_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/internal/unc"
	"github.com/jclark/satpulse/gps/scan"
)

var uncFormats = []gpsprot.PacketFormat{unc.BinPacketFormat}

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func TestUnicorePacketScanning(t *testing.T) {
	// This is a packet from the live log that was not being recognized
	loggedPacket := mustHexDecode("aa44b55c28233c0000a0480920c86d220000000000122100030000004809000038c46d2203000000b0e94efe2000e80315000000ffffffff696666000000002bbcd2100100000000f8d7b02c0000000000000000118f1252")

	// Create a reader and scanner
	r := strings.NewReader(string(loggedPacket))
	s := scan.New(r, 1024, uncFormats)

	// Scan the packet
	pkt, err := s.Scan()
	if err != nil {
		t.Fatalf("Error scanning packet: %v", err)
	}

	if !pkt.HasTag(unc.BinPacketFormat.Tag()) {
		if pkt.Format == nil {
			t.Fatalf("Packet not recognized as any format (nil)")
		} else {
			t.Fatalf("Packet recognized as %s instead of %s", pkt.Format.Tag(), unc.BinPacketFormat.Tag())
		}
	}

	if pkt.Data != string(loggedPacket) {
		t.Fatalf("Packet data mismatch: expected length %d, got length %d", len(loggedPacket), len(pkt.Data))
	}

	if !pkt.ChecksumValid {
		t.Fatalf("Packet checksum not valid")
	}
}

func TestUnicoreNextFunction(t *testing.T) {
	// Test what happens when we call Next() directly on the Unicore format
	loggedPacket := mustHexDecode("aa44b55c28233c0000a0480920c86d220000000000122100030000004809000038c46d2203000000b0e94efe2000e80315000000ffffffff696666000000002bbcd2100100000000f8d7b02c0000000000000000118f1252")

	// Test first byte (0xAA)
	state := unc.BinPacketFormat.Next(0, loggedPacket, 0, 0) // stateSync=0, packetLen=0
	if state == 0 {
		t.Errorf("Unicore.Next() returned stateSync for first byte 0xAA, should have advanced")
	} else {
		t.Logf("Unicore.Next() correctly returned state %d for first byte 0xAA", state)
	}

	// Test second byte (0x44)
	state = unc.BinPacketFormat.Next(state, loggedPacket, 1, 1)
	if state == 0 {
		t.Errorf("Unicore.Next() returned stateSync for second byte 0x44, should have advanced")
	} else {
		t.Logf("Unicore.Next() correctly returned state %d for second byte 0x44", state)
	}

	// Test third byte (0xB5)
	state = unc.BinPacketFormat.Next(state, loggedPacket, 2, 2)
	if state == 0 {
		t.Errorf("Unicore.Next() returned stateSync for third byte 0xB5, should have advanced")
	} else {
		t.Logf("Unicore.Next() correctly returned state %d for third byte 0xB5", state)
	}
}

func TestUnicoreWithUBXConflict(t *testing.T) {
	// Test that Unicore packets starting with 0xAA 0x44 0xB5 don't get confused with UBX
	// UBX packets start with 0xB5 0x62, so the third byte 0xB5 in Unicore might cause issues
	loggedPacket := mustHexDecode("aa44b55c28233c0000a0480920c86d220000000000122100030000004809000038c46d2203000000b0e94efe2000e80315000000ffffffff696666000000002bbcd2100100000000f8d7b02c0000000000000000118f1252")

	// Add some junk before the packet to test scanning
	junkData := "some random junk data"
	fullData := junkData + string(loggedPacket)

	t.Run("UnicoreOnly", func(t *testing.T) {
		r := strings.NewReader(fullData)
		s := scan.New(r, 1024, uncFormats)

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

		// Second scan should get the Unicore packet
		pkt, err = s.Scan()
		if err != nil {
			t.Fatalf("Error scanning Unicore packet with Unicore-only scanner: %v", err)
		}

		if !pkt.HasTag(unc.BinPacketFormat.Tag()) {
			if pkt.Format == nil {
				t.Fatalf("Unicore packet not recognized as any format (nil)")
			} else {
				t.Fatalf("Unicore packet recognized as %s instead of %s", pkt.Format.Tag(), unc.BinPacketFormat.Tag())
			}
		}
	})

	t.Run("WithAllFormats", func(t *testing.T) {
		// Test with full scanner including UBX
		r := strings.NewReader(fullData)
		s := scan.New(r, 1024, gpsreg.CreatePacketFormats(nil))

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

		// Second scan should get the Unicore packet (not confused with UBX)
		pkt, err = s.Scan()
		if err != nil {
			t.Fatalf("Error scanning Unicore packet with full scanner: %v", err)
		}

		if !pkt.HasTag(unc.BinPacketFormat.Tag()) {
			if pkt.Format == nil {
				t.Fatalf("Unicore packet not recognized as any format (nil)")
			} else {
				t.Fatalf("Unicore packet recognized as %s instead of %s", pkt.Format.Tag(), unc.BinPacketFormat.Tag())
			}
		}
	})
}
