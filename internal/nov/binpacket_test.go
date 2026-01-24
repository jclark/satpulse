package nov

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/novmsg"
	"github.com/jclark/satpulse/internal/scantest"
)

var testPacketsValid = []string{
	"aa44121c650000012c00c03461a04c09b019e3167b0c0d1e2499120000000000" +
		"ce4ce7adff50febe19b56918152b443e00000000000032c0e907000008150a27" +
		"c05d0000010000009ef77717",
	"aa44121c650000012c00c03461a04c09981de31664100d1e2499120000000000" +
		"8753e9bc6650febe77fefca7f028443e00000000000032c0e907000008150a27" +
		"a861000001000000c894e5b0",
	"aa44121c650000012c00c03461a04c098021e3164b140d1e2499120000000000" +
		"06b46bbdbf4ffebedfb291f7d7cd433e00000000000032c0e907000008150a27" +
		"9065000001000000588610f7",
}

func TestNovAtelBinaryPackets(t *testing.T) {
	// Create test packets including one with extended header
	testPackets := append([]string{}, testPacketsValid...)
	testPackets = append(testPackets, testExtendPacketHeader(testPacketsValid[0]))

	for i, pkt := range testPackets {
		t.Run(fmt.Sprintf("Packet %d", i+1), func(t *testing.T) {
			data, err := hex.DecodeString(pkt)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			t.Logf("Packet length: %d bytes", len(data))
			t.Logf("First 10 bytes: %x", data[:10])

			// Check if it's a valid packet
			if len(data) >= 10 {
				// Check sync bytes
				if data[0] == 0xAA && data[1] == 0x44 && data[2] == 0x12 {
					t.Logf("Sync bytes OK: AA 44 12")
					t.Logf("Header length field (byte 3): 0x%02x (%d)", data[3], data[3])
					t.Logf("Message ID (bytes 4-5): 0x%04x (%d)",
						uint16(data[4])|uint16(data[5])<<8,
						uint16(data[4])|uint16(data[5])<<8)
					t.Logf("Message type (byte 6): 0x%02x", data[6])
					t.Logf("Port address (byte 7): 0x%02x", data[7])
					if len(data) >= 10 {
						payloadLen := uint16(data[8]) | uint16(data[9])<<8
						t.Logf("Payload length (bytes 8-9): %d", payloadLen)
						expectedTotal := 28 + int(payloadLen) + 4 // header + payload + CRC
						t.Logf("Expected total packet length: %d, actual: %d", expectedTotal, len(data))
					}
				}
			}

			// Test with IsValidPacket
			ok := gpsprot.IsValidPacket(BinPacketFormat, data)
			t.Logf("IsValidPacket returned: %v", ok)

			if ok {
				t.Logf("Message ID: %s", BinPacketFormat.MsgID(data))
			}

			if !ok {
				t.Errorf("Expected to find NovAtel packet but IsValidPacket returned false")
			}

			// Also test with scantest.IsValidPacketBetween
			ok2 := scantest.IsValidPacketBetween(BinPacketFormat, data, 0, len(data))
			t.Logf("scantest.IsValidPacketBetween returned: %v", ok2)

			if ok != ok2 {
				t.Errorf("IsValidPacket and IsValidPacketBetween disagree: %v vs %v", ok, ok2)
			}

			// Test with junk before and after
			junkPrefix := []byte("some junk data")
			junkSuffix := []byte("more junk")
			fullBuffer := append(junkPrefix, data...)
			fullBuffer = append(fullBuffer, junkSuffix...)

			ok3 := scantest.IsValidPacketBetween(BinPacketFormat, fullBuffer, len(junkPrefix), len(junkPrefix)+len(data))
			t.Logf("IsValidPacketBetween with junk: %v", ok3)

			if ok != ok3 {
				t.Errorf("Packet validity changed with junk: expected %v, got %v", ok, ok3)
			}
		})
	}
}

func TestMsgID(t *testing.T) {
	const expectedMsgID = "TIME"
	data, err := hex.DecodeString(testPacketsValid[0])
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}
	got := BinPacketFormat.MsgID(data)
	if got != expectedMsgID {
		t.Errorf("Expected %q got %q", expectedMsgID, got)
	}
}

// testExtendPacketHeader creates a NovAtel binary packet with an extended header
// by taking an existing valid packet, adding extra bytes to the header, and recomputing the checksum.
func testExtendPacketHeader(hexPacket string) string {
	data, err := hex.DecodeString(hexPacket)
	if err != nil {
		panic(err)
	}

	// Original header is 28 bytes (0x1C at offset 3)
	// Split into header, payload, and checksum
	origHeaderLen := int(data[3])
	payloadLen := int(binary.LittleEndian.Uint16(data[8:10]))

	header := data[:origHeaderLen]
	payload := data[origHeaderLen : origHeaderLen+payloadLen]

	// Create extended header by adding 2 bytes (0xFE, 0xFF)
	extendedHeader := make([]byte, origHeaderLen+2)
	copy(extendedHeader, header)
	extendedHeader[origHeaderLen] = 0xFE
	extendedHeader[origHeaderLen+1] = 0xFF

	// Update header length field to 30
	extendedHeader[3] = 30

	// Combine extended header and payload
	newPacket := append(extendedHeader, payload...)

	// Compute new CRC32 checksum
	crc := novmsg.CRC32(newPacket)
	checksumBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(checksumBytes, crc)

	// Append checksum
	newPacket = append(newPacket, checksumBytes...)

	return hex.EncodeToString(newPacket)
}
