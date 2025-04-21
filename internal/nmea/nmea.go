package nmea

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

// Tag is the identifier for NMEA protocol packets
const Tag gpsprot.Tag = "NMEA"

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// For a proprietary sentence Pxxx, Format is Pxxx and TalkerId is the empty string.
type Sentence struct {
	Format           string
	TalkerID         string
	Fields           []string
	ChecksumField    byte
	ComputedChecksum byte
}

func (s *Sentence) msgID() string {
	return s.TalkerID + s.Format
}

func (s *Sentence) ChecksumOK() bool {
	return s.ChecksumField == s.ComputedChecksum
}

// PacketProcessor implements the gpsprot.PacketProcessor interface for NMEA packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
	gs gsvState
}

// NewPacketProcessor creates a new NMEA packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{
		gs: *newGSVState(),
	}
}

// ProcessPacket processes an NMEA packet's data and returns the type of the message and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	sen, err := Parse(data)
	if err != nil {
		return "", err
	}
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

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// Precondition is that data is valid according to Scanner.Read.
func Parse(data string) (*Sentence, error) {
	sen := Split(data)
	if !sen.ChecksumOK() {
		return nil, fmt.Errorf("NMEA checksum error: checksum in message %02x, computed %02x", sen.ChecksumField, sen.ComputedChecksum)
	}
	return sen, nil
}

// Dispatch handles standard messages and returns true if handled, along with any error
func (p *PacketProcessor) Dispatch(sen *Sentence, tRead time.Time, h gpsprot.MsgHandler) (bool, error) {
	handled, err := p.gs.process(sen, tRead, h)
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
	p.gs.flush(p.mh)
}

func dispatchTime(parser func(*Sentence) (*ptime.UTCTime, error), sen *Sentence, tRead time.Time, h gpsprot.MsgHandler) error {
	utc, err := parser(sen)
	if err != nil {
		return err
	}
	mt := gpsprot.TimeMsg{SrcType: "NMEA-" + sen.Format, UTCTime: utc, GNSS: talkerIDToGNSS(sen.TalkerID)}
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

// gsvState is the state used for combining GSV sentences into a single SatellitesMsg
// First, there can be a series of GSV sentences with the same talker ID, with the sentence
// explicitly saying this is M of N sentences.
// Second, there will be multiple series of GSV sentences, one for each talker ID.
// But we don't know up front which talker IDs will be used.
type gsvState struct {
	svs              []gpsprot.SVInfo    // accumulated GSV data
	tRead            time.Time           // time of first GSV message in svs
	numTalkerIDs     int                 // number of talker IDs accumulated in svs
	talkerIDExpected map[string]struct{} // talker IDs expected for GSV messages
	talkerIDsKnown   bool                // true when we know what talker IDs we are expecting
}

func newGSVState() *gsvState {
	return &gsvState{
		talkerIDExpected: make(map[string]struct{}),
	}
}

func (g *gsvState) flush(h gpsprot.MsgHandler) {
	if len(g.svs) == 0 {
		return
	}
	if h != nil {
		h.Satellites(&gpsprot.SatellitesMsg{
			Info: g.svs,
		}, g.tRead)
	}
	g.svs = nil
	g.tRead = time.Time{}
	g.numTalkerIDs = 0
}

// process processes a GSV sentence and returns true if the sentence was a GSV sentence
func (g *gsvState) process(sen *Sentence, tRead time.Time, h gpsprot.MsgHandler) (bool, error) {
	if sen.Format != "GSV" {
		g.flush(h)
		return false, nil
	}
	svs, _, final, err := parseGSV(sen)
	if err != nil {
		return false, err
	}
	if len(g.svs) == 0 {
		g.svs = svs
		g.tRead = tRead
	} else {
		g.svs = append(g.svs, svs...)
	}
	if !final {
		return true, nil
	}
	// The idea here is that we know we have seen all the talker IDs we are going to,
	// as soon as we see a final talker ID (final means the N of N sentence) that we have already seen.
	if !g.talkerIDsKnown {
		_, ok := g.talkerIDExpected[sen.TalkerID]
		g.talkerIDExpected[sen.TalkerID] = struct{}{}
		// we have already seen this talker ID, so we know all the talker IDs we are going to see
		g.talkerIDsKnown = ok
	}
	g.numTalkerIDs++
	if g.talkerIDsKnown && g.numTalkerIDs == len(g.talkerIDExpected) {
		g.flush(h)
	}
	return true, nil
}

// parseGSV parses the GSV sentence and returns a slice of SVInfo, a signal ID, a bool and an error.
// The bool indicates whether the sentence is the last in the series (i.e. msgNum == numMsg)
func parseGSV(sen *Sentence) ([]gpsprot.SVInfo, uint64, bool, error) {
	gnss := talkerIDToGNSS(sen.TalkerID)
	if gnss == 0 {
		return nil, 0, false, fmt.Errorf("GSV: unknown talker ID %s", sen.TalkerID)
	}
	msgNum, err := parseUnsignedField(sen.Fields, 0, 1, 9, "GSV")
	if err != nil {
		return nil, 0, false, err
	}
	numMsg, err := parseUnsignedField(sen.Fields, 1, 1, 9, "GSV")
	if err != nil {
		return nil, 0, false, err
	}
	_, err = parseUnsignedField(sen.Fields, 2, 0, 99, "GSV")
	if err != nil {
		return nil, 0, false, err
	}
	final := msgNum == numMsg
	i := 3
	var svs []gpsprot.SVInfo
Loop:
	for ; i+3 < len(sen.Fields); i += 4 {
		for j := 0; j < 4; j++ {
			if sen.Fields[i+j] == "" {
				continue Loop
			}
		}
		prn, err := parseUnsignedField(sen.Fields, i, 1, 999, "GSV")
		if err != nil {
			return nil, 0, false, err
		}
		elev, err := parseUnsignedField(sen.Fields, i+1, 0, 90, "GSV")
		if err != nil {
			return nil, 0, false, err
		}
		azim, err := parseUnsignedField(sen.Fields, i+2, 0, 359, "GSV")
		if err != nil {
			return nil, 0, false, err
		}
		cno, err := parseUnsignedField(sen.Fields, i+3, 0, 99, "GSV")
		if err != nil {
			return nil, 0, false, err
		}
		sv := gpsprot.SVInfo{
			SVID:      makeSVID(gnss, int16(prn)),
			Elevation: int8(elev),
			Azimuth:   int16(azim),
			CNO:       uint8(cno),
		}
		svs = append(svs, sv)
	}
	sigID := uint64(0)
	if len(sen.Fields) > i+1 {
		return nil, 0, false, fmt.Errorf("GSV: superfluous fields")
	}
	if len(sen.Fields) == i+1 {
		sigID, err = parseUnsignedField(sen.Fields, i, 1, 255, "GSV")
		if err != nil {
			return nil, 0, false, err
		}
	}
	return svs, sigID, final, nil
}

func makeSVID(gnss gpsprot.GNSS, prn int16) gpsprot.SVID {
	if gnss == gpsprot.GPS && prn >= 33 && prn <= 64 {
		prn = 120 + (prn - 33)
		gnss = gpsprot.SBAS
	} else if gnss == gpsprot.GLO && prn >= 65 && prn <= 96 {
		prn = 1 + (prn - 65)
	}
	// Other mappings are non-standard, so do not attempt to do them here.
	return gpsprot.SVID{GNSS: gnss, PRN: prn}
}

func parseUnsignedField(fields []string, i int, min uint64, max uint64, format string) (uint64, error) {
	n, err := strconv.ParseUint(fields[i], 10, 16)
	if err == nil && (n < min || n > max) {
		err = strconv.ErrRange
	}
	if err != nil {
		return 0, fmt.Errorf("%s: invalid field %d: %s: %v", format, i, fields[i], err)
	}
	return n, nil
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

func Split(data string) *Sentence {
	before, after, _ := strings.Cut(data[1:], "*")
	fields := strings.Split(before, ",")
	sen := Sentence{
		Fields:           fields[1:],
		ChecksumField:    hexToByte(after),
		ComputedChecksum: Checksum(before),
	}
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
