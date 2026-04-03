package msgfile

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

// makeNMEA builds a complete NMEA sentence from a payload (without $ or *).
func makeNMEA(payload string) string {
	s := "$" + payload
	checksum := nmeamsg.Checksum([]byte(payload))
	return fmt.Sprintf("%s*%02X\r\n", s, checksum)
}

// loadFromStringErr is like loadFromString but returns nil on error.
func loadFromStringErr(t *testing.T, content string) *Parsed {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/test.toml"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	mf, err := Load(path)
	if err != nil {
		return nil
	}
	return mf
}

func TestLoadSimple(t *testing.T) {
	toml := `[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
`
	mf := loadFromString(t, toml)
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

func TestLoadDefaults(t *testing.T) {
	toml := `[default.line]
delay = 0.1
eol = "\n"

[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
`
	mf := loadFromString(t, toml)
	if mf.Default.Line.Delay == nil || *mf.Default.Line.Delay != 0.1 {
		t.Errorf("expected default delay 0.1, got %v", mf.Default.Line.Delay)
	}
	if mf.Default.Line.EOL == nil || *mf.Default.Line.EOL != "\n" {
		t.Errorf("expected default eol \\n, got %v", mf.Default.Line.EOL)
	}
}

func TestLoadDefaultEOL(t *testing.T) {
	toml := `[[line]]
text = "TEST"
`
	mf := loadFromString(t, toml)
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
			mf := loadFromString(t, tc.toml)
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

func TestLineMsgsToRaw(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []RawMsg
	}{
		{
			name: "simple two lines",
			toml: `[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 0, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("LINE2\r\n"), Delay: 0, Tag: "", Index: 1, Count: 2},
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
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 100 * time.Millisecond, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("LINE2\r\n"), Delay: 0, Tag: "", Index: 1, Count: 2},
			},
		},
		{
			name: "custom eol",
			toml: `[[line]]
text = "LINE1"
eol = "\n"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("LINE1\n"), Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "empty eol",
			toml: `[[line]]
text = "NOTERM"
eol = ""
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("NOTERM"), Delay: 0, Tag: "", Index: 0, Count: 1},
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
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 200 * time.Millisecond, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("LINE2\r\n"), Delay: 200 * time.Millisecond, Tag: "", Index: 1, Count: 2},
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
			expected: []RawMsg{
				{Bytes: []byte("LINE1\n"), Delay: 0, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("LINE2\n"), Delay: 0, Tag: "", Index: 1, Count: 2},
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
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 500 * time.Millisecond, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("LINE2\r\n"), Delay: 200 * time.Millisecond, Tag: "", Index: 1, Count: 2},
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
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r"), Delay: 0, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("LINE2\n"), Delay: 0, Tag: "", Index: 1, Count: 2},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs)
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if !rawMsgsEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestTaggedMsgs(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []RawMsg
	}{
		{
			name: "filter by tag",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[line]]
text = "LINE3"
tag = "setup"

[[line]]
text = "LINE2"
tag = "ppp"
`,
			tags: []string{"setup"},
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 0, Tag: "setup", Index: 0, Count: 2},
				{Bytes: []byte("LINE3\r\n"), Delay: 0, Tag: "setup", Index: 1, Count: 2},
			},
		},
		{
			name: "multiple tags in order",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[line]]
text = "LINE3"
tag = "setup"

[[line]]
text = "LINE2"
tag = "ppp"
`,
			tags: []string{"ppp", "setup"},
			expected: []RawMsg{
				{Bytes: []byte("LINE2\r\n"), Delay: 0, Tag: "ppp", Index: 0, Count: 1},
				{Bytes: []byte("LINE1\r\n"), Delay: 0, Tag: "setup", Index: 0, Count: 2},
				{Bytes: []byte("LINE3\r\n"), Delay: 0, Tag: "setup", Index: 1, Count: 2},
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
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 0, Tag: "setup", Index: 0, Count: 2},
				{Bytes: []byte("LINE2\r\n"), Delay: 0, Tag: "setup", Index: 1, Count: 2},
			},
		},
		{
			name: "empty tag",
			toml: `[[line]]
text = "LINE1"

[[line]]
text = "LINE3"

[[line]]
text = "LINE2"
tag = "ppp"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 0, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("LINE3\r\n"), Delay: 0, Tag: "", Index: 1, Count: 2},
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
			expected: []RawMsg{
				{Bytes: []byte("LINE1\r\n"), Delay: 0, Tag: "foo", Index: 0, Count: 1},
				{Bytes: []byte("LINE2\r\n"), Delay: 0, Tag: "", Index: 0, Count: 1},
				{Bytes: []byte("LINE3\r\n"), Delay: 0, Tag: "bar", Index: 0, Count: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs)
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if !rawMsgsEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
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
	mf := loadFromString(t, toml)
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
		expected []RawMsg
	}{
		{
			name: "simple text without $ or checksum",
			toml: `[[nmea]]
text = "PCAS04,3"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("$PCAS04,3*1A\r\n"), Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "text with $ prefix",
			toml: `[[nmea]]
text = "$PCAS04,3"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("$PCAS04,3*1A\r\n"), Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "text with $ and checksum",
			toml: `[[nmea]]
text = "$PCAS04,3*1A"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("$PCAS04,3*1A\r\n"), Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "with delay",
			toml: `[[nmea]]
text = "PCAS04,3"
delay = 0.5
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte("$PCAS04,3*1A\r\n"), Delay: 500 * time.Millisecond, Tag: "", Index: 0, Count: 1},
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
			expected: []RawMsg{
				{Bytes: []byte("$PCAS04,3*1A\r\n"), Delay: 100 * time.Millisecond, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte("$PCAS04,7*1E\r\n"), Delay: 100 * time.Millisecond, Tag: "", Index: 1, Count: 2},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs)
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if !rawMsgsEqual(raw, tc.expected) {
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
			mf := loadFromString(t, tc.toml)
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
		expected []RawMsg
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
			expected: []RawMsg{
				{Bytes: []byte("$PCAS04,3*1A\r\n"), Delay: 0, Tag: "setup", Index: 0, Count: 1},
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
			expected: []RawMsg{
				{Bytes: []byte("$PCAS04,3*1A\r\n"), Delay: 0, Tag: "init", Index: 0, Count: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs)
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if !rawMsgsEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestResponsePatternTOML(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr bool
	}{
		{
			name: "unicore pattern",
			toml: `[default.line]
responsePattern = "unicore"

[[line]]
text = "CONFIG PPP ENABLE"
`,
		},
		{
			name: "no pattern",
			toml: `[[line]]
text = "CONFIG PPP ENABLE"
`,
		},
		{
			name: "explicit none",
			toml: `[[line]]
text = "CONFIG PPP ENABLE"
responsePattern = "none"
`,
		},
		{
			name: "unknown pattern",
			toml: `[[line]]
text = "CONFIG PPP ENABLE"
responsePattern = "unknown"
`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromStringErr(t, tc.toml)
			if tc.wantErr {
				if mf != nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if mf == nil {
				t.Fatal("unexpected load error")
			}
		})
	}
}

