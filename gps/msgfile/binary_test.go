package msgfile

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// makeUBXPacket builds a valid UBX packet from class, id, and payload.
func makeUBXPacket(cls, id byte, payload []byte) string {
	pkt, err := ubxbin.PackMsg(ubxbin.MakeMsgID(cls, id), payload)
	if err != nil {
		panic(err)
	}
	return string(pkt)
}

// makeUBXAckAck builds a UBX ACK-ACK packet echoing the given class/id.
func makeUBXAckAck(cls, id byte) string {
	return makeUBXPacket(0x05, 0x01, []byte{cls, id})
}

// makeUBXAckNak builds a UBX ACK-NAK packet echoing the given class/id.
func makeUBXAckNak(cls, id byte) string {
	return makeUBXPacket(0x05, 0x00, []byte{cls, id})
}

// makeCASBINPacket builds a valid CASBIN packet from class, id, and payload.
func makeCASBINPacket(cls, id byte, payload []byte) string {
	pkt, err := casbin.PackMsg(casbin.MakeMsgID(cls, id), payload)
	if err != nil {
		panic(err)
	}
	return string(pkt)
}

// makeCASBINAckAck builds a CASBIN ACK-ACK packet echoing the given class/id.
func makeCASBINAckAck(cls, id byte) string {
	return makeCASBINPacket(0x05, 0x01, []byte{cls, id, 0, 0})
}

// makeCASBINAckNak builds a CASBIN ACK-NAK packet echoing the given class/id.
func makeCASBINAckNak(cls, id byte) string {
	return makeCASBINPacket(0x05, 0x00, []byte{cls, id, 0, 0})
}

// makeASBINPacket builds a valid ASBIN packet from class, id, and payload.
func makeASBINPacket(cls, id byte, payload []byte) string {
	pkt, err := asbin.PackMsg(asbin.MakeMsgID(cls, id), payload)
	if err != nil {
		panic(err)
	}
	return string(pkt)
}

// makeASBINAckAck builds an ASBIN ACK-ACK packet echoing the given class/id.
func makeASBINAckAck(cls, id byte) string {
	return makeASBINPacket(0x05, 0x01, []byte{cls, id})
}

// makeASBINAckNak builds an ASBIN ACK-NAK packet echoing the given class/id.
func makeASBINAckNak(cls, id byte) string {
	return makeASBINPacket(0x05, 0x00, []byte{cls, id})
}

func TestUBXMatcher(t *testing.T) {
	sentClass, sentID := byte(0x06), byte(0x8A)
	um := &UBXMsg{}
	um.Class = sentClass
	um.ID = sentID
	tests := []struct {
		name     string
		tag      gpsprot.Tag
		data     string
		wantKind ResponseKind
		wantErr  string
	}{
		{
			name:     "ack-ack matching class/id",
			tag:      gpsreg.TagUBX,
			data:     makeUBXAckAck(sentClass, sentID),
			wantKind: AckResponse,
			wantErr:  "",
		},
		{
			name:     "ack-nak matching class/id",
			tag:      gpsreg.TagUBX,
			data:     makeUBXAckNak(sentClass, sentID),
			wantKind: AckResponse,
			wantErr:  AckNak,
		},
		{
			name:     "ack-ack wrong class/id",
			tag:      gpsreg.TagUBX,
			data:     makeUBXAckAck(0x06, 0x01),
			wantKind: NotResponse,
		},
		{
			name:     "same class/id data reply",
			tag:      gpsreg.TagUBX,
			data:     makeUBXPacket(sentClass, sentID, []byte{0x01, 0x02}),
			wantKind: OtherResponse,
		},
		{
			name:     "different class/id",
			tag:      gpsreg.TagUBX,
			data:     makeUBXPacket(0x01, 0x07, []byte{0x00}),
			wantKind: NotResponse,
		},
		{
			name:     "wrong protocol tag",
			tag:      gpsreg.TagCASICBin,
			data:     makeCASBINPacket(sentClass, sentID, nil),
			wantKind: NotResponse,
		},
		{
			name:     "NMEA tag",
			tag:      gpsreg.TagNMEA,
			data:     makeNMEA("GPRMC,,,,,,,,,,,"),
			wantKind: NotResponse,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := um.newMatcher()
			kind, ackErr := m.match(tc.tag, tc.data)
			if kind != tc.wantKind {
				t.Errorf("kind: got %d, want %d", kind, tc.wantKind)
			}
			if kind == AckResponse && ackErr != tc.wantErr {
				t.Errorf("ackErr: got %q, want %q", ackErr, tc.wantErr)
			}
		})
	}
}

func TestCASBINMatcher(t *testing.T) {
	sentClass, sentID := byte(0x06), byte(0x03)
	cm := &CASBINMsg{}
	cm.Class = sentClass
	cm.ID = sentID
	tests := []struct {
		name     string
		tag      gpsprot.Tag
		data     string
		wantKind ResponseKind
		wantErr  string
	}{
		{
			name:     "ack-ack matching",
			tag:      gpsreg.TagCASICBin,
			data:     makeCASBINAckAck(sentClass, sentID),
			wantKind: AckResponse,
			wantErr:  "",
		},
		{
			name:     "ack-nak matching",
			tag:      gpsreg.TagCASICBin,
			data:     makeCASBINAckNak(sentClass, sentID),
			wantKind: AckResponse,
			wantErr:  AckNak,
		},
		{
			name:     "ack-ack wrong class/id",
			tag:      gpsreg.TagCASICBin,
			data:     makeCASBINAckAck(0x06, 0x04),
			wantKind: NotResponse,
		},
		{
			name:     "same class/id poll response",
			tag:      gpsreg.TagCASICBin,
			data:     makeCASBINPacket(sentClass, sentID, []byte{0x01, 0x02, 0x03, 0x04}),
			wantKind: OtherResponse,
		},
		{
			name:     "wrong tag",
			tag:      gpsreg.TagUBX,
			data:     makeUBXAckAck(sentClass, sentID),
			wantKind: NotResponse,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := cm.newMatcher()
			kind, ackErr := m.match(tc.tag, tc.data)
			if kind != tc.wantKind {
				t.Errorf("kind: got %d, want %d", kind, tc.wantKind)
			}
			if kind == AckResponse && ackErr != tc.wantErr {
				t.Errorf("ackErr: got %q, want %q", ackErr, tc.wantErr)
			}
		})
	}
}

func TestASBINMatcher(t *testing.T) {
	sentClass, sentID := byte(0x06), byte(0x02)
	am := &ASBINMsg{}
	am.Class = sentClass
	am.ID = sentID
	tests := []struct {
		name     string
		tag      gpsprot.Tag
		data     string
		wantKind ResponseKind
		wantErr  string
	}{
		{
			name:     "ack-ack matching",
			tag:      gpsreg.TagAllystarBin,
			data:     makeASBINAckAck(sentClass, sentID),
			wantKind: AckResponse,
			wantErr:  "",
		},
		{
			name:     "ack-nak matching",
			tag:      gpsreg.TagAllystarBin,
			data:     makeASBINAckNak(sentClass, sentID),
			wantKind: AckResponse,
			wantErr:  AckNak,
		},
		{
			name:     "wrong tag",
			tag:      gpsreg.TagUBX,
			data:     makeUBXAckAck(sentClass, sentID),
			wantKind: NotResponse,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := am.newMatcher()
			kind, ackErr := m.match(tc.tag, tc.data)
			if kind != tc.wantKind {
				t.Errorf("kind: got %d, want %d", kind, tc.wantKind)
			}
			if kind == AckResponse && ackErr != tc.wantErr {
				t.Errorf("ackErr: got %q, want %q", ackErr, tc.wantErr)
			}
		})
	}
}

func TestBinaryMatcher(t *testing.T) {
	bm := &BinaryMsg{Hex: "B562068A"}
	tests := []struct {
		name     string
		tag      gpsprot.Tag
		data     string
		wantKind ResponseKind
	}{
		{
			name:     "NMEA not a response",
			tag:      gpsreg.TagNMEA,
			data:     makeNMEA("GPRMC,,,,,,,,,,,"),
			wantKind: NotResponse,
		},
		{
			name:     "UNCA not a response",
			tag:      gpsreg.TagUnicoreAscii,
			data:     "#MODE,0;MODE,ROVER*xx\r\n",
			wantKind: NotResponse,
		},
		{
			name:     "NOVA not a response",
			tag:      gpsreg.TagNovAtelAscii,
			data:     "#VERSION,COM1;blah*xx\r\n",
			wantKind: NotResponse,
		},
		{
			name:     "UBX maybe response",
			tag:      gpsreg.TagUBX,
			data:     makeUBXPacket(0x06, 0x8A, nil),
			wantKind: MaybeResponse,
		},
		{
			name:     "unrecognized maybe response",
			tag:      gpsprot.EmptyTag,
			data:     "\x00\x01\x02",
			wantKind: MaybeResponse,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := bm.newMatcher()
			kind, _ := m.match(tc.tag, tc.data)
			if kind != tc.wantKind {
				t.Errorf("kind: got %d, want %d", kind, tc.wantKind)
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
