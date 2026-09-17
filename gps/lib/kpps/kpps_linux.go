//go:build linux

package kpps

import (
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

// Source is an open kernel PPS source.
type Source struct {
	file   *os.File
	raw    syscall.RawConn
	closed atomic.Bool
}

// Open opens path as a kernel PPS source. It issues PPS_GETCAP to check that
// path is a PPS device, discarding the capabilities it reports. The returned
// Source owns the file descriptor and must be closed by the caller.
func Open(path string) (*Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	raw, err := f.SyscallConn()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	s := &Source{file: f, raw: raw}
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
// for information newer than previous for at most that duration. A negative
// timeout waits indefinitely. Only the assert and clear sequences in previous
// are used to decide whether information is newer.
//
// A waiting fetch reads the current information before sleeping. Poll
// readiness is shared by every descriptor for a PPS source, but the sequences
// are the caller's private baseline, so another reader cannot consume the
// condition this call is waiting for.
//
// Waiting uses the Go runtime poller rather than a blocking PPS_FETCH ioctl.
func (s *Source) Fetch(previous Info, timeout time.Duration) (Info, error) {
	var info Info
	if timeout == 0 {
		err := s.control("ioctl(PPS_FETCH)", func(fd uintptr) error {
			var err error
			info, err = ioctlFetch(fd)
			return err
		})
		return info, err
	}
	if timeout > 0 {
		if err := s.file.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return Info{}, s.wrapErr(err, "set read deadline")
		}
		defer s.file.SetReadDeadline(time.Time{})
	}
	err := waitReadable(s.raw, func(fd uintptr) (bool, error) {
		var err error
		if info, err = ioctlFetch(fd); err != nil {
			return true, err
		}
		return info.Assert.Sequence != previous.Assert.Sequence ||
			info.Clear.Sequence != previous.Clear.Sequence, nil
	})
	if err != nil {
		return Info{}, s.wrapErr(err, "fetch")
	}
	return info, nil
}

// waitReadable uses RawConn.Read to run op after arming the Go runtime poller.
// The initial callback must attempt the operation before deciding to wait;
// returning false reports that there is nothing newer and parks the goroutine
// until the descriptor becomes readable. Subsequent callbacks retry op.
func waitReadable(raw syscall.RawConn, op func(uintptr) (bool, error)) error {
	var opErr error
	if err := raw.Read(func(fd uintptr) bool {
		var done bool
		done, opErr = op(fd)
		return done || opErr != nil
	}); err != nil {
		return err
	}
	return opErr
}

func (s *Source) control(op string, f func(uintptr) error) error {
	var callErr error
	if err := s.raw.Control(func(fd uintptr) {
		callErr = f(fd)
	}); err != nil {
		return s.wrapErr(err, op)
	}
	return s.wrapErr(callErr, op)
}

// Close closes the PPS source. It interrupts a Fetch waiting for new
// information, which then fails with an error wrapping os.ErrClosed.
func (s *Source) Close() error {
	s.closed.Store(true)
	return s.file.Close()
}

// wrapErr reports a failure after Close as os.ErrClosed. RawConn.Read and
// RawConn.Control surface a closed descriptor as the unexported
// internal/poll.ErrFileClosing, which only os.File's own methods translate,
// so the flag rather than the error decides that a failure is a cancellation.
func (s *Source) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	if s.closed.Load() {
		err = os.ErrClosed
	}
	return &os.PathError{Op: op, Path: s.file.Name(), Err: err}
}
