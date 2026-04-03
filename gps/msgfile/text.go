package msgfile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

// LineMsg represents a [[line]] entry or [default.line].
type LineMsg struct {
	Text        string           `toml:"text"`
	EOL         *string          `toml:"eol"`
	RespPattern *ResponsePattern `toml:"responsePattern"`
	MsgCommon
}

func (lm *LineMsg) toRaw() (RawMsg, error) {
	if lm.Text == "" {
		return RawMsg{}, errors.New("text must not be empty")
	}
	delay, err := lm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := lm.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{
		Bytes:     []byte(lm.Text + *lm.EOL),
		Delay:     delay,
		WaitLimit: wl,
		Tag:       *lm.Tag,
	}, nil
}

func (lm *LineMsg) getTag() string { return *lm.Tag }

// NMEAMsg represents a [[nmea]] entry or [default.nmea].
type NMEAMsg struct {
	Text  string                      `toml:"text"`
	flags nmeamsg.SentenceSyntaxFlags // set by toRaw
	built string                      // full sentence from buildNMEA, set by toRaw
	MsgCommon
}

func (nm *NMEAMsg) toRaw() (RawMsg, error) {
	if nm.Text == "" {
		return RawMsg{}, errors.New("text must not be empty")
	}
	delay, err := nm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := nm.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	built, flags, err := buildNMEA(nm.Text)
	if err != nil {
		return RawMsg{}, err
	}
	nm.built = built
	nm.flags = flags
	return RawMsg{
		Bytes:     []byte(built),
		Delay:     delay,
		WaitLimit: wl,
		Tag:       *nm.Tag,
	}, nil
}

func (nm *NMEAMsg) getTag() string { return *nm.Tag }

// buildNMEA builds a complete NMEA sentence from user text.
// Prepends $ if missing, appends *XX checksum if missing, appends CRLF.
// Validates the result using nmeamsg.CheckSyntax.
// analyzeRequest implements requestAnalyzer for LineMsg.
func (lm *LineMsg) analyzeRequest(data string) requestAnalysis {
	if lm.RespPattern != nil && *lm.RespPattern == ResponsePatternUnicore {
		return lm.analyzeRequestUnicore()
	}
	eol := "\r\n"
	if lm.EOL != nil {
		eol = *lm.EOL
	}
	return requestAnalysis{
		expectAck:  ExpectAckNone,
		expectData: expectDataUnknown,
		dataMatch:  lineDataMatch(eol),
	}
}

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

func uncConfigOrMask(a *requestAnalysis, hasArgs bool, cmdWord string) {
	if hasArgs {
		a.expectData = expectDataNone
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

// lineDataMatch returns a data match function for generic line messages.
// It accepts printable ASCII data ending with the expected line terminator.
func lineDataMatch(eol string) func(string) bool {
	return func(data string) bool {
		if !strings.HasSuffix(data, eol) {
			return false
		}
		body := data[:len(data)-len(eol)]
		for i := 0; i < len(body); i++ {
			if body[i] < 0x20 || body[i] > 0x7E {
				return false
			}
		}
		return len(body) > 0
	}
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

// nmeaAnalyzer classifies incoming NMEA packets for response correlation.
type nmeaAnalyzer struct{}

func (nmeaAnalyzer) analyzeResponse(data string) responseAnalysis {
	payload := nmeaPayload(data)
	if payload == "" {
		return responseAnalysis{kind: responseMaybeData}
	}
	fields := strings.SplitN(payload, ",", 4)
	// Unicore command ACK/NAK: $command,CMD_TEXT,response: OK*XX
	if len(fields) >= 3 && fields[0] == "command" {
		return analyzeUnicoreAck(fields[1], fields[2])
	}
	// Unicore CONFIG/MASK data: $CONFIG,...
	if fields[0] == "CONFIG" {
		return responseAnalysis{kind: responseData}
	}
	// Standard GNSS talker NMEA (GPRMC, GPGGA, etc.) is not a response.
	flags := nmeamsg.CheckSyntax(data)
	if flags.IsValidGNSSTalkerNMEA() {
		return responseAnalysis{kind: responseNotData}
	}
	return responseAnalysis{kind: responseMaybeData}
}

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

// nmeaPayload extracts the payload from an NMEA packet (between $ and *).
func nmeaPayload(data string) string {
	if len(data) < 6 || data[0] != '$' {
		return ""
	}
	i := strings.LastIndexByte(data, '*')
	if i < 1 {
		return ""
	}
	return data[1:i]
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

func buildNMEA(text string) (string, nmeamsg.SentenceSyntaxFlags, error) {
	if !strings.HasPrefix(text, "$") {
		text = "$" + text
	}
	if !strings.Contains(text, "*") {
		checksum := nmeamsg.Checksum([]byte(text[1:]))
		text = fmt.Sprintf("%s*%02X", text, checksum)
	}
	text += "\r\n"
	flags := nmeamsg.CheckSyntax(text)
	if flags&nmeamsg.SentenceIsPacket == 0 {
		return "", 0, errors.New("invalid NMEA packet")
	}
	return text, flags, nil
}
