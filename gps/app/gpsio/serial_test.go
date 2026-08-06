package gpsio

import (
	"errors"
	"io"
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

func (f *fakeTerm) ModemControlLineState() (term.ModemControlLineState, error) {
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
	state, err := c.ModemControlLineState()
	if err != nil {
		t.Fatalf("ModemControlLineState: %v", err)
	}
	if !state.Asserted(ModemCTS) {
		t.Error("ModemControlLineState did not report CTS asserted")
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
	if c.CanWaitModemControlLineChange() {
		t.Error("CanWaitModemControlLineChange() = true for a terminal that cannot wait")
	}
}

// fakeWaitTerm is a terminal with the optional wait capability, as the D2XX
// backend has.
type fakeWaitTerm struct {
	fakeTerm
	waits     int
	cancelled bool
}

var _ term.ModemControlLineWaiter = (*fakeWaitTerm)(nil)

func (f *fakeWaitTerm) WaitModemControlLineChange(term.ModemControlLine) error {
	f.waits++
	return nil
}

func (f *fakeWaitTerm) CancelModemControlLineWait() { f.cancelled = true }

func TestSerialConnWaitCapability(t *testing.T) {
	f := new(fakeWaitTerm)
	c := newSerialConn(f, term.DevUSBtoUART)

	if !c.CanWaitModemControlLineChange() {
		t.Fatal("CanWaitModemControlLineChange() = false, want true")
	}
	if err := c.WaitModemControlLineChange(ModemCTS); err != nil {
		t.Fatalf("WaitModemControlLineChange: %v", err)
	}
	if f.waits != 1 {
		t.Errorf("waits = %d, want 1", f.waits)
	}
	c.Stop()
	if !f.cancelled {
		t.Error("Stop did not cancel the pending wait")
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
	if _, err := c.ModemControlLineState(); !errors.Is(err, term.ErrNotATTY) {
		t.Errorf("ModemControlLineState error = %v, want ErrNotATTY", err)
	}
	if err := c.WaitModemControlLineChange(ModemCTS); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("WaitModemControlLineChange error = %v, want ErrUnsupported", err)
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
