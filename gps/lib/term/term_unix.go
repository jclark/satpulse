//go:build !windows

package term

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type unixTerm struct {
	fd   int
	path string
	attrCache
	tsSaved     unix.Termios
	serialError serialErrorState
}

var _ Term = (*unixTerm)(nil)

// attrCache allows a terminal's cached attributes to be read from any goroutine
// concurrently with Change. Calls to Change must be serialized by the caller.
type attrCache struct {
	mu   sync.RWMutex
	attr Attr
}

func (c *attrCache) loadAttr() Attr {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.attr
}

func (c *attrCache) storeAttr(attr Attr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attr = attr
}

type Attr struct {
	ts unix.Termios
}

type AttrSetter func(*Attr) error

// Open opens and configures a serial terminal.
func Open(path string, opts ...AttrSetter) (Term, error) {
	d, ok, err := tryD2XX(path, opts)
	if err != nil {
		return nil, err
	}
	if ok {
		return d, nil
	}
	t := new(unixTerm)
	if err := t.init(path, opts...); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *unixTerm) init(path string, opts ...AttrSetter) (err error) {
	t.path = path
	// O_CLOEXEC is here, because we are using flock to lock.
	// Without O_CLOEXEC, the lock would be inherited by child processes, which is probably not what is wanted.
	// See e.g. https://github.com/Pulse-Eight/libcec/issues/477
	// O_NONBLOCK prevents open from waiting for carrier when CLOCAL is not set.
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.EBUSY) {
			err = wrapLocked(err)
		}
		err = t.wrapErr(err, "open")
		return
	}
	t.fd = fd
	defer func() {
		if err != nil {
			unix.Close(fd)
		}
	}()
	// Carrier is only waited for in open, so blocking mode can be restored immediately.
	if err = unix.SetNonblock(fd, false); err != nil {
		err = t.wrapErr(err, "fcntl(F_SETFL)")
		return
	}
	// We could make this optional, but non-exclusive use of the serial port seems like a bad idea.
	err = lock(fd, path)
	if err != nil {
		if errors.Is(err, errFlockNotSupported) {
			err = fmt.Errorf("%s: %w", t.path, ErrNotATTY)
		}
		return
	}
	tsp, err := t.getAttr()
	if err != nil {
		if errors.Is(err, unix.ENOTTY) {
			err = fmt.Errorf("%s: %w", t.path, ErrNotATTY)
		}
		return
	}
	// getAttr has established that this is a tty, so the exclusive-mode ioctls
	// cannot fail with ENOTTY.
	if err = t.setExclusive(); err != nil {
		return
	}
	defer func() {
		if err != nil {
			t.clearExclusive()
		}
	}()
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
	t.storeAttr(attr)
	_ = t.readError()
	return
}

// Change changes the attributes of the terminal after output has drained.
func (t *unixTerm) Change(opts ...AttrSetter) error {
	attr := t.loadAttr()
	for _, opt := range opts {
		err := opt(&attr)
		if err != nil {
			return err
		}
	}
	if err := t.Drain(); err != nil {
		return err
	}
	if err := t.setAttrNow(&attr.ts); err != nil {
		return err
	}
	t.storeAttr(attr)
	return nil
}

func (t *unixTerm) Speed() int {
	attr := t.loadAttr()
	return attr.speed()
}

func lock(fd int, path string) error {
	err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return fmt.Errorf("%s: %w", path, wrapLocked(err))
	}
	if err != nil {
		return fmt.Errorf("%s: could not lock device: %w", path, err)
	}
	return nil
}

// setExclusive puts the terminal into exclusive mode, making opens by processes
// without CAP_SYS_ADMIN fail with EBUSY. flock remains the primary lock; this is
// for interoperating with programs such as gpsd that use TIOCEXCL instead, and it
// also stops an unprivileged opener reprogramming the line as a side effect of
// probing it.
func (t *unixTerm) setExclusive() error {
	if err := t.checkNotExclusive(); err != nil {
		return err
	}
	return t.wrapErr(unix.IoctlSetInt(t.fd, unix.TIOCEXCL, 0), "ioctl(TIOCEXCL)")
}

// clearExclusive takes the terminal out of exclusive mode. The flag belongs to
// the tty rather than to our file descriptor, so closing without clearing it
// leaves the port unopenable for as long as some other process holds the tty
// open.
func (t *unixTerm) clearExclusive() error {
	return t.wrapErr(unix.IoctlSetInt(t.fd, unix.TIOCNXCL, 0), "ioctl(TIOCNXCL)")
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

// NoParity configures the terminal for no parity and disables input parity
// checking.
func NoParity(a *Attr) error {
	a.ts.Iflag &^= unix.INPCK
	a.ts.Cflag &^= unix.PARENB
	return nil
}

// TransmitTime returns the time it takes to send a byte using the settings in Term.
func (t *unixTerm) TransmitTime(nBytes int) time.Duration {
	if nBytes <= 0 {
		return 0
	}
	attr := t.loadAttr()
	return attr.byteTransmitTime() * time.Duration(nBytes)
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

// readCanTimeout reports whether a zero-byte read can mean a VTIME timeout,
// i.e. whether reads are configured with VMIN=0 and VTIME!=0 as ReadTimeout does.
func (attr *Attr) readCanTimeout() bool {
	return attr.ts.Cc[unix.VMIN] == 0 && attr.ts.Cc[unix.VTIME] != 0
}

const earlyZeroRead = time.Millisecond
const earlyZeroReadLimit = 5

func (t *unixTerm) Read(buf []byte) (n int, err error) {
	if len(buf) == 0 {
		return 0, nil
	}
	// A zero-byte read on a TTY with VMIN=0, VTIME!=0 normally means the VTIME
	// timeout expired, which takes around 100ms. But a removed serial device
	// (fd open, device node gone) returns zero immediately and forever. To tell
	// the cases apart, treat a zero read returned in well under VTIME as suspicious
	// and report io.EOF once we see earlyZeroReadLimit of them in a row, so the
	// scan worker exits instead of spinning. A real timeout always exceeds
	// earlyZeroRead, and one anomalously fast zero read is tolerated.
	for range earlyZeroReadLimit {
		start := time.Now()
		for {
			n, err = unix.Read(t.fd, buf)
			if err != unix.EINTR {
				break
			}
		}
		if err != nil {
			return n, t.wrapErr(err, "read")
		}
		if n != 0 {
			if serr := t.readError(); serr != nil {
				err = serr
			}
			return
		}
		attr := t.loadAttr()
		if !attr.readCanTimeout() || time.Since(start) >= earlyZeroRead {
			// Serial errors indicate line activity, so this interval cannot be
			// treated as an inter-packet timeout.
			if serr := t.readError(); serr != nil {
				return 0, serr
			}
			return 0, &os.PathError{Op: "read", Path: t.path, Err: os.ErrDeadlineExceeded}
		}
	}
	return 0, io.EOF
}

func (t *unixTerm) Write(buf []byte) (int, error) {
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

func (t *unixTerm) Buffered() (n int, err error) {
	n, err = unix.IoctlGetInt(t.fd, unix.TIOCOUTQ)
	err = t.wrapErr(err, "ioctl(TIOCOUTQ)")
	return
}

// ModemControlLineState returns the asserted modem control input lines.
func (t *unixTerm) ModemControlLineState() (ModemControlLineState, error) {
	var status int
	var err error
	for {
		status, err = unix.IoctlGetInt(t.fd, unix.TIOCMGET)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		return 0, t.wrapErr(err, "ioctl(TIOCMGET)")
	}
	var state ModemControlLineState
	if status&unix.TIOCM_CTS != 0 {
		state |= modemControlLineState(ModemCTS)
	}
	if status&unix.TIOCM_CAR != 0 {
		state |= modemControlLineState(ModemDCD)
	}
	if status&unix.TIOCM_DSR != 0 {
		state |= modemControlLineState(ModemDSR)
	}
	if status&unix.TIOCM_RI != 0 {
		state |= modemControlLineState(ModemRI)
	}
	return state, nil
}

func (t *unixTerm) Restore() error {
	return t.setAttrNow(&t.tsSaved)
}

func (t *unixTerm) Close() error {
	err := t.clearExclusive()
	fd := t.fd
	t.fd = -1
	if cerr := t.wrapErr(unix.Close(fd), "close"); err == nil {
		err = cerr
	}
	return err
}

func (t *unixTerm) Path() string {
	return t.path
}

type fallbackOpener func(string, time.Duration) (*os.File, *File, DevKind, error)

func openFallback(path string, timeout time.Duration, openChar, openFIFO fallbackOpener) (*os.File, *File, DevKind, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return nil, nil, DevUnknown, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFCHR:
		if openChar == nil {
			return nil, nil, DevUnknown, fmt.Errorf("%s: character devices are not supported", path)
		}
		return openChar(path, timeout)
	case unix.S_IFIFO:
		if openFIFO == nil {
			return nil, nil, DevUnknown, fmt.Errorf("%s: FIFOs are not supported", path)
		}
		return openFIFO(path, timeout)
	case unix.S_IFREG:
		return nil, nil, DevUnknown, fmt.Errorf("%s: regular files are not supported", path)
	case unix.S_IFBLK:
		return nil, nil, DevUnknown, fmt.Errorf("%s: block devices are not supported", path)
	case unix.S_IFDIR:
		return nil, nil, DevUnknown, fmt.Errorf("%s: is a directory", path)
	case unix.S_IFSOCK:
		return nil, nil, DevUnknown, fmt.Errorf("%s: sockets are not supported", path)
	default:
		return nil, nil, DevUnknown, fmt.Errorf("%s: unsupported file type", path)
	}
}

// File is a non-TTY device read using OS readiness waiting.
type File struct {
	fd      int
	path    string
	timeout unix.Timeval
}

func openFile(path string, mode int, timeout time.Duration) (*File, error) {
	fd, err := unix.Open(path, mode, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	var fds unix.FdSet
	limit := len(fds.Bits) * unix.NFDBITS
	if fd < 0 || fd >= limit {
		if fd >= 0 {
			unix.Close(fd)
		}
		err = fmt.Errorf("file descriptor %d is outside select range 0-%d", fd, limit-1)
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	err = lock(fd, path)
	if err != nil {
		if !errors.Is(err, errFlockNotSupported) {
			unix.Close(fd)
			return nil, err
		}
	}
	return &File{fd: fd, path: path, timeout: unix.NsecToTimeval(timeout.Nanoseconds())}, nil
}

func (f *File) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	for {
		var rfds unix.FdSet
		rfds.Set(f.fd)
		tv := f.timeout
		n, err := unix.Select(f.fd+1, &rfds, nil, nil, &tv)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, f.wrapErr(err, "select")
		}
		if n == 0 {
			return 0, f.wrapErr(os.ErrDeadlineExceeded, "read")
		}
		n, err = unix.Read(f.fd, buf)
		if err == unix.EINTR || err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			continue
		}
		if err != nil {
			return n, f.wrapErr(err, "read")
		}
		return n, nil
	}
}

func (f *File) Write(buf []byte) (int, error) {
	return 0, f.wrapErr(os.ErrInvalid, "write")
}

func (f *File) Close() error {
	fd := f.fd
	f.fd = -1
	return f.wrapErr(unix.Close(fd), "close")
}

func (f *File) Path() string {
	return f.path
}

func (f *File) Buffered() (int, error) {
	return 0, nil
}

func (f *File) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{
		Op:   op,
		Path: f.path,
		Err:  err,
	}
}

func (t *unixTerm) wrapErr(err error, op string) error {
	if err == nil {
		return err
	}
	return &os.PathError{
		Op:   op,
		Path: t.path,
		Err:  err,
	}
}
