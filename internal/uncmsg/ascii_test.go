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
