package septentrio

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/scantest"
	"github.com/jclark/satpulse/gps/scan"
)

// framesWhole asserts that s frames as exactly one reply packet spanning the
// whole input.
func framesWhole(t *testing.T, s string) {
	t.Helper()
	start, end, ok := scantest.FindPacket(ReplyPacketFormat, []byte(s))
	if start != 0 || end != len(s) || !ok {
		t.Fatalf("FindPacket(%q) = (%d, %d, %v), want (0, %d, true)", s, start, end, ok, len(s))
	}
}

// notPacket asserts that s does not frame as a reply packet.
func notPacket(t *testing.T, s string) {
	t.Helper()
	if _, _, ok := scantest.FindPacket(ReplyPacketFormat, []byte(s)); ok {
		t.Fatalf("FindPacket(%q) framed a packet, want none", s)
	}
}

func TestReplyPlain(t *testing.T) {
	framesWhole(t, "$R: setNMEAOutput, Stream1, USB1, GGA, sec1\r\nNMEAOutput, Stream1, USB1, GGA, sec1\r\nCOM1>")
}

func TestReplyNoStateLine(t *testing.T) {
	framesWhole(t, "$R: erst\r\nCOM1>")
}

func TestReplyStateLineStartsWithTokenRun(t *testing.T) {
	// "Rece" is four token-shaped chars, but the alnum token requires
	// [A-Z][A-Z0-9]{3}: the lowercase 'e' breaks it, and even if it did not
	// the fifth byte is a letter, not '>'. The body must run on to COM1>.
	framesWhole(t, "$R: grc\r\nReceiverCapabilities, Main, Base, none\r\nCOM1>")
}

func TestReplyAllUpperStateLine(t *testing.T) {
	// A state line whose first four chars are a valid token but the fifth is
	// not '>' is still body, not a terminator.
	framesWhole(t, "$R: gnss\r\nGNSS, Main, GPS\r\nUSB1>")
}

func TestReplyStopTerminator(t *testing.T) {
	framesWhole(t, "$R: erst, Soft, none\r\nResetReceiver, Soft, none\r\nSTOP>")
}

func TestReplyLstPseudoPromptTerminator(t *testing.T) {
	framesWhole(t, "$R; lstAsciiDisplay\r\n---->")
}

func TestReplyEndsAtFirstPromptBeforeUnsolicited(t *testing.T) {
	// An unsolicited "$TE" event abuts the reply; the packet ends at the
	// reply's own STOP>, not the event's trailing prompt.
	reply := "$R: erst, Soft, none\r\nResetReceiver, Soft, none\r\nSTOP>"
	full := reply + "$TE ResetReceiver\r\nSTOP>"
	start, end, ok := scantest.FindPacket(ReplyPacketFormat, []byte(full))
	if start != 0 || end != len(reply) || !ok {
		t.Fatalf("FindPacket = (%d, %d, %v), want (0, %d, true)", start, end, ok, len(reply))
	}
}

func TestReplyNotAPacket(t *testing.T) {
	cases := map[string]string{
		"mid-reply dollar": "$R: foo$bar\r\nCOM1>",
		"control byte":     "$R: foo\x01bar\r\nCOM1>",
		"high-bit byte":    "$R: foo\x80bar\r\nCOM1>",
		"unpaired CR":      "$R: foo\rbar\r\nCOM1>",
		"unpaired LF":      "$R: foo\nbar\r\nCOM1>",
		"lowercase token":  "$R: foo\r\nab12>",
		"digit-lead token": "$R: foo\r\n1234>",
		"three-char token": "$R: foo\r\nCOM>",
		"bad type char":    "$R# foo\r\nCOM1>",
		"sbf sync":         "$@ not a reply",
		"no terminator":    "$R: foo\r\nCOM1\r\n",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) { notPacket(t, s) })
	}
}

func TestReplyOverLength(t *testing.T) {
	notPacket(t, "$R: "+strings.Repeat("A", rMaxLength+100)+"\r\nCOM1>")
}

// TestReplyNotStolenByNMEA verifies that when both NMEA and the reply format
// are scanning, a "$R" reply frames under the reply format even though NMEA
// also matches a leading '$'.
func TestReplyNotStolenByNMEA(t *testing.T) {
	reply := "$R: setNMEAOutput, Stream1, USB1, GGA, sec1\r\nNMEAOutput, Stream1, USB1, GGA, sec1\r\nCOM1>"
	formats := []gpsprot.PacketFormat{nmea.PacketFormat, ReplyPacketFormat}
	s := scan.New(strings.NewReader(reply), 512, formats)
	pkt, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if pkt.Format != ReplyPacketFormat {
		t.Fatalf("Format = %v, want reply format", pkt.Format)
	}
	if pkt.Data != reply {
		t.Fatalf("Data = %q, want %q", pkt.Data, reply)
	}
}
