package term

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jclark/gps4ptp/internal/unix2"
	"golang.org/x/sys/unix"
)

type Term struct {
	fd      int
	path    string
	tsSaved unix.Termios
	iCount  *unix2.SerialICounter
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
	// O_CLOEXEC is here, because we are using flock to lock.
	// Without O_CLOEXEC, the lock would be inherited by child processes, which is probably not what is wanted.
	// See e.g. https://github.com/Pulse-Eight/libcec/issues/477
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
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
	// We could make this optional, but non-exclusive use of the serial port seems like a bad idea.
	err = t.lock()
	if err != nil {
		return
	}
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
	_ = t.GetErrorCounts()
	return
}

func (t *Term) lock() error {
	err := unix.Flock(t.fd, unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		return fmt.Errorf("%s: could not lock device (%w); probably being used by another process", t.path, err)
	}
	return nil
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

const MinTimeout = decisecond
const MaxTimeout = decisecond * 255

func ReadTimeout(timeout time.Duration) AttrSetter {
	return func(a *Attr) error {
		// VTIME is a uint8 in units of 1/10th of a second
		// The semantics of VMIN = 0 and VTIME > 10 is that the read will return 0 if there is no input
		// available after waiting for VTIME deciseconds.
		// If VTIME is 0, the read will return immediately.
		// Since these semantics are different, don't round non-zero timeouts down to 0.
		t := uint8Clamp(int64(timeout.Round(decisecond) / decisecond))
		if t == 0 && timeout > 0 {
			t = 1
		}
		a.ts.Cc[unix.VTIME] = t
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

type ErrorCounts struct {
	FrameErrs, OverrunErrs, ParityErrs, BreakErrs, BufOverrunErrs int32
}

func (c ErrorCounts) IsZero() bool {
	return c.FrameErrs == 0 && c.OverrunErrs == 0 && c.ParityErrs == 0 && c.BreakErrs == 0 && c.BufOverrunErrs == 0
}

func (c ErrorCounts) String() string {
	var s []string = make([]string, 0, 5)
	if c.FrameErrs != 0 {
		s = append(s, fmt.Sprintf("fe=%d", c.FrameErrs))
	}
	if c.OverrunErrs != 0 {
		s = append(s, fmt.Sprintf("oe=%d", c.OverrunErrs))
	}
	if c.ParityErrs != 0 {
		s = append(s, fmt.Sprintf("pe=%d", c.ParityErrs))
	}
	if c.BreakErrs != 0 {
		s = append(s, fmt.Sprintf("brk=%d", c.BreakErrs))
	}
	if c.BufOverrunErrs != 0 {
		s = append(s, fmt.Sprintf("bo=%d", c.BufOverrunErrs))
	}
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, " ")
}

// GetErrorCounts returns counts for serial errors that have occurred since the last call to GetErrorCounts.
// If the serial driver does not provide information about error flags, GetErrorCounts returns a zero value
func (t *Term) GetErrorCounts() (ec ErrorCounts) {
	icNew := unix2.SerialICounter{}
	err := unix2.IoctlGetSerialICounter(t.fd, &icNew)
	if err != nil {
		t.iCount = nil
		return
	}
	if t.iCount == nil {
		t.iCount = &icNew
		return
	}
	icPtr := t.iCount
	ec.FrameErrs = icNew.Frame - icPtr.Frame
	ec.OverrunErrs = icNew.Overrun - icPtr.Overrun
	ec.ParityErrs = icNew.Parity - icPtr.Parity
	ec.BreakErrs = icNew.Brk - icPtr.Brk
	ec.BufOverrunErrs = icNew.Buf_overrun - icPtr.Buf_overrun
	*icPtr = icNew
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

func (t *Term) Path() string {
	return t.path
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
