package msgfile

import (
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

// makeNMEA builds a complete NMEA sentence from a payload (without $ or *).
func makeNMEA(payload string) string {
	s := "$" + payload
	checksum := nmeamsg.Checksum([]byte(payload))
	return fmt.Sprintf("%s*%02X\r\n", s, checksum)
}

// NMEA recv helper for correlator tests.

type recvNMEAEvent struct{ payload string }

func recvNMEA(payload string) recvNMEAEvent { return recvNMEAEvent{payload: payload} }

func (e recvNMEAEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagNMEA, makeNMEA(e.payload))
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

func TestCorrelatorNMEA(t *testing.T) {
	runCorrelatorTests(t, "nmea-test.toml", []correlatorTest{
		{
			name: "same vendor response accepted",
			tags: []string{"send-xyz"},
			events: []event{
				sendEvent{},
				recvNMEA("PXYZ,reply,data"),
				expect{relevance: LevelMaybeResponse},
			},
		},
		{
			name: "different vendor filtered",
			tags: []string{"send-xyz"},
			events: []event{
				sendEvent{},
				recvNMEA("PABC,reply,data"),
				expect{relevance: LevelNotResponse},
			},
		},
	})
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
