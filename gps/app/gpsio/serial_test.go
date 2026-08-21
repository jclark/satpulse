package gpsio

import (
	"context"
	"errors"
	"io"
	"net"
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
	if _, _, err := c.WaitModemControlPinChange(context.Background(), ModemCTS, PPSMethodWait); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("WaitModemControlPinChange error = %v, want ErrUnsupported for a terminal that cannot wait", err)
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
}

type fakeWatchResult struct {
	change term.ModemControlPinChange
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

var _ term.ModemControlPinWatch = (*fakePinWatch)(nil)

func newFakePinWatch() *fakePinWatch {
	return &fakePinWatch{
		result:      make(chan fakeWatchResult, 1),
		cancelled:   make(chan struct{}),
		waitStarted: make(chan struct{}),
		closed:      make(chan struct{}),
	}
}

func (w *fakePinWatch) Wait() (term.ModemControlPinChange, int, error) {
	w.mu.Lock()
	w.waits++
	w.mu.Unlock()
	w.startOnce.Do(func() { close(w.waitStarted) })
	select {
	case r := <-w.result:
		return r.change, r.missed, r.err
	case <-w.cancelled:
		return term.ModemControlPinChange{}, 0, term.ErrCancelled
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

func (f *fakeWaitTerm) NewModemControlPinWatch(pin term.ModemControlPin) (term.ModemControlPinWatch, error) {
	f.pin = pin
	return f.watch, nil
}

// slowWaitTerm blocks in the middle of creating a watch, as the kernel method
// does while it waits for udev to open up a new PPS device.
type slowWaitTerm struct {
	fakeTerm
	watch    *fakePinWatch
	creating chan struct{}
	release  chan struct{}
}

var _ term.ModemControlPinWatcher = (*slowWaitTerm)(nil)

func (f *slowWaitTerm) NewModemControlPinWatch(term.ModemControlPin) (term.ModemControlPinWatch, error) {
	close(f.creating)
	<-f.release
	return f.watch, nil
}

func TestSerialConnWaitDoesNotBlockReadsWhileCreatingWatch(t *testing.T) {
	f := &slowWaitTerm{watch: newFakePinWatch(), creating: make(chan struct{}), release: make(chan struct{})}
	c := newSerialConn(f, term.DevUART)
	errCh := make(chan error, 1)
	go func() {
		_, _, err := c.WaitModemControlPinChange(context.Background(), ModemCTS, PPSMethodWait)
		errCh <- err
	}()
	<-f.creating
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read during watch creation = %v, want io.EOF from the fake", err)
	}
	// The watch is created but not yet installed, so Stop has nothing to
	// cancel and the connection must close it instead.
	c.Stop()
	close(f.release)
	if err := <-errCh; !errors.Is(err, net.ErrClosed) {
		t.Fatalf("WaitModemControlPinChange error = %v, want net.ErrClosed", err)
	}
	select {
	case <-f.watch.closed:
	case <-time.After(time.Second):
		t.Error("the watch created during Stop was not closed")
	}
}

func TestSerialConnWaitCapability(t *testing.T) {
	w := newFakePinWatch()
	now := time.Now()
	w.result <- fakeWatchResult{
		change: term.ModemControlPinChange{Timestamp: now, TRead: now, Asserted: true},
		missed: 2,
	}
	f := &fakeWaitTerm{watch: w}
	c := newSerialConn(f, term.DevUSBtoUART)

	change, missed, err := c.WaitModemControlPinChange(context.Background(), ModemCTS, PPSMethodWait)
	if err != nil || change.Timestamp != now || change.TRead != now || !change.Asserted || missed != 2 {
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

	if _, _, err := c.WaitModemControlPinChange(context.Background(), ModemCTS, PPSMethodWait); !errors.Is(err, errors.ErrUnsupported) {
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
		_, _, err := c.WaitModemControlPinChange(ctx, ModemCTS, PPSMethodWait)
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
	if _, _, err := c.WaitModemControlPinChange(context.Background(), ModemCTS, PPSMethodWait); !errors.Is(err, errors.ErrUnsupported) {
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

// fakeKernelTerm is a terminal that also offers the kernel PPS capability.
type fakeKernelTerm struct {
	fakeWaitTerm
	kernelPin term.ModemControlPin
}

var _ term.KernelModemControlPinWatcher = (*fakeKernelTerm)(nil)

func (f *fakeKernelTerm) NewKernelModemControlPinWatch(pin term.ModemControlPin) (term.ModemControlPinWatch, error) {
	f.kernelPin = pin
	return f.watch, nil
}

func TestSerialConnKernelMethod(t *testing.T) {
	w := newFakePinWatch()
	now := time.Now()
	w.result <- fakeWatchResult{
		change: term.ModemControlPinChange{Timestamp: now, TRead: now, Asserted: false},
		missed: 1,
	}
	f := &fakeKernelTerm{fakeWaitTerm: fakeWaitTerm{watch: w}}
	c := newSerialConn(f, term.DevUART)
	change, missed, err := c.WaitModemControlPinChange(context.Background(), ModemDCD, PPSMethodKernel)
	if err != nil || change.Timestamp != now || change.TRead != now || change.Asserted || missed != 1 {
		t.Fatalf("WaitModemControlPinChange = %+v, %d, %v; want supplied change, 1, nil", change, missed, err)
	}
	if f.kernelPin != term.ModemDCD {
		t.Errorf("kernel-watched pin = %v, want DCD", f.kernelPin)
	}
	c.Stop()
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSerialConnKernelMethodUnsupported(t *testing.T) {
	c := newSerialConn(&fakeWaitTerm{watch: newFakePinWatch()}, term.DevUSBtoUART)
	if _, _, err := c.WaitModemControlPinChange(context.Background(), ModemDCD, PPSMethodKernel); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("WaitModemControlPinChange error = %v, want ErrUnsupported", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
