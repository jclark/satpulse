//go:build darwin || freebsd

package term

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

type serialICounter struct{}

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
func (t *Term) readError() *Error { return nil }

func (t *Term) Flush() error {
	return t.wrapErr(unix.IoctlSetPointerInt(t.fd, unix.TIOCFLUSH, 0), "ioctl(TIOCFLUSH)")
}

func (t *Term) setAttrNow(attr *unix.Termios) error {
	return t.wrapErr(unix.IoctlSetTermios(t.fd, unix.TIOCSETA, attr), "ioctl(TIOCSETA)")
}

func (t *Term) setAttrDrain(attr *unix.Termios) error {
	return t.wrapErr(unix.IoctlSetTermios(t.fd, unix.TIOCSETAW, attr), "ioctl(TIOCSETAW)")
}

func (t *Term) getAttr() (tp *unix.Termios, err error) {
	tp, err = unix.IoctlGetTermios(t.fd, unix.TIOCGETA)
	err = t.wrapErr(err, "ioctl(TIOCGETA)")
	return
}

func isLockErrNotTTY(err error) bool {
	return errors.Is(err, unix.ENOTSUP)
}
