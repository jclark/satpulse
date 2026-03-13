package msgfile

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/pelletier/go-toml/v2"
)

func loadFromString(t *testing.T, content string) *Parsed {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	return mf
}

// rawMsgsEqual compares two RawMsg slices ignoring the unexported source field.
func rawMsgsEqual(a, b []RawMsg) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i].Bytes, b[i].Bytes) ||
			a[i].Delay != b[i].Delay ||
			a[i].Tag != b[i].Tag ||
			a[i].Index != b[i].Index ||
			a[i].Count != b[i].Count {
			return false
		}
	}
	return true
}

// makeRawMsg creates a RawMsg from a message type that implements responsePattern.
func makeRawMsg(rp responsePattern) RawMsg {
	return RawMsg{source: rp}
}

func validateMsgs(mf *Parsed, tags []string) error {
	msgs, err := mf.TaggedMsgs(tags)
	if err != nil {
		return err
	}
	_, err = ToRaw(msgs)
	return err
}

func TestValidateTags(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr bool
	}{
		{
			name: "valid tags",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[line]]
text = "LINE2"
tag = "setup"

[[line]]
text = "LINE3"
tag = "other"
`,
			wantErr: false,
		},
		{
			name: "non-consecutive tag",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[line]]
text = "LINE2"
tag = "other"

[[line]]
text = "LINE3"
tag = "setup"
`,
			wantErr: true,
		},
		{
			name: "same tag across types",
			toml: `[[line]]
text = "LINE1"
tag = "shared"

[[binary]]
hex = "AA"
tag = "shared"
`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			err := mf.ValidateTags()
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
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
	_, err := Load(path)
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
	mf := loadFromString(t, toml)
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
	msgs, err := mf.TaggedMsgs([]string{"setup"})
	if err != nil {
		t.Fatalf("TaggedMsgs error: %v", err)
	}
	raw, err := ToRaw(msgs)
	if err != nil {
		t.Fatalf("ToRaw error: %v", err)
	}
	if len(raw) != 2 {
		t.Errorf("expected 2 raw messages, got %d", len(raw))
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
			name: "same tag in multiple types fails even when not selected",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[line]]
text = "TEST2"
tag = "shared"

[[binary]]
hex = "AA"
tag = "shared"
`,
			tags:    []string{"lines"},
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
			mf := loadFromString(t, tc.toml)
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
			mf := loadFromString(t, tc.toml)
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

func TestTaggedMsgsTagValidation(t *testing.T) {
	tests := []struct {
		name       string
		toml       string
		tags       []string
		errSubstrs []string
	}{
		{
			name: "non-consecutive line tag",
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
			tags:       []string{"setup"},
			errSubstrs: []string{`messages with tag "setup" must be consecutive`},
		},
		{
			name: "non-consecutive empty effective tag",
			toml: `[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
tag = "ppp"

[[line]]
text = "LINE3"
`,
			tags:       []string{"ppp"},
			errSubstrs: []string{`messages with tag "" must be consecutive`},
		},
		{
			name: "non-consecutive default inherited tag",
			toml: `[default.line]
tag = "setup"

[[line]]
text = "LINE1"

[[line]]
text = "LINE2"
tag = "ppp"

[[line]]
text = "LINE3"
`,
			tags:       []string{"ppp"},
			errSubstrs: []string{`messages with tag "setup" must be consecutive`},
		},
		{
			name: "same tag across message types",
			toml: `[[line]]
text = "LINE1"
tag = "setup"

[[binary]]
hex = "AA"
tag = "setup"
`,
			tags:       []string{"setup"},
			errSubstrs: []string{`tag "setup" used for multiple message types`, "line", "binary"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			_, err := mf.TaggedMsgs(tc.tags)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, s := range tc.errSubstrs {
				if !strings.Contains(err.Error(), s) {
					t.Fatalf("error %q does not contain %q", err, s)
				}
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
			mf := loadFromString(t, tc.toml)
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
	mf := loadFromString(t, toml)
	descs, inconsistent := mf.TagDescs()
	if len(descs) != 1 {
		t.Errorf("expected 1 desc, got %d", len(descs))
	}
	if len(inconsistent) != 1 {
		t.Errorf("expected 1 inconsistent, got %d", len(inconsistent))
	}
	if len(inconsistent) > 0 && inconsistent[0].Tag != "setup" {
		t.Errorf("expected inconsistent tag 'setup', got %q", inconsistent[0].Tag)
	}
}

func TestTagDescriptionsConsistent(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		expected []TagDesc
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
			expected: []TagDesc{{Tag: "setup", Desc: "Setup the device", MsgCount: 2}},
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
			expected: []TagDesc{{Tag: "setup", Desc: "Setup the device", MsgCount: 2}},
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
			expected: []TagDesc{{Tag: "setup", Desc: "Setup the device", MsgCount: 2}},
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
			expected: []TagDesc{
				{Tag: "version", Desc: "Query firmware version", MsgCount: 1},
				{Tag: "query-pps", Desc: "Query PPS configuration", MsgCount: 1},
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
			expected: []TagDesc{
				{Tag: "", Desc: "Default commands", MsgCount: 1},
				{Tag: "setup", Desc: "Setup commands", MsgCount: 1},
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
			expected: []TagDesc{
				{Tag: "foo", Desc: "", MsgCount: 1},
				{Tag: "bar", Desc: "", MsgCount: 1},
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
			expected: []TagDesc{
				{Tag: "first", Desc: "", MsgCount: 1},
				{Tag: "second", Desc: "", MsgCount: 1},
				{Tag: "third", Desc: "", MsgCount: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			tds, inconsistent := mf.TagDescs()
			if len(inconsistent) > 0 {
				t.Errorf("unexpected inconsistent tags: %+v", inconsistent)
			}
			if !reflect.DeepEqual(tds, tc.expected) {
				t.Errorf("got %+v, expected %+v", tds, tc.expected)
			}
		})
	}
}

func TestMsgID(t *testing.T) {
	rm := RawMsg{Tag: "setup", Index: 2, Count: 5}
	mid := rm.MsgID()
	if mid.Tag != "setup" || mid.Index != 2 || mid.Count != 5 {
		t.Errorf("MsgID: got %+v", mid)
	}
}

func TestPacketAnalyzerUBXAck(t *testing.T) {
	pa := NewPacketAnalyzer()
	um := &UBXMsg{}
	um.Class = 0x06
	um.ID = 0x8A
	rm := makeRawMsg(um)
	rm.Tag = "cfg"
	rm.Index = 0
	rm.Count = 1
	pa.NotifySent(rm)
	result := pa.Analyze(gpsreg.TagUBX, makeUBXAckAck(0x06, 0x8A))
	if result.Kind != AckResponse {
		t.Fatalf("kind: got %d, want %d", result.Kind, AckResponse)
	}
	if result.AckError != "" {
		t.Errorf("ackErr: got %q, want empty", result.AckError)
	}
	if result.RelatedMsg == nil {
		t.Fatal("RelatedMsg is nil")
	}
	if result.RelatedMsg.Tag != "cfg" {
		t.Errorf("RelatedMsg.Tag: got %q, want %q", result.RelatedMsg.Tag, "cfg")
	}
}

func TestPacketAnalyzerUBXNak(t *testing.T) {
	pa := NewPacketAnalyzer()
	um := &UBXMsg{}
	um.Class = 0x06
	um.ID = 0x8A
	pa.NotifySent(makeRawMsg(um))
	result := pa.Analyze(gpsreg.TagUBX, makeUBXAckNak(0x06, 0x8A))
	if result.Kind != AckResponse {
		t.Fatalf("kind: got %d, want %d", result.Kind, AckResponse)
	}
	if result.AckError != AckNak {
		t.Errorf("ackErr: got %q, want %q", result.AckError, AckNak)
	}
}

func TestPacketAnalyzerAlreadyAcked(t *testing.T) {
	pa := NewPacketAnalyzer()
	um := &UBXMsg{}
	um.Class = 0x06
	um.ID = 0x8A
	pa.NotifySent(makeRawMsg(um))
	r1 := pa.Analyze(gpsreg.TagUBX, makeUBXAckAck(0x06, 0x8A))
	if r1.Kind != AckResponse {
		t.Fatalf("first ack kind: got %d, want %d", r1.Kind, AckResponse)
	}
	r2 := pa.Analyze(gpsreg.TagUBX, makeUBXAckAck(0x06, 0x8A))
	if r2.Kind != NotResponse {
		t.Errorf("second ack kind: got %d, want %d", r2.Kind, NotResponse)
	}
}

func TestPacketAnalyzerAmbiguousAck(t *testing.T) {
	pa := NewPacketAnalyzer()
	um1 := &UBXMsg{}
	um1.Class = 0x06
	um1.ID = 0x8A
	rm1 := makeRawMsg(um1)
	rm1.Tag = "cfg"
	rm1.Index = 0
	pa.NotifySent(rm1)
	um2 := &UBXMsg{}
	um2.Class = 0x06
	um2.ID = 0x8A
	rm2 := makeRawMsg(um2)
	rm2.Tag = "cfg"
	rm2.Index = 1
	pa.NotifySent(rm2)
	result := pa.Analyze(gpsreg.TagUBX, makeUBXAckAck(0x06, 0x8A))
	if result.Kind != OtherResponse {
		t.Errorf("kind: got %d, want %d (ambiguous ack -> OtherResponse)", result.Kind, OtherResponse)
	}
	if result.RelatedMsg != nil {
		t.Error("RelatedMsg should be nil for ambiguous ack")
	}
}

func TestPacketAnalyzerMultipleMessages(t *testing.T) {
	pa := NewPacketAnalyzer()
	um1 := &UBXMsg{}
	um1.Class = 0x06
	um1.ID = 0x8A
	rm1 := makeRawMsg(um1)
	rm1.Tag = "valset"
	rm1.Index = 0
	pa.NotifySent(rm1)
	um2 := &UBXMsg{}
	um2.Class = 0x06
	um2.ID = 0x01
	rm2 := makeRawMsg(um2)
	rm2.Tag = "cfg-prt"
	rm2.Index = 0
	pa.NotifySent(rm2)
	r := pa.Analyze(gpsreg.TagUBX, makeUBXAckAck(0x06, 0x01))
	if r.Kind != AckResponse {
		t.Fatalf("kind: got %d, want %d", r.Kind, AckResponse)
	}
	if r.RelatedMsg.Tag != "cfg-prt" {
		t.Errorf("RelatedMsg.Tag: got %q, want %q", r.RelatedMsg.Tag, "cfg-prt")
	}
	r = pa.Analyze(gpsreg.TagUBX, makeUBXAckAck(0x06, 0x8A))
	if r.Kind != AckResponse {
		t.Fatalf("kind: got %d, want %d", r.Kind, AckResponse)
	}
	if r.RelatedMsg.Tag != "valset" {
		t.Errorf("RelatedMsg.Tag: got %q, want %q", r.RelatedMsg.Tag, "valset")
	}
}

func TestPacketAnalyzerNotResponse(t *testing.T) {
	pa := NewPacketAnalyzer()
	um := &UBXMsg{}
	um.Class = 0x06
	um.ID = 0x8A
	pa.NotifySent(makeRawMsg(um))
	result := pa.Analyze(gpsreg.TagNMEA, makeNMEA("GPRMC,,,,,,,,,,,"))
	if result.Kind != NotResponse {
		t.Errorf("GNSS talker NMEA: got %d, want %d", result.Kind, NotResponse)
	}
	result = pa.Analyze(gpsreg.TagUBX, makeUBXPacket(0x01, 0x07, []byte{0}))
	if result.Kind != NotResponse {
		t.Errorf("different UBX class/id: got %d, want %d", result.Kind, NotResponse)
	}
}

func TestPacketAnalyzerMaybeResponse(t *testing.T) {
	pa := NewPacketAnalyzer()
	lm := &LineMsg{Text: "LOG VERSION"}
	lm.RespPattern = ptr(ResponsePatternNone)
	pa.NotifySent(makeRawMsg(lm))
	result := pa.Analyze(gpsprot.EmptyTag, "some random data")
	if result.Kind != MaybeResponse {
		t.Errorf("unrecognized: got %d, want %d", result.Kind, MaybeResponse)
	}
}

func TestPacketAnalyzerOtherResponse(t *testing.T) {
	pa := NewPacketAnalyzer()
	um := &UBXMsg{}
	um.Class = 0x06
	um.ID = 0x31 // CFG-TP5
	pa.NotifySent(makeRawMsg(um))
	result := pa.Analyze(gpsreg.TagUBX, makeUBXPacket(0x06, 0x31, []byte{0x01, 0x02, 0x03, 0x04}))
	if result.Kind != OtherResponse {
		t.Errorf("kind: got %d, want %d", result.Kind, OtherResponse)
	}
}

func TestPacketAnalyzerMixedProtocols(t *testing.T) {
	pa := NewPacketAnalyzer()
	um := &UBXMsg{}
	um.Class = 0x06
	um.ID = 0x8A
	pa.NotifySent(makeRawMsg(um))
	lm := &LineMsg{Text: "CONFIG PPP ENABLE"}
	lm.RespPattern = ptr(ResponsePatternUnicore)
	pa.NotifySent(makeRawMsg(lm))
	r := pa.Analyze(gpsreg.TagUBX, makeUBXAckAck(0x06, 0x8A))
	if r.Kind != AckResponse {
		t.Fatalf("kind: got %d, want %d", r.Kind, AckResponse)
	}
	r = pa.Analyze(gpsreg.TagNMEA, makeNMEA("command,CONFIG PPP ENABLE,response: OK"))
	if r.Kind != AckResponse {
		t.Fatalf("kind: got %d, want %d", r.Kind, AckResponse)
	}
	if r.AckError != "" {
		t.Errorf("ackErr: got %q, want empty", r.AckError)
	}
}

func TestPacketAnalyzerCASBIN(t *testing.T) {
	pa := NewPacketAnalyzer()
	cm := &CASBINMsg{}
	cm.Class = 0x06
	cm.ID = 0x03
	pa.NotifySent(makeRawMsg(cm))
	r := pa.Analyze(gpsreg.TagCASICBin, makeCASBINAckAck(0x06, 0x03))
	if r.Kind != AckResponse {
		t.Fatalf("kind: got %d, want %d", r.Kind, AckResponse)
	}
	if r.AckError != "" {
		t.Errorf("ackErr: got %q, want empty", r.AckError)
	}
}
