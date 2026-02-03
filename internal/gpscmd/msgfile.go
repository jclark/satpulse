package gpscmd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/asbin"
	"github.com/jclark/satpulse/internal/casbin"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/ubxbin"
	"github.com/pelletier/go-toml/v2"
)

// MsgCommon contains fields shared by all message types.
type MsgCommon struct {
	Delay       *float64 `toml:"delay"`
	Tag         *string  `toml:"tag"`
	Description string   `toml:"description"`
}

type tagDesc struct {
	tag  string
	desc string
}

type tagDescGetter interface {
	tagDesc() tagDesc
}

func (mc *MsgCommon) tagDesc() tagDesc {
	tag := ""
	if mc.Tag != nil {
		tag = *mc.Tag
	}
	return tagDesc{tag: tag, desc: mc.Description}
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

func (lm *LineMsg) toRaw() (rawMsg, error) {
	if lm.Text == "" {
		return rawMsg{}, errors.New("text must not be empty")
	}
	delay, err := lm.MsgCommon.delay()
	if err != nil {
		return rawMsg{}, err
	}
	return rawMsg{
		bytes: []byte(lm.Text + *lm.EOL),
		delay: delay,
		tag:   *lm.Tag,
	}, nil
}

func (lm *LineMsg) getTag() string { return *lm.Tag }

// BinaryMsg represents a [[binary]] entry or [default.binary].
type BinaryMsg struct {
	Hex string `toml:"hex"`
	MsgCommon
}

func (bm *BinaryMsg) toRaw() (rawMsg, error) {
	if bm.Hex == "" {
		return rawMsg{}, errors.New("hex must not be empty")
	}
	b, err := decodeHex(bm.Hex)
	if err != nil {
		return rawMsg{}, fmt.Errorf("hex: %w", err)
	}
	delay, err := bm.MsgCommon.delay()
	if err != nil {
		return rawMsg{}, err
	}
	return rawMsg{
		bytes: b,
		delay: delay,
		tag:   *bm.Tag,
	}, nil
}

func (bm *BinaryMsg) getTag() string { return *bm.Tag }

// NMEAMsg represents a [[nmea]] entry or [default.nmea].
type NMEAMsg struct {
	Text string `toml:"text"`
	MsgCommon
}

func (nm *NMEAMsg) toRaw() (rawMsg, error) {
	if nm.Text == "" {
		return rawMsg{}, errors.New("text must not be empty")
	}
	delay, err := nm.MsgCommon.delay()
	if err != nil {
		return rawMsg{}, err
	}
	built, err := buildNMEA(nm.Text)
	if err != nil {
		return rawMsg{}, err
	}
	return rawMsg{
		bytes: []byte(built),
		delay: delay,
		tag:   *nm.Tag,
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

func (cm *CASBINMsg) toRaw() (rawMsg, error) {
	payload, err := cm.Payload.Encode(casbin.Endian)
	if err != nil {
		return rawMsg{}, err
	}
	mid := casbin.MakeMsgID(cm.Class, cm.ID)
	pkt, err := casbin.PackMsg(mid, payload)
	if err != nil {
		return rawMsg{}, err
	}
	delay, err := cm.MsgCommon.delay()
	if err != nil {
		return rawMsg{}, err
	}
	return rawMsg{bytes: pkt, delay: delay, tag: *cm.Tag}, nil
}

func (cm *CASBINMsg) getTag() string { return *cm.Tag }

// ASBINMsg represents a [[asbin]] entry.
type ASBINMsg struct {
	UBXLikeMsg
}

func (am *ASBINMsg) toRaw() (rawMsg, error) {
	payload, err := am.Payload.Encode(asbin.Endian())
	if err != nil {
		return rawMsg{}, err
	}
	mid := asbin.MakeMsgID(am.Class, am.ID)
	pkt, err := asbin.PackMsg(mid, payload)
	if err != nil {
		return rawMsg{}, err
	}
	delay, err := am.MsgCommon.delay()
	if err != nil {
		return rawMsg{}, err
	}
	return rawMsg{bytes: pkt, delay: delay, tag: *am.Tag}, nil
}

func (am *ASBINMsg) getTag() string { return *am.Tag }

// UBXMsg represents a [[ubx]] entry.
type UBXMsg struct {
	UBXLikeMsg
}

func (um *UBXMsg) toRaw() (rawMsg, error) {
	payload, err := um.Payload.Encode(ubxbin.Endian)
	if err != nil {
		return rawMsg{}, err
	}
	mid := ubxbin.MakeMsgID(um.Class, um.ID)
	pkt, err := ubxbin.PackMsg(mid, payload)
	if err != nil {
		return rawMsg{}, err
	}
	delay, err := um.MsgCommon.delay()
	if err != nil {
		return rawMsg{}, err
	}
	return rawMsg{bytes: pkt, delay: delay, tag: *um.Tag}, nil
}

func (um *UBXMsg) getTag() string { return *um.Tag }

// buildNMEA builds a complete NMEA sentence from user text.
// Prepends $ if missing, appends *XX checksum if missing, appends CRLF.
// Validates the result using nmea.CheckSyntax.
func buildNMEA(text string) (string, error) {
	if !strings.HasPrefix(text, "$") {
		text = "$" + text
	}
	if !strings.Contains(text, "*") {
		checksum := nmea.Checksum([]byte(text[1:]))
		text = fmt.Sprintf("%s*%02X", text, checksum)
	}
	text += "\r\n"
	flags := nmea.CheckSyntax(text)
	if flags&nmea.SentenceIsPacket == 0 {
		return "", errors.New("invalid NMEA packet")
	}
	return text, nil
}

// MsgFile represents a parsed message file.
type MsgFile struct {
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

// rawMsg is an internal type for sending messages.
type rawMsg struct {
	bytes []byte
	delay time.Duration
	tag   string // for logging
	index int    // 0-based index within tag
}

// UserMsg is the interface for message types that can be converted to rawMsg.
type UserMsg interface {
	toRaw() (rawMsg, error)
	getTag() string
}

func ptr[T any](v T) *T { return &v }

func defaultMsgCommon() MsgCommon {
	return MsgCommon{Delay: ptr(0.0), Tag: ptr("")}
}

func defaultMsgFile() *MsgFile {
	mf := new(MsgFile)
	mf.Default.Line.EOL = ptr("\r\n")
	mf.Default.Line.MsgCommon = defaultMsgCommon()
	mf.Default.Binary.MsgCommon = defaultMsgCommon()
	mf.Default.NMEA.MsgCommon = defaultMsgCommon()
	mf.Default.CASBIN.MsgCommon = defaultMsgCommon()
	mf.Default.ASBIN.MsgCommon = defaultMsgCommon()
	mf.Default.UBX.MsgCommon = defaultMsgCommon()
	return mf
}

// LoadMsgFile reads and parses a TOML message file.
func LoadMsgFile(path string) (*MsgFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	mf := defaultMsgFile()
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

func (mf *MsgFile) applyLineDefaults(lm *LineMsg) {
	if lm.EOL == nil {
		lm.EOL = mf.Default.Line.EOL
	}
	applyCommonDefaults(&lm.MsgCommon, &mf.Default.Line.MsgCommon)
}

func (mf *MsgFile) applyBinaryDefaults(bm *BinaryMsg) {
	applyCommonDefaults(&bm.MsgCommon, &mf.Default.Binary.MsgCommon)
}

func (mf *MsgFile) applyNMEADefaults(nm *NMEAMsg) {
	applyCommonDefaults(&nm.MsgCommon, &mf.Default.NMEA.MsgCommon)
}

func (mf *MsgFile) applyCASBINDefaults(cm *CASBINMsg) {
	applyCommonDefaults(&cm.MsgCommon, &mf.Default.CASBIN.MsgCommon)
}

func (mf *MsgFile) applyASBINDefaults(am *ASBINMsg) {
	applyCommonDefaults(&am.MsgCommon, &mf.Default.ASBIN.MsgCommon)
}

func (mf *MsgFile) applyUBXDefaults(um *UBXMsg) {
	applyCommonDefaults(&um.MsgCommon, &mf.Default.UBX.MsgCommon)
}

func (mf *MsgFile) validateDefaults() error {
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
func (mf *MsgFile) TaggedMsgs(tags []string) (any, error) {
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
	UserMsg
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

// toRawMsgs converts a slice of messages to rawMsg.
func toRawMsgs[T any, PT interface {
	*T
	UserMsg
}](msgs []T) ([]rawMsg, error) {
	result := make([]rawMsg, len(msgs))
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
		rm.index = idx
		rm.tag = tag
		result[i] = rm
	}
	return result, nil
}

type tagDescBuilder struct {
	descs        []tagDesc
	inconsistent []tagDesc
	index        map[string]int
}

func collectDescs[T any, PT interface {
	*T
	tagDescGetter
}](msgs []T, b *tagDescBuilder) {
	for i := range msgs {
		td := PT(&msgs[i]).tagDesc()
		if j, ok := b.index[td.tag]; ok {
			if td.desc != "" {
				if b.descs[j].desc == "" {
					b.descs[j].desc = td.desc
				} else if b.descs[j].desc != td.desc {
					b.inconsistent = append(b.inconsistent, td)
				}
			}
		} else {
			b.index[td.tag] = len(b.descs)
			b.descs = append(b.descs, td)
		}
	}
}

func (mf *MsgFile) collectTagDescs() (descs, inconsistent []tagDesc) {
	b := &tagDescBuilder{index: make(map[string]int)}
	collectDescs(mf.Line, b)
	collectDescs(mf.Binary, b)
	collectDescs(mf.NMEA, b)
	collectDescs(mf.CASBIN, b)
	collectDescs(mf.ASBIN, b)
	collectDescs(mf.UBX, b)
	// Move empty tag to front if present
	for i, td := range b.descs {
		if td.tag == "" && i > 0 {
			b.descs = append([]tagDesc{td}, append(b.descs[:i], b.descs[i+1:]...)...)
			break
		}
	}
	return b.descs, b.inconsistent
}

// CheckTagDescriptions returns tag/description pairs in order of first occurrence.
// Logs warnings for any tags with inconsistent descriptions.
func (mf *MsgFile) CheckTagDescriptions(lg *slog.Logger) []tagDesc {
	descs, inconsistent := mf.collectTagDescs()
	for _, td := range inconsistent {
		lg.Warn("tag has inconsistent descriptions", "tag", td.tag, "desc", td.desc)
	}
	return descs
}

func printTagDescs(w io.Writer, tds []tagDesc) {
	if len(tds) == 0 {
		return
	}
	// Handle empty tag if present (guaranteed to be first if exists)
	if tds[0].tag == "" {
		if tds[0].desc != "" {
			fmt.Fprintf(w, "No tag: %s\n", tds[0].desc)
		}
		tds = tds[1:]
	}
	if len(tds) == 0 {
		return
	}
	// Print named tags
	fmt.Fprintln(w, "Tags:")
	for _, td := range tds {
		if td.desc != "" {
			fmt.Fprintf(w, "  %s - %s\n", td.tag, td.desc)
		} else {
			fmt.Fprintf(w, "  %s\n", td.tag)
		}
	}
}
