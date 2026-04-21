package term

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// ErrNotATTY is returned when a device does not support termios.
// Callers can check for it with errors.Is.
var ErrNotATTY = errors.New("not a serial device")

type Term struct {
	fd      int
	path    string
	attr    Attr
	tsSaved unix.Termios
	iCount  *SerialICounter
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
	err = lock(fd, path)
	if err != nil {
		return
	}
	tsp, err := t.getAttr()
	if err != nil {
		if errors.Is(err, unix.ENOTTY) {
			err = fmt.Errorf("%s: %w", t.path, ErrNotATTY)
		}
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
	err = t.setAttrNow(&attr.ts)
	t.attr = attr
	_ = t.GetErrorCounts()
	return
}

// Change changes the attributes of the terminal after output has drained.
func (t *Term) Change(opts ...AttrSetter) error {
	attr := t.attr
	for _, opt := range opts {
		err := opt(&attr)
		if err != nil {
			return err
		}
	}
	err := t.setAttrDrain(&attr.ts)
	if err != nil {
		return err
	}
	t.attr = attr
	return nil
}

func (t *Term) Speed() int {
	return t.attr.speed()
}

func lock(fd int, path string) error {
	err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		return fmt.Errorf("%s: could not lock device (%w); probably being used by another process", path, err)
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

// TransmitTime returns the time it takes to send a byte using the settings in Term.
func (t *Term) TransmitTime(nBytes int) time.Duration {
	if nBytes <= 0 {
		return 0
	}
	return t.attr.byteTransmitTime() * time.Duration(nBytes)
}

// byteTransmitTime returns the time it takes to send a byte using the given Termios settings.
func (attr *Attr) byteTransmitTime() time.Duration {
	speed := attr.speed()
	if speed <= 0 {
		return 0
	}
	bits := bitsPerByte(attr.ts)
	// speed is bits per second
	timePerBit := time.Second / time.Duration(speed)
	return time.Duration(bits) * timePerBit
}

// bitsPerByte returns the number of bits on the wire per byte for the given Termios settings.
func bitsPerByte(ts unix.Termios) int {
	bits := 2 // one start bit, one stop bit
	// Adjust bits for character size
	switch ts.Cflag & unix.CSIZE {
	case unix.CS5:
		bits += 5
	case unix.CS6:
		bits += 6
	case unix.CS7:
		bits += 7
	default:
		bits += 8
	}
	// Add bits for parity
	if ts.Cflag&unix.PARENB != 0 {
		bits++
	}
	// Add bits for two stop bits
	if ts.Cflag&unix.CSTOPB != 0 {
		bits++
	}
	return bits
}

func bToSpeed(b uint32) int {
	i := sort.Search(len(baudRates), func(i int) bool {
		return baudRates[i].b >= b
	})
	if i < len(baudRates) && baudRates[i].b == b {
		return baudRates[i].speed
	}
	return -1
}

func speedToB(speed int) (b uint32, ok bool) {
	i := sort.Search(len(baudRates), func(i int) bool {
		return baudRates[i].speed >= speed
	})
	if i < len(baudRates) && baudRates[i].speed == speed {
		return baudRates[i].b, true
	}
	return 0, false
}

func IsValidSpeed(speed int) bool {
	_, ok := speedToB(speed)
	return ok
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

const (
	// in means input to PC from modem; out vice-versa
	MODEM_DCD = unix.TIOCM_CAR // Data carrier detect; pin 1; in
	MODEM_DTR = unix.TIOCM_DTR // Data terminal ready; pin 4; out
	MODEM_DSR = unix.TIOCM_DSR // Data set ready; pin 6; in
	MODEM_RTS = unix.TIOCM_RTS // Request to send; pin 7; out
	MODEM_CTS = unix.TIOCM_CTS // Clear to send; pin 8; in
	MODEM_RI  = unix.TIOCM_RI  // Ring indicator; pin 9; in
)

func (t *Term) ModemStatus() (int, error) {
	status, err := unix.IoctlGetInt(t.fd, unix.TIOCMGET)
	if err != nil {
		return 0, t.wrapErr(err, "ioctl(TIOCMGET)")
	}
	return status, nil
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

func (t *Term) Restore() error {
	return t.setAttrNow(&t.tsSaved)
}

func (t *Term) Close() error {
	fd := t.fd
	t.fd = -1
	return t.wrapErr(unix.Close(fd), "close")
}

func (t *Term) Path() string {
	return t.path
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
