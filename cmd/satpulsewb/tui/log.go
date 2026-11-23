package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jclark/satpulse/gps/app/session"
)

// The activity log: a 200-entry ring behind the one-line status. The
// line shows the newest message at or above the severity filter
// (cycled with v), with an indicator cluster showing which severities
// hold messages newer than the one displayed. l opens a transient
// pane over the lower part of the view for browsing the filtered
// history; no colouring by severity, the level word carries it.

const maxLogEntries = 200

var logLevels = []string{"DEBUG", "INFO", "WARN", "ERROR"}

// logLevelIndex maps a LogEvent level to its severity rank; unknown
// levels rank lowest, as in the workbench.
func logLevelIndex(level string) int {
	for i, l := range logLevels {
		if l == level {
			return i
		}
	}
	return 0
}

// logRing holds the retained log entries and the filter state.
type logRing struct {
	entries []session.LogEvent
	filter  int // minimum severity index shown; default INFO
	open    bool
	off     int // pane scroll offset; -1 follows the newest
}

func newLogRing() logRing {
	return logRing{filter: 1, off: -1}
}

func (l *logRing) add(ev session.LogEvent) {
	if len(l.entries) >= maxLogEntries {
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, ev)
}

// cycleFilter steps the minimum severity: ERROR -> WARN -> INFO ->
// DEBUG -> ERROR.
func (l *logRing) cycleFilter() {
	l.filter = (l.filter + len(logLevels) - 1) % len(logLevels)
}

// shown returns the index of the newest entry at or above the filter,
// or -1.
func (l *logRing) shown() int {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if logLevelIndex(l.entries[i].Level) >= l.filter {
			return i
		}
	}
	return -1
}

// filtered returns the indices of the entries at or above the filter.
func (l *logRing) filtered() []int {
	var idx []int
	for i, e := range l.entries {
		if logLevelIndex(e.Level) >= l.filter {
			idx = append(idx, i)
		}
	}
	return idx
}

// statusLine renders the one-line status: the displayed entry on the
// left, the severity indicator cluster on the right. A cluster letter
// carries a * when that severity's newest message is newer than the
// displayed one; the current filter level is bracketed.
func (l *logRing) statusLine(width int) string {
	shown := l.shown()
	cluster := l.cluster(shown)
	left := ""
	if shown >= 0 {
		e := l.entries[shown]
		left = fmt.Sprintf("%-5s %s %s", e.Level, e.Time, e.Message)
	}
	space := width - len(cluster) - 2
	if space < 0 {
		space = 0
	}
	left = truncate(left, space)
	pad := space - lipgloss.Width(left)
	if pad < 0 {
		pad = 0
	}
	return faintStyle.Render(left) + strings.Repeat(" ", pad+2) + faintStyle.Render(cluster)
}

// cluster renders the per-severity indicators for the levels present
// in the ring, plus the current filter level so the bracket showing
// the filter is always visible.
func (l *logRing) cluster(shown int) string {
	newest := make(map[int]int) // level index -> newest entry index
	newest[l.filter] = -1
	for i, e := range l.entries {
		newest[logLevelIndex(e.Level)] = i
	}
	var parts []string
	for _, li := range slices.Sorted(maps.Keys(newest)) {
		s := string(logLevels[li][0])
		if li == l.filter {
			s = "[" + s + "]"
		}
		if newest[li] > shown {
			s += "*"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// renderPane renders the browsing pane: the filtered history, newest
// at the bottom unless scrolled.
func (l *logRing) renderPane(width, height int) []string {
	idx := l.filtered()
	lines := make([]string, 0, len(idx))
	for _, i := range idx {
		e := l.entries[i]
		line := fmt.Sprintf("%s %-5s ", e.Time, e.Level)
		if e.Component != "" {
			line += "[" + e.Component + "] "
		}
		line += e.Message
		for _, k := range slices.Sorted(maps.Keys(e.Attrs)) {
			line += fmt.Sprintf(" %s=%v", k, e.Attrs[k])
		}
		lines = append(lines, truncate(line, width))
	}
	head := labelStyle.Render(truncate(fmt.Sprintf("Log (%s and above, %d entries)  v filter  esc close",
		logLevels[l.filter], len(lines)), width))
	body := height - 1
	if body < 1 {
		return []string{head}
	}
	start := l.off
	if start < 0 || start > len(lines)-body {
		start = len(lines) - body
	}
	if start < 0 {
		start = 0
	}
	end := min(start+body, len(lines))
	if l.off >= 0 {
		l.off = start
	}
	out := make([]string, 0, body+1)
	out = append(out, head)
	out = append(out, lines[start:end]...)
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// scroll moves the pane by delta lines; scrolling to the bottom
// resumes following.
func (l *logRing) scroll(delta, height int) {
	nlines := len(l.filtered())
	body := height - 1
	bottom := nlines - body
	if bottom < 0 {
		bottom = 0
	}
	cur := l.off
	if cur < 0 {
		cur = bottom
	}
	cur += delta
	if cur >= bottom {
		l.off = -1
		return
	}
	if cur < 0 {
		cur = 0
	}
	l.off = cur
}
