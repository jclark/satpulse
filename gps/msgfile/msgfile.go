package msgfile

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
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

// LineMsg represents a [[line]] entry or [default.line].
type LineMsg struct {
	Text string  `toml:"text"`
	EOL  *string `toml:"eol"`
	MsgCommon
}

func (mc *MsgCommon) delay() (time.Duration, error) {
	if *mc.Delay < 0 {
		return 0, errors.New("delay must not be negative")
	}
	return ptime.Seconds(*mc.Delay), nil
}

func (lm *LineMsg) toRaw() (RawMsg, error) {
	if lm.Text == "" {
		return RawMsg{}, errors.New("text must not be empty")
	}
	delay, err := lm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{
		Bytes: []byte(lm.Text + *lm.EOL),
		Delay: delay,
		Tag:   *lm.Tag,
	}, nil
}

func (lm *LineMsg) getTag() string { return *lm.Tag }

// BinaryMsg represents a [[binary]] entry or [default.binary].
type BinaryMsg struct {
	Hex string `toml:"hex"`
	MsgCommon
}

func (bm *BinaryMsg) toRaw() (RawMsg, error) {
	if bm.Hex == "" {
		return RawMsg{}, errors.New("hex must not be empty")
	}
	b, err := decodeHex(bm.Hex)
	if err != nil {
		return RawMsg{}, fmt.Errorf("hex: %w", err)
	}
	delay, err := bm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{
		Bytes: b,
		Delay: delay,
		Tag:   *bm.Tag,
	}, nil
}

func (bm *BinaryMsg) getTag() string { return *bm.Tag }

// NMEAMsg represents a [[nmea]] entry or [default.nmea].
type NMEAMsg struct {
	Text string `toml:"text"`
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
	built, err := buildNMEA(nm.Text)
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{
		Bytes: []byte(built),
		Delay: delay,
		Tag:   *nm.Tag,
	}, nil
}

func (nm *NMEAMsg) getTag() string { return *nm.Tag }

// UBXLikeMsg contains fields shared by UBX and CASBIN message types.
type UBXLikeMsg struct {
	Class   uint8   `toml:"class"`
	ID      uint8   `toml:"id"`
	Payload Payload `toml:"payload"`
	MsgCommon
}

// CASBINMsg represents a [[casbin]] entry.
type CASBINMsg struct {
	UBXLikeMsg
}

func (cm *CASBINMsg) toRaw() (RawMsg, error) {
	payload, err := cm.Payload.Encode(casbin.Endian)
	if err != nil {
		return RawMsg{}, err
	}
	mid := casbin.MakeMsgID(cm.Class, cm.ID)
	pkt, err := casbin.PackMsg(mid, payload)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := cm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, Tag: *cm.Tag}, nil
}

func (cm *CASBINMsg) getTag() string { return *cm.Tag }

// ASBINMsg represents a [[asbin]] entry.
type ASBINMsg struct {
	UBXLikeMsg
}

func (am *ASBINMsg) toRaw() (RawMsg, error) {
	payload, err := am.Payload.Encode(asbin.Endian())
	if err != nil {
		return RawMsg{}, err
	}
	mid := asbin.MakeMsgID(am.Class, am.ID)
	pkt, err := asbin.PackMsg(mid, payload)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := am.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, Tag: *am.Tag}, nil
}

func (am *ASBINMsg) getTag() string { return *am.Tag }

// UBXMsg represents a [[ubx]] entry.
type UBXMsg struct {
	UBXLikeMsg
}

func (um *UBXMsg) toRaw() (RawMsg, error) {
	payload, err := um.Payload.Encode(ubxbin.Endian)
	if err != nil {
		return RawMsg{}, err
	}
	mid := ubxbin.MakeMsgID(um.Class, um.ID)
	pkt, err := ubxbin.PackMsg(mid, payload)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := um.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, Tag: *um.Tag}, nil
}

func (um *UBXMsg) getTag() string { return *um.Tag }

// buildNMEA builds a complete NMEA sentence from user text.
// Prepends $ if missing, appends *XX checksum if missing, appends CRLF.
// Validates the result using nmeamsg.CheckSyntax.
func buildNMEA(text string) (string, error) {
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
		return "", errors.New("invalid NMEA packet")
	}
	return text, nil
}

// Parsed represents a parsed message file.
type Parsed struct {
	Default struct {
		Line   LineMsg   `toml:"line"`
		Binary BinaryMsg `toml:"binary"`
		NMEA   NMEAMsg   `toml:"nmea"`
		CASBIN CASBINMsg `toml:"casbin"`
		ASBIN  ASBINMsg  `toml:"asbin"`
		UBX    UBXMsg    `toml:"ubx"`
	} `toml:"default"`
	Line   []LineMsg   `toml:"line"`
	Binary []BinaryMsg `toml:"binary"`
	NMEA   []NMEAMsg   `toml:"nmea"`
	CASBIN []CASBINMsg `toml:"casbin"`
	ASBIN  []ASBINMsg  `toml:"asbin"`
	UBX    []UBXMsg    `toml:"ubx"`
}

// RawMsg is a message ready to send: raw bytes with metadata.
type RawMsg struct {
	Bytes []byte
	Delay time.Duration
	Tag   string // for logging
	Index int    // 0-based index within tag
}

type userMsg interface {
	toRaw() (RawMsg, error)
	getTag() string
}

func ptr[T any](v T) *T { return &v }

func defaultMsgCommon() MsgCommon {
	return MsgCommon{Delay: ptr(0.0), Tag: ptr("")}
}

func newDefault() *Parsed {
	mf := new(Parsed)
	mf.Default.Line.EOL = ptr("\r\n")
	mf.Default.Line.MsgCommon = defaultMsgCommon()
	mf.Default.Binary.MsgCommon = defaultMsgCommon()
	mf.Default.NMEA.MsgCommon = defaultMsgCommon()
	mf.Default.CASBIN.MsgCommon = defaultMsgCommon()
	mf.Default.ASBIN.MsgCommon = defaultMsgCommon()
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

// TaggedMsgs returns messages for the given tags with defaults applied.
// Returns a typed slice ([]LineMsg, []BinaryMsg, []NMEAMsg, []CASBINMsg, []ASBINMsg, or []UBXMsg).
// Returns error if tags select messages of mixed types or if there are no messages with the tags.
func (mf *Parsed) TaggedMsgs(tags []string) (any, error) {
	if err := mf.validateDefaults(); err != nil {
		return nil, err
	}
	applyDefaults(mf.Line, mf.applyLineDefaults)
	applyDefaults(mf.Binary, mf.applyBinaryDefaults)
	applyDefaults(mf.NMEA, mf.applyNMEADefaults)
	applyDefaults(mf.CASBIN, mf.applyCASBINDefaults)
	applyDefaults(mf.ASBIN, mf.applyASBINDefaults)
	applyDefaults(mf.UBX, mf.applyUBXDefaults)
	var rslt any
	lineMsgs := filterMsgs(mf.Line, tags)
	if len(lineMsgs) > 0 {
		rslt = lineMsgs
	}
	binaryMsgs := filterMsgs(mf.Binary, tags)
	if len(binaryMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = binaryMsgs
	}
	nmeaMsgs := filterMsgs(mf.NMEA, tags)
	if len(nmeaMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = nmeaMsgs
	}
	casbinMsgs := filterMsgs(mf.CASBIN, tags)
	if len(casbinMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = casbinMsgs
	}
	asbinMsgs := filterMsgs(mf.ASBIN, tags)
	if len(asbinMsgs) > 0 {
		if rslt != nil {
			return nil, fmt.Errorf("selected tags have mixed message types")
		}
		rslt = asbinMsgs
	}
	ubxMsgs := filterMsgs(mf.UBX, tags)
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

func applyDefaults[T any, PT *T](msgs []T, apply func(PT)) {
	for i := range msgs {
		apply(&msgs[i])
	}
}

func filterMsgs[T any, PT interface {
	*T
	userMsg
}](msgs []T, tags []string) []T {
	tagIndex := make(map[string][]int)
	for i := range msgs {
		tag := PT(&msgs[i]).getTag()
		tagIndex[tag] = append(tagIndex[tag], i)
	}
	var result []T
	for _, tag := range tags {
		for _, i := range tagIndex[tag] {
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
		result[i] = rm
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
