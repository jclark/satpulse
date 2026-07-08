package main

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/time/lib/sse"
)

func TestHubPrimeCache(t *testing.T) {
	tests := []struct {
		name   string
		events []session.Event
		expect []sse.Event
	}{
		{
			name:   "empty",
			events: nil,
			expect: nil,
		},
		{
			name: "sticky cached in fixed order",
			events: []session.Event{
				{Name: session.EventSpeed, Data: 9600},
				{Name: session.EventState, Data: session.StateConnected},
			},
			expect: []sse.Event{
				mustMake(t, "gps:state", session.StateConnected),
				mustMake(t, "gps:speed", 9600),
			},
		},
		{
			name: "latest event wins",
			events: []session.Event{
				{Name: session.EventSpeed, Data: 9600},
				{Name: session.EventSpeed, Data: 115200},
			},
			expect: []sse.Event{mustMake(t, "gps:speed", 115200)},
		},
		{
			name: "transient events not cached",
			events: []session.Event{
				{Name: session.EventLog, Data: session.LogEvent{Message: "x"}},
				{Name: session.EventPacket, Data: struct{}{}},
			},
			expect: nil,
		},
		{
			name: "disconnect clears connection-scoped entries",
			events: []session.Event{
				{Name: session.EventSpeed, Data: 9600},
				{Name: session.EventCorrections, Data: session.CorrEvent{State: "stopped"}},
				{Name: session.EventState, Data: session.StateDisconnected},
			},
			expect: []sse.Event{
				mustMake(t, "gps:state", session.StateDisconnected),
				mustMake(t, "gps:corrections", session.CorrEvent{State: "stopped"}),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newSSEHub()
			for _, ev := range tc.events {
				h.Emit(ev)
			}
			c, prime := h.subscribe(false)
			defer h.unsubscribe(c)
			if !reflect.DeepEqual(prime, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", prime, tc.expect)
			}
		})
	}
}

func mustMake(t *testing.T, name string, data any) sse.Event {
	t.Helper()
	e, err := sse.Make(name, data)
	if err != nil {
		t.Fatalf("sse.Make: %v", err)
	}
	return e
}

func TestHubWantsPackets(t *testing.T) {
	h := newSSEHub()
	if h.Wants(session.EventPacket) {
		t.Errorf("Wants(gps:packet) true with no packet client")
	}
	if !h.Wants(session.EventState) {
		t.Errorf("Wants(gps:state) false")
	}
	regular, _ := h.subscribe(false)
	if h.Wants(session.EventPacket) {
		t.Errorf("Wants(gps:packet) true with only a regular client")
	}
	pc, _ := h.subscribe(true)
	if !h.Wants(session.EventPacket) {
		t.Errorf("Wants(gps:packet) false with a packet client")
	}
	h.unsubscribe(pc)
	if h.Wants(session.EventPacket) {
		t.Errorf("Wants(gps:packet) true after packet client unsubscribed")
	}
	h.unsubscribe(regular)
}

func TestHubPacketRouting(t *testing.T) {
	h := newSSEHub()
	regular, _ := h.subscribe(false)
	packets, _ := h.subscribe(true)
	h.Emit(session.Event{Name: session.EventPacket, Data: struct{}{}})
	h.Emit(session.Event{Name: session.EventState, Data: session.StateConnected})
	expectRegular := []sse.Event{mustMake(t, "gps:state", session.StateConnected)}
	expectPackets := []sse.Event{mustMake(t, "gps:packet", struct{}{})}
	if got := drain(regular.ch); !reflect.DeepEqual(got, expectRegular) {
		t.Errorf("regular client got %+v\nwant %+v", got, expectRegular)
	}
	if got := drain(packets.ch); !reflect.DeepEqual(got, expectPackets) {
		t.Errorf("packet client got %+v\nwant %+v", got, expectPackets)
	}
}

func drain(ch chan sse.Event) []sse.Event {
	var evs []sse.Event
	for {
		select {
		case e := <-ch:
			evs = append(evs, e)
		default:
			return evs
		}
	}
}
