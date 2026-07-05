package msgfile

import (
	"strings"

	"github.com/jclark/satpulse/gps/internal/septentrio"
)

// septCorrelate is the single shared ACK-correlation key for every Septentrio
// line message. The Septentrio replies ($R!, $R?) do not carry a verbatim or
// mechanically-derivable echo of the sent command, so literal matching is
// impossible. The protocol is single-flight (at most one un-acked command at
// a time), so a fixed key is unambiguous: ReadyToSend serializes sends on it,
// mirroring the receiver's own "wait for the prompt" rule.
const septCorrelate = "cmd"

// analyzeSeptResponse classifies a Septentrio reply packet (mosaic reference
// guide sec 3.1.3). Classification is on the "$R<type>"/"$--" prefix and the
// terminator (the real prompt vs the "---->" lst pseudo-prompt):
//
//   - "$R:" (set/get/exe) and "$R!" (user management) are single-packet acks.
//   - "$R?" is a nak; the error text is the "<name>: <error text>" remainder,
//     kept verbatim (there is no reliable way to derive <name> from the
//     command).
//   - "$R;" opens an lst reply. It is the positive ack -- the command was
//     accepted (a rejection is "$R?") -- but the "$--BLOCK" output still
//     follows, so it is reported as an ack that keeps the request open
//     (responseAckMore).
//   - A "$--BLOCK" section ending in "---->" is an intermediate lst unit,
//     shown but not correlated (responseInfo). The final "$--BLOCK", ending at
//     the real prompt, completes the command without a second ack line
//     (responseDone).
func analyzeSeptResponse(pkt string) responseAnalysis {
	if len(pkt) < 3 || pkt[0] != '$' {
		return responseAnalysis{kind: responseNotData}
	}
	if pkt[1] == 'R' {
		switch pkt[2] {
		case ':', '!':
			return responseAnalysis{kind: responseAck, ackCorrelate: septCorrelate}
		case ';':
			return responseAnalysis{kind: responseAckMore, ackCorrelate: septCorrelate}
		case '?':
			return responseAnalysis{
				kind:         responseNak,
				ackCorrelate: septCorrelate,
				ackError:     septNakError(pkt),
			}
		}
		return responseAnalysis{kind: responseNotData}
	}
	if pkt[1] == '-' && pkt[2] == '-' {
		if strings.HasSuffix(pkt, "---->") {
			return responseAnalysis{kind: responseInfo}
		}
		return responseAnalysis{kind: responseDone, ackCorrelate: septCorrelate}
	}
	return responseAnalysis{kind: responseNotData}
}

// septNakError returns the nak's message text: everything after the "$R?"
// prefix up to the terminating prompt, with surrounding whitespace trimmed.
func septNakError(pkt string) string {
	s := pkt
	if i := strings.LastIndex(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s[3:])
}

// septAnalyzer classifies incoming Septentrio reply packets.
type septAnalyzer struct{}

func (septAnalyzer) analyzeResponse(data string) responseAnalysis {
	return analyzeSeptResponse(data)
}

// analyzeRequestSeptentrio produces a requestAnalysis for Septentrio line
// messages. The whole reply -- echo, state lines, and prompt -- arrives as
// one TagReply packet, so no separate data response is expected
// (expectDataWithAck): the ack packet carries the readback, and the prompt
// that ends it is the "command done" marker.
func (lm *LineMsg) analyzeRequestSeptentrio() requestAnalysis {
	return requestAnalysis{
		ackTag:       septentrio.TagReply,
		ackCorrelate: septCorrelate,
		expectAck:    ExpectAckOrNak,
		expectData:   expectDataWithAck,
	}
}
