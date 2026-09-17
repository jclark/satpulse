package msgfile

import (
	"errors"

	"github.com/jclark/satpulse/gps/lib/ascii"
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
	if lm.RespPattern != nil {
		switch *lm.RespPattern {
		case ResponsePatternUnicore:
			return lm.analyzeRequestUnicore()
		case ResponsePatternSeptentrio:
			return lm.analyzeRequestSeptentrio()
		}
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

// extractEOL returns the trailing line ending from s.
func extractEOL(s string) string {
	i := len(s)
	if i > 0 && s[i-1] == '\n' {
		i--
	}
	if i > 0 && s[i-1] == '\r' {
		i--
	}
	return s[i:]
}

// lineDataMatch returns a data match function for generic line messages.
// It accepts printable ASCII data whose line ending matches eol.
func lineDataMatch(eol string) func(string) bool {
	return func(data string) bool {
		if extractEOL(data) != eol {
			return false
		}
		body := data[:len(data)-len(eol)]
		for i := range len(body) {
			if !ascii.IsPrint(body[i]) {
				return false
			}
		}
		return len(body) > 0
	}
}
