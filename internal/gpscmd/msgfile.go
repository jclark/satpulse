package gpscmd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
	"github.com/pelletier/go-toml/v2"
)

// MsgCommon contains fields shared by all message types.
type MsgCommon struct {
	Delay *float64 `toml:"delay"`
	Tag   *string  `toml:"tag"`
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

// MsgFile represents a parsed message file.
type MsgFile struct {
	Default struct {
		Line   LineMsg   `toml:"line"`
		Binary BinaryMsg `toml:"binary"`
	} `toml:"default"`
	Line   []LineMsg   `toml:"line"`
	Binary []BinaryMsg `toml:"binary"`
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

func (mf *MsgFile) validateDefaults() error {
	if mf.Default.Line.Text != "" {
		return errors.New("default.line.text must be empty")
	}
	if _, err := mf.Default.Line.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.line: %w", err)
	}
	if mf.Default.Binary.Hex != "" {
		return errors.New("default.binary.hex must be empty")
	}
	if _, err := mf.Default.Binary.MsgCommon.delay(); err != nil {
		return fmt.Errorf("default.binary: %w", err)
	}
	return nil
}

// TaggedMsgs returns messages for the given tags with defaults applied.
// Returns ([]LineMsg, nil) or ([]BinaryMsg, nil) depending on message type.
// Returns error if tags select messages of mixed types.
func (mf *MsgFile) TaggedMsgs(tags []string) (any, error) {
	if err := mf.validateDefaults(); err != nil {
		return nil, err
	}
	applyDefaults(mf.Line, mf.applyLineDefaults)
	applyDefaults(mf.Binary, mf.applyBinaryDefaults)
	lineMsgs := filterMsgs(mf.Line, tags)
	binaryMsgs := filterMsgs(mf.Binary, tags)
	if len(lineMsgs) > 0 && len(binaryMsgs) > 0 {
		return nil, fmt.Errorf("selected tags have mixed message types (line and binary)")
	}
	if len(binaryMsgs) > 0 {
		return binaryMsgs, nil
	}
	return lineMsgs, nil
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
