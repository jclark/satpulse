package uncmsg

import (
	"encoding/binary"
	"testing"
)

var testPacketData = []byte{
	0xAA, 0x44, 0x12, 0x1C, 0x2A, 0x00, 0x02, 0x20, 0x48, 0x00, 0x00, 0x00, 0x90,
	0xB4, 0x93, 0x05, 0xB0, 0xAB, 0xB9, 0x12, 0x00, 0x00, 0x00, 0x00, 0x45, 0x61,
	0xBC, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x1B, 0x04, 0x50,
	0xB3, 0xF2, 0x8E, 0x49, 0x40, 0x16, 0xFA, 0x6B, 0xBE, 0x7C, 0x82, 0x5C, 0xC0,
	0x00, 0x60, 0x76, 0x9F, 0x44, 0x9F, 0x90, 0x40, 0xA6, 0x2A, 0x82, 0xC1, 0x3D,
	0x00, 0x00, 0x00, 0x12, 0x5A, 0xCB, 0x3F, 0xCD, 0x9E, 0x98, 0x3F, 0xDB, 0x66,
	0x40, 0x40, 0x00, 0x30, 0x30, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x0B, 0x0B, 0x00, 0x00, 0x00, 0x06, 0x00, 0x03, 0x42, 0xdc, 0x4c, 0x48,
}

func TestCRC32(t *testing.T) {
	data := testPacketData[:len(testPacketData)-4]
	expectedChecksumBytes := testPacketData[len(testPacketData)-4:]
	expectedCRC := binary.LittleEndian.Uint32(expectedChecksumBytes)

	t.Run("TableImplementation", func(t *testing.T) {
		crc := CRC32(data)
		if crc != expectedCRC {
			t.Errorf("calculateCRC32() = 0x%X, want 0x%X", crc, expectedCRC)
		}
	})

	t.Run("ReferenceImplementation", func(t *testing.T) {
		crc := testCRC32(data)
		if crc != expectedCRC {
			t.Errorf("calculateBlockCRC32() = 0x%X, want 0x%X", crc, expectedCRC)
		}
	})
}

const testCRC32Polynomial uint32 = 0xEDB88320

// testCRC32Value computes a CRC value for a single byte, used by the reference implementation.
func testCRC32Value(i uint32) uint32 {
	crc := i
	for j := 0; j < 8; j++ {
		if crc&1 != 0 {
			crc = (crc >> 1) ^ testCRC32Polynomial
		} else {
			crc >>= 1
		}
	}
	return crc
}

// testCRC32 computes the CRC-32 of a block of data without using a lookup table.
// This serves as a cross-check for the table-based implementation.
func testCRC32(data []byte) uint32 {
	var crc uint32 = 0
	for _, b := range data {
		temp1 := (crc >> 8) & 0x00FFFFFF
		temp2 := testCRC32Value((crc ^ uint32(b)) & 0xFF)
		crc = temp1 ^ temp2
	}
	return crc
}

func FuzzCRC32(f *testing.F) {
	// Add seed data
	f.Add(testPacketData[:len(testPacketData)-4])
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		table := CRC32(data)
		reference := testCRC32(data)
		if table != reference {
			t.Errorf("CRC32 implementations differ for data %x: table=0x%X, reference=0x%X", data, table, reference)
		}
	})
}
