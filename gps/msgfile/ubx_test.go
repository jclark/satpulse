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

type recvUBXInfEvent struct{ mid ubxbin.MsgID }

func recvUBXInf(mid ubxbin.MsgID) recvUBXInfEvent { return recvUBXInfEvent{mid: mid} }

func (e recvUBXInfEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	pkt, err := ubxbin.PackMsg(e.mid, []byte("test message"))
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
				recvUBXNak(ubxbin.CfgRstID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
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
		{
			name: "INF shown during pending request",
			tags: []string{"set-tp5"},
			events: []event{
				sendEvent{},
				recvUBXInf(ubxbin.InfWarningID),
				expect{relevance: LevelMaybeResponse},
			},
		},
		{
			name: "INF shown after ACK",
			tags: []string{"set-tp5"},
			events: []event{
				sendEvent{},
				recvUBXAck(ubxbin.CfgTp5ID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUBXInf(ubxbin.InfNoticeID),
				expect{relevance: LevelMaybeResponse},
			},
		},
	})
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
			raw, err := ToRaw(msgs, "", false)
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
