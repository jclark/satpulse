package nmea

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

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
					ID:        svid,
					Azimuth:   int16(sv.azim),
					Elevation: int8(sv.elev),
					Signals:   []gpsprot.SignalInfo{sig},
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
	gnss  gpsprot.GNSS
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
	id   int // 0 means means null
	azim int
	elev int
	cn0  int
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
		1:   "B1I", // ublox
		2:   "B1Q", // restricted
		3:   "B1C", // ublox
		4:   "B1A", // restricted
		5:   "B2a", // NMEA calls it B2-a
		6:   "B2b", // NMEA calls it B2-b
		7:   "B2a+b",
		8:   "B3I", // quectel
		9:   "B3Q", // restricted
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
		1:   "L1", // friendlier name for L1 C/A
		2:   "L1C (D)",
		3:   "L1C (P)",
		4:   "L1S",
		5:   "L2C-M",
		6:   "L2C-L",
		7:   "L5-I",
		8:   "L5-Q",
		9:   "L6", // friendlier name for L6D
		0xA: "L6E",
	},
	gpsprot.NAVIC: {
		1: "L5", // friendlier name for L5-SPS
		2: "S",  // friendlier name for L5-SPS
		3: "L5-RS",
		4: "S-RS",
		5: "L1", // friendlier name for L1-SPS
	},
}

func gnssSigIDName(gnss gpsprot.GNSS, sigID int) string {
	if sigNames, ok := sigIDMap[gnss]; ok {
		return sigNames[sigID]
	}
	return fmt.Sprintf("%X", sigID)
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
