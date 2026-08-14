package gpsio

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/term"
)

type fakeIOFile struct {
	closed bool
}

var _ ioFile = (*fakeIOFile)(nil)

func (f *fakeIOFile) Read([]byte) (int, error) { return 0, io.EOF }

func (f *fakeIOFile) Write(p []byte) (int, error) { return len(p), nil }

func (f *fakeIOFile) Close() error {
	f.closed = true
	return nil
}

func (f *fakeIOFile) Path() string { return "fake" }

func (f *fakeIOFile) Buffered() (int, error) { return 0, nil }

type fakeTerm struct {
	fakeIOFile
	speed       int
	changeCalls int
	restored    bool
}

var _ term.Term = (*fakeTerm)(nil)

func (f *fakeTerm) Change(...term.AttrSetter) error {
	f.changeCalls++
	return nil
}

func (f *fakeTerm) Speed() int { return f.speed }

func (f *fakeTerm) TransmitTime(int) time.Duration { return 0 }

func (f *fakeTerm) DevKind() term.DevKind { return term.DevUnknown }

func (f *fakeTerm) ModemControlPinState() (term.ModemControlPinState, error) {
	return 1 << term.ModemCTS, nil
}

func (f *fakeTerm) Flush() error { return nil }

func (f *fakeTerm) Drain() error { return nil }

func (f *fakeTerm) Restore() error {
	f.restored = true
	return nil
}

func TestSerialConnUsesTermCapability(t *testing.T) {
	f := &fakeTerm{speed: 4800}
	c := newSerialConn(f, term.DevUART)

	if got := c.term(); got != f {
		t.Fatalf("term() = %T, want fake terminal", got)
	}
	if got := c.Speed(); got != 4800 {
		t.Errorf("Speed() = %d, want 4800", got)
	}
	state, err := c.ModemControlPinState()
	if err != nil {
		t.Fatalf("ModemControlPinState: %v", err)
	}
	if !state.Asserted(ModemCTS) {
		t.Error("ModemControlPinState did not report CTS asserted")
	}
	if n, err := c.WriteThenChangeSpeed([]byte("test"), 9600); err != nil || n != 4 {
		t.Fatalf("WriteThenChangeSpeed() = %d, %v; want 4, nil", n, err)
	}
	if f.changeCalls != 1 {
		t.Errorf("Change calls = %d, want 1", f.changeCalls)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !f.restored {
		t.Error("Close did not restore terminal settings")
	}
	if !f.closed {
		t.Error("Close did not close terminal")
	}
	if c.CanWaitModemControlPinChange() {
		t.Error("CanWaitModemControlPinChange() = true for a terminal that cannot wait")
	}
}

type fakeWatchResult struct {
	change term.PinChange
	missed int
	err    error
}

type fakePinWatch struct {
	result      chan fakeWatchResult
	cancelled   chan struct{}
	waitStarted chan struct{}
	closed      chan struct{}
	cancelOnce  sync.Once
	startOnce   sync.Once
	closeOnce   sync.Once
	mu          sync.Mutex
	waits       int
}

var _ term.PinWatch = (*fakePinWatch)(nil)

func newFakePinWatch() *fakePinWatch {
	return &fakePinWatch{
		result:      make(chan fakeWatchResult, 1),
		cancelled:   make(chan struct{}),
		waitStarted: make(chan struct{}),
		closed:      make(chan struct{}),
	}
}

func (w *fakePinWatch) Wait() (term.PinChange, int, error) {
	w.mu.Lock()
	w.waits++
	w.mu.Unlock()
	w.startOnce.Do(func() { close(w.waitStarted) })
	select {
	case r := <-w.result:
		return r.change, r.missed, r.err
	case <-w.cancelled:
		return term.PinChange{}, 0, term.ErrCancelled
	}
}

func (w *fakePinWatch) Cancel() { w.cancelOnce.Do(func() { close(w.cancelled) }) }

func (w *fakePinWatch) Close() error {
	w.closeOnce.Do(func() { close(w.closed) })
	return nil
}

func (w *fakePinWatch) waitCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.waits
}

// fakeWaitTerm is a terminal with the optional wait capability.
type fakeWaitTerm struct {
	fakeTerm
	watch *fakePinWatch
	pin   term.ModemControlPin
}

var _ term.ModemControlPinWatcher = (*fakeWaitTerm)(nil)

func (f *fakeWaitTerm) NewPinWatch(pin term.ModemControlPin) (term.PinWatch, error) {
	f.pin = pin
	return f.watch, nil
}

func TestSerialConnWaitCapability(t *testing.T) {
	w := newFakePinWatch()
	now := time.Now()
	w.result <- fakeWatchResult{
		change: term.PinChange{Wall: now, Mono: now, Asserted: true},
		missed: 2,
	}
	f := &fakeWaitTerm{watch: w}
	c := newSerialConn(f, term.DevUSBtoUART)

	if !c.CanWaitModemControlPinChange() {
		t.Fatal("CanWaitModemControlPinChange() = false, want true")
	}
	change, missed, err := c.WaitModemControlPinChange(context.Background(), ModemCTS)
	if err != nil || change.Wall != now || change.Mono != now || !change.Asserted || missed != 2 {
		t.Fatalf("WaitModemControlPinChange = %+v, %d, %v; want supplied change, 2, nil", change, missed, err)
	}
	if got := w.waitCount(); got != 1 {
		t.Errorf("waits = %d, want 1", got)
	}
	if f.pin != term.ModemCTS {
		t.Errorf("watched pin = %v, want CTS", f.pin)
	}
	c.Stop()
	select {
	case <-w.cancelled:
	default:
		t.Error("Stop did not cancel the pending wait")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-w.closed:
	default:
		t.Error("Close did not close the idle watch")
	}
}

func TestSerialConnWaitUnsupportedClosesWatch(t *testing.T) {
	w := newFakePinWatch()
	w.result <- fakeWatchResult{err: errors.ErrUnsupported}
	c := newSerialConn(&fakeWaitTerm{watch: w}, term.DevUSBtoUART)

	if _, _, err := c.WaitModemControlPinChange(context.Background(), ModemCTS); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("WaitModemControlPinChange error = %v, want ErrUnsupported", err)
	}
	select {
	case <-w.closed:
	default:
		t.Error("unsupported wait did not close the watch")
	}
	if c.watch != nil {
		t.Error("unsupported wait retained the watch")
	}
}

func TestSerialConnWaitContextCancellation(t *testing.T) {
	w := newFakePinWatch()
	c := newSerialConn(&fakeWaitTerm{watch: w}, term.DevUSBtoUART)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := c.WaitModemControlPinChange(ctx, ModemCTS)
		done <- err
	}()
	<-w.waitStarted
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitModemControlPinChange error = %v, want context.Canceled", err)
	}
	select {
	case <-w.cancelled:
	default:
		t.Error("context cancellation did not cancel the watch")
	}
	select {
	case <-w.closed:
	case <-time.After(time.Second):
		t.Error("cancelled wait did not close the watch")
	}
}

func TestSerialConnKeepsIOFileFallbackNonTerminal(t *testing.T) {
	f := new(fakeIOFile)
	c := newSerialConn(f, term.DevUnknown)

	if got := c.term(); got != nil {
		t.Fatalf("term() = %T, want nil", got)
	}
	if got := c.Speed(); got != 0 {
		t.Errorf("Speed() = %d, want 0", got)
	}
	if _, err := c.ModemControlPinState(); !errors.Is(err, term.ErrNotATTY) {
		t.Errorf("ModemControlPinState error = %v, want ErrNotATTY", err)
	}
	if _, _, err := c.WaitModemControlPinChange(context.Background(), ModemCTS); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("WaitModemControlPinChange error = %v, want ErrUnsupported", err)
	}
	if n, err := c.WriteThenChangeSpeed([]byte("test"), 9600); err != nil || n != 4 {
		t.Fatalf("WriteThenChangeSpeed() = %d, %v; want 4, nil", n, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !f.closed {
		t.Error("Close did not close fallback file")
	}
}
