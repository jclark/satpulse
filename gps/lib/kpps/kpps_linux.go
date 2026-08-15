//go:build linux

package kpps

import (
	"os"
	"time"
)

// Source is an open kernel PPS source.
type Source struct {
	file *os.File
	path string
}

// Open opens path as a kernel PPS source. The returned Source owns the file
// descriptor and must be closed by the caller.
func Open(path string) (*Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	s := &Source{file: f, path: path}
	if _, err := s.GetCap(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return s, nil
}

// GetCap returns the mode bits supported by the source.
func (s *Source) GetCap() (Mode, error) {
	var mode Mode
	err := s.control("ioctl(PPS_GETCAP)", func(fd uintptr) error {
		var err error
		mode, err = ioctlGetCap(fd)
		return err
	})
	return mode, err
}

// Fetch returns PPS information from the source. A zero timeout returns the
// most recently captured information immediately. A positive timeout waits
// for new information for at most that duration. A negative timeout waits
// indefinitely.
//
// Waiting uses the Go runtime poller rather than a blocking PPS_FETCH ioctl,
// so it does not require the source to advertise PPS_CANWAIT.
func (s *Source) Fetch(timeout time.Duration) (Info, error) {
	if timeout == 0 {
		return s.fetch()
	}
	if timeout > 0 {
		if err := s.file.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return Info{}, s.wrapErr(err, "set read deadline")
		}
		defer s.file.SetReadDeadline(time.Time{})
	} else {
		if err := s.file.SetReadDeadline(time.Time{}); err != nil {
			return Info{}, s.wrapErr(err, "set read deadline")
		}
	}
	return s.waitFetch()
}

func (s *Source) fetch() (Info, error) {
	var info Info
	err := s.control("ioctl(PPS_FETCH)", func(fd uintptr) error {
		var err error
		info, err = ioctlFetch(fd)
		return err
	})
	return info, err
}

func (s *Source) waitFetch() (Info, error) {
	var info Info
	err := waitReadable(s.file, func(fd uintptr) error {
		var err error
		info, err = ioctlFetch(fd)
		return err
	})
	if err != nil {
		return Info{}, s.wrapErr(err, "fetch")
	}
	return info, nil
}

// waitReadable uses RawConn.Read to park the goroutine in the Go runtime
// poller. The first callback returns false to request a readiness wait; after
// the descriptor becomes readable, the second callback performs op.
func waitReadable(f *os.File, op func(uintptr) error) error {
	raw, err := f.SyscallConn()
	if err != nil {
		return err
	}
	first := true
	var opErr error
	if err := raw.Read(func(fd uintptr) bool {
		if first {
			first = false
			return false
		}
		opErr = op(fd)
		return true
	}); err != nil {
		return err
	}
	return opErr
}

func (s *Source) control(op string, f func(uintptr) error) error {
	raw, err := s.file.SyscallConn()
	if err != nil {
		return s.wrapErr(err, op)
	}
	var callErr error
	if err := raw.Control(func(fd uintptr) {
		callErr = f(fd)
	}); err != nil {
		return s.wrapErr(err, op)
	}
	return s.wrapErr(callErr, op)
}

// Close closes the PPS source. It interrupts a Fetch waiting for new
// information.
func (s *Source) Close() error {
	return s.file.Close()
}

func (s *Source) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{Op: op, Path: s.path, Err: err}
}
