package gpscmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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
