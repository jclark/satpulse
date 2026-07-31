package tui

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/msgfile"
)

func newTestModel() *model {
	snk := newSink()
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	sess := session.New(lg, snk, session.Options{})
	return newModel(sess, lg, snk, nil, nil, "", 0)
}

func events(evs ...session.Event) eventsMsg {
	return eventsMsg{events: evs}
}

func TestModelHeader(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	var info gpsprot.ReceiverInfo
	info.Vendor = "u-blox"
	info.Hardware = "ZED-F9P"
	info.Firmware = "HPG 1.51"
	var re session.ReceiverEvent
	re.OK = true
	re.Info.Set(info)
	m.Update(events(
		session.Event{Name: session.EventState, Data: session.StateConnected},
		session.Event{Name: session.EventReceiver, Data: re},
		session.Event{Name: session.EventSpeed, Data: 38400},
		session.Event{Name: session.EventCorrections, Data: session.CorrEvent{State: "connected"}},
	))
	got := m.renderHeader()
	for _, want := range []string{"connected", "u-blox ZED-F9P", "HPG 1.51", "38400 bps", "corrections connected"} {
		if !strings.Contains(got, want) {
			t.Errorf("header %q does not contain %q", got, want)
		}
	}
}

func TestModelTabs(t *testing.T) {
	m := newTestModel()
	if len(m.tabs()) != 4 {
		t.Fatalf("tabs before catalog = %d, want 4 (no Messages)", len(m.tabs()))
	}
	m.Update(msgCatalogMsg([]msgfile.Entry{{Name: msgfile.Name{Vendor: "u-blox", File: "gen9"}}}))
	tabs := m.tabs()
	if len(tabs) != 5 || tabs[4].title() != "Messages" {
		t.Fatalf("tabs after catalog = %d, want 5 ending in Messages", len(tabs))
	}
}

// TestModelViewSmoke renders the full view for each tab with events
// applied, checking nothing panics and the frame carries the tab bar.
func TestModelViewSmoke(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(events(
		session.Event{Name: session.EventState, Data: session.StateConnected},
		session.Event{Name: session.EventTime, Data: &gpsprot.TimeMsg{}},
		session.Event{Name: session.EventEpochPVT, Data: gpsprot.PVMsgBundle{}},
		session.Event{Name: session.EventLog, Data: session.LogEvent{Level: "INFO", Message: "hello"}},
	))
	for range m.tabs() {
		v := m.View()
		if !strings.Contains(v.Content, "1 Monitor") {
			t.Errorf("view missing tab bar:\n%s", v.Content)
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
}
