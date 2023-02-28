package term

import (
	"fmt"
	"os"
	"time"

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
	tsp, err := t.getAttr()
	if err != nil {
		return
	}
	attr := Attr{*tsp}
	t.tsSaved = *tsp
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
	// this comes from termios(3)
	a.ts.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	a.ts.Oflag &^= unix.OPOST
	a.ts.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	a.ts.Cflag &^= unix.CSIZE | unix.PARENB
	a.ts.Cflag |= unix.CS8
	return nil
}

func Local(a *Attr) error {
	a.ts.Cflag |= unix.CLOCAL
	return nil
}

func speedToB(speed int) (b uint32, ok bool) {
	ok = true
	switch speed {
	// Not supporting 0: that has special semantics
	case 50:
		b = unix.B50
	case 75:
		b = unix.B75
	case 110:
		b = unix.B110
	case 134:
		b = unix.B134
	case 150:
		b = unix.B150
	case 200:
		b = unix.B200
	case 300:
		b = unix.B300
	case 600:
		b = unix.B600
	case 1200:
		b = unix.B1200
	case 1800:
		b = unix.B1800
	case 2400:
		b = unix.B2400
	case 4800:
		b = unix.B4800
	case 9600:
		b = unix.B9600
	case 19200:
		b = unix.B19200
	case 38400:
		b = unix.B38400
	case 57600:
		b = unix.B57600
	case 115200:
		b = unix.B115200
	case 230400:
		b = unix.B230400
	case 460800:
		b = unix.B460800
	case 500000:
		b = unix.B500000
	case 576000:
		b = unix.B576000
	case 921600:
		b = unix.B921600
	case 1000000:
		b = unix.B1000000
	case 1152000:
		b = unix.B1152000
	case 1500000:
		b = unix.B1500000
	case 2000000:
		b = unix.B2000000
	default:
		ok = false
	}
	return
}

func IsValidSpeed(speed int) bool {
	_, ok := speedToB(speed)
	return ok
}

func Speed(speed int) AttrSetter {
	b, ok := speedToB(speed)
	if !ok {
		return func(*Attr) error {
			return fmt.Errorf("invalid terminal speed: %d", speed)
		}
	}
	return func(a *Attr) error {
		a.ts.Cflag &^= unix.CBAUD | unix.CBAUDEX
		a.ts.Cflag |= b
		a.ts.Ospeed = b
		a.ts.Ispeed = b
		return nil
	}
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

func (t *Term) Buffered() (n int, err error) {
	n, err = unix.IoctlGetInt(t.fd, unix.TIOCOUTQ)
	err = t.wrapErr(err, "ioctl(TIOCOUTQ)")
	return
}

func (t *Term) Restore() error {
	return t.setAttr(&t.tsSaved)
}

func (t *Term) Close() error {
	fd := t.fd
	t.fd = -1
	return t.wrapErr(unix.Close(fd), "close")
}

func (t *Term) Flush() error {
	return t.wrapErr(unix.IoctlSetInt(t.fd, unix.TCFLSH, unix.TCIOFLUSH), "ioctl(TCFLSH)")
}

func (t *Term) setAttr(attr *unix.Termios) error {
	return t.wrapErr(unix.IoctlSetTermios(t.fd, unix.TCSETS, attr), "ioctl(TCSETS)")
}

func (t *Term) getAttr() (tp *unix.Termios, err error) {
	tp, err = unix.IoctlGetTermios(t.fd, unix.TCGETS)
	err = t.wrapErr(err, "ioctl(TCGETS)")
	return
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
