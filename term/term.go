package term

import (
	"os"
	"time"

	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
)

type Term struct {
	fd      int
	path    string
	tsSaved unix.Termios
}

type Attr struct {
	ts unix.Termios
}

type AttrSetter func(*Attr) error

func Open(path string, opts ...AttrSetter) (*Term, error) {
	t := new(Term)
	err := t.Init(path, opts...)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Term) Init(path string, opts ...AttrSetter) (err error) {
	t.path = path
	// XXX should open non-blocking and then change to blocking with fcntl
	// (in case CLOCAL is not set)
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		err = t.wrapErr(err, "open")
		return
	}
	t.fd = fd
	defer func() {
		if err != nil {
			unix.Close(fd)
		}
	}()
	attr := Attr{}
	err = termios.Tcgetattr(t.ufd(), &attr.ts)
	if err != nil {
		err = t.wrapErr(err, "tcgetattr")
		return
	}
	t.tsSaved = attr.ts
	for _, opt := range opts {
		err = opt(&attr)
		if err != nil {
			return
		}
	}
	// XXX turn of IXOFF
	err = t.setAttr(&attr.ts)
	return
}

func RawMode(a *Attr) error {
	termios.Cfmakeraw(&a.ts)
	return nil
}

func NoFlowControl(a *Attr) error {
	a.ts.Iflag &^= unix.IXON | unix.IXOFF | unix.IXANY
	a.ts.Cflag &^= unix.CRTSCTS
	return nil
}

// A tenth of a second
const decisecond = time.Second / 10

func ReadTimeout(timeout time.Duration) AttrSetter {
	return func(a *Attr) error {
		// VTIME is a uint8 in units of 1/10th of a second
		a.ts.Cc[unix.VTIME] = uint8Clamp(int64(timeout.Round(decisecond) / decisecond))
		a.ts.Cc[unix.VMIN] = 0
		return nil
	}
}

func uint8Clamp(v int64) uint8 {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return uint8(v)
}

func (t *Term) ufd() uintptr {
	return uintptr(t.fd)
}

func (t *Term) Read(buf []byte) (n int, err error) {
	for {
		n, err = unix.Read(t.fd, buf)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		err = t.wrapErr(err, "read")
	}
	return
}

func (t *Term) Write(buf []byte) (int, error) {
	total := 0
	for len(buf) > 0 {
		// Semantics of Unix write and Go Write are not the same:
		// Unix can write less than requested amount without its being an error.
		n, err := unix.Write(t.fd, buf)
		if err == nil {
			total += n
			buf = buf[n:]
		} else if err != unix.EINTR {
			return total, t.wrapErr(err, "write")
		}
	}
	return total, nil
}

func (t *Term) Buffered() (int, error) {
	return termios.Tiocoutq(t.ufd())
}

func (t *Term) Restore() error {
	return t.setAttr(&t.tsSaved)
}

func (t *Term) Close() error {
	fd := int(t.fd)
	t.fd = -1
	return t.wrapErr(unix.Close(fd), "close")
}

func (t *Term) Flush() error {
	return t.wrapErr(termios.Tcflush(t.ufd(), unix.TCIOFLUSH), "tcflush")
}

func (t *Term) setAttr(attr *unix.Termios) error {
	return t.wrapErr(termios.Tcsetattr(t.ufd(), termios.TCSANOW, attr), "tcsetattr")
}

func (t *Term) wrapErr(err error, op string) error {
	if err == nil {
		return err
	}
	return &os.PathError{
		Op:   op,
		Path: t.path,
		Err:  err,
	}
}

type DevKind int

const (
	DevUnknown DevKind = iota
	DevUART
	DevUSB
	DevUSBtoUART
	DevBT
)

func (t *Term) DevKind() DevKind {
	s := unix.Stat_t{}
	err := unix.Fstat(t.fd, &s)
	if err != nil {
		return DevUnknown
	}
	// See https://www.kernel.org/doc/html/latest/admin-guide/devices.html
	switch unix.Major(s.Dev) {
	case 4, 5:
		if unix.Minor(s.Dev) >= 64 { // ttyS0, /dev/ttycua0
			return DevUART
		}
	case 166, 167: // USB ACM "modem" /dev/ttyACM0
		return DevUSB
	case 188, 189: // USB serial converter /dev/ttyUSB0
		return DevUSBtoUART
	case 204, 205: // low-density serial port (Raspberry Pi uses /dev/ttyAMA0)
		return DevUART
	case 216, 217: // Bluetooth RFCOMM /dev/rfcomm0
		return DevBT
	}
	return DevUnknown
}
