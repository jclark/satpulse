package gpscmd

import (
	"encoding/hex"
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

func (lm *LineMsg) toRaw() rawMsg {
	return rawMsg{
		bytes: []byte(lm.Text + *lm.EOL),
		delay: ptime.Seconds(*lm.Delay),
		tag:   *lm.Tag,
	}
}

func (lm *LineMsg) getTag() string { return *lm.Tag }

// BinaryMsg represents a [[binary]] entry or [default.binary].
type BinaryMsg struct {
	Hex string `toml:"hex"`
	MsgCommon
}

func (bm *BinaryMsg) toRaw() rawMsg {
	b, _ := decodeHex(bm.Hex)
	return rawMsg{
		bytes: b,
		delay: ptime.Seconds(*bm.Delay),
		tag:   *bm.Tag,
	}
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
	toRaw() rawMsg
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

// Validate checks that the message file is valid.
func (mf *MsgFile) Validate() error {
	if mf.Default.Line.Text != "" {
		return fmt.Errorf("default.line.text must be empty")
	}
	for i, lm := range mf.Line {
		if lm.Text == "" {
			return fmt.Errorf("line[%d].text must not be empty", i)
		}
		if lm.Delay != nil && *lm.Delay < 0 {
			return fmt.Errorf("line[%d].delay must not be negative", i)
		}
	}
	if mf.Default.Line.Delay != nil && *mf.Default.Line.Delay < 0 {
		return fmt.Errorf("default.line.delay must not be negative")
	}
	if mf.Default.Binary.Hex != "" {
		return fmt.Errorf("default.binary.hex must be empty")
	}
	for i, bm := range mf.Binary {
		if bm.Hex == "" {
			return fmt.Errorf("binary[%d].hex must not be empty", i)
		}
		if _, err := decodeHex(bm.Hex); err != nil {
			return fmt.Errorf("binary[%d].hex: %w", i, err)
		}
		if bm.Delay != nil && *bm.Delay < 0 {
			return fmt.Errorf("binary[%d].delay must not be negative", i)
		}
	}
	if mf.Default.Binary.Delay != nil && *mf.Default.Binary.Delay < 0 {
		return fmt.Errorf("default.binary.delay must not be negative")
	}
	return nil
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

// TaggedMsgs returns messages for the given tags with defaults applied.
// Returns ([]LineMsg, nil) or ([]BinaryMsg, nil) depending on message type.
// Returns error if tags select messages of mixed types.
func (mf *MsgFile) TaggedMsgs(tags []string) (any, error) {
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
}](msgs []T) []rawMsg {
	result := make([]rawMsg, len(msgs))
	for i := range msgs {
		p := PT(&msgs[i])
		rm := p.toRaw()
		rm.index = i
		result[i] = rm
	}
	return result
}
