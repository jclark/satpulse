//go:build !unix && !windows

package term

import (
	"errors"
	"os"
	"time"
)

type Attr struct{}

type AttrSetter func(*Attr) error

// Open always fails: there is no serial terminal on this platform.
func Open(path string, opts ...AttrSetter) (Term, time.Time, error) {
	return nil, time.Time{}, &os.PathError{Op: "open", Path: path, Err: ErrNotATTY}
}

// OpenFallback always fails: there are no serial devices on this platform.
func OpenFallback(path string, _ time.Duration) (*os.File, *File, DevKind, error) {
	return nil, nil, DevUnknown, &os.PathError{Op: "open", Path: path, Err: errors.ErrUnsupported}
}

type File struct{}

func (f *File) Read([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *File) Write([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *File) Close() error {
	return os.ErrInvalid
}

func (f *File) Path() string {
	return ""
}

func (f *File) Buffered() (int, error) {
	return 0, nil
}

func RawMode(*Attr) error { return nil }

func Local(*Attr) error { return nil }

func NoParity(*Attr) error { return nil }

func NoFlowControl(*Attr) error { return nil }

func Speed(int) AttrSetter { return func(*Attr) error { return nil } }

func ReadTimeout(time.Duration) AttrSetter { return func(*Attr) error { return nil } }
