package msgfile

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// UBX recv helpers for correlator tests.

type recvUBXEvent struct{ mid ubxbin.MsgID }

func recvUBX(mid ubxbin.MsgID) recvUBXEvent { return recvUBXEvent{mid: mid} }

func (e recvUBXEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	pkt, err := ubxbin.PackMsg(e.mid, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagUBX, string(pkt))
}

type recvUBXAckEvent struct{ mid ubxbin.MsgID }

func recvUBXAck(mid ubxbin.MsgID) recvUBXAckEvent { return recvUBXAckEvent{mid: mid} }

func (e recvUBXAckEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := e.mid.Unpack()
	pkt, err := ubxbin.PackMsg(ubxbin.AckAckID, []byte{cls, id})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagUBX, string(pkt))
}

type recvUBXNakEvent struct{ mid ubxbin.MsgID }

func recvUBXNak(mid ubxbin.MsgID) recvUBXNakEvent { return recvUBXNakEvent{mid: mid} }

func (e recvUBXNakEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := e.mid.Unpack()
	pkt, err := ubxbin.PackMsg(ubxbin.AckNakID, []byte{cls, id})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagUBX, string(pkt))
}

func TestCorrelatorUBX(t *testing.T) {
	runCorrelatorTests(t, "ubx-test.toml", []correlatorTest{
		{
			name: "CFG set ACK",
			tags: []string{"set-tp5"},
			events: []event{
				sendEvent{},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG set NAK",
			tags: []string{"set-tp5"},
			events: []event{
				sendEvent{},
				recvUBXNak(ubxbin.CfgTp5ID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG set no response",
			tags: []string{"set-tp5"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "CFG poll ACK then data",
			tags: []string{"get-tp5"},
			events: []event{
				sendEvent{},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUBX(ubxbin.CfgTp5ID),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG poll data before ACK",
			tags: []string{"get-tp5"},
			events: []event{
				sendEvent{},
				recvUBX(ubxbin.CfgTp5ID),
				expect{relevance: LevelSoleResponse},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG poll NAK no data",
			tags: []string{"get-tp5"},
			events: []event{
				sendEvent{},
				recvUBXNak(ubxbin.CfgTp5ID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG poll missing ACK and data",
			tags: []string{"get-tp5"},
			events: []event{
				sendEvent{},
				checkMissing{ack: []int{0}, data: []int{0}},
			},
		},
		{
			name: "non-CFG poll data",
			tags: []string{"poll-pvt"},
			events: []event{
				sendEvent{},
				recvUBX(ubxbin.NavPVTID),
				expect{relevance: LevelMaybeResponse},
				checkDone{canAcceptMore: true},
			},
		},
		{
			name: "non-CFG poll unrelated packet",
			tags: []string{"poll-pvt"},
			events: []event{
				sendEvent{},
				recvUBX(ubxbin.NavDOPID),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "poll-only MON-GNSS completes on data",
			tags: []string{"poll-gnss"},
			events: []event{
				sendEvent{},
				recvUBX(ubxbin.MonGnssID),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG-RST no response expected",
			tags: []string{"reset"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: false},
				checkMissing{},
			},
		},
		{
			name: "two CFG sets different IDs pacing OK",
			tags: []string{"set-tp5", "set-prt"},
			events: []event{
				sendEvent{},
				readyToSend{want: true},
				sendEvent{},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUBXAck(ubxbin.CfgPrtID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two CFG sets same ID pacing blocks",
			tags: []string{"set-tp5", "set-tp5-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "ambiguous ACK two pending same ID",
			tags: []string{"set-tp5", "set-tp5-dup"},
			events: []event{
				sendEvent{},
				sendEvent{},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckNone, relevance: LevelAmbigResponse},
				checkDone{canAcceptMore: true},
			},
		},
	})
}

func TestLoadBinaryMsgSimple(t *testing.T) {
	toml := `[[binary]]
hex = "B562068A0900"

[[binary]]
hex = "DEADBEEF"
`
	mf := loadFromString(t, toml)
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
		expected []RawMsg
	}{
		{
			name: "simple binary",
			toml: `[[binary]]
hex = "DEADBEEF"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "hex with whitespace",
			toml: `[[binary]]
hex = "DE AD BE EF"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "with delay",
			toml: `[[binary]]
hex = "B562"
delay = 0.5
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte{0xB5, 0x62}, Delay: 500 * time.Millisecond, Tag: "", Index: 0, Count: 1},
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
			expected: []RawMsg{
				{Bytes: []byte{0xAA}, Delay: 100 * time.Millisecond, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte{0xBB}, Delay: 100 * time.Millisecond, Tag: "", Index: 1, Count: 2},
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

func TestTaggedBinaryMsgs(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []RawMsg
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
			expected: []RawMsg{
				{Bytes: []byte{0xAA}, Delay: 0, Tag: "setup", Index: 0, Count: 1},
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
			expected: []RawMsg{
				{Bytes: []byte{0xAA}, Delay: 0, Tag: "init", Index: 0, Count: 1},
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

func TestCASBINMsgsToRaw(t *testing.T) {
	tests := []struct {
		name        string
		toml        string
		tags        []string
		wantPacket  []byte
		wantDelay   time.Duration
		wantPayload []byte
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
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if len(raw) != 1 {
				t.Fatalf("expected 1 raw message, got %d", len(raw))
			}
			pkt := raw[0].Bytes
			expLen := 10 + len(tc.wantPayload)
			if len(pkt) != expLen {
				t.Errorf("packet length: got %d, expected %d", len(pkt), expLen)
			}
			if pkt[0] != 0xBA || pkt[1] != 0xCE {
				t.Errorf("sync bytes: got %02X %02X, expected BA CE", pkt[0], pkt[1])
			}
			payloadLen := int(pkt[2]) | int(pkt[3])<<8
			if payloadLen != len(tc.wantPayload) {
				t.Errorf("payload length field: got %d, expected %d", payloadLen, len(tc.wantPayload))
			}
			gotPayload := pkt[6 : 6+payloadLen]
			if !reflect.DeepEqual(gotPayload, tc.wantPayload) {
				t.Errorf("payload: got %x, expected %x", gotPayload, tc.wantPayload)
			}
			if raw[0].Delay != tc.wantDelay {
				t.Errorf("delay: got %v, expected %v", raw[0].Delay, tc.wantDelay)
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
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if len(raw) != 1 {
				t.Fatalf("expected 1 raw message, got %d", len(raw))
			}
			pkt := raw[0].Bytes
			expLen := 8 + len(tc.wantPayload)
			if len(pkt) != expLen {
				t.Errorf("packet length: got %d, expected %d", len(pkt), expLen)
			}
			if pkt[0] != 0xB5 || pkt[1] != 0x62 {
				t.Errorf("sync bytes: got %02X %02X, expected B5 62", pkt[0], pkt[1])
			}
			payloadLen := int(pkt[4]) | int(pkt[5])<<8
			if payloadLen != len(tc.wantPayload) {
				t.Errorf("payload length field: got %d, expected %d", payloadLen, len(tc.wantPayload))
			}
			gotPayload := pkt[6 : 6+payloadLen]
			if !reflect.DeepEqual(gotPayload, tc.wantPayload) {
				t.Errorf("payload: got %x, expected %x", gotPayload, tc.wantPayload)
			}
			if raw[0].Delay != tc.wantDelay {
				t.Errorf("delay: got %v, expected %v", raw[0].Delay, tc.wantDelay)
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
	mf := loadFromString(t, toml)
	_, err := mf.TaggedMsgs([]string{"mixed"})
	if err == nil {
		t.Error("expected error for mixed CASBIN and UBX types")
	}
}
