package tui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/session"
)

func TestSinkQueueing(t *testing.T) {
	tests := []struct {
		name   string
		emit   []session.Event
		expect []session.Event
	}{
		{
			name: "singletons keep only the latest value",
			emit: []session.Event{
				{Name: session.EventState, Data: session.StateConnecting},
				{Name: session.EventSpeed, Data: 9600},
				{Name: session.EventState, Data: session.StateConnected},
				{Name: session.EventSpeed, Data: 38400},
			},
			expect: []session.Event{
				{Name: session.EventState, Data: session.StateConnected},
				{Name: session.EventSpeed, Data: 38400},
			},
		},
		{
			name: "a superseded singleton moves to its latest emission position",
			emit: []session.Event{
				{Name: session.EventState, Data: session.StateConnected},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "posGeo", Time: "1"}},
				{Name: session.EventState, Data: session.StateDisconnected},
			},
			expect: []session.Event{
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "posGeo", Time: "1"}},
				{Name: session.EventState, Data: session.StateDisconnected},
			},
		},
		{
			name: "gps:msg is not coalesced, even within one kind",
			emit: []session.Event{
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "posGeo", Time: "1"}},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "posGeo", Time: "2"}},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "survey", Time: "3"}},
			},
			expect: []session.Event{
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "posGeo", Time: "1"}},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "posGeo", Time: "2"}},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "survey", Time: "3"}},
			},
		},
		{
			name: "gps:basearp is not coalesced across stations",
			emit: []session.Event{
				{Name: session.EventBaseARP, Data: session.BaseARPEvent{StationID: 1}},
				{Name: session.EventBaseARP, Data: session.BaseARPEvent{StationID: 2}},
			},
			expect: []session.Event{
				{Name: session.EventBaseARP, Data: session.BaseARPEvent{StationID: 1}},
				{Name: session.EventBaseARP, Data: session.BaseARPEvent{StationID: 2}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newSink()
			for _, ev := range tc.emit {
				s.Emit(ev)
			}
			got := s.takeBatch()
			if !reflect.DeepEqual(got.events, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got.events, tc.expect)
			}
			if len(got.packets) != 0 {
				t.Errorf("unexpected packets %v", got.packets)
			}
		})
	}
}

// TestSinkStreamBound checks that the stream queue drops its oldest
// non-singleton entries on overflow while singletons survive.
func TestSinkStreamBound(t *testing.T) {
	s := newSink()
	s.Emit(session.Event{Name: session.EventState, Data: session.StateConnected})
	n := maxPendingEvents + 10
	for i := range n {
		s.Emit(session.Event{Name: session.EventLog,
			Data: session.LogEvent{Message: fmt.Sprintf("m%d", i)}})
	}
	got := s.takeBatch()
	if len(got.events) != maxPendingEvents {
		t.Fatalf("got %d events, want %d", len(got.events), maxPendingEvents)
	}
	if got.events[0].Name != session.EventState {
		t.Errorf("singleton dropped from the front: %+v", got.events[0])
	}
	first := got.events[1].Data.(session.LogEvent)
	if want := fmt.Sprintf("m%d", n-(maxPendingEvents-1)); first.Message != want {
		t.Errorf("oldest surviving log message = %q, want %q", first.Message, want)
	}
	last := got.events[len(got.events)-1].Data.(session.LogEvent)
	if want := fmt.Sprintf("m%d", n-1); last.Message != want {
		t.Errorf("newest log message = %q, want %q", last.Message, want)
	}
}

func TestSinkPacketBound(t *testing.T) {
	s := newSink()
	n := maxPendingPackets + 10
	for i := range n {
		s.Emit(session.Event{Name: session.EventPacket,
			Data: gpsio.PacketLogEntry{Msg: fmt.Sprintf("m%d", i)}})
	}
	got := s.takeBatch()
	if len(got.packets) != maxPendingPackets {
		t.Fatalf("got %d packets, want %d", len(got.packets), maxPendingPackets)
	}
	first := got.packets[0].Data.(gpsio.PacketLogEntry)
	if first.Msg != "m10" {
		t.Errorf("oldest surviving packet = %q, want m10", first.Msg)
	}
	if len(got.events) != 0 {
		t.Errorf("packets leaked into the event queue: %v", got.events)
	}
}

func TestSinkWants(t *testing.T) {
	s := newSink()
	if s.Wants(session.EventPacket) {
		t.Error("packets wanted before the Packets view is active")
	}
	if !s.Wants(session.EventTime) {
		t.Error("gps:time not wanted")
	}
	s.setWantsPackets(true)
	if !s.Wants(session.EventPacket) {
		t.Error("packets not wanted after setWantsPackets(true)")
	}
}
