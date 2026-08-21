package tui

import (
	"slices"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/session"
)

// The pending queues are bounded so a stalled render loop (a blocked
// terminal: Ctrl-S, a dead ssh connection, a suspended process)
// cannot grow memory without bound while the session keeps emitting.
// The bounds are far above anything reached while the loop is
// draining normally. This mirrors how the workbench SSE hub treats a
// stalled client: latest-value snapshots survive, the stream tail is
// bounded, and dropped stream events either repopulate within an
// epoch or just undercount.
const (
	maxPendingEvents  = 1024
	maxPendingPackets = 1024
)

// eventsMsg is the tea.Msg delivered for a batch of session events.
// Packets are queued separately from the ordered general events: their
// relative order against other events does not matter for display, and
// keeping them apart lets the packet stream be bounded independently.
type eventsMsg struct {
	events  []session.Event
	packets []session.Event
}

// singletonKey returns the coalescing key for keep-latest events:
// events that carry the complete latest value of one piece of state,
// where the render loop only ever wants the newest. A pending one is
// superseded rather than queued again. Keyed or stream events
// (gps:msg rows keyed by native message ID, gps:basearp per station,
// correction packets, log lines, send progress, responses) return ""
// and are all delivered.
func singletonKey(ev session.Event) string {
	switch name := ev.EventName(); name {
	case session.EventState, session.EventReceiver, session.EventSpeed,
		session.EventTime, session.EventEpochPVT, session.EventInitialPos,
		session.EventNMEAPosition, session.EventCorrections:
		return string(name)
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
	coalesce  map[string]int // singleton key -> index in events
	packets   []session.Event
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
	if ev.EventName() == session.EventPacket {
		if len(s.packets) >= maxPendingPackets {
			s.packets = s.packets[1:]
		}
		s.packets = append(s.packets, ev)
	} else if key := singletonKey(ev); key != "" {
		// Supersede a pending value and append at the end, so the
		// event is delivered at its latest emission position: a
		// consumer must not see it before stream events that were
		// emitted earlier (e.g. a disconnect must not be followed by
		// messages from the dead connection).
		if i, ok := s.coalesce[key]; ok {
			s.removeAt(i)
		}
		s.coalesce[key] = len(s.events)
		s.events = append(s.events, ev)
	} else {
		if len(s.events) >= maxPendingEvents {
			s.dropOldestStream()
		}
		s.events = append(s.events, ev)
	}
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// removeAt removes the pending event at index i, keeping the
// singleton index map consistent.
func (s *sink) removeAt(i int) {
	s.events = slices.Delete(s.events, i, i+1)
	for key, j := range s.coalesce {
		if j > i {
			s.coalesce[key] = j - 1
		}
	}
}

// dropOldestStream drops the oldest pending non-singleton event.
// Singletons are kept: there are at most a handful and each is the
// latest value of its state.
func (s *sink) dropOldestStream() {
	keep := make(map[int]bool, len(s.coalesce))
	for _, i := range s.coalesce {
		keep[i] = true
	}
	for i := range s.events {
		if !keep[i] {
			s.removeAt(i)
			return
		}
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
// (before the program's loop starts, or while the terminal is
// stalled); that only delays this goroutine, never an Emit caller.
func (s *sink) run(p *tea.Program) {
	for {
		select {
		case <-s.done:
			return
		case <-s.notify:
		}
		m := s.takeBatch()
		if len(m.events) > 0 || len(m.packets) > 0 {
			p.Send(m)
		}
	}
}

// takeBatch removes and returns everything queued so far.
func (s *sink) takeBatch() eventsMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := eventsMsg{events: s.events, packets: s.packets}
	s.events = nil
	s.packets = nil
	clear(s.coalesce)
	return m
}

func (s *sink) stop() {
	close(s.done)
}
