package msgfile

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
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
			name: "CFG-SIMPLERST no response expected",
			tags: []string{"reset"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: false},
				checkMissing{},
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
