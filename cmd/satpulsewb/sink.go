package main

import (
	"sync"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/time/lib/sse"
)

// sseHub implements session.Sink, fanning session events out to SSE
// clients. It caches the latest event per sticky name (and per slow-
// changing gps:msg kind) so a late-joining client starts consistent, and
// tracks whether any client is streaming packets so the session can
// suppress the high-rate gps:packet events (the Wants mechanism).
type sseHub struct {
	mu       sync.Mutex
	clients  map[*sseClient]struct{}
	cache    map[session.EventName]sse.Event
	msgCache map[string]sse.Event // latest gps:msg per stickyMsgKind
	writer   sse.Event            // latest server-broadcast writer event (sticky)
	npackets int
}

// sseClient is one SSE connection. A packets client receives only
// gps:packet events; a regular client receives everything else.
type sseClient struct {
	ch chan sse.Event
	// dead is closed when the client falls too far behind (its buffer
	// overflows); handleSSE then ends the response so the browser
	// EventSource reconnects and re-primes from the cache.
	dead       chan struct{}
	packets    bool
	overflowed bool
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

// stickyMsgKinds are the gps:msg kinds worth priming: slow-changing
// state a reloaded client would otherwise wait a long time to see
// again. The high-rate kinds (pos/vel/time/navEpoch/satellites) are
// not cached; they repopulate within a second of reload.
var stickyMsgKinds = map[string]bool{
	"leapSecond": true,
	"survey":     true,
}

func newSSEHub() *sseHub {
	return &sseHub{
		clients:  make(map[*sseClient]struct{}),
		cache:    make(map[session.EventName]sse.Event),
		msgCache: make(map[string]sse.Event),
	}
}

// Emit implements session.Sink. It must not block: a client that falls
// behind is disconnected (its dead channel is closed) rather than blocked.
func (h *sseHub) Emit(ev session.Event) {
	pkt := ev.Name == session.EventPacket
	msgKind := ""
	if me, ok := ev.Data.(session.MsgEvent); ok {
		msgKind = me.Kind
	}
	cacheMsg := ev.Name == session.EventMsg && stickyMsgKinds[msgKind]
	h.mu.Lock()
	defer h.mu.Unlock()
	// Skip the JSON encode entirely when nothing needs it: no client
	// wants this stream and the event is not one we cache for priming.
	if !stickySet[ev.Name] && !cacheMsg {
		if pkt {
			if h.npackets == 0 {
				return
			}
		} else if len(h.clients)-h.npackets == 0 {
			return
		}
	}
	e, err := sse.Make(string(ev.Name), ev.Data)
	if err != nil {
		return
	}
	if stickySet[ev.Name] {
		h.cache[ev.Name] = e
	}
	if cacheMsg {
		h.msgCache[msgKind] = e
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
		clear(h.msgCache)
	}
	if ce, ok := ev.Data.(session.CorrEvent); ok && (ce.State == "connecting" || ce.State == "stopped") {
		// The base ARP belongs to the active correction stream. Drop the
		// cached one when the stream starts fresh or stops, so a late
		// joiner isn't primed with a stale ARP for a stream that is no
		// longer running (or sends no 1005/1006). Mirrors the frontend.
		delete(h.cache, session.EventBaseARP)
	}
	for c := range h.clients {
		if c.packets != pkt {
			continue
		}
		select {
		case c.ch <- e:
		default:
			if !c.overflowed {
				c.overflowed = true
				close(c.dead)
			}
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
	c := &sseClient{
		ch:      make(chan sse.Event, clientChanSize),
		dead:    make(chan struct{}),
		packets: packets,
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	if packets {
		h.npackets++
		return c, nil
	}
	var prime []sse.Event
	if !h.writer.IsZero() {
		prime = append(prime, h.writer)
	}
	for _, name := range stickyEvents {
		if e, ok := h.cache[name]; ok {
			prime = append(prime, e)
		}
	}
	for _, e := range h.msgCache {
		prime = append(prime, e)
	}
	return c, prime
}

// broadcastWriter caches and fans out the sticky writer event carrying the
// current seat value, which tells each window whether it holds the write seat.
// It originates from the server on every seat claim, not from the session, so
// it lives beside the event cache rather than passing through Emit.
func (h *sseHub) broadcastWriter(seat string) {
	e, err := sse.Make("writer", map[string]string{"seat": seat})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writer = e
	for c := range h.clients {
		if c.packets {
			continue
		}
		select {
		case c.ch <- e:
		default:
			if !c.overflowed {
				c.overflowed = true
				close(c.dead)
			}
		}
	}
}

func (h *sseHub) unsubscribe(c *sseClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	if c.packets {
		h.npackets--
	}
}
