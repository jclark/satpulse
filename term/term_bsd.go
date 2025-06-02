//go:build darwin || freebsd
package term

import (
	"fmt"
	"golang.org/x/sys/unix"
)

type SerialICounter struct {}

var baudRates = []struct {
	b     uint32
	speed int
}{
	{unix.B50, 50},
	{unix.B75, 75},
	{unix.B110, 110},
	{unix.B134, 134},
	{unix.B150, 150},
	{unix.B200, 200},
	{unix.B300, 300},
	{unix.B600, 600},
	{unix.B1200, 1200},
	{unix.B1800, 1800},
	{unix.B2400, 2400},
	{unix.B4800, 4800},
	{unix.B9600, 9600},
	{unix.B19200, 19200},
	{unix.B38400, 38400},
	{unix.B57600, 57600},
	{unix.B115200, 115200},
	{unix.B230400, 230400},
}

func Speed(speed int) AttrSetter {
	b, ok := speedToB(speed)
	if !ok {
		return func(*Attr) error {
			return fmt.Errorf("invalid terminal speed: %d", speed)
		}
	}
	return func(a *Attr) error {
		a.ts.Ospeed = uint64(b)
		a.ts.Ispeed = uint64(b)
		return nil
	}
}

func (attr *Attr) speed() int {
	return int(attr.ts.Ospeed)
}

func (t *Term) GetErrorCounts() (ec ErrorCounts) {
	return
}

func (t *Term) DevKind() DevKind {
	return DevUnknown
}

func (t *Term) Flush() error {
	return t.wrapErr(unix.IoctlSetInt(t.fd, unix.TIOCFLUSH, unix.TCIOFLUSH), "ioctl(TIOCFLUSH)")
}

func (t *Term) setAttr(attr *unix.Termios) error {
	return t.wrapErr(unix.IoctlSetTermios(t.fd, unix.TIOCSETA, attr), "ioctl(TIOCSETA)")
}

func (t *Term) getAttr() (tp *unix.Termios, err error) {
	tp, err = unix.IoctlGetTermios(t.fd, unix.TIOCGETA)
	err = t.wrapErr(err, "ioctl(TIOCGETA)")
	return
}
