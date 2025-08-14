package uncmsg

import (
	"testing"
)

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
