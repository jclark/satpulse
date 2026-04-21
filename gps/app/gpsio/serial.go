package gpsio

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/term"
)

// SerialConn is a connection to a serial port.
// It provides a similar interface to net.Conn.
// It implements io.Reader, io.Writer and io.Closer.
// It is safe to call Read, Write and Close on different goroutines.
// However, there must not be more than one concurrent Read
// nor more than one concurrent Write, nor more than one concurrent Close.
// Stop can be called before Close to prevent further reads and writes.
// Close will wait for any in-progress reads or writes to complete,
// before restoring serial settings and closing the underlying file descriptor.
type SerialConn struct {
	file      ioFile
	isUART    bool
	mu        sync.Mutex
	stopped   bool // protected by mu
	readLock  chan struct{}
	writeLock chan struct{}
	pktLog    *PacketLog
}

// ioFile is the minimal file-like interface SerialConn needs.
// Both *term.Term and *pollingFile satisfy it.
// TTY-specific operations (speed change, flush, restore, error counts)
// are performed via type assertion to *term.Term.
type ioFile interface {
	io.ReadWriteCloser
	Path() string
	Buffered() (int, error)
}

var _ Conn = (*SerialConn)(nil)
var _ SerialOutPort = (*SerialConn)(nil)

// OpenSerial opens a serial port at the given path and speed.
// speed can be 0 meaning to use the current speed.
// It returns the actual speed configured on the device; for devices
// that are not TTYs the returned speed is 0.
func OpenSerial(path string, speed int) (*SerialConn, int, error) {
	t, err := openTerm(path, speed)
	if err == nil {
		return newSerialConn(t, t.DevKind() == term.DevUART), t.Speed(), nil
	}
	if !errors.Is(err, term.ErrNotATTY) {
		return nil, 0, err
	}
	f, perr := term.OpenPolling(path)
	if perr != nil {
		return nil, 0, fmt.Errorf("%s and %w", perr, term.ErrNotATTY)
	}
	return newSerialConn(newPollingFile(f, readTimeout), false), 0, nil
}

func newSerialConn(f ioFile, isUART bool) *SerialConn {
	readLock := make(chan struct{}, 1)
	readLock <- struct{}{}
	writeLock := make(chan struct{}, 1)
	writeLock <- struct{}{}
	return &SerialConn{file: f, readLock: readLock, writeLock: writeLock, isUART: isUART}
}

func (c *SerialConn) LocalAddr() string {
	return c.file.Path()
}

// term returns the underlying *term.Term if this SerialConn is backed by a
// TTY, nil otherwise. TTY-specific operations (speed change, restore) are
// gated on the result.
func (c *SerialConn) term() *term.Term {
	t, _ := c.file.(*term.Term)
	return t
}

func (c *SerialConn) Read(p []byte) (int, error) {
	if c.isStopped() {
		return 0, io.EOF
	}
	select {
	case <-c.readLock:
		// this tells close that a read is in progress
	default:
		panic("concurrent reads on serial connection")
	}
	defer func() {
		c.readLock <- struct{}{}
	}()
	// now we have the read lock
	if c.isStopped() {
		return 0, io.EOF
	}
	return ioRead(c.file, p)
}

func (c *SerialConn) Write(p []byte) (int, error) {
	return c.writeThenChangeSpeed(p, 0, nil)
}

// WritePacket writes bytes to the serial port and logs the write
// using the provided format instead of doing format discovery.
func (c *SerialConn) WritePacket(p []byte, fmt gpsprot.PacketFormat) (int, error) {
	return c.writeThenChangeSpeed(p, 0, fmt)
}

// WriteThenChangeSpeed writes p then changes the serial speed.
func (c *SerialConn) WriteThenChangeSpeed(p []byte, speed int) (int, error) {
	return c.writeThenChangeSpeed(p, speed, nil)
}

func (c *SerialConn) writeThenChangeSpeed(p []byte, speed int, pktFmt gpsprot.PacketFormat) (int, error) {
	if c.isStopped() {
		return 0, net.ErrClosed
	}
	select {
	case <-c.writeLock:
		// this tells close that a write is in progress
	default:
		panic("concurrent writes on serial connection")
	}
	defer func() {
		c.writeLock <- struct{}{}
	}()
	// now we have the write lock
	if c.isStopped() {
		return 0, net.ErrClosed
	}
	n, err := c.file.Write(p)
	if err == nil {
		if speed != 0 {
			if t := c.term(); t != nil {
				// If it's a UART, then the TCSETSW flag should in theory take care of delaying the speed change
				// until the data as been transmitted.
				// But I found that on the Raspberry Pi, which uses a PL011 UART, it doesn't work without a little delay,
				// for reasons I don't understand.
				// With something like a USB-serial converter, it seems unlikely that the TCSETW flag will work,
				// since the kernel does not have access to the UART buffer to determine when it is empty.
				// So in this case, we increase the delay to ensure the data is transmitted before we change the speed,
				// since that is the most important thing.
				// We ideally want get the ACK back, which means we need to change the speed promptly.
				// But we can recover from a lost ACK.
				const minDelay = time.Millisecond
				delay := minDelay
				if !c.isUART {
					delay += t.TransmitTime(n)
				}
				time.Sleep(delay)
				err = t.Change(term.Speed(speed))
				if err != nil {
					speed = 0
				}
			} else {
				// speed change is meaningless on a non-TTY device
				speed = 0
			}
		}
		// We need to do this while we have the write lock to guarantee that the channel is not closed
		c.logWrite(p, speed, pktFmt)
	}
	return n, err
}

func (c *SerialConn) Buffered() (int, error) {
	return c.file.Buffered()
}

func (c *SerialConn) isStopped() bool {
	defer c.mu.Unlock()
	c.mu.Lock()
	return c.stopped
}

func (c *SerialConn) Stop() {
	defer c.mu.Unlock()
	c.mu.Lock()
	c.stopped = true
	if c.pktLog != nil {
		// We need close promptly so that the logging goroutine can exit.
		c.pktLog.SemiClose()
		c.pktLog = nil
	}
}

// SetPacketLog sets the packet logger for outgoing packets.
func (c *SerialConn) SetPacketLog(pl *PacketLog) {
	defer c.mu.Unlock()
	c.mu.Lock()
	c.pktLog = pl
}

func (c *SerialConn) logWrite(p []byte, speed int, fmt gpsprot.PacketFormat) {
	// Stop can be called asynchronously.
	// We need to ensure we don't use pktLog after it is closed.
	defer c.mu.Unlock()
	c.mu.Lock()
	if c.pktLog == nil {
		return
	}
	c.pktLog.LogOutput(time.Now(), p, speed, fmt)
}

func (c *SerialConn) Close() error {
	if !c.isStopped() {
		c.Stop()
	}
	_, ok := <-c.readLock
	if !ok {
		return nil // already closed
	}
	close(c.readLock)
	<-c.writeLock
	close(c.writeLock)
	// no more reads or writes are in progress
	var restoreErr error
	if t := c.term(); t != nil {
		restoreErr = t.Restore()
	}
	closeErr := c.file.Close()
	if restoreErr != nil {
		return fmt.Errorf("cannot restore serial settings: %w", restoreErr)
	}
	return closeErr
}

const readTimeout = time.Millisecond * 100

func openTerm(path string, speed int) (*term.Term, error) {
	opts := []term.AttrSetter{
		term.RawMode,
		term.Local,
		term.NoFlowControl,
		term.ReadTimeout(readTimeout),
	}
	if speed != 0 {
		if !term.IsValidSpeed(speed) {
			return nil, fmt.Errorf("non-standard serial speed %d is not supported", speed)
		}
		opts = append(opts, term.Speed(speed))
	}
	t, err := term.Open(path, opts...)
	if err != nil {
		return nil, err
	}
	err = t.Flush()
	if err != nil {
		t.Restore()
		t.Close()
		return nil, err
	}
	return t, nil
}

// pollingFile is an ioFile implementation backed by an *os.File opened
// O_NONBLOCK. It uses Go's runtime netpoller for deadline-based timeouts.
type pollingFile struct {
	f       *os.File
	timeout time.Duration
}

func newPollingFile(f *os.File, timeout time.Duration) *pollingFile {
	return &pollingFile{f: f, timeout: timeout}
}

func (pf *pollingFile) Read(p []byte) (int, error) {
	if err := pf.f.SetReadDeadline(time.Now().Add(pf.timeout)); err != nil {
		return 0, err
	}
	return pf.f.Read(p)
}

func (pf *pollingFile) Write(p []byte) (int, error) {
	return pf.f.Write(p)
}

func (pf *pollingFile) Close() error {
	return pf.f.Close()
}

func (pf *pollingFile) Path() string {
	return pf.f.Name()
}

func (pf *pollingFile) Buffered() (int, error) {
	return 0, nil
}

type timeoutError struct {
	path string
}

func (e timeoutError) Error() string {
	return e.path + ": timeout error"
}

func (e timeoutError) Timeout() bool {
	return true
}

type TermError struct {
	path   string
	counts term.ErrorCounts
}

func (e TermError) Error() string {
	return e.path + ": serial errors:" + e.counts.String()
}

func (e TermError) FramingErrs() int {
	return int(e.counts.FrameErrs)
}

func (e TermError) Temporary() bool {
	return true
}

// ioRead reads from f and, for *term.Term inputs, attaches serial
// error or timeout information. For non-TTY files, the underlying
// Read is expected to report timeouts itself (e.g. via
// os.ErrDeadlineExceeded).
func ioRead(f ioFile, p []byte) (n int, err error) {
	n, err = f.Read(p)
	if err != nil {
		return
	}
	if t, ok := f.(*term.Term); ok {
		if errCounts := t.GetErrorCounts(); !errCounts.IsZero() {
			err = TermError{path: t.Path(), counts: errCounts}
		} else if n == 0 {
			err = timeoutError{path: t.Path()}
		}
	}
	return
}
