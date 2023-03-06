package nmea

import (
	"fmt"
	"strings"

	"github.com/jclark/gps2phc/internal/gpsmsg"
	"github.com/jclark/gps2phc/internal/ptime"
)

// For a proprietary sentence Pxxx, SentenceFmt is Pxxx and TalkerId is the empty string.
type Fields struct {
	SentenceFmt string
	TalkerID    string
	DataFields  []string
	ChecksumOK  bool
}

type Message struct {
	fields Fields
	utc    *ptime.UTCTime
}

// Precondition is that data is valid according to Scanner.Read.
func Parse(data string) (*Message, error) {
	f := Split(data)
	if !f.ChecksumOK {
		return nil, fmt.Errorf("NMEA checksum error")
	}
	var utc *ptime.UTCTime
	var err error
	switch f.SentenceFmt {
	case "RMC":
		utc, err = parseRMC(f)
	case "ZDA":
		utc, err = parseZDA(f)
	}
	if err != nil {
		return nil, err
	}
	m := &Message{fields: f, utc: utc}
	return m, nil
}

func (m *Message) Fields() Fields {
	return m.fields
}

func (m *Message) Time() *gpsmsg.Time {
	if m.utc == nil {
		return nil
	}
	mt := gpsmsg.Time{UTCTime: m.utc, GNSS: talkerIDToGNSS(m.fields.TalkerID)}
	switch m.fields.SentenceFmt {
	case "RMC", "ZDA":
		return &mt
	}
	return nil
}

func parseRMC(f Fields) (*ptime.UTCTime, error) {
	k := f.TalkerID + "RMC"
	if len(f.DataFields) < 9 {
		return nil, fmt.Errorf("%s: too few fields", k)
	}
	timeStr := f.DataFields[0]
	dateStr := f.DataFields[8]
	if timeStr == "" || dateStr == "" || f.DataFields[1] != "A" {
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

func parseZDA(f Fields) (*ptime.UTCTime, error) {
	k := f.TalkerID + "ZDA"
	if len(f.DataFields) < 4 {
		return nil, fmt.Errorf("%s: too few fields", k)
	}
	timeStr := f.DataFields[0]
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
		d := f.DataFields[i]
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
	_, _ = fmt.Sscanf(f.DataFields[1], "%02d", &day)
	_, _ = fmt.Sscanf(f.DataFields[2], "%02d", &month)
	_, _ = fmt.Sscanf(f.DataFields[3], "%04d", &year)
	// Need a limit on year to ensure ptime.Time isn't out of range
	// Allow 1980 since first version of NMEA was issued was 1980.
	if year < 1980 || year > 2099 {
		return nil, fmt.Errorf("%s: %s: invalid year", k, f.DataFields[3])
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

func talkerIDToGNSS(t string) *gpsmsg.MajorGNSS {
	var g gpsmsg.MajorGNSS
	switch t {
	case "GP":
		g = gpsmsg.GPS
	case "GL":
		g = gpsmsg.GLONASS
	case "GA":
		g = gpsmsg.Galileo
	case "GB", "BD":
		g = gpsmsg.BeiDou
	default:
		return nil
	}
	return &g
}

func Split(data string) Fields {
	before, after, _ := strings.Cut(data[1:], "*")
	fields := strings.Split(before, ",")
	msg := Fields{
		DataFields: fields[1:],
		ChecksumOK: checksum(before) == hexToByte(after),
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
	return msg
}

func checksum(data string) byte {
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
