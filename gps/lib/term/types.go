package term

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Term is a configurable serial terminal.
type Term interface {
	io.ReadWriteCloser

	Path() string
	Buffered() (int, error)
	Change(...AttrSetter) error
	Speed() int
	TransmitTime(int) time.Duration
	DevKind() DevKind
	ModemControlPinState() (ModemControlPinState, error)
	Flush() error
	Drain() error
	Restore() error
}

// ErrNotATTY is returned when a device does not support termios.
// Callers can check for it with errors.Is.
var ErrNotATTY = errors.New("not a serial device")

// ErrCancelled is returned when a pin watch has been cancelled.
// Callers can check for it with errors.Is.
var ErrCancelled = errors.New("pin watch cancelled")

// LockedError indicates that a device is already in exclusive use.
// Callers can check for it with errors.As and then call Locked.
type LockedError interface {
	error
	Locked() bool
}

type lockedError struct {
	err error
}

var _ LockedError = (*lockedError)(nil)

func (e *lockedError) Error() string {
	return "device is locked by another process: " + e.err.Error()
}

func (e *lockedError) Unwrap() error { return e.err }

func (e *lockedError) Locked() bool { return true }

func wrapLocked(err error) error { return &lockedError{err: err} }

type DevKind int

const (
	DevUnknown DevKind = iota
	DevUART
	DevUSB
	DevUSBtoUART
	DevBT
	DevFIFO
)

// ModemControlPin identifies a modem control pin that is an input to the
// host.
type ModemControlPin int

const (
	ModemCTS ModemControlPin = iota
	ModemDCD
	ModemDSR
	ModemRI
)

// ModemControlPinState is the set of asserted modem control input pins.
// Its representation is independent of the platform's native modem-status
// bits.
type ModemControlPinState int

// Asserted reports whether l is asserted in s.
func (s ModemControlPinState) Asserted(l ModemControlPin) bool {
	if l < ModemCTS || l > ModemRI {
		return false
	}
	return s&(1<<l) != 0
}

func modemControlPinState(asserted ...ModemControlPin) ModemControlPinState {
	var state ModemControlPinState
	for _, pin := range asserted {
		state |= 1 << pin
	}
	return state
}

// PinChange reports one observed transition of a modem control input.
type PinChange struct {
	// Wall and Mono are the time the wakeup was observed on two clocks:
	// Wall is the most precise system-time reading the platform offers and
	// may carry no monotonic reading, so it must not be used for elapsed-time
	// arithmetic; Mono is an ordinary time.Now reading, for elapsed time
	// against other time.Now values. Where time.Now is the best clock
	// available they are the same reading. The readings are taken on the
	// waiting thread as soon as the wait ends, before any other call.
	Wall, Mono time.Time
	// Asserted is the pin's sense after the transition that ended the wait,
	// as far as the platform can determine it. Where the platform can tell
	// that the wakeup covered more than one transition it does not report the
	// wakeup at all; where it cannot, this is its best reading.
	Asserted bool
}

// PinWatch observes transitions of one modem control input. It holds a
// descriptor of its own, so it stays usable after the Term is closed and an
// abandoned Wait cannot touch a descriptor number reused after that close.
// Close must not overlap a Wait: the caller must observe Wait's return,
// directly or through a happens-before edge such as a channel receive,
// before calling Close.
type PinWatch interface {
	// Wait blocks until the pin changes; on some platforms it cannot be
	// interrupted, so callers that need cancellation must run it on a
	// goroutine they can abandon. missed counts transitions that could not
	// be reported before this one: edges that arrived while no wait was
	// armed, and edges that made an earlier wakeup ambiguous. It is a lower
	// bound, not a total, and diagnostic only; classification must not depend
	// on it.
	Wait() (c PinChange, missed int, err error)
	// Cancel prevents any further reports and ends a pending Wait as soon as
	// the platform allows. Sticky: once it has fired, every Wait, including
	// one already parked, returns ErrCancelled.
	Cancel()
	// Close releases the watch's descriptor. Calling it while a Wait is
	// pending is a contract violation and panics.
	Close() error
}

// ModemControlPinWatcher is implemented by terminals that can block until a
// modem control input changes. Callers discover the capability by asserting
// for this interface.
type ModemControlPinWatcher interface {
	NewPinWatch(pin ModemControlPin) (PinWatch, error)
}

// Error reports one or more serial errors (framing, parity, overrun, etc.)
// detected by Term.Read.
type Error struct {
	Path  string
	Flags ErrFlags
	// Counts holds per-category counts when the platform reports them
	// (Linux via TIOCGICOUNT); nil on platforms that cannot distinguish
	// beyond Flags.
	Counts *ErrorCounts
}

// ErrFlags is a bitmask of serial error categories reported by Term.Read.
type ErrFlags uint32

const (
	ErrFraming ErrFlags = 1 << iota
	ErrParity
	ErrOverrun
	ErrBreak
	ErrBufOverrun
)

// ErrorCounts holds per-category counts of serial errors observed during
// a single Term.Read.
type ErrorCounts struct {
	Framing, Parity, Overrun, Break, BufOverrun int32
}

func (e *Error) Error() string {
	var detail string
	if e.Counts != nil {
		detail = e.Counts.String()
	} else {
		detail = e.Flags.String()
	}
	return e.Path + ": serial errors: " + detail
}

// Temporary reports that the error does not affect the validity of the
// connection. Always true for a *Error.
func (e *Error) Temporary() bool { return true }

// SerialFraming reports whether the error includes a framing error.
func (e *Error) SerialFraming() bool { return e.Flags&ErrFraming != 0 }

func (c ErrorCounts) String() string {
	var s []string
	if c.Framing != 0 {
		s = append(s, fmt.Sprintf("fe=%d", c.Framing))
	}
	if c.Overrun != 0 {
		s = append(s, fmt.Sprintf("oe=%d", c.Overrun))
	}
	if c.Parity != 0 {
		s = append(s, fmt.Sprintf("pe=%d", c.Parity))
	}
	if c.Break != 0 {
		s = append(s, fmt.Sprintf("brk=%d", c.Break))
	}
	if c.BufOverrun != 0 {
		s = append(s, fmt.Sprintf("bo=%d", c.BufOverrun))
	}
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, " ")
}

func (f ErrFlags) String() string {
	var s []string
	if f&ErrFraming != 0 {
		s = append(s, "framing")
	}
	if f&ErrParity != 0 {
		s = append(s, "parity")
	}
	if f&ErrOverrun != 0 {
		s = append(s, "overrun")
	}
	if f&ErrBreak != 0 {
		s = append(s, "break")
	}
	if f&ErrBufOverrun != 0 {
		s = append(s, "bufoverrun")
	}
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, " ")
}
