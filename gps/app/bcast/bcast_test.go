package bcast

import (
	"context"
	"log/slog"
	"testing"
)

// Registration is asynchronous. The existing debug messages give tests a
// barrier after Run has processed each request, without inspecting its state
// concurrently or changing the production subscription protocol.
type broadcastTestHandler struct {
	subscribed   chan struct{}
	unsubscribed chan struct{}
}

func (*broadcastTestHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *broadcastTestHandler) Handle(_ context.Context, r slog.Record) error {
	switch r.Message {
	case "received request to subscribe to broadcast data":
		h.subscribed <- struct{}{}
	case "received request to unsubscribe to broadcast data":
		h.unsubscribed <- struct{}{}
	}
	return nil
}

func (h *broadcastTestHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *broadcastTestHandler) WithGroup(string) slog.Handler      { return h }

type broadcastFixture[T any] struct {
	b           *Bcast[T]
	input       chan T
	inputClosed bool
	cancel      context.CancelFunc
	handler     *broadcastTestHandler
	done        chan struct{}
}

func newBroadcastFixture[T any](t *testing.T) *broadcastFixture[T] {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	f := &broadcastFixture[T]{
		input:  make(chan T),
		cancel: cancel,
		handler: &broadcastTestHandler{
			subscribed:   make(chan struct{}, 8),
			unsubscribed: make(chan struct{}, 8),
		},
		done: make(chan struct{}),
	}
	f.b = New((<-chan T)(f.input))
	go func() {
		f.b.Run(ctx, slog.New(f.handler))
		close(f.done)
	}()
	t.Cleanup(func() {
		cancel()
		f.b.Close()
		f.closeInput()
		receiveTestValue(t, f.done)
	})
	return f
}

func (f *broadcastFixture[T]) closeInput() {
	if !f.inputClosed {
		close(f.input)
		f.inputClosed = true
	}
}

func (f *broadcastFixture[T]) subscribe(t *testing.T) <-chan T {
	t.Helper()
	ch := f.b.Subscribe()
	receiveTestValue(t, f.handler.subscribed)
	return ch
}

func TestBroadcastSlowSubscriberKeepsOrderedBacklog(t *testing.T) {
	f := newBroadcastFixture[selectPayload](t)
	slow := f.subscribe(t)
	fast := f.subscribe(t)
	for n := range 24 {
		sendTestValue(t, f.input, makeSelectPayload(n))
		got, ok := receiveTestValue(t, fast)
		if !ok {
			t.Fatal("fast subscriber closed")
		}
		checkSelectPayload(t, got, n)
	}
	for n := range 24 {
		got, ok := receiveTestValue(t, slow)
		if !ok {
			t.Fatal("slow subscriber closed")
		}
		checkSelectPayload(t, got, n)
	}
}

func TestBroadcastUnsubscribeLeavesChannelOpen(t *testing.T) {
	f := newBroadcastFixture[int](t)
	slow := f.subscribe(t)
	fast := f.subscribe(t)
	for n := range 4 {
		sendTestValue(t, f.input, n)
		got, _ := receiveTestValue(t, fast)
		if got != n {
			t.Fatalf("got %d; want %d", got, n)
		}
	}
	f.b.Unsubscribe(slow)
	receiveTestValue(t, f.handler.unsubscribed)
	sendTestValue(t, f.input, 5)
	got, _ := receiveTestValue(t, fast)
	if got != 5 {
		t.Fatalf("got %d after unsubscribe; want 5", got)
	}
	f.b.Unsubscribe(slow) // duplicate removal is harmless
	f.b.Close()
	if _, ok := receiveTestValue(t, fast); ok {
		t.Fatal("active subscriber remained open after Close")
	}
	select {
	case <-slow:
		t.Fatal("unsubscribed channel received a value or was closed")
	default:
	}
}

func TestBroadcastShutdown(t *testing.T) {
	for _, trigger := range []string{"close", "inputEOF", "cancel"} {
		t.Run(trigger, func(t *testing.T) {
			f := newBroadcastFixture[int](t)
			sub := f.subscribe(t)
			// Leave one undelivered message in the shared backlog. Shutdown
			// must abandon it rather than waiting for this subscriber.
			sendTestValue(t, f.input, 1)
			switch trigger {
			case "close":
				f.b.Close()
			case "inputEOF":
				f.closeInput()
			case "cancel":
				f.cancel()
			}
			// The queued value can race with observing the trigger. Wait for
			// closure without assuming which ready case was selected first.
			for {
				if _, ok := receiveTestValue(t, sub); !ok {
					break
				}
			}
			select {
			case <-f.done:
				t.Fatal("Run exited before both Close and input EOF")
			default:
			}
			if _, ok := receiveTestValue(t, f.b.Subscribe()); ok {
				t.Fatal("subscription made during shutdown remained open")
			}
			if trigger != "inputEOF" {
				// Run must keep receiving after delivery stops, so upstream
				// producers can finish and close their input channel.
				for n := range 4 {
					sendTestValue(t, f.input, n)
				}
				f.closeInput()
			}
			f.b.Close()
			f.b.Close()
			receiveTestValue(t, f.done)
		})
	}
}
