package unicore

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

// ppsStatusBPacket is a PPSSTATUSB packet captured from a UM980 receiver.
// Message ID: 9000 (0x2328)
var ppsStatusBPacket = mustHexDecode(
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
		if !gpsprot.IsValidPacket(BinPacketFormat, ppsStatusBPacket) {
			t.Errorf("IsValidPacket() failed for a valid PPSSTATUSB packet")
		}
	})

	t.Run("MsgID", func(t *testing.T) {
		const expectedID = "9000"
		msgID := BinPacketFormat.MsgID(ppsStatusBPacket)
		if msgID != expectedID {
			t.Errorf("MsgID() = %q, want %q", msgID, expectedID)
		}
	})

	t.Run("Checksum", func(t *testing.T) {
		extracted := BinPacketFormat.ExtractChecksum(ppsStatusBPacket)
		computed := BinPacketFormat.ComputeChecksum(ppsStatusBPacket)

		if len(extracted) != len(computed) || len(extracted) != 4 {
			t.Fatalf("Invalid checksum length. Got: %d, Want: 4", len(extracted))
		}

		if string(extracted) != string(computed) {
			t.Errorf("Checksum mismatch. Extracted: %X, Computed: %X", extracted, computed)
		}
	})
}

func TestBinaryHeader(t *testing.T) {
	// This test compares the binary header with the corresponding ASCII header
	// from the packet log. The ASCII header for this packet shows:
	// #PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;...
	t.Run("ParseHeaderWithBinaryRead", func(t *testing.T) {
		// Create a reader from the packet bytes
		reader := bytes.NewReader(ppsStatusBPacket[:24]) // Just the header
		
		var header BinaryHeader
		err := binary.Read(reader, binary.LittleEndian, &header)
		if err != nil {
			t.Fatalf("binary.Read() error = %v", err)
		}

		// Expected values based on the ASCII header from the packet log
		expected := BinaryHeader{
			Sync1:              0xAA,
			Sync2:              0x44,
			Sync3:              0xB5,
			CPUIdlePercent:     93,      // From ASCII: "93"
			MessageID:          9000,    // PPSSTATUS
			MessageLength:      60,      // PPSSTATUS payload size
			TimeRef:            0,       // 0 = GPS
			TimeStatus:         0xA0,    // Time status from binary
			Week:               2376,    // From ASCII: "2376"
			MillisecondsOfWeek: 540337000, // From ASCII: "540337000"
			Reserved:           0,       // Reserved field
			Version:            0,       // Version field
			LeapSec:            18,      // From ASCII: "18"
			DelayMs:            29,      // From ASCII: "29"
		}

		if header != expected {
			t.Errorf("Header mismatch:\nGot:      %+v\nExpected: %+v", header, expected)
		}
	})
}
