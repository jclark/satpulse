package msgfile

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// CASIC recv helpers for correlator tests.

type recvCASBINEvent struct{ mid casbin.MsgID }

func recvCASBIN(mid casbin.MsgID) recvCASBINEvent { return recvCASBINEvent{mid: mid} }

func (e recvCASBINEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	pkt, err := casbin.PackMsg(e.mid, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagCASICBin, string(pkt))
}

type recvCASBINAckEvent struct{ mid casbin.MsgID }

func recvCASBINAck(mid casbin.MsgID) recvCASBINAckEvent { return recvCASBINAckEvent{mid: mid} }

func (e recvCASBINAckEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := e.mid.Unpack()
	pkt, err := casbin.PackMsg(casbin.AckAckID, []byte{cls, id, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagCASICBin, string(pkt))
}

type recvCASBINNakEvent struct{ mid casbin.MsgID }

func recvCASBINNak(mid casbin.MsgID) recvCASBINNakEvent { return recvCASBINNakEvent{mid: mid} }

func (e recvCASBINNakEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := e.mid.Unpack()
	pkt, err := casbin.PackMsg(casbin.AckNakID, []byte{cls, id, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagCASICBin, string(pkt))
}

func TestCorrelatorCASBIN(t *testing.T) {
	runCorrelatorTests(t, "casbin-test.toml", []correlatorTest{
		{
			name: "CFG set ACK",
			tags: []string{"set-tp"},
			events: []event{
				sendEvent{},
				recvCASBINAck(casbin.CfgTPID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG set NAK",
			tags: []string{"set-tp"},
			events: []event{
				sendEvent{},
				recvCASBINNak(casbin.CfgTPID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG set no response",
			tags: []string{"set-tp"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "CFG poll ACK then data",
			tags: []string{"get-tp"},
			events: []event{
				sendEvent{},
				recvCASBINAck(casbin.CfgTPID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvCASBIN(casbin.CfgTPID),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG poll data before ACK",
			tags: []string{"get-tp"},
			events: []event{
				sendEvent{},
				recvCASBIN(casbin.CfgTPID),
				expect{relevance: LevelSoleResponse},
				recvCASBINAck(casbin.CfgTPID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG poll NAK no data",
			tags: []string{"get-tp"},
			events: []event{
				sendEvent{},
				recvCASBINNak(casbin.CfgTPID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "non-CFG poll data",
			tags: []string{"poll-status"},
			events: []event{
				sendEvent{},
				recvCASBIN(casbin.NavStatusID),
				expect{relevance: LevelMaybeResponse},
				checkDone{canAcceptMore: true},
			},
		},
		{
			name: "non-CFG poll unrelated packet",
			tags: []string{"poll-status"},
			events: []event{
				sendEvent{},
				recvCASBIN(casbin.CfgTPID),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "CFG-RST silent success",
			tags: []string{"reset"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{},
			},
		},
		{
			name: "CFG-RST NAK on failure",
			tags: []string{"reset"},
			events: []event{
				sendEvent{},
				recvCASBINNak(casbin.CfgRstID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two CFG sets different IDs pacing OK",
			tags: []string{"set-tp", "set-prt"},
			events: []event{
				sendEvent{},
				readyToSend{want: true},
				sendEvent{},
				recvCASBINAck(casbin.CfgTPID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvCASBINAck(casbin.CfgPrtID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two CFG sets same ID pacing blocks",
			tags: []string{"set-tp", "set-tp-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false},
				recvCASBINAck(casbin.CfgTPID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvCASBINAck(casbin.CfgTPID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "ambiguous ACK two pending same ID",
			tags: []string{"set-tp", "set-tp-dup"},
			events: []event{
				sendEvent{},
				sendEvent{},
				recvCASBINAck(casbin.CfgTPID),
				expect{ack: AckNone, relevance: LevelAmbigResponse},
				checkDone{canAcceptMore: true},
			},
		},
		{
			name: "CFG-MSG single poll ACK then data",
			tags: []string{"poll-nav-dop"},
			events: []event{
				sendEvent{},
				recvCASBINAck(casbin.CfgMsgID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvCASBIN(casbin.MakeMsgID(0x01, 0x02)), // NAV-DOP
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG-MSG single poll NAK",
			tags: []string{"poll-nav-dop"},
			events: []event{
				sendEvent{},
				recvCASBINNak(casbin.CfgMsgID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG-MSG set ACK",
			tags: []string{"set-nav-dop"},
			events: []event{
				sendEvent{},
				recvCASBINAck(casbin.CfgMsgID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG-MSG set NAK",
			tags: []string{"set-nav-dop"},
			events: []event{
				sendEvent{},
				recvCASBINNak(casbin.CfgMsgID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG-MSG all-rates ACK then multiple data",
			tags: []string{"get-all-rates"},
			events: []event{
				sendEvent{},
				recvCASBINAck(casbin.CfgMsgID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvCASBIN(casbin.CfgMsgID),
				expect{relevance: LevelMultiResponse},
				recvCASBIN(casbin.CfgMsgID),
				expect{relevance: LevelMultiResponse},
				checkDone{canAcceptMore: true},
			},
		},
		{
			name: "CFG-MSG all-rates NAK",
			tags: []string{"get-all-rates"},
			events: []event{
				sendEvent{},
				recvCASBINNak(casbin.CfgMsgID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
	})
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
			raw, err := ToRaw(msgs, false)
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
