package uncmsg

import (
	"strings"
	"testing"
)

const testPPSStatusAsciiMessage = "#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a\r\n"

func TestAsciiHeader(t *testing.T) {
	t.Run("PPSSTATUSA", func(t *testing.T) {
		// Parse the ASCII PPSSTATUS message to test header parsing
		msg, err := ParseAsciiMessage([]byte(testPPSStatusAsciiMessage))
		if err != nil {
			t.Fatalf("ParseAsciiMessage() error = %v", err)
		}

		// Expected header values based on ASCII packet
		expectedHeader := MsgHdr{
			CPUIdlePercent: 93,
			TimingHdr: TimingHdr{
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

		if msg.Hdr != expectedHeader {
			t.Errorf("Header mismatch:\nGot:      %+v\nExpected: %+v", msg.Hdr, expectedHeader)
		}
	})
}

func TestSplitDataFields(t *testing.T) {
	tests := []struct {
		input  string
		expect []string
	}{
		{
			input:  `"foo","bar","baz"`,
			expect: []string{"foo", "bar", "baz"},
		},
		{
			input:  `"foo,with,commas","bar","baz"`,
			expect: []string{"foo,with,commas", "bar", "baz"},
		},
		{
			input:  `"first","","third"`,
			expect: []string{"first", "", "third"},
		},
		{
			input:  `"only one field"`,
			expect: []string{"only one field"},
		},
		{
			input:  `unquoted,fields,here`,
			expect: []string{"unquoted", "fields", "here"},
		},
		{
			input:  `"quoted","unquoted","quoted again"`,
			expect: []string{"quoted", "unquoted", "quoted again"},
		},
		{
			input:  `"UM980","R4.10Build13504","HRPT00-S10C-P"`,
			expect: []string{"UM980", "R4.10Build13504", "HRPT00-S10C-P"},
		},
		{
			input:  ``,
			expect: []string{},
		},
		{
			input:  `single`,
			expect: []string{"single"},
		},
		{
			input:  `"quoted,field,with,many,commas"`,
			expect: []string{"quoted,field,with,many,commas"},
		},
		// Test mixed quoted and unquoted with commas inside quotes
		{
			input:  `"field,with,commas",plain,field,"another,quoted,field"`,
			expect: []string{"field,with,commas", "plain", "field", "another,quoted,field"},
		},
		{
			input:  `unquoted,"quoted,with,commas",last`,
			expect: []string{"unquoted", "quoted,with,commas", "last"},
		},
		{
			input:  `"first,comma,field",middle,"third,comma,field",final,"fifth,field"`,
			expect: []string{"first,comma,field", "middle", "third,comma,field", "final", "fifth,field"},
		},
		// Test single field with commas
		{
			input:  `"one,field,with,many,commas"`,
			expect: []string{"one,field,with,many,commas"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitDataFields(tt.input, len(tt.expect))
			if len(got) != len(tt.expect) {
				t.Errorf("splitDataFields(%q, %d) length = %d, want %d", tt.input, len(tt.expect), len(got), len(tt.expect))
				return
			}
			for i := range got {
				if got[i] != tt.expect[i] {
					t.Errorf("splitDataFields(%q, %d)[%d] = %q, want %q", tt.input, len(tt.expect), i, got[i], tt.expect[i])
				}
			}
		})
	}
}

func TestUnknownAsciiMessage(t *testing.T) {
	t.Run("ParseUnknownMessage", func(t *testing.T) {
		// Test parsing an unknown ASCII message
		unknownMsg := "#UNKNOWNMSGA,93,GPS,FINE,2376,540337000,0,0,18,29;field1,field2,field3*12345678\r\n"
		
		msg, err := ParseAsciiMessage([]byte(unknownMsg))
		if err != nil {
			t.Fatalf("ParseAsciiMessage() error = %v", err)
		}
		
		// Check that we got an UnknownAsciiMsgBody
		unknownAscii, ok := msg.Body.(*UnknownAsciiMsgBody)
		if !ok {
			t.Fatalf("Expected UnknownAsciiMsgBody, got %T", msg.Body)
		}
		
		// Check the message name
		if unknownAscii.Name != "UNKNOWNMSGA" {
			t.Errorf("UnknownAsciiMsg.Name = %q, want %q", unknownAscii.Name, "UNKNOWNMSGA")
		}
		
		// Check the payload
		expectedPayload := "field1,field2,field3"
		if unknownAscii.Payload != expectedPayload {
			t.Errorf("UnknownAsciiMsg.Payload = %q, want %q", unknownAscii.Payload, expectedPayload)
		}
		
		// Check ID() method
		id, name := unknownAscii.ID()
		if id != 0 {
			t.Errorf("UnknownAsciiMsg.ID() id = %d, want 0", id)
		}
		if name != "UNKNOWNMSGA" {
			t.Errorf("UnknownAsciiMsg.ID() name = %q, want %q", name, "UNKNOWNMSGA")
		}
		
		// Check header is still parsed correctly
		if msg.Hdr.CPUIdlePercent != 93 {
			t.Errorf("Header CPUIdlePercent = %d, want 93", msg.Hdr.CPUIdlePercent)
		}
	})
	
	t.Run("SerializeUnknownMessage", func(t *testing.T) {
		// Create an unknown ASCII message
		unknownMsg := &UnknownAsciiMsgBody{
			Name:    "TESTMSGA",
			Payload: "data1,data2,data3",
		}
		
		msg := &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 85,
				TimingHdr: TimingHdr{
				TimeRef:            TimeRefGPS,
				TimeStatus:         TimeStatusFine,
				Week:               2400,
				MillisecondsOfWeek: 123456789,
					Version:            1,
					LeapSec:            18,
					DelayMs:            42,
				},
			},
			Body: unknownMsg,
		}
		
		// Serialize the message
		packet, err := SerializeAsciiMsg(msg)
		if err != nil {
			t.Fatalf("SerializeAsciiMsg() error = %v", err)
		}
		
		// Convert to string and check it contains expected parts
		packetStr := string(packet)
		if !strings.HasPrefix(packetStr, "#TESTMSGA,") {
			t.Errorf("Serialized packet doesn't start with #TESTMSGA,: %q", packetStr)
		}
		if !strings.Contains(packetStr, ";data1,data2,data3*") {
			t.Errorf("Serialized packet doesn't contain expected payload: %q", packetStr)
		}
		
		// Parse it back and verify round-trip
		msg2, err := ParseAsciiMessage(packet)
		if err != nil {
			t.Fatalf("ParseAsciiMessage() round-trip error = %v", err)
		}
		
		unknownAscii2, ok := msg2.Body.(*UnknownAsciiMsgBody)
		if !ok {
			t.Fatalf("Round-trip: Expected UnknownAsciiMsgBody, got %T", msg2.Body)
		}
		
		if unknownAscii2.Name != unknownMsg.Name {
			t.Errorf("Round-trip: Name = %q, want %q", unknownAscii2.Name, unknownMsg.Name)
		}
		if unknownAscii2.Payload != unknownMsg.Payload {
			t.Errorf("Round-trip: Payload = %q, want %q", unknownAscii2.Payload, unknownMsg.Payload)
		}
		if msg2.Hdr.CPUIdlePercent != msg.Hdr.CPUIdlePercent {
			t.Errorf("Round-trip: CPUIdlePercent = %d, want %d", msg2.Hdr.CPUIdlePercent, msg.Hdr.CPUIdlePercent)
		}
	})
	
	t.Run("SerializeUnknownBinaryMessageFails", func(t *testing.T) {
		// Create an unknown binary message (has no name)
		unknownMsg := &UnknownBinMsgBody{
			MsgID:   12345,
			Payload: []byte{1, 2, 3, 4},
		}
		
		msg := &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 85,
			},
			Body: unknownMsg,
		}
		
		// Try to serialize as ASCII - should fail
		_, err := SerializeAsciiMsg(msg)
		if err == nil {
			t.Fatal("SerializeAsciiMsg() should have failed for UnknownBinMsg")
		}
		if !strings.Contains(err.Error(), "unknown binary message cannot be serialized as ASCII") {
			t.Errorf("Expected error about binary message, got: %v", err)
		}
	})
}
