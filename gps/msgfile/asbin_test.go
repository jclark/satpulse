package msgfile

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Allystar recv helpers for correlator tests.

// asbinMsgIDUnpack extracts class and ID bytes from an Allystar MsgID.
func asbinMsgIDUnpack(mid asbin.MsgID) (cls, id byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

type recvASBINEvent struct{ mid asbin.MsgID }

func recvASBIN(mid asbin.MsgID) recvASBINEvent { return recvASBINEvent{mid: mid} }

func (e recvASBINEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	pkt, err := asbin.PackMsg(e.mid, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagAllystarBin, string(pkt))
}

type recvASBINAckEvent struct{ mid asbin.MsgID }

func recvASBINAck(mid asbin.MsgID) recvASBINAckEvent { return recvASBINAckEvent{mid: mid} }

func (e recvASBINAckEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := asbinMsgIDUnpack(e.mid)
	pkt, err := asbin.PackMsg(asbin.AckAckID, []byte{cls, id})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagAllystarBin, string(pkt))
}

type recvASBINNakEvent struct{ mid asbin.MsgID }

func recvASBINNak(mid asbin.MsgID) recvASBINNakEvent { return recvASBINNakEvent{mid: mid} }

func (e recvASBINNakEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := asbinMsgIDUnpack(e.mid)
	pkt, err := asbin.PackMsg(asbin.AckNakID, []byte{cls, id})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagAllystarBin, string(pkt))
}

func TestCorrelatorASBIN(t *testing.T) {
	runCorrelatorTests(t, "asbin-test.toml", []correlatorTest{
		{
			name: "CFG set ACK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvASBINAck(asbin.CfgPpsID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG set NAK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvASBINNak(asbin.CfgPpsID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG set no response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "CFG poll ACK then data",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvASBINAck(asbin.CfgPpsID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvASBIN(asbin.CfgPpsID),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG poll data before ACK",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvASBIN(asbin.CfgPpsID),
				expect{relevance: LevelSoleResponse},
				recvASBINAck(asbin.CfgPpsID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG poll NAK no data",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvASBINNak(asbin.CfgPpsID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "non-CFG poll data",
			tags: []string{"poll-dop"},
			events: []event{
				sendEvent{},
				recvASBIN(asbin.NavDopID),
				expect{relevance: LevelMaybeResponse},
				checkDone{canAcceptMore: true},
			},
		},
		{
			name: "non-CFG poll unrelated packet",
			tags: []string{"poll-dop"},
			events: []event{
				sendEvent{},
				recvASBIN(asbin.NavTimeID),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "CFG-SIMPLERST silent success",
			tags: []string{"reset"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{},
			},
		},
		{
			name: "CFG-SIMPLERST NAK on failure",
			tags: []string{"reset"},
			events: []event{
				sendEvent{},
				recvASBINNak(asbin.CfgSimpleRstID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two CFG sets different IDs pacing OK",
			tags: []string{"set-pps", "set-prt"},
			events: []event{
				sendEvent{},
				readyToSend{want: true},
				sendEvent{},
				recvASBINAck(asbin.CfgPpsID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvASBINAck(asbin.CfgPrtID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two CFG sets same ID pacing blocks",
			tags: []string{"set-pps", "set-pps-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false},
				recvASBINAck(asbin.CfgPpsID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvASBINAck(asbin.CfgPpsID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "ambiguous ACK two pending same ID",
			tags: []string{"set-pps", "set-pps-dup"},
			events: []event{
				sendEvent{},
				sendEvent{},
				recvASBINAck(asbin.CfgPpsID),
				expect{ack: AckNone, relevance: LevelAmbigResponse},
				checkDone{canAcceptMore: true},
			},
		},
	})
}
