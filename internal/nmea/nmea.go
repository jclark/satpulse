package nmea

import (
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

// Tag is the identifier for NMEA protocol packets
const Tag gpsprot.Tag = "NMEA"

// Ensure PacketProcessor implements gpsprot.NMEAPacketProcessor
var _ gpsprot.NMEAPacketProcessor = (*PacketProcessor)(nil)

// For a proprietary sentence Pxxx, Format is Pxxx and TalkerId is the empty string.
type Sentence struct {
	Format           string
	TalkerID         string
	Fields           []string
}

func (s *Sentence) msgID() string {
	return s.TalkerID + s.Format
}

// PacketProcessor implements the gpsprot.PacketProcessor interface for NMEA packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
	sb satellitesBuffer
}

// NewPacketProcessor creates a new NMEA packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{
		sb: *newSatellitesBuffer(),
	}
}

// ProcessPacket processes an NMEA packet's data and returns the type of the message and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	sen := Parse(data)
	msgID := sen.msgID()
	handled, err := p.Dispatch(sen, tRead, p.mh)
	if err != nil || handled {
		return msgID, err
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, sen, tRead)
	}
	return msgID, nil
}

func (p *PacketProcessor) SetSVNumbering(numbering []gpsprot.NMEASVNumberingRange) {
	p.sb.setNumbering(numbering)
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// Dispatch handles standard messages and returns true if handled, along with any error
func (p *PacketProcessor) Dispatch(sen *Sentence, tRead time.Time, h gpsprot.MsgHandler) (bool, error) {
	handled, err := p.sb.process(sen, tRead, h)
	if err != nil || handled {
		return handled, err
	}
	switch sen.Format {
	case "RMC":
		err := dispatchTime(parseRMC, sen, tRead, h)
		return err == nil, err
	case "ZDA":
		err := dispatchTime(parseZDA, sen, tRead, h)
		return err == nil, err
	}
	return false, nil
}

func (p *PacketProcessor) Idle(_ time.Time) {
	p.sb.idle(p.mh)
}

func dispatchTime(parser func(*Sentence) (*ptime.UTCTime, error), sen *Sentence, tRead time.Time, h gpsprot.MsgHandler) error {
	utc, err := parser(sen)
	if err != nil {
		return err
	}
	mt := gpsprot.TimeMsg{Tag: Tag, NativeMsgID: sen.msgID(), UTCTime: utc, GNSS: talkerIDToGNSS(sen.TalkerID)}
	if h != nil {
		h.Time(&mt, tRead)
	}
	return nil
}

func parseRMC(sen *Sentence) (*ptime.UTCTime, error) {
	k := sen.TalkerID + "RMC"
	if len(sen.Fields) < 9 {
		return nil, fmt.Errorf("%s: too few fields", k)
	}
	timeStr := sen.Fields[0]
	dateStr := sen.Fields[8]
	if timeStr == "" || dateStr == "" || sen.Fields[1] != "A" {
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

func parseZDA(sen *Sentence) (*ptime.UTCTime, error) {
	k := sen.TalkerID + "ZDA"
	if len(sen.Fields) < 4 {
		return nil, fmt.Errorf("%s: too few fields", k)
	}
	timeStr := sen.Fields[0]
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
		d := sen.Fields[i]
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
	_, _ = fmt.Sscanf(sen.Fields[1], "%02d", &day)
	_, _ = fmt.Sscanf(sen.Fields[2], "%02d", &month)
	_, _ = fmt.Sscanf(sen.Fields[3], "%04d", &year)
	// Need a limit on year to ensure ptime.Time isn't out of range
	// Allow 1980 since first version of NMEA was issued was 1980.
	if year < 1980 || year > 2099 {
		return nil, fmt.Errorf("%s: %s: invalid year", k, sen.Fields[3])
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

// Precondition is that data is valid according to Scanner.Read.
// The checksum is assumed to have been verified already.
func Parse(data string) *Sentence {
	before, _, _ := strings.Cut(data[1:], "*")
	fields := strings.Split(before, ",")
	sen := Sentence{ Fields: fields[1:] }
	addr := fields[0]
	if strings.IndexByte(before, '^') >= 0 {
		for i := 1; i < len(fields); i++ {
			fields[i] = unescape(fields[i])
		}
	}
	if len(addr) == 5 {
		sen.TalkerID = addr[:2]
		sen.Format = addr[2:]
	} else {
		sen.Format = addr
	}
	return &sen
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

func Trim(data string) string {
	trimmed := data
	if len(data) > 0 && data[len(data)-1] == '\n' {
		trimmed = data[0 : len(data)-1]
	}
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\r' {
		trimmed = trimmed[0 : len(trimmed)-1]
	}
	return trimmed
}
