package tui

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/app/session"
)

func logEv(level, msg string) session.LogEvent {
	return session.LogEvent{Level: level, Time: "12:00:00", Message: msg}
}

func TestLogStatusLine(t *testing.T) {
	l := newLogRing()
	l.add(logEv("INFO", "hello"))
	l.add(logEv("WARN", "trouble"))
	l.add(logEv("INFO", "later"))
	s := l.statusLine(100)
	if !strings.Contains(s, "INFO") || !strings.Contains(s, "later") {
		t.Errorf("status at INFO filter = %q, want the newest INFO message", s)
	}
	if !strings.Contains(s, "[I]") {
		t.Errorf("status %q missing the filter bracket", s)
	}
	// Filter to WARN: the newest WARN shows, and the newer INFO is
	// flagged with a star.
	l.filter = 2
	s = l.statusLine(100)
	if !strings.Contains(s, "trouble") {
		t.Errorf("status at WARN filter = %q, want the WARN message", s)
	}
	if !strings.Contains(s, "I*") || !strings.Contains(s, "[W]") {
		t.Errorf("status %q missing I* or [W] indicators", s)
	}
	// Filter to ERROR: nothing shown, but the bracket stays visible.
	l.filter = 3
	s = l.statusLine(100)
	if !strings.Contains(s, "[E]") {
		t.Errorf("status %q missing [E] at an empty filter level", s)
	}
	if strings.Contains(s, "trouble") {
		t.Errorf("status %q shows a message below the filter", s)
	}
}

func TestLogRingBound(t *testing.T) {
	l := newLogRing()
	for range maxLogEntries + 10 {
		l.add(logEv("INFO", "m"))
	}
	if len(l.entries) != maxLogEntries {
		t.Fatalf("ring holds %d entries, want %d", len(l.entries), maxLogEntries)
	}
}
