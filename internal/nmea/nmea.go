package nmea

import (
	"fmt"
	"sort"
	"strconv"
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

func (p *PacketProcessor) SetSVNumbering(numbering []gpsprot.NMEASVNumberingRange) {
	p.sb.setNumbering(numbering)
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

// satellitesBuffer is the state used for combining GSV sentences into a single SatellitesMsg
// First, there can be a series of GSV sentences with the same talker ID, with the sentence
// explicitly saying this is M of N sentences.
// Second, there will be multiple series of GSV sentences, one for each talker ID.
// But we don't know up front which talker IDs will be used.
type satellitesBuffer struct {
	gsvs         []gsvSentence
	tRead        time.Time       // read time of the first buffered GSV sentence
	lastFormat   string          // format of last sentence received
	gnssComplete gpsprot.GNSSSet // the set of GNSS for which there is a complete series in gsvs
	gnssExpected gpsprot.GNSSSet
	gnssKnown    bool
	gsas         []gsaSentence
	gsaWait      bool // wait for GSV after GSA is complete
	numbering    []gpsprot.NMEASVNumberingRange
}

var dfltSVNumbering = []gpsprot.NMEASVNumberingRange{
	{MinID: 33, MaxID: 64, MinPRN: 120, GNSS: gpsprot.SBAS, SignalID: ""},
	{MinID: 65, MaxID: 96, MinPRN: 1, GNSS: gpsprot.GLO, SignalID: ""},
}

func newSatellitesBuffer() *satellitesBuffer {
	return &satellitesBuffer{
		numbering: dfltSVNumbering,
	}
}

func (sb *satellitesBuffer) setNumbering(numbering []gpsprot.NMEASVNumberingRange) {
	sb.numbering = numbering
}

func (sb *satellitesBuffer) convertSVID(gnss gpsprot.GNSS, svid int, sigID int) (gpsprot.SVID, string) {
	id := gpsprot.SVID{}
	sigIDName := ""
	if sigID != 0 {
		sigIDName = gnssSigIDName(gnss, sigID)
	}
	// Do a binary search on the numbering slice.
	i := sort.Search(len(sb.numbering), func(i int) bool {
		return svid <= int(sb.numbering[i].MaxID)
	})
	if i < len(sb.numbering) && svid >= int(sb.numbering[i].MinID) {
		r := sb.numbering[i]
		// Check for consistency in case wrong numbering table has been configured.
		if !gnssConsistent(gnss, r.GNSS) {
			return id, ""
		}
		prn := int16((svid - int(r.MinID)) + int(r.MinPRN))
		id = gpsprot.SVID{GNSS: r.GNSS, PRN: prn}
		if !id.IsValid() {
			return gpsprot.SVID{}, ""
		}
		if sigID == 0 {
			sigIDName = r.SignalID
		}
	} else if gnss == gpsprot.GLO && svid == 0 {
		return gpsprot.SVID{GNSS: gpsprot.GLO, PRN: gpsprot.GLOUnknown}, ""
	} else {
		if svid <= 32 && gnss == 0 {
			gnss = gpsprot.GPS
		}
		id = gpsprot.SVID{GNSS: gnss, PRN: int16(svid)}
	}
	if !id.IsValid() {
		return gpsprot.SVID{}, ""
	}
	return id, sigIDName
}

func gnssConsistent(sen, found gpsprot.GNSS) bool {
	if sen == found || sen == 0 {
		return true
	}
	if sen == gpsprot.GPS && found == gpsprot.SBAS {
		return true
	}
	return false
}

func (sb *satellitesBuffer) idle(h gpsprot.MsgHandler) {
	if len(sb.gsvs) == 0 && len(sb.gsas) > 0 {
		sb.gsas = nil
		sb.gsaWait = true
		return
	}
	sb.flush(h)
}

func (sb *satellitesBuffer) maybeFlush(h gpsprot.MsgHandler) {
	if len(sb.gsvs) == 0 {
		return
	}
	if sb.gsaWait {
		sb.gsaWait = false // wait only once
	} else {
		sb.flush(h)
	}
}

// flush flushes out the GSV sentences
func (sb *satellitesBuffer) flush(h gpsprot.MsgHandler) {
	if len(sb.gsvs) == 0 {
		return
	}
	if h != nil {
		h.Satellites(sb.createSatellitesMsg(), sb.tRead)
	}
	sb.gsvClear()
}

// createSatellitesMsg creates a SatellitesMsg from the current grouping state.
// Precondition is that there is at least one GSV sentence in the group.
func (sb *satellitesBuffer) createSatellitesMsg() *gpsprot.SatellitesMsg {
	svidIndex := make(map[gpsprot.SVID]int)
	svs := []gpsprot.SVInfo{}
	for _, gsv := range sb.gsvs {
		for _, sv := range gsv.svs {
			svid, sigid := sb.convertSVID(gsv.gnss, sv.id, gsv.sigID)
			if svid.GNSS == 0 {
				continue
			}
			sig := gpsprot.SignalInfo{ID: sigid, CN0: uint8(sv.cn0)}
			if i, ok := svidIndex[svid]; ok {
				svs[i].Signals = append(svs[i].Signals, sig)
			} else {
				i := len(svs)
				svidIndex[svid] = i
				svs = append(svs, gpsprot.SVInfo{
					ID: svid,
					Azimuth:  int16(sv.azim),
					Elevation: int8(sv.elev),
					Signals: []gpsprot.SignalInfo{sig},
				})
			}
		}
	}
	usedValid := true
	for _, gsa := range sb.gsas {
		for _, id := range gsa.svids {
			svid, _ := sb.convertSVID(gsa.gnss, id, 0)
			if svid.GNSS == 0 {
				continue
			}
			if i, ok := svidIndex[svid]; ok {
				svs[i].Used = true
			} else {
				usedValid = false
				break
			}
		}
	}
	if !usedValid {
		// GSA info isn't matching up with the GSV info for reasons unknown.
		// Treat this as not having information about which satellites are used.
		for i := range svs {
			svs[i].Used = false
		}
	}
	return &gpsprot.SatellitesMsg{
		SVs:         svs,
		Tag:         Tag,
		NativeMsgID: sb.talkerID() + "GSV",
		UsedValid:   usedValid,
	}
}

func (sb *satellitesBuffer) gsvClear() {
	sb.gsvs = nil
	sb.tRead = time.Time{}
	sb.gnssComplete = 0
}

func (sb *satellitesBuffer) talkerID() string {
	return "GN"
}

func (sb *satellitesBuffer) process(sen *Sentence, tRead time.Time, h gpsprot.MsgHandler) (bool, error) {
	switch sen.Format {
	case "GSV":
		return sb.gsvProcess(sen, tRead, h)
	case "GSA":
		return sb.gsaProcess(sen)
	}
	sb.maybeFlush(h)
	sb.lastFormat = sen.Format
	return false, nil
}

func (sb *satellitesBuffer) gsaProcess(sen *Sentence) (bool, error) {
	if sb.lastFormat != "GSA" {
		sb.gsas = nil
	}
	sb.lastFormat = "GSA"
	sb.gsaWait = false
	gsa, err := parseGSA(sen)
	if err != nil {
		return false, err
	}
	sb.gsas = append(sb.gsas, gsa)
	return true, nil
}

func (sb *satellitesBuffer) gsvProcess(sen *Sentence, tRead time.Time, h gpsprot.MsgHandler) (bool, error) {
	gsv, err := parseGSV(sen)
	if err != nil {
		return false, err
	}
	if sb.lastFormat != "GSV" || sb.gnssComplete.Contains(gsv.gnss) {
		sb.gsvClear()
	}
	sb.lastFormat = "GSV"
	if len(sb.gsvs) == 0 {
		sb.tRead = tRead
	}
	// If we get an error here, we will report it up so it can be logged, but still add the SVs.
	err = sb.checkMsgNum(gsv)
	sb.gsvs = append(sb.gsvs, gsv)
	if gsv.numMsg != gsv.msgNum {
		return true, err
	}
	// Now we know it is the final sentence in a series
	flag := gpsprot.GNSSFlag(gsv.gnss)
	if sb.gnssExpected&flag != 0 {
		sb.gnssKnown = true
	} else {
		sb.gnssExpected |= flag
	}
	sb.gnssComplete |= flag
	if sb.gnssKnown && sb.gnssComplete == sb.gnssExpected {
		sb.maybeFlush(h)
	}
	return true, err
}

func (sb *satellitesBuffer) checkMsgNum(gsv gsvSentence) error {
	if len(sb.gsvs) == 0 {
		if gsv.msgNum != 1 {
			// Don't give an error here, because we may have started a new group too soon,
			// or we might have started in the middle of a group.
		}
		return nil
	}
	lastGSV := sb.gsvs[len(sb.gsvs)-1]
	if lastGSV.gnss == gsv.gnss {
		if gsv.msgNum != lastGSV.msgNum+1 {
			return fmt.Errorf("invalid GSV message number: expected %d, got %d", lastGSV.msgNum+1, gsv.msgNum)
		}
	} else if gsv.msgNum != 1 {
		return fmt.Errorf("invalid GSV message number: expected 1, got %d", gsv.msgNum)
	} else if lastGSV.msgNum != lastGSV.numMsg {
		// Don't give an error here, because errors apply to current sentence
	}
	return nil
}

type ggaSentence struct {
	numSV int
}

func parseGGA(sen *Sentence) (ggaSentence, error) {
	gga := ggaSentence{}
	if len(sen.Fields) < 7 {
		return gga, fmt.Errorf("GGA: too few fields")
	}
	numSV, err := parseIntField(sen.Fields, 6, 0, 99, "GGA")
	if err != nil && sen.Fields[6] != "" {
		return gga, err
	}
	gga.numSV = int(numSV)
	return gga, nil
}

type gsaSentence struct {
	gnss gpsprot.GNSS
	svids []int
}

func parseGSA(sen *Sentence) (gsaSentence, error) {
	gsa := gsaSentence{}
	// Fix, Auto, 12*SVID, PDOP, HDOP, VDOP, opt sysID
	gnss := gpsprot.GNSS(0)
	if len(sen.Fields) >= 18 {
		sysID, err := parseIntField(sen.Fields, 17, 1, 6, "GSA")
		if err != nil {
			return gsa, err
		}
		gnss = systemIDToGNSS(sysID)
	} else if len(sen.Fields) < 17 {
		return gsa, fmt.Errorf("GSA: too few fields")
	} else {
		gnss = talkerIDToGNSS(sen.TalkerID)
	}
	svids := make([]int, 0, 12)
	for i := 2; i < 14; i++ {
		if sen.Fields[i] == "" {
			continue
		}
		svid, err := parseIntField(sen.Fields, i, 1, 999, "GSA")
		if err != nil {
			return gsa, err
		}
		svids = append(svids, svid)
	}
	gsa.svids = svids
	gsa.gnss = gnss
	return gsa, nil
}

type gsvSentence struct {
	gnss   gpsprot.GNSS
	svs    []svInfo
	msgNum int
	numMsg int
	numSV  int
	sigID  int
}

type svInfo struct {
	id    int // 0 means means null
	azim  int
	elev  int
	cn0   int
}

// parseGSV parses the GSV sentence and returns a slice of SVInfo, a signal ID, a bool and an error.
// The bool indicates whether the sentence is the last in the series (i.e. msgNum == numMsg)
func parseGSV(sen *Sentence) (gsvSentence, error) {
	gsv := gsvSentence{}

	if len(sen.Fields) < 4 {
		return gsv, fmt.Errorf("GSV: too few fields")
	}
	gnss := talkerIDToGNSS(sen.TalkerID)
	if gnss == 0 {
		return gsv, fmt.Errorf("GSV: unknown talker ID %s", sen.TalkerID)
	}
	numMsg, err := parseIntField(sen.Fields, 0, 1, 9, "GSV")
	if err != nil {
		return gsv, err
	}
	msgNum, err := parseIntField(sen.Fields, 1, 1, 9, "GSV")
	if err != nil {
		return gsv, err
	}
	numSV, err := parseIntField(sen.Fields, 2, 0, 99, "GSV")
	if err != nil {
		return gsv, err
	}
	i := 3
	var svs []svInfo
Loop:
	for ; i+3 < len(sen.Fields); i += 4 {
		var err error
		sv := svInfo{}
		sv.id, err = parseIntField(sen.Fields, i, 1, 999, "GSV")
		if err != nil {
			if sen.Fields[i] == "" {
				if gnss != gpsprot.GLO {
					continue Loop
				}
				sv.id = 0
			} else {
				return gsv, err
			}
		}
		for j := 1; j < 4; j++ {
			if sen.Fields[i+j] == "" {
				continue Loop
			}
		}
		sv.elev, err = parseIntField(sen.Fields, i+1, -90, 90, "GSV")
		if err != nil {
			return gsv, err
		}
		sv.azim, err = parseIntField(sen.Fields, i+2, 0, 359, "GSV")
		if err != nil {
			return gsv, err
		}
		sv.cn0, err = parseIntField(sen.Fields, i+3, 0, 99, "GSV")
		if err != nil {
			return gsv, err
		}
		svs = append(svs, sv)
	}
	sigID := 0
	if len(sen.Fields) > i+1 {
		return gsv, fmt.Errorf("GSV: superfluous fields")
	}
	if len(sen.Fields) == i+1 {
		sigID, err = parseHexField(sen.Fields, i, "GSV")
		if err != nil {
			return gsv, err
		}
	}
	gsv = gsvSentence{
		gnss:   gnss,
		svs:    svs,
		sigID:  sigID,
		numMsg: numMsg,
		msgNum: msgNum,
		numSV:  numSV,
	}
	return gsv, nil
}

func parseIntField(fields []string, i int, min int, max int, format string) (int, error) {
	n, err := strconv.ParseInt(fields[i], 10, 16)
	if err == nil && (n < int64(min) || n > int64(max)) {
		err = strconv.ErrRange
	}
	if err != nil {
		return 0, fmt.Errorf("%s: invalid field %d: %s: %v", format, i, fields[i], err)
	}
	return int(n), nil
}

func parseHexField(fields []string, i int, format string) (int, error) {
	s := fields[i]
	if len(s) != 1 {
		return 0, fmt.Errorf("%s: invalid field %d: %s: length must be 1", format, i, fields[i])
	}
	n := hexDigit(s[0])
	if n < 0 {
		return 0, fmt.Errorf("%s: invalid field %d: %s: invalid character", format, i, fields[i])
	}
	return n, nil
}

func hexDigit(b byte) int {
	if b >= '0' && b <= '9' {
		return int(b - '0')
	}
	if b >= 'A' && b <= 'F' {
		return int(b - 'A' + 10)
	}
	return -1
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

func systemIDToGNSS(sysID int) gpsprot.GNSS {
	switch sysID {
	case 1:
		return gpsprot.GPS
	case 2:
		return gpsprot.GLO
	case 3:
		return gpsprot.GAL
	case 4:
		return gpsprot.BDS
	case 5:
		return gpsprot.QZSS
	case 6:
		return gpsprot.NAVIC
	default:
		return 0
	}
}

// This is from NMEA 4.11, which isn't freely available.
// See Table 7-34 in
// Unicore Reference Commands Manual for N4 High Precision Products R1.6
var sigIDMap = map[gpsprot.GNSS]map[int]string{
	gpsprot.GPS: {
		1: "L1 C/A",
		2: "L1 P(Y)",
		3: "L1 M",
		4: "L2 P(Y)",
		5: "L2C-M",
		6: "L2C-L",
		7: "L5-I",
		8: "L5-Q",
		9: "L1C", // Allystar, guessing a bit
	},
	gpsprot.GAL: {
		1: "E5a",
		2: "E5b",
		3: "E5a+b",
		4: "E6-BC",
		5: "E6", // friendlier name for E6-BC
		6: "L1-A",
		7: "E1", // friendlier name for L1-BC
	},
	gpsprot.BDS: {
		1: "B1I", // ublox
		2: "B1Q", // restricted
		3: "B1C", // ublox
		4: "B1A", // restricted
		5: "B2a", // NMEA calls it B2-a
		6: "B2b", // NMEA calls it B2-b
		7: "B2a+b",
		8: "B3I", // quectel
		9: "B3Q", // restricted
		0xA: "B3A", // restricted
		0xB: "B2I", // ublox
		0xC: "B2Q", // restricted
	},
	gpsprot.GLO: {
		1: "L1", // friendlier name for L1 C/A
		2: "L1 P",
		3: "L2", // friendlier name for L2 C/A
		4: "L2 P",
	},
	gpsprot.QZSS: {
		1: "L1 C/A",
		2: "L1C (D)",
		3: "L1C (P)",
		4: "L1S",
		5: "L2C-M",
		6: "L2C-L",
		7: "L5-I",
		8: "L5-Q",
		9: "L6", // friendlier name for L6D
		0xA: "L6E",
	},
	gpsprot.NAVIC: {
	 	1: "L5", // friendlier name for L5-SPS
		2: "S", // friendlier name for L5-SPS
		3: "L5-RS",
		4: "S-RS",
		5: "L1", // friendlier name for L1-SPS
	},
}

func gnssSigIDName(gnss gpsprot.GNSS, sigID int) string {
	if sigNames, ok := sigIDMap[gnss]; ok {
		return sigNames[sigID];
	}
	return fmt.Sprintf("%X", sigID)
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
