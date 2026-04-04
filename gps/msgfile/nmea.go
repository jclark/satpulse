package msgfile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

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

// analyzeRequest implements requestAnalyzer for NMEAMsg.
func (nm *NMEAMsg) analyzeRequest(data string) requestAnalysis {
	payload := nmeaPayload(data)
	if payload == "" {
		return requestAnalysis{}
	}
	if nm.flags.IsValidProprietaryNMEA() && len(payload) >= 4 {
		vendor := payload[1:4] // skip 'P', take 3-letter vendor code
		if c, ok := proprietaryClassifiers[vendor]; ok {
			return c.classifyRequest(payload)
		}
	}
	return requestAnalysis{
		expectAck:  ExpectAckNone,
		expectData: expectDataUnknown,
	}
}

// proprietaryNMEA classifies proprietary NMEA requests and responses.
type proprietaryNMEA interface {
	classifyRequest(payload string) requestAnalysis
	classifyResponse(payload string) responseAnalysis
}

var proprietaryClassifiers = map[string]proprietaryNMEA{
	"QTM": pqtmClassifier{},
	"AIR": pairClassifier{},
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
	// Proprietary NMEA: dispatch by vendor code.
	if flags.IsValidProprietaryNMEA() && len(payload) >= 4 {
		vendor := payload[1:4]
		if c, ok := proprietaryClassifiers[vendor]; ok {
			return c.classifyResponse(payload)
		}
	}
	return responseAnalysis{kind: responseMaybeData}
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
