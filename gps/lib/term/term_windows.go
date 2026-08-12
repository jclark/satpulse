package term

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 comm error flags from winbase.h, not exported by x/sys/windows.
const (
	ceRxOver   = 0x0001
	ceOverrun  = 0x0002
	ceRxParity = 0x0004
	ceFrame    = 0x0008
	ceBreak    = 0x0010
)

// DCB.Flags bit layout from winbase.h.
const (
	dcbBinary           = 1 << 0
	dcbParity           = 1 << 1
	dcbOutxCtsFlow      = 1 << 2
	dcbOutxDsrFlow      = 1 << 3
	dcbDtrControlMask   = 0x3 << 4
	dcbDsrSensitivity   = 1 << 6
	dcbTXContinueOnXoff = 1 << 7
	dcbOutX             = 1 << 8
	dcbInX              = 1 << 9
	dcbErrorChar        = 1 << 10
	dcbNull             = 1 << 11
	dcbRtsControlMask   = 0x3 << 12
	dcbAbortOnError     = 1 << 14
)

const maxDWORD = 0xffffffff

// baudRates is used by IsValidSpeed so the set of accepted speeds matches
// Unix. Windows accepts the baud rate value directly in DCB.BaudRate.
var baudRates = []int{
	50, 75, 110, 134, 150, 200, 300, 600, 1200, 1800,
	2400, 4800, 9600, 19200, 38400, 57600, 115200,
	230400, 460800, 921600,
}

type windowsTerm struct {
	handle        windows.Handle
	path          string
	attr          Attr
	dcbSaved      windows.DCB
	timeoutsSaved windows.CommTimeouts
}

var _ Term = (*windowsTerm)(nil)

type Attr struct {
	dcb      windows.DCB
	timeouts windows.CommTimeouts
}

type AttrSetter func(*Attr) error

// Open opens and configures a serial terminal.
func Open(path string, opts ...AttrSetter) (Term, error) {
	t := new(windowsTerm)
	if err := t.init(path, opts...); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *windowsTerm) init(path string, opts ...AttrSetter) (err error) {
	t.path = path
	name, err := windows.UTF16PtrFromString(normalizeCOM(path))
	if err != nil {
		return t.wrapErr(err, "open")
	}
	h, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, // exclusive: no sharing
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			err = wrapLocked(err)
		}
		return t.wrapErr(err, "open")
	}
	t.handle = h
	var dcbChanged, timeoutsChanged bool
	defer func() {
		if err == nil {
			return
		}
		if dcbChanged {
			windows.SetCommState(h, &t.dcbSaved)
		}
		if timeoutsChanged {
			windows.SetCommTimeouts(h, &t.timeoutsSaved)
		}
		windows.CloseHandle(h)
		t.handle = windows.InvalidHandle
	}()
	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	err = windows.GetCommState(h, &dcb)
	if err != nil {
		// GetCommState fails with ERROR_INVALID_FUNCTION on a handle that is
		// not a serial device (e.g. a named pipe used as a replay sink), the
		// Windows analog of tcgetattr returning ENOTTY. Map it to ErrNotATTY
		// so OpenSerial falls through to the OpenFallback path.
		if errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
			err = fmt.Errorf("%s: %w", t.path, ErrNotATTY)
			return
		}
		return t.wrapErr(err, "GetCommState")
	}
	t.dcbSaved = dcb
	var timeouts windows.CommTimeouts
	err = windows.GetCommTimeouts(h, &timeouts)
	if err != nil {
		return t.wrapErr(err, "GetCommTimeouts")
	}
	t.timeoutsSaved = timeouts
	attr := Attr{dcb: dcb, timeouts: timeouts}
	for _, opt := range opts {
		err = opt(&attr)
		if err != nil {
			return
		}
	}
	err = windows.SetCommState(h, &attr.dcb)
	if err != nil {
		return t.wrapErr(err, "SetCommState")
	}
	dcbChanged = true
	err = windows.SetCommTimeouts(h, &attr.timeouts)
	if err != nil {
		return t.wrapErr(err, "SetCommTimeouts")
	}
	timeoutsChanged = true
	t.attr = attr
	return
}

// normalizeCOM rewrites bare COMn names to \\.\COMn so ports above COM9 work.
func normalizeCOM(path string) string {
	if strings.HasPrefix(path, `\\.\`) || strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if len(path) >= 3 && strings.EqualFold(path[:3], "COM") {
		return `\\.\` + path
	}
	return path
}

// Change changes the attributes of the terminal after output has drained.
func (t *windowsTerm) Change(opts ...AttrSetter) error {
	attr := t.attr
	for _, opt := range opts {
		err := opt(&attr)
		if err != nil {
			return err
		}
	}
	if err := t.Drain(); err != nil {
		return err
	}
	err := windows.SetCommState(t.handle, &attr.dcb)
	if err != nil {
		return t.wrapErr(err, "SetCommState")
	}
	err = windows.SetCommTimeouts(t.handle, &attr.timeouts)
	if err != nil {
		return t.wrapErr(err, "SetCommTimeouts")
	}
	t.attr = attr
	return nil
}

func (t *windowsTerm) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	var n uint32
	err := windows.ReadFile(t.handle, buf, &n, nil)
	if err != nil {
		return int(n), t.wrapErr(err, "read")
	}
	if n == 0 {
		// Serial errors indicate line activity, so this interval cannot be
		// treated as an inter-packet timeout.
		if serr := t.readError(); serr != nil {
			return 0, serr
		}
		return 0, &os.PathError{Op: "read", Path: t.path, Err: os.ErrDeadlineExceeded}
	}
	if serr := t.readError(); serr != nil {
		return int(n), serr
	}
	return int(n), nil
}

func (t *windowsTerm) Write(buf []byte) (int, error) {
	total := 0
	for len(buf) > 0 {
		var n uint32
		err := windows.WriteFile(t.handle, buf, &n, nil)
		if err != nil {
			return total, t.wrapErr(err, "write")
		}
		total += int(n)
		buf = buf[n:]
	}
	return total, nil
}

func (t *windowsTerm) Buffered() (int, error) {
	var errs uint32
	var stat windows.ComStat
	err := windows.ClearCommError(t.handle, &errs, &stat)
	if err != nil {
		return 0, t.wrapErr(err, "ClearCommError")
	}
	return int(stat.CBOutQue), nil
}

func (t *windowsTerm) Flush() error {
	err := windows.PurgeComm(t.handle, windows.PURGE_RXCLEAR|windows.PURGE_TXCLEAR)
	return t.wrapErr(err, "PurgeComm")
}

// Drain blocks until all pending output has been transmitted.
func (t *windowsTerm) Drain() error {
	return t.wrapErr(windows.FlushFileBuffers(t.handle), "FlushFileBuffers")
}

func (t *windowsTerm) Restore() error {
	err := windows.SetCommState(t.handle, &t.dcbSaved)
	if err != nil {
		return t.wrapErr(err, "SetCommState")
	}
	err = windows.SetCommTimeouts(t.handle, &t.timeoutsSaved)
	return t.wrapErr(err, "SetCommTimeouts")
}

func (t *windowsTerm) Close() error {
	h := t.handle
	t.handle = windows.InvalidHandle
	return t.wrapErr(windows.CloseHandle(h), "close")
}

func (t *windowsTerm) Path() string {
	return t.path
}

func (t *windowsTerm) Speed() int {
	return int(t.attr.dcb.BaudRate)
}

// TransmitTime returns the time it takes to send nBytes at the current speed.
func (t *windowsTerm) TransmitTime(nBytes int) time.Duration {
	if nBytes <= 0 {
		return 0
	}
	return t.attr.byteTransmitTime() * time.Duration(nBytes)
}

func (attr *Attr) byteTransmitTime() time.Duration {
	speed := int(attr.dcb.BaudRate)
	if speed <= 0 {
		return 0
	}
	bits := 2 + int(attr.dcb.ByteSize)
	if attr.dcb.Parity != windows.NOPARITY {
		bits++
	}
	if attr.dcb.StopBits == windows.TWOSTOPBITS {
		bits++
	}
	return time.Duration(bits) * time.Second / time.Duration(speed)
}

// DevKind on Windows returns DevUnknown. The plan allows best-effort
// classification via registry lookup; that is not yet implemented.
func (t *windowsTerm) DevKind() DevKind {
	return DevUnknown
}

// readError returns a *Error describing serial errors reported by
// ClearCommError, or nil if none. Windows does not expose per-category
// counters, so Error.Counts is always nil.
func (t *windowsTerm) readError() *Error {
	var errs uint32
	var stat windows.ComStat
	if err := windows.ClearCommError(t.handle, &errs, &stat); err != nil {
		return nil
	}
	var flags ErrFlags
	if errs&ceFrame != 0 {
		flags |= ErrFraming
	}
	if errs&ceRxParity != 0 {
		flags |= ErrParity
	}
	if errs&ceOverrun != 0 {
		flags |= ErrOverrun
	}
	if errs&ceBreak != 0 {
		flags |= ErrBreak
	}
	if errs&ceRxOver != 0 {
		flags |= ErrBufOverrun
	}
	if flags == 0 {
		return nil
	}
	return &Error{Path: t.path, Flags: flags}
}

func (t *windowsTerm) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{
		Op:   op,
		Path: t.path,
		Err:  err,
	}
}

type File struct{}

func (f *File) Read([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *File) Write([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *File) Close() error {
	return os.ErrInvalid
}

func (f *File) Path() string {
	return ""
}

func (f *File) Buffered() (int, error) {
	return 0, nil
}

// OpenFallback opens a non-serial GPS input, such as a named pipe used as a
// replay sink, as an asynchronous *os.File. It skips the GetCommState/
// SetCommState setup a COM port needs (and that fails on a pipe; see init).
// The handle is opened FILE_FLAG_OVERLAPPED so os.NewFile associates it with
// the Go runtime poller, giving Read the SetReadDeadline support the scan loop
// relies on (Go 1.25+). It is classified DevFIFO, so writes are rejected at
// the gpsio layer, matching the Unix FIFO. The read timeout is applied later
// by the caller via SetReadDeadline, so it is ignored here.
func OpenFallback(path string, _ time.Duration) (*os.File, *File, DevKind, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, nil, DevUnknown, &os.PathError{Op: "open", Path: path, Err: err}
	}
	// A freshly opened handle is not yet associated with a completion port; if
	// it were, os.NewFile would silently downgrade it to synchronous I/O and
	// the deadline methods would have no effect.
	h, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, // exclusive: no sharing
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			err = wrapLocked(err)
		}
		return nil, nil, DevUnknown, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil, DevFIFO, nil
}

func RawMode(a *Attr) error {
	a.dcb.ByteSize = 8
	a.dcb.Parity = windows.NOPARITY
	a.dcb.StopBits = windows.ONESTOPBIT
	a.dcb.Flags |= dcbBinary
	a.dcb.Flags &^= dcbErrorChar | dcbNull | dcbAbortOnError
	return nil
}

// Local is a no-op on Windows; CLOCAL has no direct equivalent. Clearing
// fDsrSensitivity ensures incoming bytes are not dropped based on DSR.
func Local(a *Attr) error {
	a.dcb.Flags &^= dcbDsrSensitivity
	return nil
}

// NoParity configures the terminal for no parity and disables input parity
// checking.
func NoParity(a *Attr) error {
	a.dcb.Parity = windows.NOPARITY
	a.dcb.Flags &^= dcbParity
	return nil
}

func NoFlowControl(a *Attr) error {
	a.dcb.Flags &^= dcbOutxCtsFlow | dcbOutxDsrFlow | dcbOutX | dcbInX | dcbDsrSensitivity | dcbTXContinueOnXoff
	a.dcb.Flags = (a.dcb.Flags &^ dcbDtrControlMask) | windows.DTR_CONTROL_ENABLE
	a.dcb.Flags = (a.dcb.Flags &^ dcbRtsControlMask) | windows.RTS_CONTROL_ENABLE
	return nil
}

func Speed(speed int) AttrSetter {
	if !IsValidSpeed(speed) {
		return func(*Attr) error {
			return fmt.Errorf("invalid terminal speed: %d", speed)
		}
	}
	return func(a *Attr) error {
		a.dcb.BaudRate = uint32(speed)
		return nil
	}
}

// ReadTimeout configures ReadFile to return immediately if data is
// available, or after the given timeout if none arrives.
func ReadTimeout(timeout time.Duration) AttrSetter {
	return func(a *Attr) error {
		ms := timeout.Milliseconds()
		if ms <= 0 && timeout > 0 {
			ms = 1
		}
		if ms >= maxDWORD {
			ms = maxDWORD - 1
		}
		a.timeouts.ReadIntervalTimeout = maxDWORD
		a.timeouts.ReadTotalTimeoutMultiplier = maxDWORD
		a.timeouts.ReadTotalTimeoutConstant = uint32(ms)
		a.timeouts.WriteTotalTimeoutMultiplier = 0
		a.timeouts.WriteTotalTimeoutConstant = 0
		return nil
	}
}

func IsValidSpeed(speed int) bool {
	i := sort.SearchInts(baudRates, speed)
	return i < len(baudRates) && baudRates[i] == speed
}
