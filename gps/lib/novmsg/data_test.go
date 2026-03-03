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

type dataTestCase[P ~uint8] struct {
	name    string
	ascii   string
	hex     string
	hdr     MsgHdr[P]
	value   MsgBody
	disable bool
	// fixupValueForBin transforms value from ASCII message into value expected by binary format
	fixupValueForBin func(MsgBody) MsgBody
	// fixupValueForAscii transforms value from binary message into value expected by ASCII format
	fixupValueForAscii func(MsgBody) MsgBody
	// fixupHeaderForAscii transforms header from binary baseline to ASCII format
	fixupHeaderForAscii func(MsgHdr[P]) MsgHdr[P]
}

func testDataBin[P ~uint8](t *testing.T, tests []dataTestCase[P],
	binCtors map[MsgID]func() MsgBody) {
	t.Helper()
	for _, tt := range tests {
		if tt.disable {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			binPacket, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}
			if len(binPacket) < 4 {
				t.Fatalf("Packet too short for CRC")
			}
			expectedCRC := binary.LittleEndian.Uint32(binPacket[len(binPacket)-4:])
			actualCRC := CRC32(binPacket[:len(binPacket)-4])
			if actualCRC != expectedCRC {
				t.Errorf("Test data CRC mismatch: got %08x, want %08x", actualCRC, expectedCRC)
			}
			msg, err := ParseBinMsgUsing[P](binPacket, binCtors)
			if err != nil {
				t.Fatalf("ParseBinMsgUsing() error = %v", err)
			}
			expectedHeader := tt.hdr
			if !reflect.DeepEqual(msg.Hdr, expectedHeader) {
				t.Errorf("ParseBinMsgUsing() header mismatch:\nGot:  %+v\nWant: %+v", msg.Hdr, expectedHeader)
			}
			expectedValue := tt.value
			if tt.fixupValueForBin != nil {
				expectedValue = tt.fixupValueForBin(tt.value)
			}
			if !reflect.DeepEqual(msg.Body, expectedValue) {
				t.Errorf("ParseBinMsgUsing() mismatch:\nGot:  %+v\nWant: %+v", msg.Body, expectedValue)
			}
			testMsg := &Msg[P]{Hdr: expectedHeader, Body: expectedValue}
			serialized, err := SerializeBinMsg(testMsg)
			if err != nil {
				t.Fatalf("SerializeBinMsg() error = %v", err)
			}
			if !bytes.Equal(serialized, binPacket) {
				t.Errorf("SerializeBinMsg() round-trip failed:\nGot:  %x\nWant: %x", serialized, binPacket)
			}
		})
	}
}

func testDataAscii[P ~uint8](t *testing.T, tests []dataTestCase[P],
	asciiCtors map[string]func() MsgBody) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseAsciiMsgUsing[P]([]byte(tt.ascii), asciiCtors)
			if err != nil {
				t.Fatalf("ParseAsciiMsgUsing() error = %v", err)
			}
			expectedHeader := tt.hdr
			if tt.fixupHeaderForAscii != nil {
				expectedHeader = tt.fixupHeaderForAscii(tt.hdr)
			}
			if !reflect.DeepEqual(msg.Hdr, expectedHeader) {
				t.Errorf("ParseAsciiMsgUsing() header mismatch:\nGot:  %+v\nWant: %+v", msg.Hdr, expectedHeader)
			}
			expectedValue := tt.value
			if tt.fixupValueForAscii != nil {
				expectedValue = tt.fixupValueForAscii(tt.value)
			}
			if !reflect.DeepEqual(msg.Body, expectedValue) {
				t.Errorf("ParseAsciiMsgUsing() mismatch:\nGot:  %+v\nWant: %+v", msg.Body, expectedValue)
			}
			testMsg := &Msg[P]{Hdr: expectedHeader, Body: expectedValue}
			serialized, err := SerializeAsciiMsg(testMsg)
			if err != nil {
				t.Fatalf("SerializeAsciiMsg() error = %v", err)
			}
			msg2, err := ParseAsciiMsgUsing[P](serialized, asciiCtors)
			if err != nil {
				t.Fatalf("ParseAsciiMsgUsing() on serialized packet error = %v", err)
			}
			if !reflect.DeepEqual(msg2.Hdr, expectedHeader) {
				t.Errorf("ParseAsciiMsgUsing() on serialized header mismatch:\nGot:  %+v\nWant: %+v", msg2.Hdr, expectedHeader)
			}
			if !reflect.DeepEqual(msg2.Body, expectedValue) {
				t.Errorf("ParseAsciiMsgUsing() on serialized mismatch:\nGot:  %+v\nWant: %+v", msg2.Body, expectedValue)
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
