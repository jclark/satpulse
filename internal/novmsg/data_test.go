package novmsg

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

type dataTestCase struct {
	name   string
	ascii  string
	hex    string
	hdr    MessageHeader
	value  Msg
	// fixupValueForBin transforms value from ASCII message into value expected by binary format
	// This is used for VERSIONA/VERSIONB inconsistency:
	// VERSIONB has just build number; VERSIONA has additional info
	fixupValueForBin func(Msg) Msg // Transform ASCII baseline to binary format
	// fixupValueForAscii transforms value from binary message into value expected by ASCII format
	// This is used to handle floating point differences, where ASCII has limited precision.
	fixupValueForAscii func(Msg) Msg // Transform binary baseline to ASCII format
	// fixupHeaderForAscii transforms header from binary baseline to ASCII format
	fixupHeaderForAscii func(MessageHeader) MessageHeader
}

func testDataBin(t *testing.T, tests []dataTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Decode hex string to binary packet
			binPacket, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			// Verify the test data has correct CRC
			if len(binPacket) < 4 {
				t.Fatalf("Packet too short for CRC")
			}
			expectedCRC := binary.LittleEndian.Uint32(binPacket[len(binPacket)-4:])
			actualCRC := CRC32(binPacket[:len(binPacket)-4])
			if actualCRC != expectedCRC {
				t.Errorf("Test data CRC mismatch: got %08x, want %08x", actualCRC, expectedCRC)
			}

			// Test parsing binary packet
			header, msg, err := ParseBinMsg(binPacket)
			if err != nil {
				t.Fatalf("ParseBinMsg() error = %v", err)
			}

			// Use the baseline header directly for binary comparison (binary is canonical)
			expectedHeader := tt.hdr

			if !reflect.DeepEqual(header, expectedHeader) {
				t.Errorf("ParseBinMsg() header mismatch:\nGot:  %+v\nWant: %+v", header, expectedHeader)
			}

			// Apply fixup for binary comparison if needed
			expectedValue := tt.value
			if tt.fixupValueForBin != nil {
				expectedValue = tt.fixupValueForBin(tt.value)
			}

			if !reflect.DeepEqual(msg, expectedValue) {
				t.Errorf("ParseBinMsg() mismatch:\nGot:  %+v\nWant: %+v", msg, expectedValue)
			}

			// Test round-trip: value -> serialize -> compare bytes directly
			serialized, err := SerializeBinMsg(expectedHeader, expectedValue)
			if err != nil {
				t.Fatalf("SerializeBinMsg() error = %v", err)
			}

			if !bytes.Equal(serialized, binPacket) {
				t.Errorf("SerializeBinMsg() round-trip failed:\nGot:  %x\nWant: %x", serialized, binPacket)
			}
		})
	}
}

func testDataAscii(t *testing.T, tests []dataTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing ASCII packet
			header, msg, err := ParseAsciiMessage([]byte(tt.ascii))
			if err != nil {
				t.Fatalf("ParseAsciiMessage() error = %v", err)
			}

			// Apply header fixup for ASCII comparison if needed
			expectedHeader := tt.hdr
			if tt.fixupHeaderForAscii != nil {
				expectedHeader = tt.fixupHeaderForAscii(tt.hdr)
			}

			if !reflect.DeepEqual(header, expectedHeader) {
				t.Errorf("ParseAsciiMessage() header mismatch:\nGot:  %+v\nWant: %+v", header, expectedHeader)
			}

			// Apply fixup for ASCII comparison if needed
			expectedValue := tt.value
			if tt.fixupValueForAscii != nil {
				expectedValue = tt.fixupValueForAscii(tt.value)
			}

			if !reflect.DeepEqual(msg, expectedValue) {
				t.Errorf("ParseAsciiMessage() mismatch:\nGot:  %+v\nWant: %+v", msg, expectedValue)
			}

			// Test round-trip: value -> serialize -> parse using expectedValue for both directions
			serialized, err := SerializeAsciiMsg(expectedHeader, expectedValue)
			if err != nil {
				t.Fatalf("SerializeAsciiMsg() error = %v", err)
			}

			header2, msg2, err := ParseAsciiMessage(serialized)
			if err != nil {
				t.Fatalf("ParseAsciiMessage() on serialized packet error = %v", err)
			}

			if !reflect.DeepEqual(header2, expectedHeader) {
				t.Errorf("ParseAsciiMessage() on serialized header mismatch:\nGot:  %+v\nWant: %+v", header2, expectedHeader)
			}

			if !reflect.DeepEqual(msg2, expectedValue) {
				t.Errorf("ParseAsciiMessage() on serialized mismatch:\nGot:  %+v\nWant: %+v", msg2, expectedValue)
			}
		})
	}
}

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// fixupFloat simulates the receiver's float formatting by doing: binary -> sprintf -> parse
func fixupFloat(val *float64, format string) {
	str := fmt.Sprintf(format, *val)
	result, err := strconv.ParseFloat(str, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to round-trip float64 %v: %v", *val, err))
	}
	*val = result
}
