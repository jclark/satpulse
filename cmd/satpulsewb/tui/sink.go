package tui

import (
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/session"
)

// maxPendingPackets bounds the pending gps:packet queue; beyond it the
// oldest pending packets are dropped and counted, so a stalled render
// loop cannot grow memory without bound.
const maxPendingPackets = 1024

// eventsMsg is the tea.Msg delivered for a batch of session events.
// Packets are queued separately from the ordered general events: their
// relative order against other events does not matter for display, and
// keeping them apart lets the packet stream be bounded independently.
type eventsMsg struct {
	events  []session.Event
	packets []session.Event
	dropped int // packets dropped from the pending queue
}

// coalesced returns the coalescing key for keep-latest events: the
// render loop only ever wants the newest snapshot of these, so a
// pending one is replaced in place instead of queued again. Stream
// events (packets, correction packets, log lines, send progress,
// responses) return "" and are all delivered.
func coalesced(ev session.Event) string {
	switch ev.Name {
	case session.EventState, session.EventReceiver, session.EventSpeed,
		session.EventTime, session.EventEpochPVT, session.EventInitialPos,
		session.EventNMEAPosition, session.EventCorrections, session.EventBaseARP:
		return string(ev.Name)
	case session.EventMsg:
		if me, ok := ev.Data.(session.MsgEvent); ok {
			return string(ev.Name) + "/" + me.Kind
		}
	}
	return ""
}

// sink implements session.Sink for the TUI. Emit never blocks: events
// are queued under a mutex and a drain goroutine delivers them to the
// Bubble Tea program with Program.Send, batched per wakeup.
type sink struct {
	packetsOn atomic.Bool
	mu        sync.Mutex
	events    []session.Event
	coalesce  map[string]int // coalescing key -> index in events
	packets   []session.Event
	dropped   int
	notify    chan struct{} // cap 1; signals the drain goroutine
	done      chan struct{}
}

func newSink() *sink {
	return &sink{
		coalesce: make(map[string]int),
		notify:   make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
}

// Emit implements session.Sink.
func (s *sink) Emit(ev session.Event) {
	s.mu.Lock()
	if ev.Name == session.EventPacket {
		if len(s.packets) >= maxPendingPackets {
			s.packets = s.packets[1:]
			s.dropped++
		}
		s.packets = append(s.packets, ev)
	} else if key := coalesced(ev); key != "" {
		if i, ok := s.coalesce[key]; ok {
			s.events[i] = ev
		} else {
			s.coalesce[key] = len(s.events)
			s.events = append(s.events, ev)
		}
	} else {
		s.events = append(s.events, ev)
	}
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Wants implements session.Sink: gps:packet only while the Packets
// view is streaming, mirroring how the workbench gates packet
// streaming on visibility.
func (s *sink) Wants(name session.EventName) bool {
	if name == session.EventPacket {
		return s.packetsOn.Load()
	}
	return true
}

// setWantsPackets turns the gps:packet stream on or off.
func (s *sink) setWantsPackets(on bool) {
	s.packetsOn.Store(on)
}

// run delivers queued events to p until stop is called. Send may block
// briefly (e.g. before the program's loop starts); that only delays
// this goroutine, never an Emit caller.
func (s *sink) run(p *tea.Program) {
	for {
		select {
		case <-s.done:
			return
		case <-s.notify:
		}
		m := s.takeBatch()
		if len(m.events) > 0 || len(m.packets) > 0 || m.dropped > 0 {
			p.Send(m)
		}
	}
}

// takeBatch removes and returns everything queued so far.
func (s *sink) takeBatch() eventsMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := eventsMsg{events: s.events, packets: s.packets, dropped: s.dropped}
	s.events = nil
	s.packets = nil
	s.dropped = 0
	clear(s.coalesce)
	return m
}

func (s *sink) stop() {
	close(s.done)
}
