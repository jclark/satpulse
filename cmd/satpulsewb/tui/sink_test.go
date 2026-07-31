package tui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/session"
)

func TestSinkCoalescing(t *testing.T) {
	tests := []struct {
		name   string
		emit   []session.Event
		expect []session.Event
	}{
		{
			name: "keep-latest per name",
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
			name: "gps:msg coalesced per kind",
			emit: []session.Event{
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "time", Time: "1"}},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "survey", Time: "2"}},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "time", Time: "3"}},
			},
			expect: []session.Event{
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "time", Time: "3"}},
				{Name: session.EventMsg, Data: session.MsgEvent{Kind: "survey", Time: "2"}},
			},
		},
		{
			name: "stream events all delivered in order",
			emit: []session.Event{
				{Name: session.EventLog, Data: session.LogEvent{Message: "a"}},
				{Name: session.EventLog, Data: session.LogEvent{Message: "b"}},
				{Name: session.EventResponse, Data: session.ResponseEvent{Session: 1}},
			},
			expect: []session.Event{
				{Name: session.EventLog, Data: session.LogEvent{Message: "a"}},
				{Name: session.EventLog, Data: session.LogEvent{Message: "b"}},
				{Name: session.EventResponse, Data: session.ResponseEvent{Session: 1}},
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
			if len(got.packets) != 0 || got.dropped != 0 {
				t.Errorf("unexpected packets %v dropped %d", got.packets, got.dropped)
			}
		})
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
	if len(got.packets) != maxPendingPackets || got.dropped != 10 {
		t.Fatalf("got %d packets, %d dropped, want %d and 10", len(got.packets), got.dropped, maxPendingPackets)
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
