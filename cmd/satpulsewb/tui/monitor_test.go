package tui

import (
	"maps"
	"slices"
	"strings"
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

// TestSignalFilters checks the constellation and used-only filters
// over the signal table.
func TestSignalFilters(t *testing.T) {
	m := newMonitorView(nil)
	m.sats = &gpsprot.SatellitesMsg{
		UsedValidity: gpsprot.SatelliteUsedSV,
		SVs: []gpsprot.SVInfo{
			{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1}, Used: true,
				Signals: []gpsprot.SignalInfo{{ID: "L1", CN0: 40}}},
			{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 2}, Used: false,
				Signals: []gpsprot.SignalInfo{{ID: "E1", CN0: 30}}},
		},
	}
	joined := func() string { return strings.Join(m.renderSignals(100), "\n") }
	if s := joined(); !strings.Contains(s, "GPS") || !strings.Contains(s, "GAL   02") {
		t.Fatalf("unfiltered table missing rows:\n%s", s)
	}
	m.excluded[gpsprot.GAL] = true
	if s := joined(); strings.Contains(s, "GAL   02") {
		t.Errorf("excluded constellation still listed:\n%s", s)
	}
	m.excluded[gpsprot.GAL] = false
	m.usedOnly = true
	if s := joined(); strings.Contains(s, "GAL   02") || !strings.Contains(s, "GPS   01") {
		t.Errorf("used-only filter wrong:\n%s", s)
	}
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
