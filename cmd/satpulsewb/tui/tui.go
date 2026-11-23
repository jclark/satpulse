// Package tui implements the terminal UI mode of satpulsewb (--tui):
// a Bubble Tea shell over gps/app/session, covering the same ground
// as the web workbench where a terminal does it well.
package tui

import (
	"io"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/msgfile"
)

// Options configures a TUI run.
type Options struct {
	LogLevel  slog.Level
	Vendors   []gpsreg.Vendor // resolved vendor list for every connect
	PacketLog io.Writer       // optional JSONL packet log (nil to disable)
	MsgDirs   []msgfile.Dir   // message-file library search path
	Device    string          // initial device; auto-connect when Speed is also set
	Speed     int
}

// Run runs the terminal UI until the user quits. The session is used
// in process; slog is routed through session.NewLogHandler with a
// discard base handler so nothing writes to the terminal behind
// Bubble Tea's back (log records reach the UI as gps:log events).
func Run(opts Options) error {
	snk := newSink()
	base := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: opts.LogLevel})
	lg := slog.New(session.NewLogHandler(snk, base))
	sess := session.New(lg, snk, session.Options{PacketLog: opts.PacketLog})
	m := newModel(sess, lg, snk, opts.Vendors, opts.MsgDirs, opts.Device, opts.Speed)
	p := tea.NewProgram(m)
	go snk.run(p)
	_, err := p.Run()
	snk.stop()
	sess.Disconnect()
	return err
}
