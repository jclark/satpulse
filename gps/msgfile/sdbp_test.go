package msgfile

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

// SDBP recv helpers for correlator tests.

// sdbpMsgIDUnpack extracts class and ID bytes from an SDBP MsgID.
func sdbpMsgIDUnpack(mid sdbpbin.MsgID) (cls, id byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

type recvSDBPEvent struct{ mid sdbpbin.MsgID }

func recvSDBP(mid sdbpbin.MsgID) recvSDBPEvent { return recvSDBPEvent{mid: mid} }

func (e recvSDBPEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	pkt, err := sdbpbin.PackMsg(e.mid, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagSDBP, string(pkt))
}

type recvSDBPAckEvent struct{ mid sdbpbin.MsgID }

func recvSDBPAck(mid sdbpbin.MsgID) recvSDBPAckEvent { return recvSDBPAckEvent{mid: mid} }

func (e recvSDBPAckEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := sdbpMsgIDUnpack(e.mid)
	pkt, err := sdbpbin.PackMsg(sdbpbin.PubAckID, []byte{cls, id})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagSDBP, string(pkt))
}

type recvSDBPNakEvent struct{ mid sdbpbin.MsgID }

func recvSDBPNak(mid sdbpbin.MsgID) recvSDBPNakEvent { return recvSDBPNakEvent{mid: mid} }

func (e recvSDBPNakEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	cls, id := sdbpMsgIDUnpack(e.mid)
	pkt, err := sdbpbin.PackMsg(sdbpbin.PubNakID, []byte{cls, id})
	if err != nil {
		t.Fatal(err)
	}
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagSDBP, string(pkt))
}

func TestCorrelatorSDBP(t *testing.T) {
	runCorrelatorTests(t, "sdbp-test.toml", []correlatorTest{
		{
			name: "CFG set PubAck",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvSDBPAck(sdbpbin.CfgPPSID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG set PubNak",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvSDBPNak(sdbpbin.CfgPPSID),
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
			name: "CFG query PubAck then data",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvSDBPAck(sdbpbin.CfgPPSID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvSDBP(sdbpbin.CfgPPSID),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG query data before PubAck",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvSDBP(sdbpbin.CfgPPSID),
				expect{relevance: LevelSoleResponse},
				recvSDBPAck(sdbpbin.CfgPPSID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG query PubNak no data",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvSDBPNak(sdbpbin.CfgPPSID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CFG query missing ACK and data",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				checkMissing{ack: []int{0}, data: []int{0}},
			},
		},
		{
			name: "QUE-VER query PubAck then data",
			tags: []string{"get-ver"},
			events: []event{
				sendEvent{},
				recvSDBPAck(sdbpbin.QueVerID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvSDBP(sdbpbin.QueVerID),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CTL-RESTART silent success",
			tags: []string{"restart"},
			events: []event{
				sendEvent{},
				// ExpectAckNakOnly: completion never confirmed on success.
				checkDone{canAcceptMore: true},
				// Not reported as missing (NakOnly is not firm).
				checkMissing{},
			},
		},
		{
			name: "CTL-RESTART PubNak on failure",
			tags: []string{"restart"},
			events: []event{
				sendEvent{},
				recvSDBPNak(sdbpbin.CtlRestartID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CTL-STANDBY silent success",
			tags: []string{"standby"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{},
			},
		},
		{
			name: "CTL-STANDBY PubNak on failure",
			tags: []string{"standby"},
			events: []event{
				sendEvent{},
				recvSDBPNak(sdbpbin.CtlStandbyID),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two CFG sets different IDs pacing OK",
			tags: []string{"set-pps", "set-uart"},
			events: []event{
				sendEvent{},
				readyToSend{want: true},
				sendEvent{},
				recvSDBPAck(sdbpbin.CfgPPSID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvSDBPAck(sdbpbin.CfgUARTID),
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
				recvSDBPAck(sdbpbin.CfgPPSID),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvSDBPAck(sdbpbin.CfgPPSID),
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
				recvSDBPAck(sdbpbin.CfgPPSID),
				expect{ack: AckNone, relevance: LevelAmbigResponse},
				checkDone{canAcceptMore: true},
			},
		},
	})
}
