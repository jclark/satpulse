package term

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type Term struct {
	fd               int
	path             string
	byteTransmitTime time.Duration
	speed            int
	tsSaved          unix.Termios
	iCount           *SerialICounter
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
	t.byteTransmitTime = attr.byteTransmitTime()
	t.speed = attr.speed()
	_ = t.GetErrorCounts()
	return
}

func (t *Term) Speed() int {
	return t.speed
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

// TransmitTime returns the time it takes to send a byte using the settings in Term.
func (t *Term) TransmitTime(nBytes int) time.Duration {
	if nBytes <= 0 {
		return 0
	}
	return t.byteTransmitTime * time.Duration(nBytes)
}

func (attr *Attr) speed() int {
	b := attr.ts.Ospeed
	if b == 0 {
		b = attr.ts.Cflag & unix.CBAUD
	}
	speed := bToSpeed(b)
	if speed <= 0 {
		return 0
	}
	return speed
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
	{unix.B460800, 460800},
	{unix.B921600, 921600},
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
	icNew, err := IoctlGetSerialICounter(t.fd)
	if err != nil {
		t.iCount = nil
		return
	}
	if t.iCount == nil {
		t.iCount = icNew
		return
	}
	ic := t.iCount
	ec.FrameErrs = icNew.Frame - ic.Frame
	ec.OverrunErrs = icNew.Overrun - ic.Overrun
	ec.ParityErrs = icNew.Parity - ic.Parity
	ec.BreakErrs = icNew.Brk - ic.Brk
	ec.BufOverrunErrs = icNew.Buf_overrun - ic.Buf_overrun
	*ic = *icNew
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
