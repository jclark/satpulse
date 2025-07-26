package unicore

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestPPSSTATUS(t *testing.T) {
	t.Run("BinaryDecode", func(t *testing.T) {
		// Extract payload (skip header, exclude CRC)
		payload := testPPSStatusBPacket[headerLength : len(testPPSStatusBPacket)-crcLength]

		// Decode using binary.Read
		reader := bytes.NewReader(payload)
		var decoded PPSSTATUS
		err := binary.Read(reader, binary.LittleEndian, &decoded)
		if err != nil {
			t.Fatalf("binary.Read() error = %v", err)
		}

		// Expected values based on the ASCII PPSSTATUS message from packet log:
		// #PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a
		expected := PPSSTATUS{
			Status:       3,          // First field after semicolon
			Week:         2376,       // Second field
			MsSecInWeek:  540336000,  // Third field
			PPSPulseErr:  -4,         // Fourth field
			OffsetTime:   -27676000,  // Fifth field
			ConfigInfo:   0x03E80020, // Sixth field (hex format as in ASCII)
			Register:     0x00000015, // Seventh field (hex format as in ASCII)
			TimeEstErr:   0,          // Eighth field
			InnerQuality: 0x00666669, // Ninth field (hex format as in ASCII)
			PPSInnerSta:  0x2B000000, // Tenth field (hex format as in ASCII)
			PPSCfgSta:    0x0110D2BC, // Eleventh field (hex format as in ASCII)
			PPSStage:     0x00000000, // Twelfth field (hex format as in ASCII)
			Reserved:     0x2CB0ECAC, // Thirteenth field (hex format as in ASCII)
			// Remaining reserved fields: 14th, 15th are 0x00000000, 0x00000000
		}

		if decoded != expected {
			t.Errorf("PPSSTATUS decode mismatch:\nGot:      %+v\nExpected: %+v", decoded, expected)
		}
	})
}
