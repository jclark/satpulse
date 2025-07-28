package unc

import (
	"encoding/hex"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

// testPPSStatusBPacket is a PPSSTATUSB packet captured from a UM980 receiver.
// Message ID: 9000 (0x2328)
var testPPSStatusBPacket = mustHexDecode(
	"aa44b55d28233c0000a0480968e334200000000000121d000300000048090000" +
		"80df3420fcffffffa0b259fe2000e803150000000000000069666600000000" +
		"2bbcd2100100000000acecb02c000000000000000091a800a5",
)

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func TestBinPacketFormat(t *testing.T) {
	t.Run("ValidPPSSTATUSB", func(t *testing.T) {
		if !testIsValidPacket(t, BinPacketFormat, testPPSStatusBPacket) {
			t.Errorf("testIsValidPacket() failed for a valid PPSSTATUSB packet")
		}
	})

	t.Run("MsgID", func(t *testing.T) {
		const expectedID = "PPSSTATUS"
		msgID := BinPacketFormat.MsgID(testPPSStatusBPacket)
		if msgID != expectedID {
			t.Errorf("MsgID() = %q, want %q", msgID, expectedID)
		}
	})

	t.Run("Checksum", func(t *testing.T) {
		extracted := BinPacketFormat.ExtractChecksum(testPPSStatusBPacket)
		computed := BinPacketFormat.ComputeChecksum(testPPSStatusBPacket)

		if len(extracted) != len(computed) || len(extracted) != 4 {
			t.Fatalf("Invalid checksum length. Got: %d, Want: 4", len(extracted))
		}

		if string(extracted) != string(computed) {
			t.Errorf("Checksum mismatch. Extracted: %X, Computed: %X", extracted, computed)
		}
	})
}

// testIsValidPacketBetween tests if a packet is valid when embedded in a larger buffer
// start and end are indices in buf where the packet data is located
func testIsValidPacketBetween(pf gpsprot.PacketFormat, buf []byte, start, end int) bool {
	if start < 0 || end > len(buf) || start >= end {
		return false
	}
	
	state := gpsprot.ScanStateSync
	packetLen := 0
	
	for i := start; i < end; i++ {
		state = pf.Next(state, buf, i, packetLen)
		if state == gpsprot.ScanStateSync {
			return false
		}
		packetLen++
	}
	
	return pf.IsFinal(state)
}

// testIsValidPacket tests if a packet is valid, including with junk before/after
// Returns true only if the packet maintains its validity in all junk scenarios
func testIsValidPacket(t *testing.T, pf gpsprot.PacketFormat, packet []byte) bool {
	t.Helper()
	
	// Establish expectation with baseline test
	expected := gpsprot.IsValidPacket(pf, packet)
	
	// Different types of junk to test with
	junkVariants := [][]byte{
		[]byte("some random text data"),
		[]byte{0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD},           // Binary junk
		[]byte{0xB5, 0x62, 0x01, 0x02},                       // UBX-like prefix (potential conflict)
		[]byte("$GPGGA,123456.00,1234.5678,N"),               // NMEA-like prefix
		[]byte{0xD3, 0x00, 0x13, 0x3E, 0xD7, 0xD3, 0x02, 0x02}, // RTCM-like prefix
		[]byte{0xAA, 0x44},                                   // Partial Unicore sync
		[]byte{0xAA, 0x44, 0xB5, 0x99, 0x88},                // Unicore sync with wrong 4th byte
		make([]byte, 50),                                     // Zeros
	}
	
	// Test with different junk prefixes
	for i, junk := range junkVariants {
		fullBuffer := append(junk, packet...)
		start := len(junk)
		end := len(fullBuffer)
		
		actual := testIsValidPacketBetween(pf, fullBuffer, start, end)
		if actual != expected {
			t.Errorf("Validity changed with junk prefix %d: expected %v, got %v", i, expected, actual)
			t.Logf("Junk prefix: %x", junk)
			return false
		}
	}
	
	// Test with junk suffix
	for i, junk := range junkVariants {
		fullBuffer := append(packet, junk...)
		start := 0
		end := len(packet)
		
		actual := testIsValidPacketBetween(pf, fullBuffer, start, end)
		if actual != expected {
			t.Errorf("Validity changed with junk suffix %d: expected %v, got %v", i, expected, actual)
			t.Logf("Junk suffix: %x", junk)
			return false
		}
	}
	
	// Test with junk both before and after (limited combinations)
	for i, prefixJunk := range junkVariants[:3] {
		for j, suffixJunk := range junkVariants[:3] {
			fullBuffer := append(prefixJunk, packet...)
			fullBuffer = append(fullBuffer, suffixJunk...)
			start := len(prefixJunk)
			end := start + len(packet)
			
			actual := testIsValidPacketBetween(pf, fullBuffer, start, end)
			if actual != expected {
				t.Errorf("Validity changed with junk prefix %d + suffix %d: expected %v, got %v", i, j, expected, actual)
				t.Logf("Prefix: %x, Suffix: %x", prefixJunk, suffixJunk)
				return false
			}
		}
	}
	
	return expected
}

func TestLoggedPacket(t *testing.T) {
	// This is a packet from the live log that was not being recognized
	loggedPacket := mustHexDecode("aa44b55c28233c0000a0480920c86d220000000000122100030000004809000038c46d2203000000b0e94efe2000e80315000000ffffffff696666000000002bbcd2100100000000f8d7b02c0000000000000000118f1252")
	
	if !testIsValidPacket(t, BinPacketFormat, loggedPacket) {
		t.Errorf("testIsValidPacket() failed for logged packet - this explains why packets aren't being recognized")
	}
	
	msgID := BinPacketFormat.MsgID(loggedPacket)
	t.Logf("Message ID of logged packet: %s", msgID)
}

