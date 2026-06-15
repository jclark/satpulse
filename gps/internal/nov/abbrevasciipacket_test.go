package nov

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestAbbrevAsciiPacketValidation(t *testing.T) {
	tests := []struct {
		name   string
		packet string
		valid  bool
	}{
		{
			"UM980 LOGLIST header line",
			"<LOGLIST COM3 17548 95.000000 FINE 2413 3196.000000 42155794 830 18\r\n",
			true,
		},
		{
			"UM980 count line with TAB",
			"<\t1\r\n",
			true,
		},
		{
			"UM980 log entry line with TAB",
			"<\tRECTIMEB COM3 1 \r\n",
			true,
		},
		{
			"OEM7 continuation line with spaces",
			"<     32\r\n",
			true,
		},
		{
			"OEM7 OK response",
			"<OK\r\n",
			true,
		},
		{
			"empty line after '<'",
			"<\r\n",
			true,
		},
		{
			"missing '<' prefix",
			"LOGLIST COM3 17548\r\n",
			false,
		},
		{
			"missing CR",
			"<LOGLIST COM3\n",
			false,
		},
		{
			"missing LF",
			"<LOGLIST COM3\r",
			false,
		},
		{
			"non-printable byte in body",
			"<LOGLIST \x01 COM3\r\n",
			false,
		},
		{
			"line at maximum length",
			"<" + strings.Repeat("x", abbrevMaxLength-3) + "\r\n",
			true,
		},
		{
			"line over maximum length",
			"<" + strings.Repeat("x", abbrevMaxLength-2) + "\r\n",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gpsprot.IsValidPacket(AbbrevAsciiPacketFormat, []byte(tt.packet))
			if result != tt.valid {
				t.Errorf("IsValidPacket() = %v, want %v for packet: %q",
					result, tt.valid, tt.packet)
			}
		})
	}
}

func TestAbbrevAsciiPacketMsgID(t *testing.T) {
	tests := []struct {
		name   string
		packet string
		expect string
	}{
		{
			"LOGLIST header line",
			"<LOGLIST COM3 17548 95.000000 FINE 2413 3196.000000 42155794 830 18\r\n",
			"LOGLIST",
		},
		{
			"OK response",
			"<OK\r\n",
			"OK",
		},
		{
			"ERROR response stops at colon",
			"<ERROR:Invalid Message ID\r\n",
			"ERROR",
		},
		{
			"TAB continuation line",
			"<\tRECTIMEB COM3 1 \r\n",
			"",
		},
		{
			"space continuation line",
			"<     32\r\n",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AbbrevAsciiPacketFormat.MsgID([]byte(tt.packet))
			if got != tt.expect {
				t.Errorf("MsgID() = %q, want %q", got, tt.expect)
			}
		})
	}
}
