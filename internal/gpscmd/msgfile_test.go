package gpscmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	casbin "github.com/jclark/satpulse/internal/casbin"
	"github.com/pelletier/go-toml/v2"
)

func TestLoadMsgFileSimple(t *testing.T) {
	toml := `[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
`
	mf := loadMsgFileFromString(t, toml)
	if len(mf.Line) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(mf.Line))
	}
	if mf.Line[0].Text != "CONFIG PPP CONVERGE 10 20" {
		t.Errorf("expected first line text, got %q", mf.Line[0].Text)
	}
	if mf.Line[1].Text != "CONFIG PPP ENABLE E6-HAS" {
		t.Errorf("expected second line text, got %q", mf.Line[1].Text)
	}
}

func TestLoadMsgFileDefaults(t *testing.T) {
	toml := `[default.line]
delay = 0.1
eol = "\n"

[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
`
	mf := loadMsgFileFromString(t, toml)
	if mf.Default.Line.Delay == nil || *mf.Default.Line.Delay != 0.1 {
		t.Errorf("expected default delay 0.1, got %v", mf.Default.Line.Delay)
	}
	if mf.Default.Line.EOL == nil || *mf.Default.Line.EOL != "\n" {
		t.Errorf("expected default eol \\n, got %v", mf.Default.Line.EOL)
	}
}

func TestLoadMsgFileDefaultEOL(t *testing.T) {
	toml := `[[line]]
text = "TEST"
`
	mf := loadMsgFileFromString(t, toml)
	if mf.Default.Line.EOL == nil || *mf.Default.Line.EOL != "\r\n" {
		t.Errorf("expected default eol \\r\\n, got %v", mf.Default.Line.EOL)
	}
}

func TestValidateLineMsg(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr bool
	}{
		{
			name: "valid simple",
			toml: `[[line]]
text = "TEST"
`,
			wantErr: false,
		},
		{
			name: "empty text",
			toml: `[[line]]
text = ""
`,
			wantErr: true,
		},
		{
			name: "default text not empty",
			toml: `[default.line]
text = "BAD"

[[line]]
text = "TEST"
`,
			wantErr: true,
		},
		{
			name: "negative delay in line",
			toml: `[[line]]
text = "TEST"
delay = -1.0
`,
			wantErr: true,
		},
		{
			name: "negative delay in default",
			toml: `[default.line]
delay = -0.5

[[line]]
text = "TEST"
`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			err := validateMsgs(mf, []string{""})
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func validateMsgs(mf *MsgFile, tags []string) error {
	msgs, err := mf.TaggedMsgs(tags)
	if err != nil {
		return err
	}
	switch m := msgs.(type) {
	case []LineMsg:
		_, err = toRawMsgs(m)
	case []BinaryMsg:
		_, err = toRawMsgs(m)
	case []NMEAMsg:
		_, err = toRawMsgs(m)
	case []CASBINMsg:
		_, err = toRawMsgs(m)
	case []UBXMsg:
		_, err = toRawMsgs(m)
	}
	return err
}

func TestLineMsgsToRaw(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []rawMsg
	}{
		{
			name: "simple two lines",
			toml: `[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 0, tag: "", index: 0},
				{bytes: []byte("LINE2\r\n"), delay: 0, tag: "", index: 1},
			},
		},
		{
			name: "with delay",
			toml: `[[line]]
text = "LINE1"
delay = 0.1

[[line]]
text = "LINE2"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 100 * time.Millisecond, tag: "", index: 0},
				{bytes: []byte("LINE2\r\n"), delay: 0, tag: "", index: 1},
			},
		},
		{
			name: "custom eol",
			toml: `[[line]]
text = "LINE1"
eol = "\n"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\n"), delay: 0, tag: "", index: 0},
			},
		},
		{
			name: "empty eol",
			toml: `[[line]]
text = "NOTERM"
eol = ""
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("NOTERM"), delay: 0, tag: "", index: 0},
			},
		},
		{
			name: "default delay",
			toml: `[default.line]
delay = 0.2

[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 200 * time.Millisecond, tag: "", index: 0},
				{bytes: []byte("LINE2\r\n"), delay: 200 * time.Millisecond, tag: "", index: 1},
			},
		},
		{
			name: "default eol",
			toml: `[default.line]
eol = "\n"

[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\n"), delay: 0, tag: "", index: 0},
				{bytes: []byte("LINE2\n"), delay: 0, tag: "", index: 1},
			},
		},
		{
			name: "override default delay",
			toml: `[default.line]
delay = 0.2

[[line]]
text = "LINE1"
delay = 0.5

[[line]]
text = "LINE2"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 500 * time.Millisecond, tag: "", index: 0},
				{bytes: []byte("LINE2\r\n"), delay: 200 * time.Millisecond, tag: "", index: 1},
			},
		},
		{
			name: "override default eol",
			toml: `[default.line]
eol = "\n"

[[line]]
text = "LINE1"
eol = "\r"

[[line]]
text = "LINE2"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r"), delay: 0, tag: "", index: 0},
				{bytes: []byte("LINE2\n"), delay: 0, tag: "", index: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			lineMsgs, ok := msgs.([]LineMsg)
			if !ok {
				t.Fatalf("expected []LineMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(lineMsgs)
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if !reflect.DeepEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestLoadBinaryMsgSimple(t *testing.T) {
	toml := `[[binary]]
hex = "B562068A0900"

[[binary]]
hex = "DEADBEEF"
`
	mf := loadMsgFileFromString(t, toml)
	if len(mf.Binary) != 2 {
		t.Fatalf("expected 2 binary messages, got %d", len(mf.Binary))
	}
	if mf.Binary[0].Hex != "B562068A0900" {
		t.Errorf("expected first hex, got %q", mf.Binary[0].Hex)
	}
	if mf.Binary[1].Hex != "DEADBEEF" {
		t.Errorf("expected second hex, got %q", mf.Binary[1].Hex)
	}
}

func TestBinaryMsgsToRaw(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []rawMsg
	}{
		{
			name: "simple binary",
			toml: `[[binary]]
hex = "DEADBEEF"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, delay: 0, tag: "", index: 0},
			},
		},
		{
			name: "hex with whitespace",
			toml: `[[binary]]
hex = "DE AD BE EF"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, delay: 0, tag: "", index: 0},
			},
		},
		{
			name: "with delay",
			toml: `[[binary]]
hex = "B562"
delay = 0.5
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte{0xB5, 0x62}, delay: 500 * time.Millisecond, tag: "", index: 0},
			},
		},
		{
			name: "default delay",
			toml: `[default.binary]
delay = 0.1

[[binary]]
hex = "AA"

[[binary]]
hex = "BB"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte{0xAA}, delay: 100 * time.Millisecond, tag: "", index: 0},
				{bytes: []byte{0xBB}, delay: 100 * time.Millisecond, tag: "", index: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			binaryMsgs, ok := msgs.([]BinaryMsg)
			if !ok {
				t.Fatalf("expected []BinaryMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(binaryMsgs)
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if !reflect.DeepEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestValidateBinary(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		tags    []string
		wantErr bool
	}{
		{
			name: "valid binary",
			toml: `[[binary]]
hex = "DEADBEEF"
`,
			tags:    []string{""},
			wantErr: false,
		},
		{
			name: "empty hex",
			toml: `[[binary]]
hex = ""
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "invalid hex chars",
			toml: `[[binary]]
hex = "GHIJ"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "odd length hex",
			toml: `[[binary]]
hex = "ABC"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "default hex not empty",
			toml: `[default.binary]
hex = "AA"

[[binary]]
hex = "BB"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "negative delay in binary",
			toml: `[[binary]]
hex = "AA"
delay = -1.0
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "negative delay in default binary",
			toml: `[default.binary]
delay = -0.5

[[binary]]
hex = "AA"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "mixed line and binary in file",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[binary]]
hex = "AA"
tag = "bins"
`,
			tags:    []string{"lines"},
			wantErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			err := validateMsgs(mf, tc.tags)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUnknownField(t *testing.T) {
	toml := `[[line]]
text = "TEST"
unknown = "bad"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadMsgFile(path)
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestDescriptionAllowedAndIgnored(t *testing.T) {
	toml := `[[line]]
text = "LINE1"
tag = "setup"
description = "Configure the device"

[[line]]
text = "LINE2"
tag = "setup"

[[nmea]]
text = "PCAS04,3"
tag = "nmea-setup"
description = "Set NMEA mode"

[[binary]]
hex = "DEADBEEF"
tag = "bin-setup"
description = "Send binary command"
`
	mf := loadMsgFileFromString(t, toml)
	// Check description is parsed
	if mf.Line[0].Description != "Configure the device" {
		t.Errorf("expected line description, got %q", mf.Line[0].Description)
	}
	if mf.Line[1].Description != "" {
		t.Errorf("expected empty description, got %q", mf.Line[1].Description)
	}
	if mf.NMEA[0].Description != "Set NMEA mode" {
		t.Errorf("expected nmea description, got %q", mf.NMEA[0].Description)
	}
	if mf.Binary[0].Description != "Send binary command" {
		t.Errorf("expected binary description, got %q", mf.Binary[0].Description)
	}
	// Check that messages still work (description is ignored for sending)
	msgs, err := mf.TaggedMsgs([]string{"setup"})
	if err != nil {
		t.Fatalf("TaggedMsgs error: %v", err)
	}
	lineMsgs, ok := msgs.([]LineMsg)
	if !ok {
		t.Fatalf("expected []LineMsg, got %T", msgs)
	}
	raw, err := toRawMsgs(lineMsgs)
	if err != nil {
		t.Fatalf("toRawMsgs error: %v", err)
	}
	if len(raw) != 2 {
		t.Errorf("expected 2 raw messages, got %d", len(raw))
	}
}

func TestTaggedMsgs(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []rawMsg
	}{
		{
			name: "filter by tag",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[line]]
text = "LINE2"
tag = "ppp"

[[line]]
text = "LINE3"
tag = "setup"
`,
			tags: []string{"setup"},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 0, tag: "setup", index: 0},
				{bytes: []byte("LINE3\r\n"), delay: 0, tag: "setup", index: 1},
			},
		},
		{
			name: "multiple tags in order",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[line]]
text = "LINE2"
tag = "ppp"

[[line]]
text = "LINE3"
tag = "setup"
`,
			tags: []string{"ppp", "setup"},
			expected: []rawMsg{
				{bytes: []byte("LINE2\r\n"), delay: 0, tag: "ppp", index: 0},
				{bytes: []byte("LINE1\r\n"), delay: 0, tag: "setup", index: 0},
				{bytes: []byte("LINE3\r\n"), delay: 0, tag: "setup", index: 1},
			},
		},
		{
			name: "default tag",
			toml: `[default.line]
tag = "setup"

[[line]]
text = "LINE1"

[[line]]
text = "LINE2"

[[line]]
text = "LINE3"
tag = "other"
`,
			tags: []string{"setup"},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 0, tag: "setup", index: 0},
				{bytes: []byte("LINE2\r\n"), delay: 0, tag: "setup", index: 1},
			},
		},
		{
			name: "empty tag",
			toml: `[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
tag = "ppp"

[[line]]
text = "LINE3"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 0, tag: "", index: 0},
				{bytes: []byte("LINE3\r\n"), delay: 0, tag: "", index: 1},
			},
		},
		{
			name: "empty tag in middle",
			toml: `[[line]]
text = "LINE1"
tag = "foo"

[[line]]
text = "LINE2"

[[line]]
text = "LINE3"
tag = "bar"
`,
			tags: []string{"foo", "", "bar"},
			expected: []rawMsg{
				{bytes: []byte("LINE1\r\n"), delay: 0, tag: "foo", index: 0},
				{bytes: []byte("LINE2\r\n"), delay: 0, tag: "", index: 0},
				{bytes: []byte("LINE3\r\n"), delay: 0, tag: "bar", index: 0},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			lineMsgs, ok := msgs.([]LineMsg)
			if !ok {
				t.Fatalf("expected []LineMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(lineMsgs)
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if !reflect.DeepEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestTaggedBinaryMsgs(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []rawMsg
	}{
		{
			name: "filter by tag",
			toml: `[[binary]]
hex = "AA"
tag = "setup"

[[binary]]
hex = "BB"
tag = "other"
`,
			tags: []string{"setup"},
			expected: []rawMsg{
				{bytes: []byte{0xAA}, delay: 0, tag: "setup", index: 0},
			},
		},
		{
			name: "default tag",
			toml: `[default.binary]
tag = "init"

[[binary]]
hex = "AA"

[[binary]]
hex = "BB"
tag = "other"
`,
			tags: []string{"init"},
			expected: []rawMsg{
				{bytes: []byte{0xAA}, delay: 0, tag: "init", index: 0},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			binaryMsgs, ok := msgs.([]BinaryMsg)
			if !ok {
				t.Fatalf("expected []BinaryMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(binaryMsgs)
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if !reflect.DeepEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestTaggedMsgsMixedTypes(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		tags    []string
		wantErr bool
	}{
		{
			name: "mixed types with separate tags is valid",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[binary]]
hex = "AA"
tag = "bins"
`,
			tags:    []string{"lines"},
			wantErr: false,
		},
		{
			name: "selecting only binary tag is valid",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[binary]]
hex = "AA"
tag = "bins"
`,
			tags:    []string{"bins"},
			wantErr: false,
		},
		{
			name: "selecting both types fails",
			toml: `[[line]]
text = "TEST"
tag = "mixed"

[[binary]]
hex = "AA"
tag = "mixed"
`,
			tags:    []string{"mixed"},
			wantErr: true,
		},
		{
			name: "selecting multiple tags with mixed types fails",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[binary]]
hex = "AA"
tag = "bins"
`,
			tags:    []string{"lines", "bins"},
			wantErr: true,
		},
		{
			name: "no matching tag fails",
			toml: `[[line]]
text = "LINE1"
tag = "setup"
`,
			tags:    []string{"other"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			_, err := mf.TaggedMsgs(tc.tags)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadNMEAMsgSimple(t *testing.T) {
	toml := `[[nmea]]
text = "PCAS04,3"

[[nmea]]
text = "PUBX,40,GGA,0,0,0,0"
`
	mf := loadMsgFileFromString(t, toml)
	if len(mf.NMEA) != 2 {
		t.Fatalf("expected 2 nmea messages, got %d", len(mf.NMEA))
	}
	if mf.NMEA[0].Text != "PCAS04,3" {
		t.Errorf("expected first nmea text, got %q", mf.NMEA[0].Text)
	}
	if mf.NMEA[1].Text != "PUBX,40,GGA,0,0,0,0" {
		t.Errorf("expected second nmea text, got %q", mf.NMEA[1].Text)
	}
}

func TestNMEAMsgsToRaw(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []rawMsg
	}{
		{
			name: "simple text without $ or checksum",
			toml: `[[nmea]]
text = "PCAS04,3"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("$PCAS04,3*1A\r\n"), delay: 0, tag: "", index: 0},
			},
		},
		{
			name: "text with $ prefix",
			toml: `[[nmea]]
text = "$PCAS04,3"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("$PCAS04,3*1A\r\n"), delay: 0, tag: "", index: 0},
			},
		},
		{
			name: "text with $ and checksum",
			toml: `[[nmea]]
text = "$PCAS04,3*1A"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("$PCAS04,3*1A\r\n"), delay: 0, tag: "", index: 0},
			},
		},
		{
			name: "with delay",
			toml: `[[nmea]]
text = "PCAS04,3"
delay = 0.5
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("$PCAS04,3*1A\r\n"), delay: 500 * time.Millisecond, tag: "", index: 0},
			},
		},
		{
			name: "default delay",
			toml: `[default.nmea]
delay = 0.1

[[nmea]]
text = "PCAS04,3"

[[nmea]]
text = "PCAS04,7"
`,
			tags: []string{""},
			expected: []rawMsg{
				{bytes: []byte("$PCAS04,3*1A\r\n"), delay: 100 * time.Millisecond, tag: "", index: 0},
				{bytes: []byte("$PCAS04,7*1E\r\n"), delay: 100 * time.Millisecond, tag: "", index: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			nmeaMsgs, ok := msgs.([]NMEAMsg)
			if !ok {
				t.Fatalf("expected []NMEAMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(nmeaMsgs)
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if !reflect.DeepEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestValidateNMEA(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		tags    []string
		wantErr bool
	}{
		{
			name: "valid nmea",
			toml: `[[nmea]]
text = "PCAS04,3"
`,
			tags:    []string{""},
			wantErr: false,
		},
		{
			name: "empty text",
			toml: `[[nmea]]
text = ""
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "default text not empty",
			toml: `[default.nmea]
text = "BAD"

[[nmea]]
text = "PCAS04,3"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "negative delay",
			toml: `[[nmea]]
text = "PCAS04,3"
delay = -1.0
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "negative delay in default",
			toml: `[default.nmea]
delay = -0.5

[[nmea]]
text = "PCAS04,3"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "invalid nmea syntax",
			toml: `[[nmea]]
text = "BAD*ZZ"
`,
			tags:    []string{""},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			err := validateMsgs(mf, tc.tags)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTaggedNMEAMsgs(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []rawMsg
	}{
		{
			name: "filter by tag",
			toml: `[[nmea]]
text = "PCAS04,3"
tag = "setup"

[[nmea]]
text = "PCAS04,7"
tag = "other"
`,
			tags: []string{"setup"},
			expected: []rawMsg{
				{bytes: []byte("$PCAS04,3*1A\r\n"), delay: 0, tag: "setup", index: 0},
			},
		},
		{
			name: "default tag",
			toml: `[default.nmea]
tag = "init"

[[nmea]]
text = "PCAS04,3"

[[nmea]]
text = "PCAS04,7"
tag = "other"
`,
			tags: []string{"init"},
			expected: []rawMsg{
				{bytes: []byte("$PCAS04,3*1A\r\n"), delay: 0, tag: "init", index: 0},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			nmeaMsgs, ok := msgs.([]NMEAMsg)
			if !ok {
				t.Fatalf("expected []NMEAMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(nmeaMsgs)
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if !reflect.DeepEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestTaggedMsgsMixedWithNMEA(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		tags    []string
		wantErr bool
	}{
		{
			name: "line and nmea with separate tags is valid",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[nmea]]
text = "PCAS04,3"
tag = "nmeas"
`,
			tags:    []string{"lines"},
			wantErr: false,
		},
		{
			name: "selecting nmea tag only is valid",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[nmea]]
text = "PCAS04,3"
tag = "nmeas"
`,
			tags:    []string{"nmeas"},
			wantErr: false,
		},
		{
			name: "selecting both line and nmea fails",
			toml: `[[line]]
text = "TEST"
tag = "mixed"

[[nmea]]
text = "PCAS04,3"
tag = "mixed"
`,
			tags:    []string{"mixed"},
			wantErr: true,
		},
		{
			name: "binary and nmea with same tag fails",
			toml: `[[binary]]
hex = "AA"
tag = "mixed"

[[nmea]]
text = "PCAS04,3"
tag = "mixed"
`,
			tags:    []string{"mixed"},
			wantErr: true,
		},
		{
			name: "all three types with separate tags selecting nmea",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[binary]]
hex = "AA"
tag = "bins"

[[nmea]]
text = "PCAS04,3"
tag = "nmeas"
`,
			tags:    []string{"nmeas"},
			wantErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			_, err := mf.TaggedMsgs(tc.tags)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPayloadFromTOML(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		expected []byte
		wantErr  bool
	}{
		{
			name: "single U1",
			toml: `types = "U1"
values = [42]
`,
			expected: []byte{0x2A},
		},
		{
			name: "single U2",
			toml: `types = "U2"
values = [0x1234]
`,
			expected: []byte{0x34, 0x12},
		},
		{
			name: "single U4",
			toml: `types = "U4"
values = [0x12345678]
`,
			expected: []byte{0x78, 0x56, 0x34, 0x12},
		},
		{
			name: "multiple values U1U2U4",
			toml: `types = "U1U2U4"
values = [1, 100, 0x12345678]
`,
			expected: []byte{0x01, 0x64, 0x00, 0x78, 0x56, 0x34, 0x12},
		},
		{
			name: "signed I1 negative",
			toml: `types = "I1"
values = [-1]
`,
			expected: []byte{0xFF},
		},
		{
			name: "signed I2 negative",
			toml: `types = "I2"
values = [-256]
`,
			expected: []byte{0x00, 0xFF},
		},
		{
			name: "empty payload",
			toml: `types = ""
values = []
`,
			expected: nil,
		},
		{
			name: "mixed signed and unsigned",
			toml: `types = "U1I1U2I2"
values = [255, -1, 0x1234, -1000]
`,
			expected: []byte{0xFF, 0xFF, 0x34, 0x12, 0x18, 0xFC},
		},
		{
			name:    "type count mismatch",
			toml:    `types = "U1U2"` + "\n" + `values = [1]` + "\n",
			wantErr: true,
		},
		{
			name:    "unknown type",
			toml:    `types = "U3"` + "\n" + `values = [1]` + "\n",
			wantErr: true,
		},
		{
			name:    "value out of range",
			toml:    `types = "U1"` + "\n" + `values = [256]` + "\n",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := loadPayloadFromString(t, tc.toml)
			got, err := p.Encode(casbin.Endian)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %x, expected %x", got, tc.expected)
			}
		})
	}
}

func loadPayloadFromString(t *testing.T, content string) Payload {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	var p Payload
	if err := toml.Unmarshal(data, &p); err != nil {
		t.Fatalf("TOML parse error: %v", err)
	}
	return p
}

func TestCASBINMsgsToRaw(t *testing.T) {
	tests := []struct {
		name        string
		toml        string
		tags        []string
		wantPacket  []byte
		wantDelay   time.Duration
		wantPayload []byte // payload portion only (for easier verification)
		wantErr     bool
	}{
		{
			name: "simple U1 payload",
			toml: `[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1"
payload.values = [42]
`,
			tags:        []string{""},
			wantPayload: []byte{0x2A},
		},
		{
			name: "U1U2U4 payload",
			toml: `[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1U2U4"
payload.values = [1, 100, 0x12345678]
`,
			tags:        []string{""},
			wantPayload: []byte{0x01, 0x64, 0x00, 0x78, 0x56, 0x34, 0x12},
		},
		{
			name: "with delay",
			toml: `[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1"
payload.values = [1]
delay = 0.5
`,
			tags:        []string{""},
			wantPayload: []byte{0x01},
			wantDelay:   500 * time.Millisecond,
		},
		{
			name: "default delay",
			toml: `[default.casbin]
delay = 0.2

[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1"
payload.values = [1]
`,
			tags:        []string{""},
			wantPayload: []byte{0x01},
			wantDelay:   200 * time.Millisecond,
		},
		{
			name: "override default delay",
			toml: `[default.casbin]
delay = 0.2

[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1"
payload.values = [1]
delay = 0.5
`,
			tags:        []string{""},
			wantPayload: []byte{0x01},
			wantDelay:   500 * time.Millisecond,
		},
		{
			name: "tag filtering",
			toml: `[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1"
payload.values = [1]
tag = "cfg"

[[casbin]]
class = 0x06
id = 0x04
payload.types = "U1"
payload.values = [2]
tag = "other"
`,
			tags:        []string{"cfg"},
			wantPayload: []byte{0x01},
		},
		{
			name: "empty payload",
			toml: `[[casbin]]
class = 0x05
id = 0x01
payload.types = ""
payload.values = []
`,
			tags:        []string{""},
			wantPayload: []byte{},
		},
		{
			name: "signed values",
			toml: `[[casbin]]
class = 0x06
id = 0x03
payload.types = "I1I2I4"
payload.values = [-1, -256, -65536]
`,
			tags:        []string{""},
			wantPayload: []byte{0xFF, 0x00, 0xFF, 0x00, 0x00, 0xFF, 0xFF},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			casbinMsgs, ok := msgs.([]CASBINMsg)
			if !ok {
				t.Fatalf("expected []CASBINMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(casbinMsgs)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if len(raw) != 1 {
				t.Fatalf("expected 1 raw message, got %d", len(raw))
			}
			// Verify packet structure: sync(2) + len(2) + class(1) + id(1) + payload + checksum(4)
			pkt := raw[0].bytes
			expLen := 10 + len(tc.wantPayload)
			if len(pkt) != expLen {
				t.Errorf("packet length: got %d, expected %d", len(pkt), expLen)
			}
			if pkt[0] != 0xBA || pkt[1] != 0xCE {
				t.Errorf("sync bytes: got %02X %02X, expected BA CE", pkt[0], pkt[1])
			}
			// Extract and verify payload
			payloadLen := int(pkt[2]) | int(pkt[3])<<8
			if payloadLen != len(tc.wantPayload) {
				t.Errorf("payload length field: got %d, expected %d", payloadLen, len(tc.wantPayload))
			}
			gotPayload := pkt[6 : 6+payloadLen]
			if !reflect.DeepEqual(gotPayload, tc.wantPayload) {
				t.Errorf("payload: got %x, expected %x", gotPayload, tc.wantPayload)
			}
			// Verify delay
			if raw[0].delay != tc.wantDelay {
				t.Errorf("delay: got %v, expected %v", raw[0].delay, tc.wantDelay)
			}
		})
	}
}

func TestUBXMsgsToRaw(t *testing.T) {
	tests := []struct {
		name        string
		toml        string
		tags        []string
		wantPayload []byte
		wantDelay   time.Duration
		wantErr     bool
	}{
		{
			name: "simple U1 payload",
			toml: `[[ubx]]
class = 0x06
id = 0x8A
payload.types = "U1"
payload.values = [42]
`,
			tags:        []string{""},
			wantPayload: []byte{0x2A},
		},
		{
			name: "CFG-VALSET example",
			toml: `[[ubx]]
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x10320001, 1]
`,
			tags:        []string{""},
			wantPayload: []byte{0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x32, 0x10, 0x01},
		},
		{
			name: "with delay",
			toml: `[[ubx]]
class = 0x06
id = 0x8A
payload.types = "U1"
payload.values = [1]
delay = 0.5
`,
			tags:        []string{""},
			wantPayload: []byte{0x01},
			wantDelay:   500 * time.Millisecond,
		},
		{
			name: "tag filtering",
			toml: `[[ubx]]
class = 0x06
id = 0x8A
payload.types = "U1"
payload.values = [1]
tag = "valset"

[[ubx]]
class = 0x06
id = 0x8B
payload.types = "U1"
payload.values = [2]
tag = "other"
`,
			tags:        []string{"valset"},
			wantPayload: []byte{0x01},
		},
		{
			name: "empty payload (poll)",
			toml: `[[ubx]]
class = 0x06
id = 0x8A
payload.types = ""
payload.values = []
`,
			tags:        []string{""},
			wantPayload: []byte{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			ubxMsgs, ok := msgs.([]UBXMsg)
			if !ok {
				t.Fatalf("expected []UBXMsg, got %T", msgs)
			}
			raw, err := toRawMsgs(ubxMsgs)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("toRawMsgs error: %v", err)
			}
			if len(raw) != 1 {
				t.Fatalf("expected 1 raw message, got %d", len(raw))
			}
			// Verify packet structure: sync(2) + class(1) + id(1) + len(2) + payload + checksum(2)
			pkt := raw[0].bytes
			expLen := 8 + len(tc.wantPayload)
			if len(pkt) != expLen {
				t.Errorf("packet length: got %d, expected %d", len(pkt), expLen)
			}
			if pkt[0] != 0xB5 || pkt[1] != 0x62 {
				t.Errorf("sync bytes: got %02X %02X, expected B5 62", pkt[0], pkt[1])
			}
			// Extract and verify payload
			payloadLen := int(pkt[4]) | int(pkt[5])<<8
			if payloadLen != len(tc.wantPayload) {
				t.Errorf("payload length field: got %d, expected %d", payloadLen, len(tc.wantPayload))
			}
			gotPayload := pkt[6 : 6+payloadLen]
			if !reflect.DeepEqual(gotPayload, tc.wantPayload) {
				t.Errorf("payload: got %x, expected %x", gotPayload, tc.wantPayload)
			}
			// Verify delay
			if raw[0].delay != tc.wantDelay {
				t.Errorf("delay: got %v, expected %v", raw[0].delay, tc.wantDelay)
			}
		})
	}
}

func TestMixedCASBINAndUBXFails(t *testing.T) {
	toml := `[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1"
payload.values = [1]
tag = "mixed"

[[ubx]]
class = 0x06
id = 0x8A
payload.types = "U1"
payload.values = [1]
tag = "mixed"
`
	mf := loadMsgFileFromString(t, toml)
	_, err := mf.TaggedMsgs([]string{"mixed"})
	if err == nil {
		t.Error("expected error for mixed CASBIN and UBX types")
	}
}

func loadMsgFileFromString(t *testing.T, content string) *MsgFile {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	mf, err := LoadMsgFile(path)
	if err != nil {
		t.Fatalf("LoadMsgFile error: %v", err)
	}
	return mf
}

func TestDescriptionInDefaultsNotAllowed(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{
			name: "default.line.description",
			toml: `[default.line]
description = "not allowed"

[[line]]
text = "TEST"
`,
		},
		{
			name: "default.binary.description",
			toml: `[default.binary]
description = "not allowed"

[[binary]]
hex = "AA"
`,
		},
		{
			name: "default.nmea.description",
			toml: `[default.nmea]
description = "not allowed"

[[nmea]]
text = "PCAS04,3"
`,
		},
		{
			name: "default.casbin.description",
			toml: `[default.casbin]
description = "not allowed"

[[casbin]]
class = 0x06
id = 0x03
payload.types = "U1"
payload.values = [1]
`,
		},
		{
			name: "default.ubx.description",
			toml: `[default.ubx]
description = "not allowed"

[[ubx]]
class = 0x06
id = 0x8A
payload.types = "U1"
payload.values = [1]
`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			_, err := mf.TaggedMsgs([]string{""})
			if err == nil {
				t.Error("expected error for description in default section")
			}
		})
	}
}

func TestTagDescriptionsConflict(t *testing.T) {
	toml := `[[line]]
text = "LINE1"
tag = "setup"
description = "First description"

[[line]]
text = "LINE2"
tag = "setup"
description = "Different description"
`
	mf := loadMsgFileFromString(t, toml)
	descs, inconsistent := mf.collectTagDescs()
	if len(descs) != 1 {
		t.Errorf("expected 1 desc, got %d", len(descs))
	}
	if len(inconsistent) != 1 {
		t.Errorf("expected 1 inconsistent, got %d", len(inconsistent))
	}
	if len(inconsistent) > 0 && inconsistent[0].tag != "setup" {
		t.Errorf("expected inconsistent tag 'setup', got %q", inconsistent[0].tag)
	}
}

func TestTagDescriptionsConsistent(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		expected []tagDesc
	}{
		{
			name: "description on first only",
			toml: `[[line]]
text = "LINE1"
tag = "setup"
description = "Setup the device"

[[line]]
text = "LINE2"
tag = "setup"
`,
			expected: []tagDesc{{tag: "setup", desc: "Setup the device"}},
		},
		{
			name: "description on second only",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[line]]
text = "LINE2"
tag = "setup"
description = "Setup the device"
`,
			expected: []tagDesc{{tag: "setup", desc: "Setup the device"}},
		},
		{
			name: "same description on both",
			toml: `[[line]]
text = "LINE1"
tag = "setup"
description = "Setup the device"

[[line]]
text = "LINE2"
tag = "setup"
description = "Setup the device"
`,
			expected: []tagDesc{{tag: "setup", desc: "Setup the device"}},
		},
		{
			name: "multiple tags with descriptions",
			toml: `[[nmea]]
text = "PQTMVERNO"
tag = "version"
description = "Query firmware version"

[[nmea]]
text = "PQTMCFGPPS,R,1"
tag = "query-pps"
description = "Query PPS configuration"
`,
			expected: []tagDesc{
				{tag: "version", desc: "Query firmware version"},
				{tag: "query-pps", desc: "Query PPS configuration"},
			},
		},
		{
			name: "empty tag with description",
			toml: `[[line]]
text = "LINE1"
tag = "setup"
description = "Setup commands"

[[line]]
text = "LINE2"
description = "Default commands"
`,
			expected: []tagDesc{
				{tag: "", desc: "Default commands"},
				{tag: "setup", desc: "Setup commands"},
			},
		},
		{
			name: "tags without descriptions",
			toml: `[[line]]
text = "LINE1"
tag = "foo"

[[line]]
text = "LINE2"
tag = "bar"
`,
			expected: []tagDesc{
				{tag: "foo", desc: ""},
				{tag: "bar", desc: ""},
			},
		},
		{
			name: "order preserved across types",
			toml: `[[line]]
text = "LINE1"
tag = "first"

[[binary]]
hex = "AA"
tag = "second"

[[nmea]]
text = "PCAS04,3"
tag = "third"
`,
			expected: []tagDesc{
				{tag: "first", desc: ""},
				{tag: "second", desc: ""},
				{tag: "third", desc: ""},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadMsgFileFromString(t, tc.toml)
			tds, inconsistent := mf.collectTagDescs()
			if len(inconsistent) > 0 {
				t.Errorf("unexpected inconsistent tags: %+v", inconsistent)
			}
			if !reflect.DeepEqual(tds, tc.expected) {
				t.Errorf("got %+v, expected %+v", tds, tc.expected)
			}
		})
	}
}
