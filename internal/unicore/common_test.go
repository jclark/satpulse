package unicore

import (
	"bytes"
	"encoding/binary"
	"testing"
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

const testPPSStatusAsciiMessage = "#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a\r\n"

func TestAsciiHeader(t *testing.T) {
	t.Run("PPSSTATUSA", func(t *testing.T) {
		// Parse the ASCII PPSSTATUS message to test header parsing
		msgHeader, _, err := ParseAsciiMessage([]byte(testPPSStatusAsciiMessage))
		if err != nil {
			t.Fatalf("ParseAsciiMessage() error = %v", err)
		}

		// Expected header values based on ASCII packet
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
