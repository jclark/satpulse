package msgfile

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/pelletier/go-toml/v2"
)

// MsgCommon contains fields shared by all message types.
type MsgCommon struct {
	Delay       *float64 `toml:"delay"`
	Tag         *string  `toml:"tag"`
	Description string   `toml:"description"`
}

// TagDesc is a tag with an optional description and message count.
type TagDesc struct {
	Tag      string
	Desc     string
	MsgCount int
}

type tagDescGetter interface {
	tagDesc() TagDesc
}

func (mc *MsgCommon) tagDesc() TagDesc {
	tag := ""
	if mc.Tag != nil {
		tag = *mc.Tag
	}
	return TagDesc{Tag: tag, Desc: mc.Description}
}

// ResponsePattern selects the response matching behavior for a LineMsg.
type ResponsePattern int

const (
	ResponsePatternNone    ResponsePattern = iota // zero value: no response matching
	ResponsePatternUnicore                        // "unicore"
)

var responsePatternStrings = [...]string{
	ResponsePatternNone:    "none",
	ResponsePatternUnicore: "unicore",
}

// String returns the string representation of the ResponsePattern.
func (rp ResponsePattern) String() string {
	if rp >= 0 && int(rp) < len(responsePatternStrings) {
		return responsePatternStrings[rp]
	}
	return fmt.Sprintf("ResponsePattern(%d)", int(rp))
}

// ParseResponsePattern parses a string into a ResponsePattern.
func ParseResponsePattern(s string) (ResponsePattern, error) {
	for i, name := range responsePatternStrings {
		if s == name {
			return ResponsePattern(i), nil
		}
	}
	return 0, fmt.Errorf("unknown responsePattern: %q", s)
}

// MarshalText implements encoding.TextMarshaler.
func (rp ResponsePattern) MarshalText() ([]byte, error) {
	return []byte(rp.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (rp *ResponsePattern) UnmarshalText(data []byte) error {
	v, err := ParseResponsePattern(string(data))
	if err != nil {
		return err
	}
	*rp = v
	return nil
}

func (mc *MsgCommon) delay() (time.Duration, error) {
	if *mc.Delay < 0 {
		return 0, errors.New("delay must not be negative")
	}
	return ptime.Seconds(*mc.Delay), nil
}

// ResponseKind classifies how an incoming packet relates to sent messages.
type ResponseKind int

const (
	NotResponse   ResponseKind = iota // definitely not a response to anything we sent
	MaybeResponse                     // might be a response, can't tell
	AckResponse                       // definite ACK/NAK of a specific sent message
	OtherResponse                     // definitely a response (not ACK/NAK)
)

// AckNak is the default AckError when no detail is available.
const AckNak = "NAK"

// PacketAnalysis is the result of analyzing an incoming packet.
type PacketAnalysis struct {
	Kind       ResponseKind
	AckError   string  // AckResponse only: empty = success, non-empty = failure
	RelatedMsg *RawMsg // AckResponse only: the sent message this responds to
}

// responsePattern creates a responseMatcher for a specific sent message.
type responsePattern interface {
	newMatcher() responseMatcher
}

// responseMatcher examines an incoming packet and returns how it relates
// to the specific sent message this matcher was created for.
type responseMatcher interface {
	match(tag gpsprot.Tag, data string) (ResponseKind, string)
}

// Parsed represents a parsed message file.
type Parsed struct {
	Default struct {
		Line   LineMsg   `toml:"line"`
		Binary BinaryMsg `toml:"binary"`
		NMEA   NMEAMsg   `toml:"nmea"`
		CASBIN CASBINMsg `toml:"casbin"`
		ASBIN  ASBINMsg  `toml:"asbin"`
		SDBP   SDBPMsg   `toml:"sdbp"`
		UBX    UBXMsg    `toml:"ubx"`
	} `toml:"default"`
	Line   []LineMsg   `toml:"line"`
	Binary []BinaryMsg `toml:"binary"`
	NMEA   []NMEAMsg   `toml:"nmea"`
	CASBIN []CASBINMsg `toml:"casbin"`
	ASBIN  []ASBINMsg  `toml:"asbin"`
	SDBP   []SDBPMsg   `toml:"sdbp"`
	UBX    []UBXMsg    `toml:"ubx"`
}

// RawMsg is a message ready to send: raw bytes with metadata.
type RawMsg struct {
	Bytes  []byte
	Delay  time.Duration
	Tag    string // for logging
	Index  int    // 0-based index within tag
	Count  int    // total messages with this tag
	source responsePattern
}

// MsgID identifies a sent message for display purposes.
type MsgID struct {
	Tag   string // tag name
	Index int    // 0-based index among messages with this tag
	Count int    // total messages with this tag
}

// MsgID returns the identity of this sent message.
func (rm *RawMsg) MsgID() MsgID {
	return MsgID{Tag: rm.Tag, Index: rm.Index, Count: rm.Count}
}

type userMsg interface {
	toRaw() (RawMsg, error)
	getTag() string
	responsePattern
}

// PacketAnalyzer classifies incoming packets and correlates ACK/NAK
// responses to specific sent messages.
type PacketAnalyzer struct {
	msgs     []RawMsg
	matchers []responseMatcher
	acked    []bool
}

// NewPacketAnalyzer creates a new PacketAnalyzer.
func NewPacketAnalyzer() *PacketAnalyzer {
	return &PacketAnalyzer{}
}

// NotifySent records a sent message for future response matching.
func (pa *PacketAnalyzer) NotifySent(rm RawMsg) {
	pa.msgs = append(pa.msgs, rm)
	pa.matchers = append(pa.matchers, rm.source.newMatcher())
	pa.acked = append(pa.acked, false)
}

// Analyze classifies an incoming packet and correlates acks to sent messages.
func (pa *PacketAnalyzer) Analyze(tag gpsprot.Tag, data string) PacketAnalysis {
	type ackResult struct {
		idx    int
		ackErr string
	}
	var acks []ackResult
	hasOther := false
	hasMaybe := false
	for i, m := range pa.matchers {
		kind, ackErr := m.match(tag, data)
		if kind == AckResponse && pa.acked[i] {
			continue // ignore acks from already-acked matchers
		}
		switch kind {
		case AckResponse:
			acks = append(acks, ackResult{i, ackErr})
		case OtherResponse:
			hasOther = true
		case MaybeResponse:
			hasMaybe = true
		}
	}
	if len(acks) == 1 {
		idx := acks[0].idx
		pa.acked[idx] = true
		return PacketAnalysis{
			Kind:       AckResponse,
			AckError:   acks[0].ackErr,
			RelatedMsg: &pa.msgs[idx],
		}
	}
	if len(acks) > 1 {
		return PacketAnalysis{Kind: OtherResponse}
	}
	if hasOther {
		return PacketAnalysis{Kind: OtherResponse}
	}
	if hasMaybe {
		return PacketAnalysis{Kind: MaybeResponse}
	}
	return PacketAnalysis{Kind: NotResponse}
}

func ptr[T any](v T) *T { return &v }

func defaultMsgCommon() MsgCommon {
	return MsgCommon{Delay: ptr(0.0), Tag: ptr("")}
}

func newDefault() *Parsed {
	mf := new(Parsed)
	mf.Default.Line.EOL = ptr("\r\n")
	mf.Default.Line.RespPattern = ptr(ResponsePatternNone)
	mf.Default.Line.MsgCommon = defaultMsgCommon()
	mf.Default.Binary.MsgCommon = defaultMsgCommon()
	mf.Default.NMEA.MsgCommon = defaultMsgCommon()
	mf.Default.CASBIN.MsgCommon = defaultMsgCommon()
	mf.Default.ASBIN.MsgCommon = defaultMsgCommon()
	mf.Default.SDBP.MsgCommon = defaultMsgCommon()
	mf.Default.UBX.MsgCommon = defaultMsgCommon()
	return mf
}

// Load reads and parses a TOML message file.
func Load(path string) (*Parsed, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	mf := newDefault()
	err = toml.NewDecoder(f).DisallowUnknownFields().Decode(mf)
	if err != nil {
		return nil, err
	}
	return mf, nil
}

// decodeHex decodes a hex string, ignoring whitespace.
func decodeHex(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\t", "")
	return hex.DecodeString(s)
}

func applyCommonDefaults(dst, src *MsgCommon) {
	if dst.Delay == nil {
		dst.Delay = src.Delay
	}
	if dst.Tag == nil {
		dst.Tag = src.Tag
	}
}

func (mf *Parsed) applyLineDefaults(lm *LineMsg) {
	if lm.EOL == nil {
		lm.EOL = mf.Default.Line.EOL
	}
	if lm.RespPattern == nil {
		lm.RespPattern = mf.Default.Line.RespPattern
	}
	applyCommonDefaults(&lm.MsgCommon, &mf.Default.Line.MsgCommon)
}

func (mf *Parsed) applyBinaryDefaults(bm *BinaryMsg) {
	applyCommonDefaults(&bm.MsgCommon, &mf.Default.Binary.MsgCommon)
}

func (mf *Parsed) applyNMEADefaults(nm *NMEAMsg) {
	applyCommonDefaults(&nm.MsgCommon, &mf.Default.NMEA.MsgCommon)
}

func (mf *Parsed) applyCASBINDefaults(cm *CASBINMsg) {
	applyCommonDefaults(&cm.MsgCommon, &mf.Default.CASBIN.MsgCommon)
}

func (mf *Parsed) applyASBINDefaults(am *ASBINMsg) {
	applyCommonDefaults(&am.MsgCommon, &mf.Default.ASBIN.MsgCommon)
}

func (mf *Parsed) applySDBPDefaults(sm *SDBPMsg) {
	applyCommonDefaults(&sm.MsgCommon, &mf.Default.SDBP.MsgCommon)
}

func (mf *Parsed) applyUBXDefaults(um *UBXMsg) {
	applyCommonDefaults(&um.MsgCommon, &mf.Default.UBX.MsgCommon)
}

func (mf *Parsed) validateDefaults() error {
	if mf.Default.Line.Text != "" {
		return errors.New("default.line.text must be empty")
	}
	if mf.Default.Line.Description != "" {
		return errors.New("default.line.description must be empty")
	}
	if _, err := mf.Default.Line.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.line: %w", err)
	}
	if mf.Default.Binary.Hex != "" {
		return errors.New("default.binary.hex must be empty")
	}
	if mf.Default.Binary.Description != "" {
		return errors.New("default.binary.description must be empty")
	}
	if _, err := mf.Default.Binary.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.binary: %w", err)
	}
	if mf.Default.NMEA.Text != "" {
		return errors.New("default.nmea.text must be empty")
	}
	if mf.Default.NMEA.Description != "" {
		return errors.New("default.nmea.description must be empty")
	}
	if _, err := mf.Default.NMEA.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.nmea: %w", err)
	}
	if mf.Default.CASBIN.Class != 0 || mf.Default.CASBIN.ID != 0 {
		return errors.New("default.casbin.class and default.casbin.id must be zero")
	}
	if mf.Default.CASBIN.Description != "" {
		return errors.New("default.casbin.description must be empty")
	}
	if _, err := mf.Default.CASBIN.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.casbin: %w", err)
	}
	if mf.Default.ASBIN.Class != 0 || mf.Default.ASBIN.ID != 0 {
		return errors.New("default.asbin.class and default.asbin.id must be zero")
	}
	if mf.Default.ASBIN.Description != "" {
		return errors.New("default.asbin.description must be empty")
	}
	if _, err := mf.Default.ASBIN.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.asbin: %w", err)
	}
	if mf.Default.SDBP.Class != 0 || mf.Default.SDBP.ID != 0 {
		return errors.New("default.sdbp.class and default.sdbp.id must be zero")
	}
	if mf.Default.SDBP.Description != "" {
		return errors.New("default.sdbp.description must be empty")
	}
	if _, err := mf.Default.SDBP.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.sdbp: %w", err)
	}
	if mf.Default.UBX.Class != 0 || mf.Default.UBX.ID != 0 {
		return errors.New("default.ubx.class and default.ubx.id must be zero")
	}
	if mf.Default.UBX.Description != "" {
		return errors.New("default.ubx.description must be empty")
	}
	if _, err := mf.Default.UBX.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.ubx: %w", err)
	}
	return nil
}

type tagIndex struct {
	byTag map[string][]int
	tags  []string
}

type msgType string

const (
	msgTypeLine   msgType = "line"
	msgTypeBinary msgType = "binary"
	msgTypeNMEA   msgType = "nmea"
	msgTypeCASBIN msgType = "casbin"
	msgTypeASBIN  msgType = "asbin"
	msgTypeSDBP   msgType = "sdbp"
	msgTypeUBX    msgType = "ubx"
)

type tagIndices map[msgType]tagIndex

// ValidateTags checks tag-related message file constraints without selecting
// or sending messages.
func (mf *Parsed) ValidateTags() error {
	_, err := mf.buildTagIndices()
	return err
}

// TaggedMsgs returns messages for the given tags with defaults applied.
// Returns a typed slice ([]LineMsg, []BinaryMsg, []NMEAMsg, []CASBINMsg, []ASBINMsg, or []UBXMsg).
// Returns error if tags select messages of mixed types or if there are no messages with the tags.
func (mf *Parsed) TaggedMsgs(tags []string) (any, error) {
	ti, err := mf.buildTagIndices()
	if err != nil {
		return nil, err
	}
	var rslt any
	lineMsgs := filterMsgs(mf.Line, tags, ti[msgTypeLine].byTag)
	if len(lineMsgs) > 0 {
		rslt = lineMsgs
	}
	binaryMsgs := filterMsgs(mf.Binary, tags, ti[msgTypeBinary].byTag)
	if len(binaryMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = binaryMsgs
	}
	nmeaMsgs := filterMsgs(mf.NMEA, tags, ti[msgTypeNMEA].byTag)
	if len(nmeaMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = nmeaMsgs
	}
	casbinMsgs := filterMsgs(mf.CASBIN, tags, ti[msgTypeCASBIN].byTag)
	if len(casbinMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = casbinMsgs
	}
	asbinMsgs := filterMsgs(mf.ASBIN, tags, ti[msgTypeASBIN].byTag)
	if len(asbinMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = asbinMsgs
	}
	sdbpMsgs := filterMsgs(mf.SDBP, tags, ti[msgTypeSDBP].byTag)
	if len(sdbpMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = sdbpMsgs
	}
	ubxMsgs := filterMsgs(mf.UBX, tags, ti[msgTypeUBX].byTag)
	if len(ubxMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = ubxMsgs
	}
	if rslt == nil {
		return nil, noMessagesError(tags)
	}
	return rslt, nil
}

func noMessagesError(tags []string) error {
	if len(tags) == 1 {
		if tags[0] == "" {
			return errors.New("no messages without a tag; use -t to specify a tag")
		}
		return fmt.Errorf("no messages found for tag %q", tags[0])
	}
	return fmt.Errorf("no messages found for tags: %s", strings.Join(tags, ", "))
}

func (mf *Parsed) buildTagIndices() (tagIndices, error) {
	if err := mf.validateDefaults(); err != nil {
		return tagIndices{}, err
	}
	tagTypes := make(map[string]msgType)
	tis := make(tagIndices, 6)

	if err := prepareTagIndex(mf.Line, mf.applyLineDefaults, msgTypeLine, tagTypes, tis); err != nil {
		return tagIndices{}, err
	}
	if err := prepareTagIndex(mf.Binary, mf.applyBinaryDefaults, msgTypeBinary, tagTypes, tis); err != nil {
		return tagIndices{}, err
	}
	if err := prepareTagIndex(mf.NMEA, mf.applyNMEADefaults, msgTypeNMEA, tagTypes, tis); err != nil {
		return tagIndices{}, err
	}
	if err := prepareTagIndex(mf.CASBIN, mf.applyCASBINDefaults, msgTypeCASBIN, tagTypes, tis); err != nil {
		return tagIndices{}, err
	}
	if err := prepareTagIndex(mf.ASBIN, mf.applyASBINDefaults, msgTypeASBIN, tagTypes, tis); err != nil {
		return tagIndices{}, err
	}
	if err := prepareTagIndex(mf.SDBP, mf.applySDBPDefaults, msgTypeSDBP, tagTypes, tis); err != nil {
		return tagIndices{}, err
	}
	if err := prepareTagIndex(mf.UBX, mf.applyUBXDefaults, msgTypeUBX, tagTypes, tis); err != nil {
		return tagIndices{}, err
	}

	return tis, nil
}

func prepareTagIndex[T any, PT interface {
	*T
	userMsg
}](msgs []T, apply func(PT), mt msgType, tagTypes map[string]msgType, tis tagIndices) error {
	applyDefaults(msgs, apply)
	ti, err := buildTagIndex[T, PT](msgs)
	if err != nil {
		return err
	}
	for _, tag := range ti.tags {
		if prevType, ok := tagTypes[tag]; ok && prevType != mt {
			return fmt.Errorf("tag %q used for multiple message types: %s and %s", tag, prevType, mt)
		}
		tagTypes[tag] = mt
	}
	tis[mt] = ti
	return nil
}

func buildTagIndex[T any, PT interface {
	*T
	userMsg
}](msgs []T) (tagIndex, error) {
	ti := tagIndex{byTag: make(map[string][]int)}
	var prevTag string
	hasPrev := false
	for i := range msgs {
		tag := PT(&msgs[i]).getTag()
		// If a seen tag reappears after we moved to another tag block, it is non-consecutive.
		if hasPrev && tag != prevTag && len(ti.byTag[tag]) > 0 {
			return tagIndex{}, fmt.Errorf("messages with tag %q must be consecutive", tag)
		}
		if !hasPrev || tag != prevTag {
			prevTag = tag
			hasPrev = true
		}
		if _, ok := ti.byTag[tag]; !ok {
			ti.tags = append(ti.tags, tag)
		}
		ti.byTag[tag] = append(ti.byTag[tag], i)
	}
	return ti, nil
}

func applyDefaults[T any, PT *T](msgs []T, apply func(PT)) {
	for i := range msgs {
		apply(&msgs[i])
	}
}

func filterMsgs[T any](msgs []T, tags []string, byTag map[string][]int) []T {
	var result []T
	for _, tag := range tags {
		for _, i := range byTag[tag] {
			result = append(result, msgs[i])
		}
	}
	return result
}

// toRawMsgs converts a slice of messages to RawMsg.
func toRawMsgs[T any, PT interface {
	*T
	userMsg
}](msgs []T) ([]RawMsg, error) {
	result := make([]RawMsg, len(msgs))
	tagCount := make(map[string]int)
	for i := range msgs {
		p := PT(&msgs[i])
		tag := p.getTag()
		idx := tagCount[tag]
		tagCount[tag]++
		rm, err := p.toRaw()
		if err != nil {
			return nil, fmt.Errorf("message %d with tag %q: %w", idx+1, tag, err)
		}
		rm.Index = idx
		rm.Tag = tag
		rm.source = p
		result[i] = rm
	}
	// Second pass to set Count.
	for i := range result {
		result[i].Count = tagCount[result[i].Tag]
	}
	return result, nil
}

// ToRaw converts a typed message slice to raw messages.
// The msgs parameter must be a typed slice as returned by Parsed.TaggedMsgs.
func ToRaw(msgs any) ([]RawMsg, error) {
	switch m := msgs.(type) {
	case []LineMsg:
		return toRawMsgs(m)
	case []BinaryMsg:
		return toRawMsgs(m)
	case []NMEAMsg:
		return toRawMsgs(m)
	case []CASBINMsg:
		return toRawMsgs(m)
	case []ASBINMsg:
		return toRawMsgs(m)
	case []SDBPMsg:
		return toRawMsgs(m)
	case []UBXMsg:
		return toRawMsgs(m)
	default:
		panic(fmt.Sprintf("unexpected message type: %T", msgs))
	}
}

type tagDescBuilder struct {
	descs        []TagDesc
	inconsistent []TagDesc
	index        map[string]int
}

func collectDescs[T any, PT interface {
	*T
	tagDescGetter
}](msgs []T, b *tagDescBuilder) {
	for i := range msgs {
		td := PT(&msgs[i]).tagDesc()
		if j, ok := b.index[td.Tag]; ok {
			b.descs[j].MsgCount++
			if td.Desc != "" {
				if b.descs[j].Desc == "" {
					b.descs[j].Desc = td.Desc
				} else if b.descs[j].Desc != td.Desc {
					b.inconsistent = append(b.inconsistent, td)
				}
			}
		} else {
			b.index[td.Tag] = len(b.descs)
			b.descs = append(b.descs, TagDesc{Tag: td.Tag, Desc: td.Desc, MsgCount: 1})
		}
	}
}

// TagDescs returns tag/description pairs in order of first occurrence.
// Inconsistent contains any tags where messages have conflicting descriptions.
func (mf *Parsed) TagDescs() (descs, inconsistent []TagDesc) {
	b := &tagDescBuilder{index: make(map[string]int)}
	collectDescs(mf.Line, b)
	collectDescs(mf.Binary, b)
	collectDescs(mf.NMEA, b)
	collectDescs(mf.CASBIN, b)
	collectDescs(mf.ASBIN, b)
	collectDescs(mf.SDBP, b)
	collectDescs(mf.UBX, b)
	// Move empty tag to front if present
	for i, td := range b.descs {
		if td.Tag == "" && i > 0 {
			b.descs = append([]TagDesc{td}, append(b.descs[:i], b.descs[i+1:]...)...)
			break
		}
	}
	return b.descs, b.inconsistent
}

// PrintTagDescs writes tag descriptions to w.
func PrintTagDescs(w io.Writer, tds []TagDesc) {
	if len(tds) == 0 {
		return
	}
	// Handle empty tag if present (guaranteed to be first if exists)
	if tds[0].Tag == "" {
		if tds[0].Desc != "" {
			fmt.Fprintf(w, "No tag: %s\n", tds[0].Desc)
		}
		tds = tds[1:]
	}
	if len(tds) == 0 {
		return
	}
	// Print named tags
	fmt.Fprintln(w, "Tags:")
	for _, td := range tds {
		if td.Desc != "" {
			fmt.Fprintf(w, "  %s - %s", td.Tag, td.Desc)
		} else {
			fmt.Fprintf(w, "  %s", td.Tag)
		}
		if td.MsgCount > 1 {
			fmt.Fprintf(w, " [%d messages]", td.MsgCount)
		}
		fmt.Fprintln(w)
	}
}
