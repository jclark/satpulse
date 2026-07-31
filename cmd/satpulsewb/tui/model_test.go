package tui

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
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
		session.StateConnected,
		re,
		session.SpeedEvent(38400),
		session.CorrEvent{State: "connected"},
	))
	got := m.renderHeader()
	for _, want := range []string{"Connected", "u-blox ZED-F9P (FW HPG 1.51)", "38400 bps", "corrections connected"} {
		if !strings.Contains(got, want) {
			t.Errorf("header %q does not contain %q", got, want)
		}
	}
	m.Update(events(session.StateDisconnected))
	got = m.renderHeader()
	for _, stale := range []string{"u-blox", "38400"} {
		if strings.Contains(got, stale) {
			t.Errorf("header %q still shows %q after disconnect", got, stale)
		}
	}
}

func TestModelTabs(t *testing.T) {
	m := newTestModel()
	tabs := m.tabs()
	if len(tabs) != 5 || tabs[4].title() != "Messages" {
		t.Fatalf("tabs = %d, want 5 ending in Messages", len(tabs))
	}
}

// TestConfigTabGating checks that the Configuration tab cannot be
// activated until the probe identifies the receiver, as in the
// workbench.
func TestConfigTabGating(t *testing.T) {
	m := newTestModel()
	m.conn = nil
	m.Update(events(session.StateConnected))
	configIdx := 3
	if _, ok := m.tabs()[configIdx].(*configView); !ok {
		t.Fatalf("tab %d is not the Configuration view", configIdx)
	}
	m.switchTab(configIdx)
	if m.active == configIdx {
		t.Fatal("Configuration tab activated without an identified receiver")
	}
	var info gpsprot.ReceiverInfo
	info.Vendor = "u-blox"
	var re session.ReceiverEvent
	re.OK = true
	re.Info.Set(info)
	m.Update(events(re))
	m.switchTab(configIdx)
	if m.active != configIdx {
		t.Fatal("Configuration tab refused with an identified receiver")
	}
}

// TestModelViewSmoke renders the full view for each tab with events
// applied, checking nothing panics and the frame carries the tab bar.
func TestModelViewSmoke(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(events(
		session.StateConnected,
		session.TimeEvent{TimeMsg: &gpsprot.TimeMsg{}},
		session.EpochPVTEvent{},
		session.LogEvent{Level: "INFO", Message: "hello"},
	))
	for range m.tabs() {
		v := m.View()
		if !strings.Contains(v.Content, "1 Monitor") {
			t.Errorf("view missing tab bar:\n%s", v.Content)
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
}
