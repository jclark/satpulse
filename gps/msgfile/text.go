package msgfile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/airmsg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
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
	return RawMsg{
		Bytes: []byte(lm.Text + *lm.EOL),
		Delay: delay,
		Tag:   *lm.Tag,
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
	built, flags, err := buildNMEA(nm.Text)
	if err != nil {
		return RawMsg{}, err
	}
	nm.built = built
	nm.flags = flags
	return RawMsg{
		Bytes: []byte(built),
		Delay: delay,
		Tag:   *nm.Tag,
	}, nil
}

func (nm *NMEAMsg) getTag() string { return *nm.Tag }

// buildNMEA builds a complete NMEA sentence from user text.
// Prepends $ if missing, appends *XX checksum if missing, appends CRLF.
// Validates the result using nmeamsg.CheckSyntax.
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

// nmeaMatcher handles NMEAMsg responses.
type nmeaMatcher struct {
	sentFlags   nmeamsg.SentenceSyntaxFlags
	sentPayload string // payload between $ and *
}

func (nm *NMEAMsg) newMatcher() responseMatcher {
	flags := nm.flags
	built := nm.built
	if built == "" {
		built, flags, _ = buildNMEA(nm.Text)
	}
	return &nmeaMatcher{
		sentFlags:   flags,
		sentPayload: nmeaPayload(built),
	}
}

func (m *nmeaMatcher) match(tag gpsprot.Tag, data string) (ResponseKind, string) {
	if binaryTags[tag] {
		return NotResponse, ""
	}
	if tag != gpsreg.TagNMEA {
		return MaybeResponse, ""
	}
	flags := nmeamsg.CheckSyntax(data)
	if flags.IsValidGNSSTalkerNMEA() {
		return NotResponse, ""
	}
	payload := nmeaPayload(data)
	if flags.IsValidProprietaryNMEA() && m.sentFlags.IsValidProprietaryNMEA() {
		vendor := nmeaVendor(payload)
		if vendor != nmeaVendor(m.sentPayload) {
			return NotResponse, ""
		}
		if classify, ok := proprietaryClassifiers[vendor]; ok {
			return classify(m.sentPayload, payload)
		}
	}
	return MaybeResponse, ""
}

// nmeaVendor extracts the vendor ID from a proprietary NMEA payload.
// The payload starts with "P" followed by a 3-letter vendor ID (e.g. "PQTM..." -> "QTM").
func nmeaVendor(payload string) string {
	if len(payload) < 4 || payload[0] != 'P' {
		return ""
	}
	return payload[1:4]
}

// proprietaryClassifier classifies a received proprietary NMEA payload
// as a response to a sent payload. sentPayload may be empty if the
// sent message was not proprietary NMEA of the same vendor.
type proprietaryClassifier func(sentPayload, recvPayload string) (ResponseKind, string)

var proprietaryClassifiers = map[string]proprietaryClassifier{
	"AIR": classifyPAIRResponse,
	"QTM": classifyPQTMResponse,
}

func classifyPAIRResponse(sentPayload, recvPayload string) (ResponseKind, string) {
	switch kind, msg := airmsg.CheckResponse(sentPayload, recvPayload); kind {
	case airmsg.ResponseOK:
		return AckResponse, ""
	case airmsg.ResponseError:
		return AckResponse, msg
	case airmsg.ResponseWait, airmsg.ResponseData, airmsg.ResponseInvalid:
		return OtherResponse, ""
	}
	return NotResponse, ""
}

func classifyPQTMResponse(sentPayload, recvPayload string) (ResponseKind, string) {
	switch kind, msg := qtmmsg.CheckResponse(sentPayload, recvPayload); kind {
	case qtmmsg.ResponseOK:
		return AckResponse, ""
	case qtmmsg.ResponseError:
		return AckResponse, msg
	case qtmmsg.ResponseData:
		return OtherResponse, ""
	}
	return NotResponse, ""
}

// lineMatcher handles LineMsg responses (without a ResponsePattern).
type lineMatcher struct{}

func (m *lineMatcher) match(tag gpsprot.Tag, data string) (ResponseKind, string) {
	if binaryTags[tag] {
		return NotResponse, ""
	}
	if tag == gpsreg.TagNMEA && nmeamsg.CheckSyntax(data).IsValidGNSSTalkerNMEA() {
		return NotResponse, ""
	}
	return MaybeResponse, ""
}

// unicoreMatcher handles LineMsg responses with ResponsePatternUnicore.
type unicoreMatcher struct {
	command string // first word of sent text, upper case
	text    string // full sent text
}

func (m *unicoreMatcher) match(tag gpsprot.Tag, data string) (ResponseKind, string) {
	if tag == gpsreg.TagNMEA {
		payload := nmeaPayload(data)
		// Unicore command acks use NMEA framing but are not standard NMEA.
		// Format: $command,FULL_CMD_TEXT,response: OK*XX
		if strings.HasPrefix(payload, "command,") {
			return m.matchCommandAck(payload[len("command,"):])
		}
		return m.matchNMEA(payload)
	}
	if tag == gpsreg.TagUnicoreAscii {
		return m.matchUNCA(data)
	}
	return NotResponse, ""
}

func (m *unicoreMatcher) matchCommandAck(rest string) (ResponseKind, string) {
	// rest is "FULL_CMD_TEXT,response: OK" or "FULL_CMD_TEXT,response: ERROR_TEXT"
	cmdText, resp, found := strings.Cut(rest, ",")
	if !found || cmdText != m.text {
		return NotResponse, ""
	}
	const prefix = "response: "
	if !strings.HasPrefix(resp, prefix) {
		return NotResponse, ""
	}
	r := resp[len(prefix):]
	if r == "OK" {
		return AckResponse, ""
	}
	return AckResponse, r
}

func (m *unicoreMatcher) matchNMEA(payload string) (ResponseKind, string) {
	// $CONFIG,... data reply (regardless of sent command).
	if strings.HasPrefix(payload, "CONFIG,") {
		return OtherResponse, ""
	}
	return NotResponse, ""
}

func (m *unicoreMatcher) matchUNCA(data string) (ResponseKind, string) {
	// UNCA packets: #MSGNAME,header;data*checksum\r\n
	// Match one-shot query responses (VERSION, MODE).
	name, _, _ := strings.Cut(strings.TrimPrefix(data, "#"), ",")
	if strings.ToUpper(name) == m.command {
		return OtherResponse, ""
	}
	return NotResponse, ""
}

func (lm *LineMsg) newMatcher() responseMatcher {
	if lm.RespPattern != nil && *lm.RespPattern == ResponsePatternUnicore {
		cmd, _, _ := strings.Cut(lm.Text, " ")
		return &unicoreMatcher{
			command: strings.ToUpper(cmd),
			text:    lm.Text,
		}
	}
	return &lineMatcher{}
}

// nmeaPayload extracts the payload between $ and * from an NMEA sentence.
func nmeaPayload(data string) string {
	if len(data) < 1 || data[0] != '$' {
		return data
	}
	payload := data[1:]
	if i := strings.IndexByte(payload, '*'); i >= 0 {
		payload = payload[:i]
	}
	return payload
}
