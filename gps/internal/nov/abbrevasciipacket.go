package nov

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// TagAbbrevAscii is the identifier for NovAtel abbreviated ASCII packets
const TagAbbrevAscii gpsprot.Tag = "NOVAA"

// AbbrevAsciiPacketFormat is the NovAtel abbreviated ASCII packet format, shared
// by the NovAtel-compatible vendors (NovAtel, Unicore, Bynav, SinoGNSS).
// Receivers use it for responses to abbreviated ASCII input (e.g. "<OK" and
// the multi-line LOGLIST response) and for logs requested without an A or B
// suffix. Each line is one packet:
//
//  1. Starts with '<'
//  2. Body consists of printable ASCII (0x20-0x7E) and TAB characters
//     (continuation lines are indented with spaces or, on Unicore, a TAB)
//  3. Ends with CR/LF
//  4. Total length is at most 160 bytes
//
// The format has no checksum: NovAtel documents it as intended for viewing
// by the user. The length cap bounds how much stream a false match in
// garbage data can consume.
var AbbrevAsciiPacketFormat gpsprot.PacketFormat = abbrevAsciiPacketFormat{}

type abbrevAsciiPacketFormat struct{}

const (
	// abbrevStateSync is the initial state looking for '<'
	abbrevStateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	// abbrevStateBody means we have seen '<' and are reading the line
	abbrevStateBody
	// abbrevStateHadCR means we have CR
	abbrevStateHadCR
	// abbrevStateComplete means we have CR LF
	abbrevStateComplete
)

// abbrevMaxLength is the maximum packet length including the CR/LF.
// The longest documented line (SinoGNSS VERSION GPSCARD) is about 115 bytes.
const abbrevMaxLength = 160

func (f abbrevAsciiPacketFormat) Tag() gpsprot.Tag {
	return TagAbbrevAscii
}

func (f abbrevAsciiPacketFormat) IsBinary() bool {
	return false
}

func (f abbrevAsciiPacketFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case abbrevStateSync:
		if b == novmsg.AbbrevSync {
			return abbrevStateBody
		}
	case abbrevStateBody:
		if b == '\r' {
			return abbrevStateHadCR
		}
		if (isPrintableAscii(b) || b == '\t') && packetLen < abbrevMaxLength-2 {
			return abbrevStateBody
		}
	case abbrevStateHadCR:
		if b == '\n' {
			return abbrevStateComplete
		}
	}
	return abbrevStateSync
}

func (f abbrevAsciiPacketFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == abbrevStateComplete
}

// MsgID returns the leading alphanumeric token after the '<', which is the
// message name on a header line (e.g. "LOGLIST") or the response word
// (e.g. "OK"). Continuation lines start with whitespace and return "".
func (f abbrevAsciiPacketFormat) MsgID(pkt []byte) string {
	i := 1
	for i < len(pkt) && isAlnum(pkt[i]) {
		i++
	}
	return string(pkt[1:i])
}

// ExtractChecksum returns nil: the format has no checksum, so packets
// always validate.
func (f abbrevAsciiPacketFormat) ExtractChecksum(pkt []byte) []byte {
	return nil
}

// ComputeChecksum returns nil: the format has no checksum.
func (f abbrevAsciiPacketFormat) ComputeChecksum(pkt []byte) []byte {
	return nil
}

func (f abbrevAsciiPacketFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}

func isAlnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}
