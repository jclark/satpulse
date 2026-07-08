package main

import (
	"sync"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/time/lib/sse"
)

// sseHub implements session.Sink, fanning session events out to SSE
// clients. It caches the latest event per sticky name so a late-joining
// client starts consistent, and tracks whether any client is streaming
// packets so the session can suppress the high-rate gps:packet events
// (the Wants mechanism).
type sseHub struct {
	mu       sync.Mutex
	clients  map[*sseClient]struct{}
	cache    map[session.EventName]sse.Event
	npackets int
}

// sseClient is one SSE connection. A packets client receives only
// gps:packet events; a regular client receives everything else.
type sseClient struct {
	ch      chan sse.Event
	packets bool
}

const clientChanSize = 256

// stickyEvents lists the events primed to a new client, in the order
// they are sent. Each reflects current state rather than a transient
// occurrence, so replaying the latest one is meaningful.
var stickyEvents = []session.EventName{
	session.EventState,
	session.EventReceiver,
	session.EventSpeed,
	session.EventCorrections,
	session.EventInitialPos,
	session.EventNMEAPosition,
	session.EventTime,
	session.EventEpochPVT,
	session.EventBaseARP,
}

var stickySet = func() map[session.EventName]bool {
	m := make(map[session.EventName]bool)
	for _, name := range stickyEvents {
		m[name] = true
	}
	return m
}()

func newSSEHub() *sseHub {
	return &sseHub{
		clients: make(map[*sseClient]struct{}),
		cache:   make(map[session.EventName]sse.Event),
	}
}

// Emit implements session.Sink. It must not block: a slow client's
// events are dropped once its channel is full.
func (h *sseHub) Emit(ev session.Event) {
	e, err := sse.Make(string(ev.Name), ev.Data)
	if err != nil {
		return
	}
	pkt := ev.Name == session.EventPacket
	h.mu.Lock()
	defer h.mu.Unlock()
	if stickySet[ev.Name] {
		h.cache[ev.Name] = e
	}
	if ev.Name == session.EventState && ev.Data == session.StateDisconnected {
		// Connection-scoped state is stale once disconnected; a late
		// joiner must not be primed with it. Corrections state carries
		// its own lifecycle events, so it stays.
		for _, name := range stickyEvents {
			if name != session.EventState && name != session.EventCorrections {
				delete(h.cache, name)
			}
		}
	}
	for c := range h.clients {
		if c.packets != pkt {
			continue
		}
		select {
		case c.ch <- e:
		default:
		}
	}
}

// Wants implements session.Sink: gps:packet is wanted only while a
// packets client is connected.
func (h *sseHub) Wants(name session.EventName) bool {
	if name != session.EventPacket {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.npackets > 0
}

// subscribe registers a client and returns it along with the priming
// events, snapshotted atomically with registration so no event is lost
// or misordered between snapshot and live delivery.
func (h *sseHub) subscribe(packets bool) (*sseClient, []sse.Event) {
	c := &sseClient{ch: make(chan sse.Event, clientChanSize), packets: packets}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	if packets {
		h.npackets++
		return c, nil
	}
	var prime []sse.Event
	for _, name := range stickyEvents {
		if e, ok := h.cache[name]; ok {
			prime = append(prime, e)
		}
	}
	return c, prime
}

func (h *sseHub) unsubscribe(c *sseClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	if c.packets {
		h.npackets--
	}
}
