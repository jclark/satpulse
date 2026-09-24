package nmea

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ascii"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

// Tag is the identifier for NMEA protocol packets
const Tag gpsprot.Tag = "NMEA"

// Ensure PacketProcessor implements gpsprot.NMEAPacketProcessor
var _ gpsprot.NMEAPacketProcessor = (*PacketProcessor)(nil)

type Sentence struct {
	SyntaxFlags nmeamsg.SentenceSyntaxFlags
	Payload     string
}

type ApprovedSentence struct {
	TalkerID string   // talker ID, e.g. "GP" for GPS
	Format   string   // sentence format, e.g. "RMC"
	Fields   []string // the data fields
}

func NewSentence(data string) *Sentence {
	flags := nmeamsg.CheckSyntax(data)
	if flags&nmeamsg.SentenceIsPacket == 0 {
		return nil
	}
	asteriskIndex := len(data) - 4 // 3 for *XX and 1 for \n
	if flags&nmeamsg.SentenceEndsWithCRLF != 0 {
		asteriskIndex -= 1
	}
	return &Sentence{
		SyntaxFlags: flags,
		Payload:     data[1:asteriskIndex],
	}
}

func (s *Sentence) ApprovedSentence() *ApprovedSentence {
	if !s.SyntaxFlags.IsValidGNSSTalkerNMEA() {
		return nil // Not a valid NMEA approved sentence
	}
	fields := strings.Split(s.Payload, ",")
	addr := fields[0]
	fields = fields[1:] // Skip the address field
	if s.SyntaxFlags&nmeamsg.SentenceNoCarets == 0 {
		for i := range fields {
			fields[i] = unescape(fields[i])
		}
	}
	return &ApprovedSentence{
		TalkerID: addr[:2],
		Format:   addr[2:],
		Fields:   fields,
	}
}

// Assumes that the string has the SentenceValidCaretEscaping flag set
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

// TimeOfDay returns the sentence's time-of-day field (hhmmss.ss) and
// true, for the time-bearing approved sentences (RMC, GGA, GLL, ZDA);
// false otherwise. It lets an observer time these sentences without
// depending on this package's types.
func (s *Sentence) TimeOfDay() (string, bool) {
	as := s.ApprovedSentence()
	if as == nil {
		return "", false
	}
	var idx int
	switch as.Format {
	case "RMC", "GGA", "ZDA":
		idx = 0
	case "GLL":
		idx = 4
	default:
		return "", false
	}
	if idx >= len(as.Fields) {
		return "", false
	}
	return as.Fields[idx], true
}

func (s *Sentence) AddressField() string {
	if s.SyntaxFlags&nmeamsg.SentenceApprovedAddressFormat != 0 {
		return s.Payload[:5] // e.g. GPRMC
	}
	addr, _, _ := strings.Cut(s.Payload, ",")
	return addr
}

func (s *ApprovedSentence) msgID() string {
	return s.TalkerID + s.Format
}

// NavEpoch tracks epoch state. It embeds the NavEpochMsg that will be
// emitted at the end of the epoch, plus a TimeOfDay field for boundary
// detection. Exported because ExtSentenceHandler implementations (in
// other packages) receive and return it.
type NavEpoch struct {
	gpsprot.NavEpochMsg
	startTime  time.Time // tRead of first message in this epoch
	TimeOfDay  string    // UTC time-of-day string from the sentence; "" means no time yet
	rmcExtMode byte      // RMC extended mode indicator ('R', 'F', 'P'); 0 if not seen
}

// CheckEpoch is called by a message handler that participates in the
// epoch. tod is the message's time-of-day, or "" if the message has
// none. If the time-of-day matches the current epoch, it returns the
// same epoch. If the epoch has no time-of-day yet, it sets it.
// Otherwise (nil epoch or time-of-day mismatch), it allocates a new
// epoch.
func CheckEpoch(epoch *NavEpoch, tod string) *NavEpoch {
	if epoch != nil {
		if tod == "" || epoch.TimeOfDay == tod {
			return epoch
		}
		if epoch.TimeOfDay == "" {
			epoch.TimeOfDay = tod
			return epoch
		}
	}
	return &NavEpoch{TimeOfDay: tod}
}

// ExtSentenceHandler handles non-standard NMEA sentences that are not
// approved GNSS talker sentences (e.g. proprietary PQTM sentences).
// Handlers are called for any sentence not handled by approved-sentence
// processing.
type ExtSentenceHandler interface {
	// HandleSentence attempts to handle a non-standard NMEA sentence.
	// flags contains the syntax flags from the packet scanner.
	// payload is the NMEA payload between $ and *XX.
	// epoch is the current *NavEpoch, or nil if no epoch is in progress.
	//
	// Return values:
	//   - (nil, nil, ErrNotHandled): not handled
	//   - (nil, nil, err): recognized but parse failed.
	//   - (msgs, sameEpoch, nil): handled; messages belong to the current epoch.
	//   - (msgs, newEpoch, nil): handled; messages start a new epoch.
	//   - (msgs, nil, nil): handled; end of epoch, flush.
	HandleSentence(flags nmeamsg.SentenceSyntaxFlags, payload string, epoch *NavEpoch) ([]gpsprot.Msg, *NavEpoch, error)
}

// PacketProcessor implements the gpsprot.PacketProcessor interface for NMEA packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh          gpsprot.MsgHandler
	mgr         *gpsprot.NavEpochManager
	sb          satellitesBuffer
	extHandlers []ExtSentenceHandler
	curNavEpoch *NavEpoch
}

// NewPacketProcessor creates a new NMEA packet processor
func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{
		mgr: mgr,
		sb:  *newSatellitesBuffer(),
	}
}

// ProcessPacket processes an NMEA packet's data and returns the type of the message and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	sen := NewSentence(data)
	if sen == nil {
		return "", fmt.Errorf("not a valid NMEA packet: %s", data)
	}
	msgID := sen.AddressField()
	approvSen := sen.ApprovedSentence()
	if approvSen != nil {
		handled, err := p.Dispatch(approvSen, tRead, p.mh)
		if err != nil {
			return msgID, err
		}
		if handled {
			// Offer the consumed sentence to an observer (the rate
			// estimator) via the optional handled channel: NMEA is the
			// main traffic on a fresh unit, and its time-bearing
			// sentences carry content time.
			if hh, ok := p.GetNativeMsgHandler().(gpsprot.HandledNativeMsgHandler); ok {
				return msgID, hh.HandledNativeMsg(Tag, msgID, sen, tRead)
			}
			return msgID, nil
		}
	}
	for _, eh := range p.extHandlers {
		msgs, epoch, err := eh.HandleSentence(sen.SyntaxFlags, sen.Payload, p.curNavEpoch)
		if err != nil {
			if errors.Is(err, gpsprot.ErrNotHandled) {
				continue
			}
			return msgID, err
		}
		p.handleEpoch(epoch, tRead)
		if h := p.mh; h != nil {
			p.setTimeMsgReadDelay(msgs, tRead)
			gpsprot.DispatchMsgs(msgs, h, tRead)
		}
		return msgID, nil
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, sen, tRead)
	}
	return msgID, nil
}

func (p *PacketProcessor) handleEpoch(epoch *NavEpoch, tRead time.Time) {
	if epoch == nil {
		p.mgr.EndOfEpoch(tRead)
		return
	}
	if epoch != p.curNavEpoch {
		p.mgr.EpochStarted(p, tRead)
		epoch.startTime = tRead
		p.curNavEpoch = epoch
	}
}

// FlushNavEpoch implements gpsprot.EpochFlusher.
func (p *PacketProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	epoch := p.curNavEpoch
	p.curNavEpoch = nil
	if epoch == nil {
		return nil, gpsprot.PriGenericHigh, p.mh
	}
	// GSA has no time-of-day, so pending quality could reasonably
	// be attached to either this epoch or the next. We choose to
	// attach it to the outgoing epoch.
	p.sb.commitGSAQuality(epoch)
	finalizeNavEpoch(epoch)
	epoch.Tag = Tag
	return &epoch.NavEpochMsg, gpsprot.PriGenericHigh, p.mh
}

// AddExtHandler registers an extension handler for non-standard sentences.
func (p *PacketProcessor) AddExtHandler(h ExtSentenceHandler) {
	p.extHandlers = append(p.extHandlers, h)
}

func (p *PacketProcessor) SetSVNumbering(numbering []gpsprot.NMEASVNumberingRange) {
	p.sb.setNumbering(numbering)
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// Dispatch handles standard messages and returns true if handled, along with any error
func (p *PacketProcessor) Dispatch(sen *ApprovedSentence, tRead time.Time, h gpsprot.MsgHandler) (bool, error) {
	handled, err := p.sb.process(sen, tRead, h, p.curNavEpoch)
	if err != nil || handled {
		return handled, err
	}
	switch sen.Format {
	case "RMC":
		msgs, epoch, err := parseRMC(sen, p.curNavEpoch)
		if err != nil {
			return false, err
		}
		gpsprot.SetMsgsPriority(msgs, gpsprot.PriGenericLow)
		p.handleEpoch(epoch, tRead)
		if h != nil {
			p.setTimeMsgReadDelay(msgs, tRead)
			gpsprot.DispatchMsgs(msgs, h, tRead)
		}
		return true, nil
	case "GGA":
		pos, epoch, err := parseGGA(sen, p.curNavEpoch)
		if err != nil {
			return false, err
		}
		p.handleEpoch(epoch, tRead)
		if h != nil && pos != nil {
			pos.Priority = gpsprot.PriGenericHigh
			h.PosGeo(pos, tRead)
		}
		return true, nil
	case "VTG":
		vel, epoch, err := parseVTG(sen, p.curNavEpoch)
		if err != nil {
			return false, err
		}
		p.handleEpoch(epoch, tRead)
		if h != nil && vel != nil {
			vel.Priority = gpsprot.PriGenericHigh
			h.VelGeo(vel, tRead)
		}
		return true, nil
	case "ZDA":
		utc, err := parseZDA(sen)
		if err != nil {
			return false, err
		}
		epoch := CheckEpoch(p.curNavEpoch, sen.Fields[0])
		p.handleEpoch(epoch, tRead)
		mt := gpsprot.TimeMsg{Tag: Tag, NativeMsgID: sen.msgID(), UTCTime: utc, GNSS: talkerIDToGNSS(sen.TalkerID)}
		if h != nil {
			if ne := p.curNavEpoch; ne != nil {
				mt.ReadDelay = gpsprot.Duration(tRead.Sub(ne.startTime))
			}
			h.Time(&mt, tRead)
		}
		return true, nil
	}
	return false, nil
}

func (p *PacketProcessor) Idle(_ time.Time) {
	p.sb.idle(p.mh, p.curNavEpoch)
}

// setTimeMsgReadDelay sets ReadDelay on the first TimeMsg in msgs, if curNavEpoch is known.
func (p *PacketProcessor) setTimeMsgReadDelay(msgs []gpsprot.Msg, tRead time.Time) {
	ne := p.curNavEpoch
	if ne == nil {
		return
	}
	for _, m := range msgs {
		if tm, ok := m.(*gpsprot.TimeMsg); ok {
			tm.ReadDelay = gpsprot.Duration(tRead.Sub(ne.startTime))
			return
		}
	}
}

// Sentence parsers: each parser sets Tag and NativeMsgID on the
// messages it creates.

func parseRMC(sen *ApprovedSentence, epoch *NavEpoch) ([]gpsprot.Msg, *NavEpoch, error) {
	k := sen.TalkerID + "RMC"
	if len(sen.Fields) < 9 {
		return nil, nil, fmt.Errorf("%s: too few fields", k)
	}
	epoch = CheckEpoch(epoch, sen.Fields[0])
	rmcQuality(epoch, sen.Fields)
	var msgs []gpsprot.Msg
	if sen.Fields[1] != "A" {
		msgs = append(msgs, &gpsprot.TimeMsg{Tag: Tag, NativeMsgID: k, GNSS: talkerIDToGNSS(sen.TalkerID)})
		return msgs, epoch, nil
	}
	// Status is "A" -- active/valid
	utc, err := parseDateTime(sen.Fields[0], sen.Fields[8])
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", k, err)
	}
	msgs = append(msgs, &gpsprot.TimeMsg{Tag: Tag, NativeMsgID: k, UTCTime: utc, GNSS: talkerIDToGNSS(sen.TalkerID)})
	if ll, ok, err := parseLatLon(sen.Fields[2], sen.Fields[3], sen.Fields[4], sen.Fields[5]); err != nil {
		return nil, nil, fmt.Errorf("%s: %v", k, err)
	} else if ok {
		msgs = append(msgs, &gpsprot.PosGeoMsg{LatLon: ll, Tag: Tag, NativeMsgID: k})
	}
	vel := &gpsprot.VelGeoMsg{Tag: Tag, NativeMsgID: k}
	vel.GroundSpeed = parseSpeedKnots(sen.Fields[6])
	vel.Course, err = parseCourse(sen.Fields[7])
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", k, err)
	}
	if vel.GroundSpeed.IsSet() || vel.Course.IsSet() {
		msgs = append(msgs, vel)
	}
	return msgs, epoch, nil
}

// rmcQuality populates quality metadata on the epoch from the RMC
// mode indicator (field 11, NMEA 2.3+). Extended modes R/F/P are
// stored on the epoch and applied authoritatively by finalizeNavEpoch.
// Basic modes are a fallback: they only write when GGA hasn't set
// quality yet.
func rmcQuality(epoch *NavEpoch, fields []string) {
	if len(fields) <= 11 || fields[11] == "" {
		return
	}
	switch fields[11] {
	case "R", "F", "P":
		epoch.rmcExtMode = fields[11][0]
	case "N":
		if epoch.FixLevel == 0 {
			epoch.FixLevel = gpsprot.FixLevelNone
		}
	case "A":
		if epoch.FixLevel == 0 {
			epoch.FixLevel = gpsprot.FixLevelCode
		}
	case "D":
		if epoch.FixLevel == 0 {
			epoch.FixLevel = gpsprot.FixLevelCode
			epoch.Correction = gpsprot.CorrUsed
		}
	case "E":
		if epoch.FixLevel == 0 {
			epoch.FixLevel = gpsprot.FixLevelNone
			epoch.AuxSrc = gpsprot.AuxSrcDR
		}
	case "M", "S":
		if epoch.FixLevel == 0 {
			epoch.FixLevel = gpsprot.FixLevelNotMeasured
		}
	}
}

// finalizeNavEpoch applies deferred quality overrides before the epoch
// is flushed. RMC extended modes (R/F/P) are authoritative and
// override GGA-derived quality.
func finalizeNavEpoch(epoch *NavEpoch) {
	switch epoch.rmcExtMode {
	case 'R':
		epoch.FixLevel = gpsprot.FixLevelCarrierFixed
		epoch.Correction = gpsprot.CorrOSR | gpsprot.CorrUsed
		epoch.AuxSrc = 0
	case 'F':
		epoch.FixLevel = gpsprot.FixLevelCarrierFloat
		epoch.Correction = gpsprot.CorrOSR | gpsprot.CorrUsed
		epoch.AuxSrc = 0
	case 'P':
		epoch.FixLevel = gpsprot.FixLevelCode
		epoch.Correction = gpsprot.CorrSSR | gpsprot.CorrUsed
		epoch.AuxSrc = 0
	}
	if epoch.FixLevel < gpsprot.FixLevelCode {
		epoch.SolutionDim = 0
	}
}

// parseDateTime parses NMEA time (HHMMSS.sss) and date (DDMMYY)
// fields into a UTCTime. Returns an unset value (not an error) when
// either field is empty.
func parseDateTime(timeStr, dateStr string) (opt.Val[ptime.UTCTime], error) {
	if timeStr == "" || dateStr == "" {
		return opt.Val[ptime.UTCTime]{}, nil
	}
	var year uint16
	var month, day, hour, min, sec uint8
	var nanos int32
	if !scanTime(timeStr, &hour, &min, &sec, &nanos) {
		return opt.Val[ptime.UTCTime]{}, fmt.Errorf("%s: invalid time", timeStr)
	}
	if len(dateStr) != 6 || !isDigits(dateStr) {
		return opt.Val[ptime.UTCTime]{}, fmt.Errorf("%s: invalid date", dateStr)
	}
	// Sscanf is very forgiving, so we check for length and all digits first, so that Sscanf is guaranteed to succeed.
	_, _ = fmt.Sscanf(dateStr, "%02d%02d%02d", &day, &month, &year)
	// There are some test examples from the 1990s.
	// Start with 1980 since NMEA first issued in the 1980s
	if year >= 80 {
		year += 1900
	} else {
		year += 2000
	}
	return opt.Make(ptime.UTC(year, month, day, hour, min, sec, nanos)), nil
}

// parseGGA parses a GGA sentence into a PosGeoMsg and populates
// quality metadata on the epoch.
func parseGGA(sen *ApprovedSentence, epoch *NavEpoch) (*gpsprot.PosGeoMsg, *NavEpoch, error) {
	k := sen.TalkerID + "GGA"
	if len(sen.Fields) < 11 {
		return nil, nil, fmt.Errorf("%s: too few fields", k)
	}
	epoch = CheckEpoch(epoch, sen.Fields[0])
	ggaQuality(epoch, sen.Fields)
	qual := sen.Fields[5]
	if qual == "0" || qual == "" {
		return nil, epoch, nil
	}
	ll, ok, err := parseLatLon(sen.Fields[1], sen.Fields[2], sen.Fields[3], sen.Fields[4])
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", k, err)
	}
	if !ok {
		return nil, epoch, nil
	}
	pos := &gpsprot.PosGeoMsg{LatLon: ll, Tag: Tag, NativeMsgID: k}
	altMSL, haveAlt := parseFloatField(sen.Fields[8])
	if haveAlt {
		pos.HeightMSL.Set(gpsprot.Meters(altMSL))
		if sep, ok := parseFloatField(sen.Fields[10]); ok {
			pos.Height.Set(gpsprot.Meters(altMSL + sep))
		}
	}
	return pos, epoch, nil
}

// ggaQuality populates quality metadata on the epoch from GGA fields.
func ggaQuality(epoch *NavEpoch, fields []string) {
	qual := fields[5]
	if qual == "" {
		return
	}
	var fl gpsprot.FixLevel
	var corr gpsprot.CorrKind
	var aux gpsprot.AuxSrc
	switch qual {
	case "0":
		fl = gpsprot.FixLevelNone
	case "1":
		fl = gpsprot.FixLevelCode
	case "2", "3":
		fl = gpsprot.FixLevelCode
		corr = gpsprot.CorrUsed
	case "4":
		fl = gpsprot.FixLevelCarrierFixed
		corr = gpsprot.CorrOSR | gpsprot.CorrUsed
	case "5":
		fl = gpsprot.FixLevelCarrierFloat
		corr = gpsprot.CorrOSR | gpsprot.CorrUsed
	case "6":
		fl = gpsprot.FixLevelNone
		aux = gpsprot.AuxSrcDR
	case "7", "8":
		fl = gpsprot.FixLevelNotMeasured
	default:
		return
	}
	epoch.FixLevel = fl
	epoch.Correction = corr
	epoch.AuxSrc = aux
	if n, ok := parseUintField(fields[6]); ok && n <= 999 {
		epoch.NumSVUsed = opt.Make(uint16(n))
	}
	if f, ok := parseFloatField(fields[7]); ok {
		epoch.DOP.Hor = opt.Make(f)
	}
	if len(fields) > 12 {
		if f, ok := parseFloatField(fields[12]); ok {
			epoch.DiffAge = opt.Make(gpsprot.Seconds(f))
		}
	}
	if len(fields) > 13 {
		if n, ok := parseUintField(fields[13]); ok && n <= 4095 {
			epoch.RTCMRefBaseID = opt.Make(uint16(n))
		}
	}
}

// parseVTG parses a VTG sentence into a VelGeoMsg.
func parseVTG(sen *ApprovedSentence, epoch *NavEpoch) (*gpsprot.VelGeoMsg, *NavEpoch, error) {
	k := sen.TalkerID + "VTG"
	if len(sen.Fields) < 7 {
		return nil, nil, fmt.Errorf("%s: too few fields", k)
	}
	epoch = CheckEpoch(epoch, "")
	// Mode field (field 8) is optional; "N" means no fix.
	if len(sen.Fields) > 8 && sen.Fields[8] == "N" {
		return nil, epoch, nil
	}
	vel := &gpsprot.VelGeoMsg{Tag: Tag, NativeMsgID: k}
	vel.GroundSpeed = parseSpeedKmh(sen.Fields[6])
	var err error
	vel.Course, err = parseCourse(sen.Fields[0])
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", k, err)
	}
	if !vel.GroundSpeed.IsSet() && !vel.Course.IsSet() {
		return nil, epoch, nil
	}
	return vel, epoch, nil
}

// parseSpeedKnots parses a speed field in knots.
func parseSpeedKnots(s string) opt.Val[gpsprot.Speed] {
	if f, ok := parseFloatField(s); ok {
		return opt.Make(gpsprot.MetersPerSecondFromFloat(f * 1852.0 / 3600.0))
	}
	return opt.Val[gpsprot.Speed]{}
}

// parseSpeedKmh parses a speed field in km/h.
func parseSpeedKmh(s string) opt.Val[gpsprot.Speed] {
	if f, ok := parseFloatField(s); ok {
		return opt.Make(gpsprot.MetersPerSecondFromFloat(f / 3.6))
	}
	return opt.Val[gpsprot.Speed]{}
}

// parseCourse parses a course-over-ground field in degrees. Returns
// an unset value for an empty field. The value must be in [0, 360];
// 360 is normalized to 0.
func parseCourse(s string) (opt.Val[gpsprot.Angle], error) {
	f, ok := parseFloatField(s)
	if !ok {
		return opt.Val[gpsprot.Angle]{}, nil
	}
	if f < 0 || f > 360 {
		return opt.Val[gpsprot.Angle]{}, fmt.Errorf("course %v out of range [0,360]", f)
	}
	if f == 360 {
		f = 0
	}
	return opt.Make(gpsprot.DegreesFromFloat(f)), nil
}

func parseLatLon(latField, nsField, lonField, ewField string) ([2]gpsprot.Angle, bool, error) {
	lat, ok := parseDegMin(latField)
	if !ok {
		return [2]gpsprot.Angle{}, false, nil
	}
	lon, ok := parseDegMin(lonField)
	if !ok {
		return [2]gpsprot.Angle{}, false, nil
	}
	switch nsField {
	case "N":
	case "S":
		lat = -lat
	default:
		return [2]gpsprot.Angle{}, false, fmt.Errorf("invalid N/S field %q", nsField)
	}
	switch ewField {
	case "E":
	case "W":
		lon = -lon
	default:
		return [2]gpsprot.Angle{}, false, fmt.Errorf("invalid E/W field %q", ewField)
	}
	return [2]gpsprot.Angle{gpsprot.DegreesFromFloat(lat), gpsprot.DegreesFromFloat(lon)}, true, nil
}

// parseDegMin parses an NMEA latitude (DDMM.MMMM) or longitude
// (DDDMM.MMMM) field. The decimal point position determines the
// split: the two characters before the decimal point are the start of
// the minutes field.
func parseDegMin(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	dot := strings.IndexByte(s, '.')
	if dot < 2 {
		return 0, false
	}
	degStr := s[:dot-2]
	minStr := s[dot-2:]
	deg, err := strconv.ParseFloat(degStr, 64)
	if err != nil {
		return 0, false
	}
	min, err := strconv.ParseFloat(minStr, 64)
	if err != nil {
		return 0, false
	}
	return deg + min/60.0, true
}

func parseFloatField(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func parseUintField(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseZDA(sen *ApprovedSentence) (opt.Val[ptime.UTCTime], error) {
	k := sen.TalkerID + "ZDA"
	if len(sen.Fields) < 4 {
		return opt.Val[ptime.UTCTime]{}, fmt.Errorf("%s: too few fields", k)
	}
	timeStr := sen.Fields[0]
	if timeStr == "" {
		return opt.Val[ptime.UTCTime]{}, nil
	}
	var year uint16
	var month, day, hour, min, sec uint8
	var nanos int32
	if !scanTime(timeStr, &hour, &min, &sec, &nanos) {
		return opt.Val[ptime.UTCTime]{}, fmt.Errorf("%s: %s: invalid time", k, timeStr)
	}
	for i := 1; i < 4; i++ {
		d := sen.Fields[i]
		if d == "" {
			return opt.Val[ptime.UTCTime]{}, nil
		}
		expectLen := 2
		if i == 3 {
			expectLen = 4
		}
		if !isDigits(d) || len(d) != expectLen {
			return opt.Val[ptime.UTCTime]{}, fmt.Errorf("%s: %s: invalid date field", k, d)
		}
	}
	_, _ = fmt.Sscanf(sen.Fields[1], "%02d", &day)
	_, _ = fmt.Sscanf(sen.Fields[2], "%02d", &month)
	_, _ = fmt.Sscanf(sen.Fields[3], "%04d", &year)
	// Need a limit on year to ensure ptime.Time isn't out of range
	// Allow 1980 since first version of NMEA was issued was 1980.
	if year < 1980 || year > 2099 {
		return opt.Val[ptime.UTCTime]{}, fmt.Errorf("%s: %s: invalid year", k, sen.Fields[3])
	}
	return opt.Make(ptime.UTC(year, month, day, hour, min, sec, nanos)), nil
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
	for i := 0; i < len(s); i++ {
		if !ascii.IsDigit(s[i]) {
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

func hexToByte(digits string) byte {
	hi, _ := ascii.UpperHexVal(digits[0])
	lo, _ := ascii.UpperHexVal(digits[1])
	return hi<<4 | lo
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
