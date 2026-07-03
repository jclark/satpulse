package msgfile

import (
	"strings"

	"github.com/jclark/satpulse/gps/internal/septentrio"
)

// sepCorrelate is the single shared ACK-correlation key for every Septentrio
// line message. The Septentrio replies ($R!, $R?) do not carry a verbatim or
// mechanically-derivable echo of the sent command, so literal matching is
// impossible. The protocol is single-flight (at most one un-acked command at
// a time), so a fixed key is unambiguous: ReadyToSend serializes sends on it,
// mirroring the receiver's own "wait for the prompt" rule.
const sepCorrelate = "cmd"

// analyzeSeptentrioResponse classifies a Septentrio "$R" reply packet
// (mosaic reference guide sec 3.1.3). Classification is on the type char at
// byte 2: ':' (set/get/exe), '!' (user management), and ';' (lst) are acks;
// '?' is a nak whose error text is the "<name>: <error text>" remainder,
// kept verbatim (there is no reliable way to derive <name> from the command).
func analyzeSeptentrioResponse(pkt string) responseAnalysis {
	if len(pkt) < 3 || pkt[0] != '$' || pkt[1] != 'R' {
		return responseAnalysis{kind: responseNotData}
	}
	switch pkt[2] {
	case ':', '!', ';':
		return responseAnalysis{kind: responseAck, ackCorrelate: sepCorrelate}
	case '?':
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: sepCorrelate,
			ackError:     sepNakError(pkt),
		}
	}
	return responseAnalysis{kind: responseNotData}
}

// sepNakError returns the nak's message text: everything after the "$R?"
// prefix up to the terminating prompt, with surrounding whitespace trimmed.
func sepNakError(pkt string) string {
	s := pkt
	if i := strings.LastIndex(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s[3:])
}

// sepAnalyzer classifies incoming Septentrio reply packets.
type sepAnalyzer struct{}

func (sepAnalyzer) analyzeResponse(data string) responseAnalysis {
	return analyzeSeptentrioResponse(data)
}

// analyzeRequestSeptentrio produces a requestAnalysis for Septentrio line
// messages. The whole reply -- echo, state lines, and prompt -- arrives as
// one TagSepReply packet, so no separate data response is expected
// (expectDataWithAck): the ack packet carries the readback, and the prompt
// that ends it is the "command done" marker.
func (lm *LineMsg) analyzeRequestSeptentrio() requestAnalysis {
	return requestAnalysis{
		ackTag:       septentrio.TagSepReply,
		ackCorrelate: sepCorrelate,
		expectAck:    ExpectAckOrNak,
		expectData:   expectDataWithAck,
	}
}
