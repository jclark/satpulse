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
		reader := bytes.NewReader(ppsStatusBPacket[:24]) // Just the header
		
		var header BinaryHeader
		err := binary.Read(reader, binary.LittleEndian, &header)
		if err != nil {
			t.Fatalf("binary.Read() error = %v", err)
		}

		// Expected values based on the ASCII header from the packet log
		expected := BinaryHeader{
			Sync1:         0xAA,  // Sync byte 1
			Sync2:         0x44,  // Sync byte 2
			Sync3:         0xB5,  // Sync byte 3
			CPUIdlePercent: 93,   // From ASCII: "93"
			MessageID:     9000,  // PPSSTATUS
			MessageLength: 60,    // PPSSTATUS payload size
			TimingHeader: TimingHeader{
				TimeRef:            0,                 // 0 = GPS
				TimeStatus:         TimeStatusFine,    // 0xA0 = 160 = FINE, from ASCII: "FINE"
				Week:               2376,              // From ASCII: "2376"
				MillisecondsOfWeek: 540337000,         // From ASCII: "540337000"
				Reserved:           0,                 // Reserved field
				Version:            0,                 // Version field
				LeapSec:            18,                // From ASCII: "18"
				DelayMs:            29,                // From ASCII: "29"
			},
		}

		if header != expected {
			t.Errorf("Header mismatch:\nGot:      %+v\nExpected: %+v", header, expected)
		}
	})
}

func TestParseAndSerializeMsg(t *testing.T) {
	t.Run("PPSSTATUSRoundTrip", func(t *testing.T) {
		// Parse the captured PPSSTATUS packet
		msgHeader, msg, err := ParseMsg(ppsStatusBPacket)
		if err != nil {
			t.Fatalf("ParseMsg() error = %v", err)
		}

		// Verify we got a PPSSTATUS message
		ppsMsg, ok := msg.(*PPSSTATUS)
		if !ok {
			t.Fatalf("Expected *PPSSTATUS, got %T", msg)
		}

		// Verify message ID
		if ppsMsg.ID() != PPSStatusID {
			t.Errorf("Message ID = %v, want %v", ppsMsg.ID(), PPSStatusID)
		}

		// Verify header values match expected
		expectedHeader := MessageHeader{
			CPUIdlePercent: 93, // From test data
			TimingHeader: TimingHeader{
				TimeRef:            0,             // GPS
				TimeStatus:         TimeStatusFine, // FINE
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

		// Serialize the message back to binary
		serialized, err := SerializeMsg(msgHeader, ppsMsg)
		if err != nil {
			t.Fatalf("SerializeMsg() error = %v", err)
		}

		// Verify the serialized packet matches the original
		if len(serialized) != len(ppsStatusBPacket) {
			t.Errorf("Serialized length = %d, want %d", len(serialized), len(ppsStatusBPacket))
		}

		if string(serialized) != string(ppsStatusBPacket) {
			t.Errorf("Serialized packet does not match original")
			t.Logf("Original:   %x", ppsStatusBPacket)
			t.Logf("Serialized: %x", serialized)
		}
	})
}