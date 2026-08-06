package term

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotATTY is returned when a device does not support termios.
// Callers can check for it with errors.Is.
var ErrNotATTY = errors.New("not a serial device")

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

// ModemControlLine identifies a modem control line that is an input to the
// host.
type ModemControlLine int

const (
	ModemCTS ModemControlLine = iota
	ModemDCD
	ModemDSR
	ModemRI
)

// ModemControlLineState is the set of asserted modem control input lines.
// Its representation is independent of the platform's native modem-status
// bits.
type ModemControlLineState int

// Asserted reports whether l is asserted in s.
func (s ModemControlLineState) Asserted(l ModemControlLine) bool {
	if l < ModemCTS || l > ModemRI {
		return false
	}
	return s&(1<<l) != 0
}

func modemControlLineState(asserted ...ModemControlLine) ModemControlLineState {
	var state ModemControlLineState
	for _, line := range asserted {
		state |= 1 << line
	}
	return state
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
