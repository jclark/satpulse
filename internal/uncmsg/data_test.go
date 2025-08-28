package uncmsg

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

type dataTestCase struct {
	name        string
	binPacket   []byte
	asciiPacket string
	msg         *Msg
	// fixupMsgForBin transforms msg from ASCII message into msg expected by binary format
	// This is used for VERSIONA/VERSIONB inconsistency:
	// VERSIONB has just build number; VERSIONA has additional info
	fixupMsgForBin func(*Msg) *Msg // Transform ASCII baseline to binary format
	// fixupMsgForAscii transforms msg from binary message into msg expected by ASCII format
	// This is used to handle floating point differences, where ASCII has limited precision.
	fixupMsgForAscii func(*Msg) *Msg // Transform binary baseline to ASCII format
}

func testDataBin(t *testing.T, tests []dataTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing binary packet
			msg, err := ParseBinMsg(tt.binPacket)
			if err != nil {
				t.Fatalf("ParseBinMsg() error = %v", err)
			}

			// Apply fixup for binary comparison if needed
			expectedMsg := tt.msg
			if tt.fixupMsgForBin != nil {
				expectedMsg = tt.fixupMsgForBin(tt.msg)
			}

			compareMsg(t, msg, expectedMsg, "ParseBinMsg()")

			// Test round-trip: msg -> serialize -> compare bytes directly
			serialized, err := SerializeBinMsg(expectedMsg)
			if err != nil {
				t.Fatalf("SerializeBinMsg() error = %v", err)
			}

			if !bytes.Equal(serialized, tt.binPacket) {
				t.Errorf("SerializeBinMsg() round-trip failed:\nGot:  %x\nWant: %x", serialized, tt.binPacket)
			}
		})
	}
}

func testDataAscii(t *testing.T, tests []dataTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing ASCII packet
			msg, err := ParseAsciiMessage([]byte(tt.asciiPacket))
			if err != nil {
				t.Fatalf("ParseAsciiMessage() error = %v", err)
			}

			// Apply fixup for ASCII comparison if needed
			expectedMsg := tt.msg
			if tt.fixupMsgForAscii != nil {
				expectedMsg = tt.fixupMsgForAscii(tt.msg)
			}

			compareMsg(t, msg, expectedMsg, "ParseAsciiMessage()")

			// Test round-trip: msg -> serialize -> parse using expectedMsg for both directions
			serialized, err := SerializeAsciiMsg(expectedMsg)
			if err != nil {
				t.Fatalf("SerializeAsciiMsg() error = %v", err)
			}

			msg2, err := ParseAsciiMessage(serialized)
			if err != nil {
				t.Fatalf("ParseAsciiMessage() on serialized packet error = %v", err)
			}

			compareMsg(t, msg2, expectedMsg, "ParseAsciiMessage() on serialized")
		})
	}
}

// compareMsg compares two Msg structs and reports detailed differences
func compareMsg(t *testing.T, got, want *Msg, context string) {
	t.Helper()
	if !reflect.DeepEqual(got.Hdr, want.Hdr) {
		t.Errorf("%s header mismatch:\nGot:  %+v\nWant: %+v", context, got.Hdr, want.Hdr)
	}	
	if !reflect.DeepEqual(got.Body, want.Body) {
		t.Errorf("%s body mismatch:\nGot:  %+v\nWant: %+v", context, got.Body, want.Body)
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
