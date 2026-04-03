package msgfile

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
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

// NMEA recv helper for correlator tests.

type recvNMEAEvent struct{ payload string }

func recvNMEA(payload string) recvNMEAEvent { return recvNMEAEvent{payload: payload} }

func (e recvNMEAEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagNMEA, makeNMEA(e.payload))
}

// Unicore ASCII recv helper.

type recvUNCAEvent struct{ content string }

// recvUnicoreAscii constructs a UNCA packet from the content (including #).
func recvUnicoreAscii(content string) recvUNCAEvent {
	return recvUNCAEvent{content: content}
}

func makeUNCA(content string) string {
	// content starts with '#', checksum covers everything after '#'
	body := content[1:]
	var ck byte
	for i := 0; i < len(body); i++ {
		ck ^= body[i]
	}
	return fmt.Sprintf("%s*%02x\r\n", content, ck)
}

func (e recvUNCAEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagUnicoreAscii, makeUNCA(e.content))
}

// recvEmptyTag sends a raw line with empty tag for line message tests.
type recvEmptyTagEvent struct{ data string }

func recvEmptyTag(data string) recvEmptyTagEvent { return recvEmptyTagEvent{data: data} }

func (e recvEmptyTagEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsprot.EmptyTag, e.data)
}

func TestCorrelatorUnicore(t *testing.T) {
	runCorrelatorTests(t, "unc-test.toml", []correlatorTest{
		{
			name: "set command ACK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command NAK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: not support"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command no response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "CONFIG query ACK then data",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("CONFIG,PPS,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0"),
				expect{relevance: LevelMultiResponse},
				recvNMEA("CONFIG,SIGNALGROUP,CONFIG SIGNALGROUP 2"),
				expect{relevance: LevelMultiResponse},
				checkDone{canAcceptMore: true}, // multiple data, completion never known
				checkMissing{},                 // not missing: data was received
			},
		},
		{
			name: "CONFIG query NAK",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG,response: unknown command"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CONFIG query missing ACK and data",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				checkMissing{ack: []int{0}, data: []int{0}},
			},
		},
		{
			name: "MASK query ACK then data",
			tags: []string{"get-mask"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MASK,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("CONFIG,MASK,MASK 5.000000"),
				expect{relevance: LevelMultiResponse},
				recvNMEA("CONFIG,MASK,MASK GPS"),
				expect{relevance: LevelMultiResponse},
				checkDone{canAcceptMore: true},
			},
		},
		{
			name: "CONFIG data does not match MASK query",
			tags: []string{"get-mask"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MASK,response: OK"),
				expect{ack: AckAck},
				recvNMEA("CONFIG,PPS,CONFIG PPS ENABLE"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "MASK data does not match CONFIG query",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG,response: OK"),
				expect{ack: AckAck},
				recvNMEA("CONFIG,MASK,MASK GPS"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "MODE query ACK then UNCA data",
			tags: []string{"get-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#MODE,81;MODE ROVER SURVEY,"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "MODE query NAK",
			tags: []string{"get-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE,response: unknown command"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two different set commands pacing OK",
			tags: []string{"set-pps", "set-sg"},
			events: []event{
				sendEvent{},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("command,CONFIG SIGNALGROUP 2,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "same set command pacing blocks",
			tags: []string{"set-pps", "set-pps-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "GNSS talker NMEA not a response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("GPRMC,123456.00,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "UNCA data for non-MODE does not match MODE query",
			tags: []string{"get-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE,response: OK"),
				expect{ack: AckAck},
				recvUnicoreAscii("#BESTNAV,81;some data,"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "VERSION query ACK then UNCA data",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("command,VERSION,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#VERSION,97;UM980,R4.10Build17548,"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "data output command one-shot ambiguous",
			tags: []string{"get-versiona"},
			events: []event{
				sendEvent{},
				recvNMEA("command,VERSIONA,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#VERSIONA,97;UM980,R4.10Build17548,"),
				expect{relevance: LevelMaybeResponse},
			},
		},
		{
			name: "data output command periodic ambiguous",
			tags: []string{"set-rectimeb"},
			events: []event{
				sendEvent{},
				recvNMEA("command,RECTIMEB 1,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#RECTIMEB,97;some data,"),
				expect{relevance: LevelMaybeResponse},
			},
		},
		{
			name: "MODE set command ACK only",
			tags: []string{"set-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE ROVER SURVEY,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
	})
}

func TestCorrelatorPQTM(t *testing.T) {
	runCorrelatorTests(t, "pqtm-test.toml", []correlatorTest{
		{
			name: "write command ACK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "write command NAK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,ERROR,1"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "write command no response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "query ACK with data",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK,1,1,100000,1000,0,0"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query bare OK still completes",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				// Bare OK for a query is unusual but the correlator
				// treats any ACK as data-received for expectDataWithAck.
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query NAK",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,ERROR,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "PQTMVERNO data without OK",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "PQTMVERNO NAK",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,ERROR,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "PQTMVERNO no response",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{data: []int{0}},
			},
		},
		{
			name: "two different write commands pacing OK",
			tags: []string{"set-pps", "set-fixrate"},
			events: []event{
				sendEvent{},
				readyToSend{want: true}, // different sentence name, no conflict
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("PQTMCFGFIXRATE,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "same write command pacing blocks",
			tags: []string{"set-pps", "set-pps-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false}, // same sentence PQTMCFGPPS
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "unrelated PQTM not matched",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGFIXRATE,OK"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "GNSS talker NMEA not a response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("GPRMC,123456.00,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "PQTMVERNO pacing unblocks after data received",
			tags: []string{"get-version", "get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
				readyToSend{want: true}, // completed request must not block
			},
		},
		{
			name: "PQTMVERNO NAK after first completed by data",
			tags: []string{"get-version", "get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07"),
				expect{relevance: LevelSoleResponse},
				readyToSend{want: true},
				sendEvent{},
				// NAK must match the second request, not ambiguously match both.
				recvNMEA("PQTMVERNO,ERROR,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "savepar command ACK",
			tags: []string{"savepar"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMSAVEPAR,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
	})
}

func TestCorrelatorPAIR(t *testing.T) {
	runCorrelatorTests(t, "pair-test.toml", []correlatorTest{
		{
			name: "set command ACK",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command NAK",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,072,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command no response",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "set command wait then ACK",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,072,1"),
				expect{ack: AckOther, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: true},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query ACK then data",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,073,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("PAIR073,5"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query data before ACK",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR073,5"),
				expect{relevance: LevelSoleResponse},
				recvNMEA("PAIR001,073,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query NAK no data",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,073,4"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query no response",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}, data: []int{0}},
			},
		},
		{
			name: "two different set commands pacing OK",
			tags: []string{"set-elev", "set-nmea-ver"},
			events: []event{
				sendEvent{},
				readyToSend{want: true}, // different command IDs
				sendEvent{},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("PAIR001,100,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "same set command pacing blocks",
			tags: []string{"set-elev", "set-elev-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false}, // same command ID 072
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "unrelated PAIR001 not matched",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,100,0"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "GNSS talker NMEA not a response",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("GPRMC,123456.00,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "query data does not match wrong command",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,073,0"),
				expect{ack: AckAck},
				recvNMEA("PAIR101,1"),
				expect{relevance: LevelNotResponse},
			},
		},
	})
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

