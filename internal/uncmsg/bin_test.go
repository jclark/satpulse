package uncmsg

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// testPPSStatusBPacket is a PPSSTATUSB packet captured from a UM980 receiver.
// Message ID: 9000 (0x2328)
var testPPSStatusBPacket = mustHexDecode(
	"aa44b55d28233c0000a0480968e334200000000000121d000300000048090000" +
		"80df3420fcffffffa0b259fe2000e803150000000000000069666600000000" +
		"2bbcd2100100000000acecb02c000000000000000091a800a5",
)

func TestBinaryHeader(t *testing.T) {
	// This test compares the binary header with the corresponding ASCII header
	// from the packet log. The ASCII header for this packet shows:
	// #PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;...
	t.Run("ParseHeaderWithBinaryRead", func(t *testing.T) {
		// Create a reader from the packet bytes
		reader := bytes.NewReader(testPPSStatusBPacket[:24]) // Just the header

		var header BinaryHeader
		err := binary.Read(reader, binary.LittleEndian, &header)
		if err != nil {
			t.Fatalf("binary.Read() error = %v", err)
		}

		// Expected values based on the ASCII header from the packet log
		expected := BinaryHeader{
			Sync1:          0xAA, // Sync byte 1
			Sync2:          0x44, // Sync byte 2
			Sync3:          0xB5, // Sync byte 3
			CPUIdlePercent: 93,   // From ASCII: "93"
			MessageID:      9000, // PPSSTATUS
			MessageLength:  60,   // PPSSTATUS payload size
			TimingHeader: TimingHeader{
				TimeRef:            0,              // 0 = GPS
				TimeStatus:         TimeStatusFine, // 0xA0 = 160 = FINE, from ASCII: "FINE"
				Week:               2376,           // From ASCII: "2376"
				MillisecondsOfWeek: 540337000,      // From ASCII: "540337000"
				Reserved:           0,              // Reserved field
				Version:            0,              // Version field
				LeapSec:            18,             // From ASCII: "18"
				DelayMs:            29,             // From ASCII: "29"
			},
		}

		if header != expected {
			t.Errorf("Header mismatch:\nGot:      %+v\nExpected: %+v", header, expected)
		}
	})
}

func TestBinHeader(t *testing.T) {
	t.Run("PPSSTATUSB", func(t *testing.T) {
		// Parse the binary PPSSTATUS packet to test header parsing
		msgHeader, _, err := ParseBinMsg(testPPSStatusBPacket)
		if err != nil {
			t.Fatalf("ParseBinMsg() error = %v", err)
		}

		// Expected header values - should match ASCII header
		expectedHeader := MessageHeader{
			CPUIdlePercent: 93,
			TimingHeader: TimingHeader{
				TimeRef:            0,
				TimeStatus:         TimeStatusFine,
				Week:               2376,
				MillisecondsOfWeek: 540337000,
				Reserved:           0,
				Version:            0,
				LeapSec:            18,
				DelayMs:            29,
			},
		}

		if msgHeader != expectedHeader {
			t.Errorf("Header mismatch:\nGot:      %+v\nExpected: %+v", msgHeader, expectedHeader)
		}
	})
}

func TestUnknownBinaryMessage(t *testing.T) {
	t.Run("SerializeUnknownAsciiMessageFails", func(t *testing.T) {
		// Create an unknown ASCII message (has no numeric ID)
		unknownMsg := &UnknownAsciiMsg{
			Name:    "TESTMSGA",
			Payload: "data1,data2,data3",
		}
		
		header := MessageHeader{
			CPUIdlePercent: 85,
		}
		
		// Try to serialize as binary - should fail
		_, err := SerializeBinMsg(header, unknownMsg)
		if err == nil {
			t.Fatal("SerializeBinMsg() should have failed for UnknownAsciiMsg")
		}
		if !strings.Contains(err.Error(), "unknown ASCII message cannot be serialized as binary") {
			t.Errorf("Expected error about ASCII message, got: %v", err)
		}
	})
}
