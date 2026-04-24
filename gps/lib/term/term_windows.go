package term

import (
	"errors"
	"fmt"
	"time"
)

// This file is a placeholder: the full Win32 implementation described in
// plan/windows-serial.md is not yet written. Everything here compiles but
// returns errNotImplemented at runtime.

var errNotImplemented = errors.New("term: serial I/O not yet implemented on windows")

type Term struct {
	path string
}

type Attr struct{}

type AttrSetter func(*Attr) error

func Open(path string, opts ...AttrSetter) (*Term, error) {
	return nil, fmt.Errorf("%s: %w", path, errNotImplemented)
}

func (t *Term) Change(opts ...AttrSetter) error       { return errNotImplemented }
func (t *Term) Read(buf []byte) (int, error)          { return 0, errNotImplemented }
func (t *Term) Write(buf []byte) (int, error)         { return 0, errNotImplemented }
func (t *Term) Buffered() (int, error)                { return 0, errNotImplemented }
func (t *Term) Flush() error                          { return errNotImplemented }
func (t *Term) Restore() error                        { return errNotImplemented }
func (t *Term) Close() error                          { return nil }
func (t *Term) Speed() int                            { return 0 }
func (t *Term) TransmitTime(nBytes int) time.Duration { return 0 }
func (t *Term) Path() string                          { return t.path }
func (t *Term) DevKind() DevKind                      { return DevUnknown }

func RawMode(a *Attr) error       { return nil }
func Local(a *Attr) error         { return nil }
func NoFlowControl(a *Attr) error { return nil }

func Speed(speed int) AttrSetter {
	return func(a *Attr) error { return nil }
}

func ReadTimeout(timeout time.Duration) AttrSetter {
	return func(a *Attr) error { return nil }
}

func IsValidSpeed(speed int) bool {
	return speed > 0
}
