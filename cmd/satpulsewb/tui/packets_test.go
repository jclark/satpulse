package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
)

func pktE(t time.Time, tag, msg string, out bool) session.Event {
	return session.Event{Name: session.EventPacket, Data: gpsio.PacketLogEntry{
		T: gpsio.TimeMicro(t), Tag: gpsprot.Tag(tag), Msg: msg, Out: out,
	}}
}

func TestPacketAggregation(t *testing.T) {
	t0 := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	type rowState struct {
		count  int
		recent int
	}
	tests := []struct {
		name   string
		events []session.Event
		expect map[string]rowState
	}{
		{
			name: "one row per protocol, message, and direction",
			events: []session.Event{
				pktE(t0, "UBX", "NAV-PVT", false),
				pktE(t0.Add(10*time.Millisecond), "UBX", "NAV-PVT", false),
				pktE(t0.Add(20*time.Millisecond), "UBX", "NAV-SAT", false),
				pktE(t0.Add(30*time.Millisecond), "UBX", "NAV-PVT", true),
			},
			expect: map[string]rowState{
				"UBX:NAV-PVT:i": {count: 2, recent: 2},
				"UBX:NAV-SAT:i": {count: 1, recent: 1},
				"UBX:NAV-PVT:o": {count: 1, recent: 1},
			},
		},
		{
			name: "burst pruning keeps the count but drops old entries",
			events: []session.Event{
				pktE(t0, "UBX", "NAV-PVT", false),
				pktE(t0.Add(500*time.Millisecond), "UBX", "NAV-PVT", false),
				pktE(t0.Add(2*time.Second), "UBX", "NAV-PVT", false),
			},
			expect: map[string]rowState{
				"UBX:NAV-PVT:i": {count: 3, recent: 1},
			},
		},
		{
			name: "a new connection clears the rows",
			events: []session.Event{
				pktE(t0, "UBX", "NAV-PVT", false),
				{Name: session.EventState, Data: session.StateDisconnected},
				{Name: session.EventState, Data: session.StateConnecting},
				pktE(t0.Add(5*time.Second), "NMEA", "GGA", false),
			},
			expect: map[string]rowState{
				"NMEA:GGA:i": {count: 1, recent: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &packetsView{live: make(map[string]*pktRow), expanded: make(map[string]bool),
				sel: pktItem{child: -1}, prevState: session.StateConnected}
			for _, ev := range tc.events {
				v.handleEvent(ev)
			}
			got := make(map[string]rowState)
			for key, r := range v.live {
				got[key] = rowState{count: r.count, recent: len(r.recent)}
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestPacketSortOrder(t *testing.T) {
	t0 := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	v := &packetsView{live: make(map[string]*pktRow), expanded: make(map[string]bool),
		sel: pktItem{child: -1}}
	for _, ev := range []session.Event{
		pktE(t0, "UBX", "NAV-SAT", false),
		pktE(t0, "NMEA", "GGA", false),
		pktE(t0, "UBX", "NAV-PVT", false),
		pktE(t0, "RTCM", "1005", true),
	} {
		v.handleEvent(ev)
	}
	expect := []string{"RTCM:1005:o", "NMEA:GGA:i", "UBX:NAV-PVT:i", "UBX:NAV-SAT:i"}
	if got := v.sortedKeys(); !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %v\nwant %v", got, expect)
	}
}
