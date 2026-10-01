package msgfile

import (
	"strings"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ascii"
)

// novatelCorrelate is the single shared ACK-correlation key for every
// NovAtel-format line message. A command entered in abbreviated ASCII gets
// an abbreviated ASCII response, "<OK" or "<ERROR:" plus the error text
// (OEM7 1.5.1), with nothing identifying the command, so, as for Septentrio,
// a fixed key makes ReadyToSend send one command at a time and each response
// belongs to the one command awaiting it.
const novatelCorrelate = "cmd"

// analyzeRequestNovAtel produces a requestAnalysis for NovAtel-format line
// commands. Every command gets a response. Only LOG commands produce other
// output, which may come before or after the response; it is treated as for
// a line message without a response pattern, except that the port prompt,
// which follows each response, is not a reply. Any other command is complete
// at its response.
func (lm *LineMsg) analyzeRequestNovAtel() requestAnalysis {
	a := requestAnalysis{
		ackTag:       gpsreg.TagNovAtelAbbrevAscii,
		ackCorrelate: novatelCorrelate,
		expectAck:    ExpectAckOrNak,
		expectData:   expectDataNone,
	}
	cmdWord, _, _ := strings.Cut(strings.TrimSpace(lm.Text), " ")
	switch strings.ToUpper(cmdWord) {
	case "LOG":
		eol := "\r\n"
		if lm.EOL != nil {
			eol = *lm.EOL
		}
		lineMatch := lineDataMatch(eol)
		a.expectData = expectDataUnknown
		a.dataMatch = func(d string) bool { return lineMatch(d) && !isPortPrompt(d) }
	case "SERIALCONFIG":
		// A speed change on the current port makes the response arrive at the
		// new speed, so we cannot require it.
		a.expectAck = ExpectAckNakOnly
	}
	return a
}

// isPortPrompt reports whether d is a port prompt such as "[COM1]", which
// names the port a command was received on and follows every response.
func isPortPrompt(d string) bool {
	s := strings.TrimRight(d, "\r\n")
	if len(s) < 3 || s[0] != '[' || s[len(s)-1] != ']' {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		if !ascii.IsAlnum(s[i]) {
			return false
		}
	}
	return true
}

// novaaAnalyzer classifies NovAtel abbreviated ASCII packets: "<OK" and
// "<ERROR:" are responses; anything else, such as an abbreviated ASCII log,
// may be data.
type novaaAnalyzer struct{}

func (novaaAnalyzer) analyzeResponse(data string) responseAnalysis {
	s := strings.TrimRight(data, "\r\n")
	if s == "<OK" {
		return responseAnalysis{kind: responseAck, ackCorrelate: novatelCorrelate}
	}
	if msg, ok := strings.CutPrefix(s, "<ERROR:"); ok {
		return responseAnalysis{kind: responseNak, ackCorrelate: novatelCorrelate, ackError: msg}
	}
	return responseAnalysis{kind: responseMaybeData}
}
