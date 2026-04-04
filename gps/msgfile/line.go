package msgfile

import (
	"errors"
	"strings"
)

// LineMsg represents a [[line]] entry or [default.line].
type LineMsg struct {
	Text        string           `toml:"text"`
	EOL         *string          `toml:"eol"`
	RespPattern *ResponsePattern `toml:"responsePattern"`
	MsgCommon
}

func (lm *LineMsg) toRaw() (RawMsg, error) {
	if lm.Text == "" {
		return RawMsg{}, errors.New("text must not be empty")
	}
	delay, err := lm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := lm.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{
		Bytes:     []byte(lm.Text + *lm.EOL),
		Delay:     delay,
		WaitLimit: wl,
		Tag:       *lm.Tag,
	}, nil
}

func (lm *LineMsg) getTag() string { return *lm.Tag }

// analyzeRequest implements requestAnalyzer for LineMsg.
func (lm *LineMsg) analyzeRequest(data string) requestAnalysis {
	if lm.RespPattern != nil && *lm.RespPattern == ResponsePatternUnicore {
		return lm.analyzeRequestUnicore()
	}
	eol := "\r\n"
	if lm.EOL != nil {
		eol = *lm.EOL
	}
	return requestAnalysis{
		expectAck:  ExpectAckNone,
		expectData: expectDataUnknown,
		dataMatch:  lineDataMatch(eol),
	}
}

// lineDataMatch returns a data match function for generic line messages.
// It accepts printable ASCII data ending with the expected line terminator.
func lineDataMatch(eol string) func(string) bool {
	return func(data string) bool {
		if !strings.HasSuffix(data, eol) {
			return false
		}
		body := data[:len(data)-len(eol)]
		for i := 0; i < len(body); i++ {
			if body[i] < 0x20 || body[i] > 0x7E {
				return false
			}
		}
		return len(body) > 0
	}
}
