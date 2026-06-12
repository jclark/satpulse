package nmea

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// satellitesBuffer is the state used for combining GSV sentences into a single SatellitesMsg
// First, there can be a series of GSV sentences with the same talker ID, with the sentence
// explicitly saying this is M of N sentences.
// Second, there will be multiple series of GSV sentences, one for each talker ID.
// But we don't know up front which talker IDs will be used.
//
// gsvKeys and haveBoundary are used to implement the flushing logic.
// The flushing logic is based on the assumption that all the GSV and GSA sentences
// describing a single navigation solution wil be emitted in a butst with no idle period
// between sentences. (Idle periods of at least 0.1s will turn into calls to idle().)
// The goal is to combine GSV (satellite view) and GSA (satellite active) messages
// into complete SatellitesMsg reports. NMEA 4.10+ receivers send multiple GSV
// sequences per GNSS constellation (one per signal ID), making it impossible to
// predict when "all GPS data" is complete.
//
// Our approach uses two flush triggers:
// 1. idle() calls - Primary mechanism when receiver pauses between bursts
// 2. Repeated GNSS/signal combinations - Protection when idle() never comes
//
// For approach 2, we try to establish a reasonable boundary between bursts:
// this is the first idle() call or sentence format change (e.g. RMC to GSV).
type satellitesBuffer struct {
	numbering    []gpsprot.NMEASVNumberingRange
	gsvs         []gsvSentence
	gsvKeys      map[gsvKey]struct{} // the set of gnss and signal IDs occurring in gsvs
	tRead        time.Time           // read time of the first buffered GSV sentence
	lastFormat   string              // format of last sentence received
	gsas         []gsaSentence
	gsaFixDim    gpsprot.SolutionDim // buffered fix dimension from GSA
	gsaDOP       gpsprot.DOP         // buffered DOPs from GSA (Pos, Hor, Vert)
	haveBoundary bool                // have we established a plausible boundary between bursts
	mixedSigIDs  bool                // true if signal IDs are mixed within a single GSV series
}

type gsvKey struct {
	gnss  gpsprot.GNSS
	sigID int // signal ID, 0 means no signal ID
}

var dfltSVNumbering = []gpsprot.NMEASVNumberingRange{
	{MinID: 33, MaxID: 64, MinNum: 20, GNSS: gpsprot.SBAS, SignalID: ""},
	{MinID: 65, MaxID: 96, MinNum: 1, GNSS: gpsprot.GLO, SignalID: ""},
}

func newSatellitesBuffer() *satellitesBuffer {
	return &satellitesBuffer{
		numbering: dfltSVNumbering,
		gsvKeys:   make(map[gsvKey]struct{}),
	}
}

func (sb *satellitesBuffer) setNumbering(numbering []gpsprot.NMEASVNumberingRange) {
	sb.numbering = numbering
}

func (sb *satellitesBuffer) convertSVID(gnss gpsprot.GNSS, svid int, sigID int) (gpsprot.SVID, gpsprot.SignalID) {
	sigIDName := gpsprot.SignalID("")
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
			return gpsprot.SVID{}, ""
		}
		svid = (svid - int(r.MinID)) + int(r.MinNum)
		gnss = r.GNSS
		if sigID == 0 {
			sigIDName = r.SignalID
		}
	} else if gnss == gpsprot.GLO && svid == 0 {
		return gpsprot.SVID{GNSS: gpsprot.GLO, Num: gpsprot.GLOUnknown}, ""
	} else {
		if svid <= 32 && gnss == 0 {
			gnss = gpsprot.GPS
		}
	}
	if !gnss.IsValidSVNum(svid) {
		return gpsprot.SVID{}, ""
	}
	return gpsprot.SVID{GNSS: gnss, Num: uint8(svid)}, sigIDName
}

func gnssConsistent(sen, found gpsprot.GNSS) bool {
	if sen == found || sen == 0 {
		return true
	}
	if sen == gpsprot.GPS && (found == gpsprot.SBAS || found == gpsprot.QZSS) {
		return true
	}
	return false
}

func (sb *satellitesBuffer) idle(h gpsprot.MsgHandler, epoch *NavEpoch) {
	// Do possibleBoundary before flush, so that partial (probably incorrect) set is not used.
	sb.possibleBoundary()
	sb.flush(h, epoch)
}

func (sb *satellitesBuffer) possibleBoundary() {
	if sb.haveBoundary {
		return
	}
	sb.haveBoundary = true
	sb.clear()
}

func (sb *satellitesBuffer) flush(h gpsprot.MsgHandler, epoch *NavEpoch) {
	sb.commitGSAQuality(epoch)
	if h != nil && len(sb.gsvs) > 0 {
		h.Satellites(sb.createSatellitesMsg(), sb.tRead)
	}
	sb.clear()
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
					LookAngles: opt.Make(gpsprot.LookAngles{
						Azimuth:   int16(sv.azim),
						Elevation: int8(sv.elev),
					}),
					Signals: []gpsprot.SignalInfo{sig},
				})
			}
		}
	}
	usedValidity := gpsprot.SatelliteUsedSV
	for _, gsa := range sb.gsas {
		for _, id := range gsa.svids {
			svid, _ := sb.convertSVID(gsa.gnss, id, 0)
			if svid.GNSS == 0 {
				continue
			}
			if i, ok := svidIndex[svid]; ok {
				svs[i].Used = true
			} else {
				usedValidity = gpsprot.SatelliteUsedInvalid
				break
			}
		}
	}
	if usedValidity == gpsprot.SatelliteUsedInvalid {
		// GSA info isn't matching up with the GSV info for reasons unknown.
		// Treat this as not having information about which satellites are used.
		for i := range svs {
			svs[i].Used = false
		}
	}
	return &gpsprot.SatellitesMsg{
		SVs:          svs,
		Tag:          Tag,
		NativeMsgID:  sb.talkerID() + "GSV",
		UsedValidity: usedValidity,
	}
}

func (sb *satellitesBuffer) clear() {
	sb.gsvs = nil
	sb.gsvKeys = make(map[gsvKey]struct{}) // reset the keys
	sb.gsas = nil
	sb.tRead = time.Time{}
	// Reset mixedSigIDs detection. Must be re-detected for each batch of GSV
	// sentences. This prevents bad data from permanently flipping the mode.
	sb.mixedSigIDs = false
	// gsaFixDim and gsaDOP are intentionally NOT cleared here.
	// They are cleared only by commitGSAQuality, which copies them
	// to the epoch first. This allows GSA quality to survive
	// possibleBoundary() clears when GSA arrives before the epoch.
}

func (sb *satellitesBuffer) talkerID() string {
	return "GN"
}

func (sb *satellitesBuffer) process(sen *ApprovedSentence, tRead time.Time, h gpsprot.MsgHandler, epoch *NavEpoch) (bool, error) {
	if sb.lastFormat != sen.Format && sb.lastFormat != "" {
		sb.possibleBoundary()
	}
	defer func() { sb.lastFormat = sen.Format }()
	switch sen.Format {
	case "GSV":
		return sb.gsvProcess(sen, tRead, h, epoch)
	case "GSA":
		return sb.gsaProcess(sen)
	}
	return false, nil
}

func (sb *satellitesBuffer) gsaProcess(sen *ApprovedSentence) (bool, error) {
	gsa, err := parseGSA(sen)
	if err != nil {
		return false, err
	}
	sb.gsas = append(sb.gsas, gsa)
	sb.gsaQuality(sen.Fields)
	return true, nil
}

// gsaQuality buffers SolutionDim and DOPs from GSA fields. Multiple GSA
// sentences per epoch (one per constellation) carry identical DOP and
// fix type values, so overwriting is safe.
func (sb *satellitesBuffer) gsaQuality(fields []string) {
	// Fix type (field 1): 1=no fix, 2=2D, 3=3D
	switch fields[1] {
	case "2":
		sb.gsaFixDim = gpsprot.SolutionDim2D
	case "3":
		sb.gsaFixDim = gpsprot.SolutionDim3D
	}
	// DOPs (fields 14-16): PDOP, HDOP, VDOP
	if f, ok := parseFloatField(fields[14]); ok {
		sb.gsaDOP.Pos = opt.Make(f)
	}
	if f, ok := parseFloatField(fields[15]); ok {
		sb.gsaDOP.Hor = opt.Make(f)
	}
	if f, ok := parseFloatField(fields[16]); ok {
		sb.gsaDOP.Vert = opt.Make(f)
	}
}

// commitGSAQuality copies buffered SolutionDim and DOP to the epoch and
// clears the buffer. If epoch is nil, the buffer is preserved so that
// quality can be committed to a later epoch (e.g. GSA arriving before
// the first epoch-starting sentence).
func (sb *satellitesBuffer) commitGSAQuality(epoch *NavEpoch) {
	if epoch == nil {
		return
	}
	if sb.gsaFixDim != 0 {
		epoch.SolutionDim = sb.gsaFixDim
	}
	if sb.gsaDOP.Pos.IsSet() {
		epoch.DOP.Pos = sb.gsaDOP.Pos
	}
	if sb.gsaDOP.Hor.IsSet() {
		epoch.DOP.Hor = sb.gsaDOP.Hor
	}
	if sb.gsaDOP.Vert.IsSet() {
		epoch.DOP.Vert = sb.gsaDOP.Vert
	}
	sb.gsaFixDim = 0
	sb.gsaDOP = gpsprot.DOP{}
}

func (sb *satellitesBuffer) gsvProcess(sen *ApprovedSentence, tRead time.Time, h gpsprot.MsgHandler, epoch *NavEpoch) (bool, error) {
	gsv, err := parseGSV(sen)
	if err != nil {
		return false, err
	}
	// checkMsgNum decides whether gsv starts a new sequence (vs. continues
	// the immediately preceding one). At every sequence start we enforce
	// the invariant that sb.gsvs holds at most one sequence per effective
	// (gnss, sigID) key: if the new key is already in gsvKeys, either drop
	// the preceding F9T-style duplicate or flush. We can't gate this on
	// msgNum == 1 alone because the leading sentence(s) of a sequence may
	// be lost in a noisy stream.
	seqStart, err := sb.checkMsgNum(gsv)
	if seqStart {
		if _, exists := sb.gsvKeys[gsvKey{gnss: gsv.gnss, sigID: sb.gsvSigID(gsv)}]; exists {
			// dropDuplicate handles the F9T-style back-to-back duplicate
			// pattern by dropping the older sequence; otherwise treat the
			// collision as a real cycle boundary and flush. The key stays
			// in gsvKeys so a later cycle 2 (collision that isn't the
			// immediate predecessor) still triggers the flush.
			// In practice receivers emit sentences in a consistent order
			// within a cycle, so the wrap from "first key recorded" back
			// to the same key usually brackets one full cycle even pre-
			// haveBoundary (although this is making assumptions that we
			// do not make elsewhere).
			if !sb.dropDuplicate(gsv) {
				sb.flush(h, epoch)
			}
		}
		// Recompute the key after a possible flush: flush -> clear() resets
		// mixedSigIDs, so gsvSigID(gsv) may now differ from what it was
		// above.
		sb.gsvKeys[gsvKey{gnss: gsv.gnss, sigID: sb.gsvSigID(gsv)}] = struct{}{}
	}
	if len(sb.gsvs) == 0 {
		sb.tRead = tRead
	}
	sb.gsvs = append(sb.gsvs, gsv)
	return true, err
}

// checkMsgNum classifies gsv against the immediately preceding GSV in the
// buffer. It returns seqStart == false only when gsv is a valid
// continuation of that sequence (same gnss, msgNum == lastGSV.msgNum+1).
// All other cases -- empty buffer, different gnss, different sigID with
// msgNum == 1, or any malformed numbering -- count as the start of a new
// sequence. err is non-nil for malformed numbering that the caller should
// log as a lost-data diagnostic; the sentence is still treated as a
// sequence start and accumulated.
func (sb *satellitesBuffer) checkMsgNum(gsv gsvSentence) (bool, error) {
	if gsv.msgNum == 1 {
		return true, nil
	}
	if len(sb.gsvs) == 0 {
		return true, nil
	}
	lastGSV := sb.gsvs[len(sb.gsvs)-1]
	if lastGSV.gnss != gsv.gnss {
		return true, fmt.Errorf("invalid GSV message number: expected 1, got %d", gsv.msgNum)
	}
	// A valid continuation requires lastGSV to be mid-sequence with matching
	// numMsg; if lastGSV already completed or numMsg differs, the new
	// sentence belongs to a different sequence even when the numbers happen
	// to line up (e.g. previous 1/1 then next cycle's 2/N with msgNum=1
	// lost). Treating those as continuations would let same-key data from
	// two cycles accumulate together.
	if lastGSV.msgNum < lastGSV.numMsg && gsv.numMsg == lastGSV.numMsg && gsv.msgNum == lastGSV.msgNum+1 {
		// If sigID differs, the receiver is mixing signal IDs within a
		// single sequence (Allystar style); switch into mixedSigIDs mode.
		if lastGSV.sigID != gsv.sigID {
			sb.setMixedSigIDs()
		}
		return false, nil
	}
	if lastGSV.msgNum == lastGSV.numMsg {
		return true, fmt.Errorf("invalid GSV message number: expected 1, got %d", gsv.msgNum)
	}
	return true, fmt.Errorf("invalid GSV message number: expected 1 or %d, got %d", lastGSV.msgNum+1, gsv.msgNum)
}

// dropDuplicate detects the F9T-style duplicate-sigID firmware bug, where
// a u-blox ZED-F9T emits two back-to-back GAGSV sequences in a single
// burst carrying the same sigID instead of distinct sigIDs. The pattern
// is: gsv starts a fresh sequence (msgNum == 1), the previous processed
// sentence was also GSV (no other sentence type between them), and the
// immediately preceding entries in sb.gsvs are a completed same-key
// sequence. When all three hold, drop those entries and return true.
// Otherwise return false (the caller will flush, treating it as a real
// cycle boundary).
func (sb *satellitesBuffer) dropDuplicate(gsv gsvSentence) bool {
	if gsv.msgNum != 1 || sb.lastFormat != "GSV" || len(sb.gsvs) == 0 {
		return false
	}
	if last := sb.gsvs[len(sb.gsvs)-1]; last.msgNum != last.numMsg {
		return false
	}
	gsvSigID := sb.gsvSigID(gsv)
	i := len(sb.gsvs)
	for i > 0 {
		prev := sb.gsvs[i-1]
		if prev.gnss != gsv.gnss || sb.gsvSigID(prev) != gsvSigID {
			break
		}
		i--
	}
	if i == len(sb.gsvs) {
		return false
	}
	sb.gsvs = sb.gsvs[:i]
	return true
}

// setMixedSigIDs switches to mixed signal ID mode (e.g. Allystar receivers).
// Rebuilds gsvKeys to use GNSS-only keys (sigID=0).
func (sb *satellitesBuffer) setMixedSigIDs() {
	if sb.mixedSigIDs {
		return
	}
	sb.mixedSigIDs = true
	newKeys := make(map[gsvKey]struct{})
	for k := range sb.gsvKeys {
		newKeys[gsvKey{gnss: k.gnss, sigID: 0}] = struct{}{}
	}
	sb.gsvKeys = newKeys
}

// gsvSigID returns the effective signal ID for gsv, collapsing to 0 in
// mixedSigIDs mode so callers don't have to repeat the check.
func (sb *satellitesBuffer) gsvSigID(gsv gsvSentence) int {
	if sb.mixedSigIDs {
		return 0
	}
	return gsv.sigID
}

type gsaSentence struct {
	gnss  gpsprot.GNSS
	svids []int
}

func parseGSA(sen *ApprovedSentence) (gsaSentence, error) {
	gsa := gsaSentence{}
	// Fix, Auto, 12*SVID, PDOP, HDOP, VDOP, opt sysID
	if len(sen.Fields) < 17 {
		return gsa, fmt.Errorf("GSA: too few fields")
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
	var gnss gpsprot.GNSS
	if len(sen.Fields) >= 18 && sen.Fields[17] != "" {
		sysID, err := parseIntField(sen.Fields, 17, 1, 6, "GSA")
		if err != nil {
			return gsa, err
		}
		gnss = systemIDToGNSS(sysID)
	} else if len(sen.Fields) >= 18 && len(svids) > 0 {
		return gsa, fmt.Errorf("GSA: empty system ID with SVIDs present")
	} else {
		gnss = talkerIDToGNSS(sen.TalkerID)
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
func parseGSV(sen *ApprovedSentence) (gsvSentence, error) {
	gsv := gsvSentence{}

	if len(sen.Fields) < 3 {
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
		sv.azim, err = parseIntField(sen.Fields, i+2, 0, 360, "GSV")
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
		sigID, _, err = parseHexField(sen.Fields, i, "GSV") // treat missing sigID as 0
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
// It is also in IEC 61162-1-2010, which is corresponds to NMEA 4.00;
// I have this, but it includes only GPS, GAL and GLO.
var sigIDMap = map[gpsprot.GNSS]map[int]gpsprot.SignalID{
	gpsprot.GPS: {
		1: gpsprot.SigIDGPSL1CA,
		2: gpsprot.SigIDGPSL1PY,
		3: gpsprot.SigIDGPSL1M,
		4: gpsprot.SigIDGPSL2P,
		5: gpsprot.SigIDGPSL2CM,
		6: gpsprot.SigIDGPSL2CL,
		7: gpsprot.SigIDGPSL5I,
		8: gpsprot.SigIDGPSL5Q,
		// 9: gpsprot.SigIDGPSL1C, // not in 4.00; Allystar, guessing a bit
	},
	gpsprot.GAL: {
		1: gpsprot.SigIDGALE5a,
		2: gpsprot.SigIDGALE5b,
		3: gpsprot.SigIDGALE5,
		4: gpsprot.SigIDGALE6A,
		5: gpsprot.SigIDGALE6,
		6: gpsprot.SigIDGALE1A,
		7: gpsprot.SigIDGALE1,
	},
	gpsprot.BDS: {
		1:   gpsprot.SigIDBDSB1I,
		2:   gpsprot.SigIDBDSB1Q,
		3:   gpsprot.SigIDBDSB1C,
		4:   gpsprot.SigIDBDSB1A,
		5:   gpsprot.SigIDBDSB2a,
		6:   gpsprot.SigIDBDSB2b,
		7:   gpsprot.SigIDBDSB2ab,
		8:   gpsprot.SigIDBDSB3I,
		9:   gpsprot.SigIDBDSB3Q,
		0xA: gpsprot.SigIDBDSB3A,
		0xB: gpsprot.SigIDBDSB2I,
		0xC: gpsprot.SigIDBDSB2Q,
	},
	gpsprot.GLO: {
		1: gpsprot.SigIDGLOL1,
		2: gpsprot.SigIDGLOL1P,
		3: gpsprot.SigIDGLOL2,
		4: gpsprot.SigIDGLOL2P,
	},
	gpsprot.QZSS: {
		1:   gpsprot.SigIDQZSSL1CA,
		2:   gpsprot.SigIDQZSSL1CD,
		3:   gpsprot.SigIDQZSSL1CP,
		4:   gpsprot.SigIDQZSSL1S,
		5:   gpsprot.SigIDQZSSL2CM,
		6:   gpsprot.SigIDQZSSL2CL,
		7:   gpsprot.SigIDQZSSL5I,
		8:   gpsprot.SigIDQZSSL5Q,
		9:   gpsprot.SigIDQZSSL6,
		0xA: gpsprot.SigIDQZSSL6E,
	},
	gpsprot.NAVIC: {
		1: gpsprot.SigIDNAVICL5,
		2: gpsprot.SigIDNAVICS,
		3: gpsprot.SigIDNAVICL5RS,
		4: gpsprot.SigIDNAVICSRS,
		5: gpsprot.SigIDNAVICL1,
	},
}

func gnssSigIDName(gnss gpsprot.GNSS, sigID int) gpsprot.SignalID {
	if sigNames, ok := sigIDMap[gnss]; ok {
		return sigNames[sigID]
	}
	return gpsprot.SignalID(fmt.Sprintf("%X", sigID))
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

func parseHexField(fields []string, i int, format string) (int, bool, error) {
	s := fields[i]
	if len(s) == 0 {
		return 0, false, nil
	}
	if len(s) != 1 {
		return 0, false, fmt.Errorf("%s: invalid field %d: %s: length must be 1", format, i, fields[i])
	}
	n := hexDigit(s[0])
	if n < 0 {
		return 0, false, fmt.Errorf("%s: invalid field %d: %s: invalid character", format, i, fields[i])
	}
	return n, true, nil
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
