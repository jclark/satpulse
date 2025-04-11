package nmea

import (
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

// For a proprietary sentence Pxxx, SentenceFmt is Pxxx and TalkerId is the empty string.
type Message struct {
	SentenceFmt string
	TalkerID    string
	Fields      []string
	ChecksumOK  bool
}

// Protocol-specific handler
type ProtHandler interface {
	NMEA(msg *Message, tRead time.Time)
}

func ProcessPacket(data string, tRead time.Time, h gpsprot.MsgHandler, ph ProtHandler) error {
	msg, err := Parse(data)
	if err != nil {
		return err
	}
	return Dispatch(msg, tRead, h, ph)
}

// Precondition is that data is valid according to Scanner.Read.
func Parse(data string) (*Message, error) {
	msg := Split(data)
	if !msg.ChecksumOK {
		return nil, fmt.Errorf("NMEA checksum error")
	}
	return msg, nil
}

func Dispatch(msg *Message, tRead time.Time, h gpsprot.MsgHandler, ph ProtHandler) error {
	switch msg.SentenceFmt {
	case "RMC":
		return dispatchTime(parseRMC, msg, tRead, h)
	case "ZDA":
		return dispatchTime(parseZDA, msg, tRead, h)
	}
	if ph != nil {
		ph.NMEA(msg, tRead)
	}
	return nil
}

func dispatchTime(parser func(*Message) (*ptime.UTCTime, error), msg *Message, tRead time.Time, h gpsprot.MsgHandler) error {
	utc, err := parser(msg)
	if err != nil {
		return err
	}
	mt := gpsprot.TimeMsg{SrcType: "NMEA-" + msg.SentenceFmt, UTCTime: utc, GNSS: talkerIDToGNSS(msg.TalkerID)}
	if h != nil {
		h.Time(&mt, tRead)
	}
	return nil
}

func parseRMC(msg *Message) (*ptime.UTCTime, error) {
	k := msg.TalkerID + "RMC"
	if len(msg.Fields) < 9 {
		return nil, fmt.Errorf("%s: too few fields", k)
	}
	timeStr := msg.Fields[0]
	dateStr := msg.Fields[8]
	if timeStr == "" || dateStr == "" || msg.Fields[1] != "A" {
		return nil, nil
	}
	var year uint16
	var month, day, hour, min, sec uint8
	var nanos int32
	if !scanTime(timeStr, &hour, &min, &sec, &nanos) {
		return nil, fmt.Errorf("%s: %s: invalid time", k, timeStr)
	}
	if len(dateStr) != 6 || !isDigits(dateStr) {
		return nil, fmt.Errorf("%s: %s: invalid date", k, dateStr)
	}
	// Sscanf is very forgiving, so we check for length and all digits first, so that Sscanf is guarannteed to succeed.
	_, _ = fmt.Sscanf(dateStr, "%02d%02d%02d", &day, &month, &year)
	// There are some test examples from the 1990s.
	// Start with 1980 since NMEA first issued in the 1980s
	if year >= 80 {
		year += 1900
	} else {
		year += 2000
	}
	utc := ptime.UTC(year, month, day, hour, min, sec, nanos)
	return &utc, nil
}

func parseZDA(msg *Message) (*ptime.UTCTime, error) {
	k := msg.TalkerID + "ZDA"
	if len(msg.Fields) < 4 {
		return nil, fmt.Errorf("%s: too few fields", k)
	}
	timeStr := msg.Fields[0]
	if timeStr == "" {
		return nil, nil
	}
	var year uint16
	var month, day, hour, min, sec uint8
	var nanos int32
	if !scanTime(timeStr, &hour, &min, &sec, &nanos) {
		return nil, fmt.Errorf("%s: %s: invalid time", k, timeStr)
	}
	for i := 1; i < 4; i++ {
		d := msg.Fields[i]
		if d == "" {
			return nil, nil
		}
		expectLen := 2
		if i == 3 {
			expectLen = 4
		}
		if !isDigits(d) || len(d) != expectLen {
			return nil, fmt.Errorf("%s: %s: invalid date field", k, d)
		}
	}
	_, _ = fmt.Sscanf(msg.Fields[1], "%02d", &day)
	_, _ = fmt.Sscanf(msg.Fields[2], "%02d", &month)
	_, _ = fmt.Sscanf(msg.Fields[3], "%04d", &year)
	// Need a limit on year to ensure ptime.Time isn't out of range
	// Allow 1980 since first version of NMEA was issued was 1980.
	if year < 1980 || year > 2099 {
		return nil, fmt.Errorf("%s: %s: invalid year", k, msg.Fields[3])
	}
	utc := ptime.UTC(year, month, day, hour, min, sec, nanos)
	return &utc, nil
}

func scanTime(s string, hour, min, sec *uint8, nanos *int32) bool {
	fixed, frac, _ := strings.Cut(s, ".")
	if !isDigits(fixed) || len(fixed) != 6 {
		return false
	}
	n, err := fmt.Sscanf(fixed, "%02d%02d%02d", hour, min, sec)
	if n != 3 || err != nil {
		return false
	}
	if !isDigits(frac) {
		return false
	}
	pad := 9 - len(frac)
	if pad < 0 {
		return false
	}
	frac += strings.Repeat("0", pad)
	_, _ = fmt.Sscanf(frac, "%d", nanos)
	return true
}

func isDigits(s string) bool {
	for _, d := range s {
		if d < '0' || d > '9' {
			return false
		}
	}
	return true
}

func talkerIDToGNSS(t string) gpsprot.GNSS {
	switch t {
	case "GP":
		return gpsprot.GPS
	case "GL":
		return gpsprot.GLO
	case "GA":
		return gpsprot.GAL
	case "GB", "BD":
		return gpsprot.BDS
	case "GI":
		return gpsprot.NAVIC
	case "GQ":
		return gpsprot.QZSS
	default:
		return 0
	}
}

func Split(data string) *Message {
	before, after, _ := strings.Cut(data[1:], "*")
	fields := strings.Split(before, ",")
	msg := Message{
		Fields:     fields[1:],
		ChecksumOK: Checksum(before) == hexToByte(after),
	}
	addr := fields[0]
	if strings.IndexByte(before, '^') >= 0 {
		for i := 1; i < len(fields); i++ {
			fields[i] = unescape(fields[i])
		}
	}
	if len(addr) == 5 {
		msg.TalkerID = addr[:2]
		msg.SentenceFmt = addr[2:]
	} else {
		msg.SentenceFmt = addr
	}
	return &msg
}

func Checksum(data string) byte {
	var c byte
	for i := 0; i < len(data); i++ {
		c ^= data[i]
	}
	return c
}

// Assumes that use of ^ has been validated by Scanner.Read.
func unescape(s string) string {
	unescaped := ""
	for s != "" {
		before, after, ok := strings.Cut(s, "^")
		unescaped += before
		if !ok {
			break
		}
		unescaped += string(rune(hexToByte(after)))
		s = after[2:]
	}
	return unescaped
}

func hexToByte(digits string) byte {
	return (hexWeight(digits[0]) << 4) | hexWeight(digits[1])
}

func hexWeight(b byte) byte {
	if '0' <= b && b <= '9' {
		return b - '0'
	}
	return (b - 'A') + 10
}
