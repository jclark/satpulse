package kpps

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Source is an open kernel PPS source. FreeBSD implements the RFC 2783
// ioctls directly on the serial device, so a Source is created on a TTY
// descriptor rather than opened from a separate PPS device path.
type Source struct {
	file   *os.File
	raw    syscall.RawConn
	closed atomic.Bool
}

// fetchSlice bounds each blocking PPS_FETCH ioctl. The kernel wait cannot be
// interrupted from user space, so a waiting Fetch sleeps in bounded slices
// and rechecks between them; Close makes it fail within one slice.
const fetchSlice = 250 * time.Millisecond

// OpenFD creates a kernel PPS source on the TTY whose descriptor is fd,
// taking ownership of fd even on failure. It issues PPS_IOC_CREATE, checks
// that the device can capture edges and wait for them, and enables capture
// of both edge polarities with PPS_IOC_SETPARAMS. The capture parameters are
// a property of the device, not the descriptor, and are not restored on
// Close.
func OpenFD(fd int, name string) (*Source, error) {
	f := os.NewFile(uintptr(fd), name)
	raw, err := f.SyscallConn()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	s := &Source{file: f, raw: raw}
	if err := s.control("ioctl(PPS_IOC_CREATE)", ioctlCreate); err != nil {
		_ = f.Close()
		return nil, err
	}
	caps, err := s.GetCap()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if caps&(CaptureAssert|CaptureClear) == 0 || caps&ppsCanWait == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("%s: PPS source cannot capture and wait for edges: %w", name, errors.ErrUnsupported)
	}
	if err := s.control("ioctl(PPS_IOC_SETPARAMS)", func(fd uintptr) error {
		return ioctlSetParams(fd, caps&(CaptureAssert|CaptureClear))
	}); err != nil {
		_ = f.Close()
		return nil, err
	}
	return s, nil
}

// GetCap returns the mode bits supported by the source.
func (s *Source) GetCap() (Mode, error) {
	var mode Mode
	err := s.control("ioctl(PPS_IOC_GETCAP)", func(fd uintptr) error {
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
// A waiting fetch reads the current information before sleeping. The kernel
// wait returns only on an edge captured after it is entered, so waiting runs
// in bounded slices with a fresh read before each: an edge that arrives in
// the gap between a read and the slice that follows it is reported when that
// slice ends rather than immediately.
func (s *Source) Fetch(previous Info, timeout time.Duration) (Info, error) {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		var info Info
		err := s.control("ioctl(PPS_FETCH)", func(fd uintptr) error {
			var err error
			info, err = ioctlFetch(fd, 0)
			return err
		})
		if err != nil || timeout == 0 {
			return info, err
		}
		if info.Assert.Sequence != previous.Assert.Sequence ||
			info.Clear.Sequence != previous.Clear.Sequence {
			return info, nil
		}
		slice := fetchSlice
		if timeout > 0 {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return Info{}, s.wrapErr(os.ErrDeadlineExceeded, "ioctl(PPS_FETCH)")
			}
			if remaining < slice {
				slice = remaining
			}
		}
		err = s.control("ioctl(PPS_FETCH)", func(fd uintptr) error {
			var err error
			info, err = ioctlFetch(fd, slice)
			return err
		})
		if err == nil {
			// The slice observed a change, and the sequences only advance, so
			// the information is newer than previous.
			return info, nil
		}
		if !errors.Is(err, unix.ETIMEDOUT) && !errors.Is(err, unix.EINTR) {
			return Info{}, err
		}
	}
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

// Close closes the PPS source. A Fetch waiting for new information fails with
// an error wrapping os.ErrClosed when its current bounded slice ends; Close
// waits for that slice before closing the descriptor.
func (s *Source) Close() error {
	s.closed.Store(true)
	return s.file.Close()
}

// wrapErr reports a failure after Close as os.ErrClosed. RawConn.Control
// surfaces a closed descriptor as the unexported internal/poll.ErrFileClosing,
// which only os.File's own methods translate, so the flag rather than the
// error decides that a failure is a cancellation.
func (s *Source) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	if s.closed.Load() {
		err = os.ErrClosed
	}
	return &os.PathError{Op: op, Path: s.file.Name(), Err: err}
}
