//go:build darwin || freebsd

package term

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type serialErrorState struct{}

func Speed(speed int) AttrSetter {
	b, ok := speedToB(speed)
	if !ok {
		return func(*Attr) error {
			return fmt.Errorf("invalid terminal speed: %d", speed)
		}
	}
	return func(a *Attr) error {
		a.ts.Ospeed = unixspeed(b)
		a.ts.Ispeed = unixspeed(b)
		return nil
	}
}

func (attr *Attr) speed() int {
	return int(attr.ts.Ospeed)
}

// readError is a stub on BSD -- there is no way to detect serial errors
// through the kernel, so Read never returns *Error on these platforms.
func (t *unixTerm) readError() *Error { return nil }

func (t *unixTerm) Flush() error {
	return t.wrapErr(unix.IoctlSetPointerInt(t.fd, unix.TIOCFLUSH, 0), "ioctl(TIOCFLUSH)")
}

func (t *unixTerm) setAttrNow(attr *unix.Termios) error {
	return t.wrapErr(unix.IoctlSetTermios(t.fd, unix.TIOCSETA, attr), "ioctl(TIOCSETA)")
}

// Drain blocks until all pending output has been transmitted. The ioctl
// blocks for the transmit time of the buffered output, and the Go
// runtime's preemption and timer signals interrupt blocking syscalls
// routinely, so EINTR here is runtime noise, not an event: retry.
func (t *unixTerm) Drain() error {
	for {
		err := unix.IoctlSetInt(t.fd, unix.TIOCDRAIN, 0)
		if err != unix.EINTR {
			return t.wrapErr(err, "ioctl(TIOCDRAIN)")
		}
	}
}

func (t *unixTerm) getAttr() (tp *unix.Termios, err error) {
	tp, err = unix.IoctlGetTermios(t.fd, unix.TIOCGETA)
	err = t.wrapErr(err, "ioctl(TIOCGETA)")
	return
}

// checkNotExclusive is a no-op on BSD: there is no TIOCGEXCL, so exclusive mode
// can be set but not queried.
func (t *unixTerm) checkNotExclusive() error { return nil }

var errFlockNotSupported error = unix.ENOTSUP

// OpenFallback opens supported non-TTY devices using OS readiness waiting.
func OpenFallback(path string, timeout time.Duration) (*os.File, *File, DevKind, error) {
	return openFallback(path, timeout, nil, openSelectFIFO)
}

func openSelectFIFO(path string, timeout time.Duration) (*os.File, *File, DevKind, error) {
	f, err := openFile(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, timeout)
	if err != nil {
		return nil, nil, DevUnknown, err
	}
	return nil, f, DevFIFO, nil
}
