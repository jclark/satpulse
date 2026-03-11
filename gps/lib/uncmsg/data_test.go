package uncmsg

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/lib/novmsg"
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

			validateAsciiChecksum(t, serialized, asciiChecksumHexLen(tt.asciiPacket))

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

// asciiChecksumHexLen returns the number of hex digits in the checksum of an ASCII packet.
func asciiChecksumHexLen(packet string) int {
	s := strings.TrimRight(packet, "\r\n")
	i := strings.LastIndex(s, "*")
	if i < 0 {
		return 0
	}
	return len(s) - i - 1
}

// validateAsciiChecksum checks that serialized has a checksum with wantHexLen hex digits
// and that the checksum value is correct for the packet data.
func validateAsciiChecksum(t *testing.T, serialized []byte, wantHexLen int) {
	t.Helper()
	s := strings.TrimRight(string(serialized), "\r\n")
	if len(s) == 0 || s[0] != '#' {
		t.Errorf("validateAsciiChecksum: packet does not start with '#'")
		return
	}
	data := s[1:]
	i := strings.LastIndex(data, "*")
	if i < 0 {
		t.Errorf("validateAsciiChecksum: no '*' found in packet")
		return
	}
	dataForChecksum := data[:i]
	checksumHex := data[i+1:]
	if len(checksumHex) != wantHexLen {
		t.Errorf("SerializeAsciiMsg() checksum has %d hex digits, want %d (checksum: *%s)", len(checksumHex), wantHexLen, checksumHex)
		return
	}
	val, err := strconv.ParseUint(checksumHex, 16, 32)
	if err != nil {
		t.Errorf("validateAsciiChecksum: invalid checksum %q: %v", checksumHex, err)
		return
	}
	switch wantHexLen {
	case 2:
		var xor byte
		for _, b := range []byte(dataForChecksum) {
			xor ^= b
		}
		if byte(val) != xor {
			t.Errorf("SerializeAsciiMsg() XOR checksum = %02x, want %02x", val, xor)
		}
	case 8:
		want := novmsg.CRC32([]byte(dataForChecksum))
		if uint32(val) != want {
			t.Errorf("SerializeAsciiMsg() CRC32 checksum = %08x, want %08x", val, want)
		}
	}
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
