package novmsg

import (
	"strings"
	"testing"
)

func TestAsciiHdrHex(t *testing.T) {
	// The receiver status and reserved values are those of the IONUTC example
	// in the OEM7 manual.
	const header = "#TIMEA,COM1,0,52.5,FINESTEERING,2209,507000.000,02000020,ec21,16809"
	const packet = header + ";VALID,1.0e-08,0.0,-18.00000000000,2022,5,13,20,49,42000,VALID*ab9d81e7\r\n"
	msg, err := ParseAsciiMessage([]byte(packet))
	if err != nil {
		t.Fatalf("ParseAsciiMessage: %v", err)
	}
	if msg.Hdr.RecvStatus != 0x02000020 || msg.Hdr.Reserved != 0xec21 {
		t.Errorf("receiver status %#x, reserved %#x, want 0x2000020, 0xec21", msg.Hdr.RecvStatus, msg.Hdr.Reserved)
	}
	out, err := SerializeAsciiMsg[Port, AsciiHdr](msg)
	if err != nil {
		t.Fatalf("SerializeAsciiMsg: %v", err)
	}
	if got, _, _ := strings.Cut(string(out), ";"); got != header {
		t.Errorf("serialized header %q, want %q", got, header)
	}
}
