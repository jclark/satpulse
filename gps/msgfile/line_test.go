package msgfile

import (
	"os"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

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

// recvEmptyTag sends a raw line with empty tag for line message tests.
type recvEmptyTagEvent struct{ data string }

func recvEmptyTag(data string) recvEmptyTagEvent { return recvEmptyTagEvent{data: data} }

func (e recvEmptyTagEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsprot.EmptyTag, e.data)
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
			raw, err := ToRaw(msgs, "", false)
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
			raw, err := ToRaw(msgs, "", false)
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if !rawMsgsEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestCorrelatorLine(t *testing.T) {
	runCorrelatorTests(t, "line-test.toml", []correlatorTest{
		{
			name: "line message shows printable text as maybe response",
			tags: []string{"raw-cmd"},
			events: []event{
				sendEvent{},
				recvEmptyTag("OK some response\r\n"),
				expect{relevance: LevelMaybeResponse},
			},
		},
		{
			name: "line message rejects binary data",
			tags: []string{"raw-cmd"},
			events: []event{
				sendEvent{},
				recvEmptyTag("\x00\x01\x02\r\n"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "line message completion never known",
			tags: []string{"raw-cmd"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
			},
		},
	})
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
