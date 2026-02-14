package as

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

type testMsgHandler struct {
	gpsprot.DefaultHandler
	msgs []struct {
		msgType string
		msg     any
		tRead   time.Time
	}
}

func (h *testMsgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     any
		tRead   time.Time
	}{"time", msg, tRead})
}

func (h *testMsgHandler) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     any
		tRead   time.Time
	}{"navepoch", msg, tRead})
}

func TestNavEpochMsg(t *testing.T) {
	pp := NewPacketProcessor()
	handler := &testMsgHandler{}
	pp.SetMsgHandler(handler)

	// Send messages for epoch 1 - no NavEpochMsg should be emitted
	msg := &asbin.NavTimeUTC{NavITOW: asbin.NavITOW{ITow: 100000}}
	msg.ValidFlag = asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid
	msg.Year = 2026
	msg.Month = 2
	msg.Day = 14
	msg.Sec = 1
	packet, err := asbin.Serialize(msg)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	if _, err := pp.ProcessPacket(string(packet), time.Unix(1, 0)); err != nil {
		t.Fatalf("failed to process: %v", err)
	}
	for _, m := range handler.msgs {
		if m.msgType == "navepoch" {
			t.Fatal("NavEpochMsg emitted before first epoch boundary")
		}
	}

	// Send first message of epoch 2 - should trigger NavEpochMsg for epoch 1
	msg2 := &asbin.NavTimeUTC{NavITOW: asbin.NavITOW{ITow: 200000}}
	msg2.ValidFlag = asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid
	msg2.Year = 2026
	msg2.Month = 2
	msg2.Day = 14
	msg2.Sec = 2
	packet2, err := asbin.Serialize(msg2)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	beforeCount := len(handler.msgs)
	if _, err := pp.ProcessPacket(string(packet2), time.Unix(2, 0)); err != nil {
		t.Fatalf("failed to process: %v", err)
	}
	var epochMsg *gpsprot.NavEpochMsg
	var epochTRead time.Time
	for _, m := range handler.msgs[beforeCount:] {
		if m.msgType == "navepoch" {
			epochMsg = m.msg.(*gpsprot.NavEpochMsg)
			epochTRead = m.tRead
		}
	}
	if epochMsg == nil {
		t.Fatal("no NavEpochMsg emitted at epoch boundary")
	}
	if epochMsg.Tag != Tag {
		t.Fatalf("NavEpochMsg.Tag = %q, want %q", epochMsg.Tag, Tag)
	}
	if epochMsg.StartTime != time.Unix(1, 0) {
		t.Fatalf("NavEpochMsg.StartTime = %v, want %v", epochMsg.StartTime, time.Unix(1, 0))
	}
	if epochTRead != time.Unix(2, 0) {
		t.Fatalf("NavEpochMsg tRead = %v, want %v", epochTRead, time.Unix(2, 0))
	}
}
