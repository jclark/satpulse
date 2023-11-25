package gpsio

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/jclark/satpulse/term"
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
	term      *term.Term
	mu        sync.Mutex
	stopped   bool // protected by mu
	readLock  chan struct{}
	writeLock chan struct{}
}

var _ Conn = (*SerialConn)(nil)

func OpenSerial(path string, speed int) (*SerialConn, error) {
	t, err := openTerm(path, speed)
	if err != nil {
		return nil, err
	}
	readLock := make(chan struct{}, 1)
	readLock <- struct{}{}
	writeLock := make(chan struct{}, 1)
	writeLock <- struct{}{}
	return &SerialConn{term: t, readLock: readLock, writeLock: writeLock}, nil
}

func (c *SerialConn) LocalAddr() string {
	return c.term.Path()
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
	return termRead(c.term, p)
}

func (c *SerialConn) Write(p []byte) (int, error) {
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
	return c.term.Write(p)
}

func (c *SerialConn) Buffered() (int, error) {
	return c.term.Buffered()
}

func (c *SerialConn) TransmitTime(nBytes int) time.Duration {
	return c.term.TransmitTime(nBytes)
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
	restoreErr := c.term.Restore()
	closeErr := c.term.Close()
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

func termRead(t *term.Term, p []byte) (n int, err error) {
	n, err = t.Read(p)
	if err == nil {
		if errCounts := t.GetErrorCounts(); !errCounts.IsZero() {
			err = TermError{path: t.Path(), counts: errCounts}
		} else if n == 0 {
			err = timeoutError{path: t.Path()}
		}
	}
	return
}
