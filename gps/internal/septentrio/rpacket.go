package septentrio

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ascii"
)

// TagReply is the identifier for Septentrio ASCII command-reply packets.
const TagReply gpsprot.Tag = "SEPTR"

// ReplyPacketFormat is the Septentrio ASCII command-reply packet format. The
// receiver answers each set/get/exe/lst command on its ASCII command line
// (mosaic reference guide sec 3.1) with a reply that begins with "$R" and a
// type char, spans one echo line plus zero or more state lines, and ends with
// the command prompt (e.g. "COM1>"). The whole reply -- echo, state lines, and
// prompt -- frames as a single packet whose final byte is the prompt's ">";
// that ">" is the "command done" signal, so the reply's arrival marks the
// completion of the command.
//
// An lst command's reply is a succession of units: the "$R;" echo line
// followed by the "---->" pseudo-prompt, then one or more "$--BLOCK" sections,
// each intermediate one ending with another "---->" and only the last ending
// with the real prompt (sec 3.1.3). Each unit frames as its own packet: the
// format also syncs on "$--" so the "$--BLOCK" sections frame, and a "---->"
// closes a packet just as a real prompt does. The septAnalyzer stitches the
// units back together, completing the command only at the real prompt.
// A block may contain the exact line "$TD", which introduces the ASCII display
// returned by lstAsciiDisplay; other dollar signs still end the candidate.
//
// The format is checksum-free, modeled on nov.AbbrevAsciiPacketFormat:
// ExtractChecksum and ComputeChecksum return nil. It syncs on '$' then 'R' (or
// "$--" for block sections), which disambiguates it from SBF's "$@" and from
// NMEA's checksummed '$' sentences.
var ReplyPacketFormat gpsprot.PacketFormat = replyPacketFormat{}

type replyPacketFormat struct{}

const (
	// rStateSync is the initial state looking for '$'.
	rStateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	// rStateDollar means we have seen '$'.
	rStateDollar
	// rStateType means we have seen "$R" and expect a type char (: ; ! ?).
	rStateType
	// rStateDash1 means we have seen "$-" and expect the second '-' of the
	// "$--BLOCK" header that opens an lst reply's block section.
	rStateDash1
	// rStateBody means we are inside a body line (not at a line start).
	rStateBody
	// rStateCR means we have seen a CR and expect the paired LF.
	rStateCR
	// rStateLineStart means we are at the start of a line after a CR LF,
	// where a terminator token may begin.
	rStateLineStart
	// rStateHy1..rStateHy3 track a run of 1..3 leading hyphens ("----").
	rStateHy1
	rStateHy2
	rStateHy3
	// rStateAl1..rStateAl3 track an alphanumeric token: [A-Z] then up to
	// two more [A-Z0-9].
	rStateAl1
	rStateAl2
	rStateAl3
	// rStateTok4 means a full 4-char token has matched at a line start and
	// the packet completes if the next byte is '>'.
	rStateTok4
	// rStateBlockDollar..rStateBlockTD match the exact "$TD" line allowed
	// inside a "$--BLOCK" section.
	rStateBlockDollar
	rStateBlockT
	rStateBlockTD
	// rStateComplete means we have seen the terminating '>'.
	rStateComplete
)

// rMaxLength caps the packet length as a backstop against a false match
// running away in garbage data. Real replies are well under 200 bytes; only
// the lst pseudo-prompt could run longer, and it frames at its first "---->".
const rMaxLength = 4096

func (f replyPacketFormat) Tag() gpsprot.Tag {
	return TagReply
}

func (f replyPacketFormat) IsBinary() bool {
	return false
}

func (f replyPacketFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	if state != rStateSync && packetLen >= rMaxLength {
		return rStateSync
	}
	switch state {
	case rStateSync:
		if b == '$' {
			return rStateDollar
		}
	case rStateDollar:
		if b == 'R' {
			return rStateType
		}
		if b == '-' {
			return rStateDash1
		}
	case rStateType:
		if b == ':' || b == ';' || b == '!' || b == '?' {
			return rStateBody
		}
	case rStateDash1:
		if b == '-' {
			return rStateBody
		}
	case rStateBody:
		return rReadBody(b)
	case rStateCR:
		if b == '\n' {
			return rStateLineStart
		}
	case rStateLineStart:
		if b == '$' && rIsBlock(buf, nextScanIndex, packetLen) {
			return rStateBlockDollar
		}
		if b == '-' {
			return rStateHy1
		}
		if ascii.IsUpper(b) {
			return rStateAl1
		}
		return rReadBody(b)
	case rStateHy1:
		if b == '-' {
			return rStateHy2
		}
		return rReadBody(b)
	case rStateHy2:
		if b == '-' {
			return rStateHy3
		}
		return rReadBody(b)
	case rStateHy3:
		if b == '-' {
			return rStateTok4
		}
		return rReadBody(b)
	case rStateAl1:
		if ascii.IsAlnum(b) && !ascii.IsLower(b) {
			return rStateAl2
		}
		return rReadBody(b)
	case rStateAl2:
		if ascii.IsAlnum(b) && !ascii.IsLower(b) {
			return rStateAl3
		}
		return rReadBody(b)
	case rStateAl3:
		if ascii.IsAlnum(b) && !ascii.IsLower(b) {
			return rStateTok4
		}
		return rReadBody(b)
	case rStateTok4:
		if b == '>' {
			return rStateComplete
		}
		return rReadBody(b)
	case rStateBlockDollar:
		if b == 'T' {
			return rStateBlockT
		}
	case rStateBlockT:
		if b == 'D' {
			return rStateBlockTD
		}
	case rStateBlockTD:
		if b == '\r' {
			return rStateCR
		}
	}
	return rStateSync
}

func rIsBlock(buf []byte, nextScanIndex, packetLen int) bool {
	i := nextScanIndex - packetLen
	return packetLen >= 3 && buf[i] == '$' && buf[i+1] == '-' && buf[i+2] == '-'
}

// rReadBody handles a byte that is not part of a terminator: a CR opens a
// line break, any other printable non-'$' byte continues the body line, and
// anything else (a '$', a control or high-bit byte, a lone LF) ends the
// candidate.
func rReadBody(b byte) gpsprot.ScanState {
	if b == '\r' {
		return rStateCR
	}
	if b != '$' && ascii.IsPrint(b) {
		return rStateBody
	}
	return rStateSync
}

func (f replyPacketFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == rStateComplete
}

// MsgID returns a fixed name: reply packets carry no per-message identifier
// that the command-line tool needs, and they are displayed as text.
func (f replyPacketFormat) MsgID(pkt []byte) string {
	return "reply"
}

// ExtractChecksum returns nil: the reply format has no checksum.
func (f replyPacketFormat) ExtractChecksum(pkt []byte) []byte {
	return nil
}

// ComputeChecksum returns nil: the reply format has no checksum.
func (f replyPacketFormat) ComputeChecksum(pkt []byte) []byte {
	return nil
}

func (f replyPacketFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}
