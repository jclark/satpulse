package msgfile

import (
	"strings"

	"github.com/jclark/satpulse/gps/gpsreg"
)

func analyzeUnicoreAck(cmd, resp string) responseAnalysis {
	if resp == "response: OK" {
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: cmd,
		}
	}
	errText := resp
	if after, ok := strings.CutPrefix(resp, "response: "); ok {
		errText = after
	} else if after, ok := strings.CutPrefix(resp, "response "); ok {
		errText = after
	}
	return responseAnalysis{
		kind:         responseNak,
		ackCorrelate: cmd,
		ackError:     errText,
	}
}

// uncaAnalyzer classifies incoming Unicore ASCII packets.
// Any UNCA packet is treated as confirmed data.
type uncaAnalyzer struct{}

func (uncaAnalyzer) analyzeResponse(data string) responseAnalysis {
	return responseAnalysis{kind: responseData}
}

// uncaMessageName extracts the message name from a UNCA packet header.
func uncaMessageName(data string) string {
	if len(data) < 2 || data[0] != '#' {
		return ""
	}
	end := strings.IndexAny(data[1:], ",;")
	if end < 0 {
		return ""
	}
	return data[1 : 1+end]
}

// analyzeRequestUnicore produces a requestAnalysis for Unicore line messages.
func (lm *LineMsg) analyzeRequestUnicore() requestAnalysis {
	cmd := lm.Text
	a := requestAnalysis{
		ackTag:       gpsreg.TagNMEA,
		ackCorrelate: cmd,
		expectAck:    ExpectAckOrNak,
	}
	cmdWord, _, hasArgs := strings.Cut(cmd, " ")
	switch cmdWord {
	case "CONFIG", "MASK":
		uncConfigOrMask(&a, hasArgs, cmdWord)
	case "MODE":
		if hasArgs {
			a.expectData = expectDataNone
		} else {
			uncExpectUNCA(&a, "MODE", expectDataSingle)
		}
	case "VERSION":
		uncExpectUNCA(&a, "VERSION", expectDataSingle)
	case "LOGLIST":
		a.expectData = expectDataMultiple
		a.dataTag = gpsreg.TagNovAtelAbbrevAscii
	default:
		last := cmdWord[len(cmdWord)-1]
		if last == 'A' || last == 'B' {
			uncExpectUNCA(&a, cmdWord, expectDataAmbig)
		} else {
			a.expectData = expectDataUnknown
		}
	}
	return a
}

// isComPortConfig reports whether args starts with "COM" followed by a digit,
// e.g. "COM1 460800". Speed changes on the current port mean the ACK
// arrives at the new speed, so we cannot require it.
func isComPortConfig(args string) bool {
	return len(args) >= 4 && args[:3] == "COM" && args[3] >= '1' && args[3] <= '9'
}

func uncConfigOrMask(a *requestAnalysis, hasArgs bool, cmdWord string) {
	if hasArgs {
		a.expectData = expectDataNone
		if cmdWord == "CONFIG" && isComPortConfig(a.ackCorrelate[len("CONFIG "):]) {
			a.expectAck = ExpectAckNakOnly
		}
		return
	}
	a.dataTag = gpsreg.TagNMEA
	a.expectData = expectDataMultiple
	a.dataMatch = func(d string) bool { return uncConfigDataMatch(d, cmdWord == "MASK") }
}

func uncExpectUNCA(a *requestAnalysis, name string, de dataExpectation) {
	a.dataTag = gpsreg.TagUnicoreAscii
	a.expectData = de
	a.dataMatch = func(d string) bool { return uncaMessageName(d) == name }
}

// uncConfigDataMatch checks whether data is a $CONFIG NMEA sentence
// matching either CONFIG or MASK query responses.
func uncConfigDataMatch(data string, wantMask bool) bool {
	payload := nmeaPayload(data)
	if payload == "" {
		return false
	}
	fields := strings.SplitN(payload, ",", 3)
	if len(fields) < 2 || fields[0] != "CONFIG" {
		return false
	}
	isMask := strings.HasPrefix(fields[1], "MASK")
	return isMask == wantMask
}
