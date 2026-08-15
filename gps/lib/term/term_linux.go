package term

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

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

func Speed(speed int) AttrSetter {
	b, ok := speedToB(speed)
	if !ok {
		return func(*Attr) error {
			return fmt.Errorf("invalid terminal speed: %d", speed)
		}
	}
	return func(a *Attr) error {
		// B0 in CIBAUD makes the input rate follow the output rate.
		a.ts.Cflag &^= unix.CBAUD | unix.CIBAUD
		a.ts.Cflag |= b
		a.ts.Ospeed = b
		a.ts.Ispeed = b
		return nil
	}
}

func (attr *Attr) speed() int {
	b := attr.ts.Cflag & unix.CBAUD
	if b == unix.BOTHER {
		return int(attr.ts.Ospeed)
	}
	speed := bToSpeed(b)
	if speed <= 0 {
		return 0
	}
	return speed
}

func (t *unixTerm) Flush() error {
	return t.wrapErr(unix.IoctlSetInt(t.fd, unix.TCFLSH, unix.TCIOFLUSH), "ioctl(TCFLSH)")
}

func (t *unixTerm) setAttrNow(attr *unix.Termios) error {
	return t.wrapErr(unix.IoctlSetTermios(t.fd, unix.TCSETS2, attr), "ioctl(TCSETS2)")
}

// Drain blocks until all pending output has been transmitted. The nonzero
// TCSBRK argument selects drain-without-break, which is how tcdrain is
// implemented on Linux. The ioctl blocks for the transmit time of the
// buffered output - hundreds of ms on a slow line - and the Go runtime's
// preemption and timer signals interrupt blocking syscalls routinely, so
// EINTR here is runtime noise, not an event: retry. (POSIX tcdrain may
// legitimately return EINTR; under Go we must be more diligent than libc
// callers about retrying it.)
func (t *unixTerm) Drain() error {
	for {
		err := unix.IoctlSetInt(t.fd, unix.TCSBRK, 1)
		if err != unix.EINTR {
			return t.wrapErr(err, "ioctl(TCSBRK)")
		}
	}
}

func (t *unixTerm) getAttr() (tp *unix.Termios, err error) {
	tp, err = unix.IoctlGetTermios(t.fd, unix.TCGETS2)
	err = t.wrapErr(err, "ioctl(TCGETS2)")
	return
}

var errExclusive = errors.New("terminal is in exclusive mode (TIOCEXCL)")

// checkNotExclusive returns a LockedError if the terminal is already in
// exclusive mode. It is worth looking only when we hold CAP_SYS_ADMIN: without
// it, open would already have failed with EBUSY had the flag been set, so a
// flag seen here was set by another process after our open succeeded, and
// failing on it would just hand the port to a latecomer.
func (t *unixTerm) checkNotExclusive() error {
	if !capSysAdmin() {
		return nil
	}
	v, err := unix.IoctlGetInt(t.fd, unix.TIOCGEXCL)
	if err != nil {
		return t.wrapErr(err, "ioctl(TIOCGEXCL)")
	}
	if v != 0 {
		return t.wrapErr(wrapLocked(errExclusive), "open")
	}
	return nil
}

// capSysAdmin reports whether the process has CAP_SYS_ADMIN, which exempts it
// from the kernel's exclusive-mode check on open.
func capSysAdmin() bool {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	err := unix.Capget(&hdr, &data[0])
	return err == nil && data[0].Effective&(1<<unix.CAP_SYS_ADMIN) != 0
}

var errFlockNotSupported error

// OpenFallback opens path as an *os.File suitable for reading with
// SetReadDeadline-based timeouts. It is intended for GNSS devices that
// speak a GPS protocol over a non-termios interface (e.g. /dev/gnss0)
// and for FIFOs used as a replay sink.
//
// Character devices must have a driver that implements .poll; they are
// opened O_RDWR|O_NOCTTY|O_CLOEXEC|O_NONBLOCK and classified as
// DevUnknown (the UART/USB/BT classification applies only to TTYs).
// FIFOs are opened O_RDWR|O_NONBLOCK -- so that our own fd keeps the
// write side of the pipe open. This avoids EOF both before an external
// writer connects and after it exits; an idle replay FIFO should look
// like a silent receiver, not a disconnected one. Writes into a DevFIFO
// port are rejected at the gpsio layer to prevent self-feeding.
// The file descriptor is locked exclusively with flock.
func OpenFallback(path string, timeout time.Duration) (*os.File, *File, DevKind, error) {
	return openFallback(path, timeout, openPollingChar, openPollingFIFO)
}

func openPollingChar(path string, _ time.Duration) (*os.File, *File, DevKind, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, DevUnknown, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := probePoll(fd); err != nil {
		unix.Close(fd)
		return nil, nil, DevUnknown, fmt.Errorf("%s: device does not support polling: %w", path, err)
	}
	if err := lock(fd, path); err != nil {
		unix.Close(fd)
		return nil, nil, DevUnknown, err
	}
	return os.NewFile(uintptr(fd), path), nil, DevUnknown, nil
}

func openPollingFIFO(path string, _ time.Duration) (*os.File, *File, DevKind, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, DevUnknown, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := lock(fd, path); err != nil {
		unix.Close(fd)
		return nil, nil, DevUnknown, err
	}
	return os.NewFile(uintptr(fd), path), nil, DevFIFO, nil
}

// probePoll verifies that the driver backing fd implements .poll by
// attempting to register the fd with a throwaway epoll instance.
func probePoll(fd int) error {
	ep, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return fmt.Errorf("epoll_create1: %w", err)
	}
	defer unix.Close(ep)
	ev := unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(fd)}
	if err := unix.EpollCtl(ep, unix.EPOLL_CTL_ADD, fd, &ev); err != nil {
		return fmt.Errorf("epoll_ctl: %w", err)
	}
	return nil
}

type serialErrorState struct {
	count serialICounter
	valid bool
}

// readError returns a *Error describing serial errors that have occurred
// since the previous call, or nil if none. It also establishes the
// baseline counters on the first successful call after initialization or
// after a transient TIOCGICOUNT failure.
func (t *unixTerm) readError() *Error {
	icNew, err := ioctlGetSerialICounter(t.fd)
	if err != nil {
		t.serialError.valid = false
		return nil
	}
	if !t.serialError.valid {
		t.serialError.count = *icNew
		t.serialError.valid = true
		return nil
	}
	ic := &t.serialError.count
	ec := ErrorCounts{
		Framing:    icNew.Frame - ic.Frame,
		Overrun:    icNew.Overrun - ic.Overrun,
		Parity:     icNew.Parity - ic.Parity,
		Break:      icNew.Brk - ic.Brk,
		BufOverrun: icNew.Buf_overrun - ic.Buf_overrun,
	}
	*ic = *icNew
	if ec == (ErrorCounts{}) {
		return nil
	}
	var flags ErrFlags
	if ec.Framing != 0 {
		flags |= ErrFraming
	}
	if ec.Parity != 0 {
		flags |= ErrParity
	}
	if ec.Overrun != 0 {
		flags |= ErrOverrun
	}
	if ec.Break != 0 {
		flags |= ErrBreak
	}
	if ec.BufOverrun != 0 {
		flags |= ErrBufOverrun
	}
	return &Error{Path: t.path, Flags: flags, Counts: &ec}
}

var _ ModemControlPinWatcher = (*unixTerm)(nil)

// NewModemControlPinWatch creates a watch with a private descriptor. All watch syscalls
// use the duplicate, so the watch remains safe if the terminal is later
// closed.
func (t *unixTerm) NewModemControlPinWatch(pin ModemControlPin) (ModemControlPinWatch, error) {
	return newWaitPinWatch(t, pin)
}

func (t *unixTerm) DevKind() DevKind {
	s := unix.Stat_t{}
	err := unix.Fstat(t.fd, &s)
	if err != nil {
		return DevUnknown
	}
	// See https://www.kernel.org/doc/html/latest/admin-guide/devices.html
	switch unix.Major(s.Rdev) {
	case 4, 5:
		if unix.Minor(s.Rdev) >= 64 { // ttyS0, /dev/ttycua0
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
