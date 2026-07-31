package tui

import (
	"maps"
	"slices"
	"testing"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
)

func posGeoEvent(id string) session.Event {
	return session.MsgEvent{Kind: "posGeo", Msg: &gpsprot.PosGeoMsg{NativeMsgID: id}}
}

func epochEvent() session.Event {
	return session.EpochPVTEvent{}
}

// TestMonitorRowEviction checks the per-epoch eviction the workbench
// applies: once any row in a table is refreshed in a newer epoch,
// rows not refreshed in that epoch are dropped.
func TestMonitorRowEviction(t *testing.T) {
	tests := []struct {
		name   string
		events []session.Event
		expect []string // surviving position row IDs
	}{
		{
			name:   "both rows survive while both refresh",
			events: []session.Event{posGeoEvent("A"), posGeoEvent("B"), epochEvent()},
			expect: []string{"A", "B"},
		},
		{
			name: "stale row evicted after an epoch without it",
			events: []session.Event{
				posGeoEvent("A"), posGeoEvent("B"), epochEvent(),
				posGeoEvent("A"), epochEvent(),
			},
			expect: []string{"A"},
		},
		{
			name: "no eviction before any row refreshes",
			events: []session.Event{
				posGeoEvent("A"), posGeoEvent("B"), epochEvent(), epochEvent(),
			},
			expect: []string{"A", "B"},
		},
		{
			name: "disconnect clears everything",
			events: []session.Event{
				posGeoEvent("A"), epochEvent(),
				session.StateDisconnected,
			},
			expect: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMonitorView(nil)
			for _, ev := range tc.events {
				m.handleEvent(ev)
			}
			got := slices.Sorted(maps.Keys(m.posRows))
			if !slices.Equal(got, tc.expect) {
				t.Errorf("got  %v\nwant %v", got, tc.expect)
			}
		})
	}
}
