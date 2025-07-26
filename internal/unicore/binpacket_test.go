package unicore

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
		if !gpsprot.IsValidPacket(BinPacketFormat, testPPSStatusBPacket) {
			t.Errorf("IsValidPacket() failed for a valid PPSSTATUSB packet")
		}
	})

	t.Run("MsgID", func(t *testing.T) {
		const expectedID = "9000"
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
