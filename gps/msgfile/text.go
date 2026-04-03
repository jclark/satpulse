package msgfile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/lib/nmeamsg"
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

// NMEAMsg represents a [[nmea]] entry or [default.nmea].
type NMEAMsg struct {
	Text  string                      `toml:"text"`
	flags nmeamsg.SentenceSyntaxFlags // set by toRaw
	built string                      // full sentence from buildNMEA, set by toRaw
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
	wl, err := nm.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	built, flags, err := buildNMEA(nm.Text)
	if err != nil {
		return RawMsg{}, err
	}
	nm.built = built
	nm.flags = flags
	return RawMsg{
		Bytes:     []byte(built),
		Delay:     delay,
		WaitLimit: wl,
		Tag:       *nm.Tag,
	}, nil
}

func (nm *NMEAMsg) getTag() string { return *nm.Tag }

// buildNMEA builds a complete NMEA sentence from user text.
// Prepends $ if missing, appends *XX checksum if missing, appends CRLF.
// Validates the result using nmeamsg.CheckSyntax.
func buildNMEA(text string) (string, nmeamsg.SentenceSyntaxFlags, error) {
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
		return "", 0, errors.New("invalid NMEA packet")
	}
	return text, flags, nil
}

