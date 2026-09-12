package gpsio

import (
	"context"
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
// SerialPinState can be called concurrently with Read and Write, but
// the caller must stop all SerialPinState calls before calling Close.
// However, there must not be more than one concurrent Read
// nor more than one concurrent Write, nor more than one concurrent Close.
// Stop can be called before Close to prevent further reads and writes.
// Close will wait for any in-progress reads or writes to complete,
// before restoring serial settings and closing the underlying file descriptor.
// At most one WaitSerialPinChange call may be in progress, and it must
// have returned before Close is called. Its context cancels it, and Stop
// prevents further waits and cancels the watch; how soon a cancelled wait
// itself returns is up to the platform, and where the wait primitive cannot
// be interrupted it may be no sooner than the next pin change. After an error
// return the logical watch is released. A cancelled wait is abandoned rather
// than waited for: the goroutine keeps the watch, and with it the watch's own
// claim on the port, until it wakes and releases it, which on such a platform
// may not be before the process exits. That separate claim is what makes the
// wait safe relative to Close.
type SerialConn struct {
	file         ioFile
	kind         term.DevKind
	mu           sync.Mutex
	stopped      bool // protected by mu
	readLock     chan struct{}
	writeLock    chan struct{}
	pktLog       *PacketLog
	lastWriteLen int // bytes of the most recent write; read by Drain
	watch        term.ModemControlPinWatch
	watchPin     SerialPin
	watchMethod  PPSMethod
}

// ioFile is the minimal file-like interface SerialConn needs.
// term.Term, *term.File, and *pollingFile satisfy it.
// TTY-specific operations (speed change, flush, restore, error counts,
// modem control pins) are performed via type assertion to term.Term.
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
		return newSerialConn(t, t.DevKind()), t.Speed(), nil
	}
	if !errors.Is(err, term.ErrNotATTY) {
		return nil, 0, err
	}
	pf, wf, kind, perr := term.OpenFallback(path, readTimeout)
	if perr != nil {
		return nil, 0, fmt.Errorf("%s and %w", perr, term.ErrNotATTY)
	}
	var f ioFile = wf
	if pf != nil {
		f = newPollingFile(pf, readTimeout)
	}
	return newSerialConn(f, kind), 0, nil
}

func newSerialConn(f ioFile, kind term.DevKind) *SerialConn {
	readLock := make(chan struct{}, 1)
	readLock <- struct{}{}
	writeLock := make(chan struct{}, 1)
	writeLock <- struct{}{}
	return &SerialConn{file: f, readLock: readLock, writeLock: writeLock, kind: kind}
}

func (c *SerialConn) LocalAddr() string {
	return c.file.Path()
}

// ReadOnly reports whether writes to this port are rejected.
// True only for FIFOs today.
func (c *SerialConn) ReadOnly() bool {
	return c.kind == term.DevFIFO
}

// Direct reports whether this port is a classified hardware
// attachment -- a UART, USB serial receiver, USB-to-UART bridge, or
// Bluetooth RFCOMM device -- that will produce data continuously
// when healthy. Anything unclassified (including /dev/gnss0,
// pseudo-terminals, and unknown TTY majors) and FIFOs return false
// on the conservative assumption that we cannot promise prompt input.
func (c *SerialConn) Direct() bool {
	switch c.kind {
	case term.DevUART, term.DevUSB, term.DevUSBtoUART, term.DevBT:
		return true
	}
	return false
}

// term returns the underlying terminal capability if this SerialConn is backed
// by a configurable terminal, nil otherwise. Terminal-specific operations
// (speed change, restore) are gated on the result.
func (c *SerialConn) term() term.Term {
	t, _ := c.file.(term.Term)
	return t
}

// Speed returns the current speed of the underlying configurable terminal,
// or 0 if this connection does not provide terminal capabilities.
func (c *SerialConn) Speed() int {
	if t := c.term(); t != nil {
		return t.Speed()
	}
	return 0
}

// SerialPinState returns the asserted modem control input pins. It
// fails with term.ErrNotATTY when the connection uses a FIFO or another
// non-TTY fallback. It must not be called concurrently with Close.
func (c *SerialConn) SerialPinState() (SerialPinState, error) {
	if t := c.term(); t != nil {
		s, err := t.ModemControlPinState()
		return SerialPinState(s), err
	}
	return 0, fmt.Errorf("%s: %w", c.file.Path(), term.ErrNotATTY)
}

// newPinWatch creates the watch that method selects. Inherent impossibility --
// a backend without the capability at all, or a pin the kernel method can
// never report -- fails with an error wrapping errors.ErrUnsupported; an
// available backend whose device or driver cannot provide it fails with an
// error wrapping ErrUnavailable. Other failures retain their underlying
// error.
func (c *SerialConn) newPinWatch(pin SerialPin, method PPSMethod) (term.ModemControlPinWatch, error) {
	switch method {
	case PPSMethodWait:
		watcher, ok := c.file.(term.ModemControlPinWatcher)
		if !ok {
			return nil, fmt.Errorf("%s: cannot wait for a modem control pin change: %w", c.file.Path(), errors.ErrUnsupported)
		}
		return watcher.NewModemControlPinWatch(pin.termPin())
	case PPSMethodKernel:
		watcher, ok := c.file.(term.KernelModemControlPinWatcher)
		if !ok {
			return nil, fmt.Errorf("%s: kernel PPS is not available on this platform or device: %w", c.file.Path(), errors.ErrUnsupported)
		}
		return watcher.NewKernelModemControlPinWatch(pin.termPin())
	default:
		panic("gpsio: invalid PPS method for a pin watch")
	}
}

// WaitSerialPinChange blocks until a modem control input changes,
// watching it with the given detection method (PPSMethodWait or
// PPSMethodKernel; anything else is a contract violation). The watch is
// created on the first call and kept until an error or cancellation releases
// it; every call must pass the same pin and method.
func (c *SerialConn) WaitSerialPinChange(ctx context.Context, pin SerialPin, method PPSMethod) (SerialPinChange, int, error) {
	if method != PPSMethodWait && method != PPSMethodKernel {
		panic("gpsio: WaitSerialPinChange requires the wait or kernel method")
	}
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return SerialPinChange{}, 0, net.ErrClosed
	}
	w := c.watch
	if w != nil && (pin != c.watchPin || method != c.watchMethod) {
		c.mu.Unlock()
		panic("gpsio: WaitSerialPinChange called with a different pin or method")
	}
	c.mu.Unlock()
	if w == nil {
		// Creating the watch can be slow: the kernel method attaches a line
		// discipline, scans sysfs, and waits for udev to open up the new
		// device. Reads and writes take mu, so it is not held here.
		fresh, err := c.newPinWatch(pin, method)
		if err != nil {
			return SerialPinChange{}, 0, err
		}
		c.mu.Lock()
		// A Stop during the creation above cancelled a watch that was not
		// there yet, so this one is closed rather than installed.
		if c.stopped {
			c.mu.Unlock()
			_ = fresh.Close()
			return SerialPinChange{}, 0, net.ErrClosed
		}
		if c.watch == nil {
			c.watch = fresh
			c.watchPin = pin
			c.watchMethod = method
		}
		w = c.watch
		c.mu.Unlock()
		if w != fresh {
			_ = fresh.Close()
		}
	}

	type waitResult struct {
		change term.ModemControlPinChange
		missed int
		err    error
	}
	ch := make(chan waitResult, 1)
	delivered := make(chan bool, 1)
	go func() {
		change, missed, err := w.Wait()
		cancelled := errors.Is(err, term.ErrCancelled)
		if cancelled {
			_ = w.Close()
			c.dropPinWatch(w)
		}
		ch <- waitResult{change: change, missed: missed, err: err}
		// A completed result can race context cancellation. If the caller
		// selected the context instead, this wait goroutine still owns the
		// abandoned watch and closes it.
		if accepted := <-delivered; !accepted && !cancelled {
			_ = w.Close()
		}
	}()

	select {
	case r := <-ch:
		if err := ctx.Err(); err != nil {
			w.Cancel()
			c.dropPinWatch(w)
			delivered <- false
			return SerialPinChange{}, 0, err
		}
		delivered <- true
		if errors.Is(r.err, term.ErrCancelled) {
			return SerialPinChange{}, 0, net.ErrClosed
		}
		if r.err != nil {
			_ = w.Close()
			c.dropPinWatch(w)
		}
		return SerialPinChange(r.change), r.missed, r.err
	case <-ctx.Done():
		w.Cancel()
		c.dropPinWatch(w)
		delivered <- false
		return SerialPinChange{}, 0, ctx.Err()
	}
}

func (c *SerialConn) dropPinWatch(w term.ModemControlPinWatch) {
	c.mu.Lock()
	if c.watch == w {
		c.watch = nil
	}
	c.mu.Unlock()
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
	return c.file.Read(p)
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
	if c.ReadOnly() {
		return 0, fmt.Errorf("%s: device is not writable", c.file.Path())
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
		c.lastWriteLen = n
		if speed != 0 {
			if t := c.term(); t != nil {
				// If it's a UART, then the drain in Change should in theory take care of delaying the speed change
				// until the data as been transmitted.
				// But I found that on the Raspberry Pi, which uses a PL011 UART, it doesn't work without a little delay,
				// for reasons I don't understand.
				// With something like a USB-serial converter, it seems unlikely that the drain will work,
				// since the kernel does not have access to the UART buffer to determine when it is empty.
				// So in this case, we increase the delay to ensure the data is transmitted before we change the speed,
				// since that is the most important thing.
				// We ideally want get the ACK back, which means we need to change the speed promptly.
				// But we can recover from a lost ACK.
				delay := minDelay
				if c.kind != term.DevUART {
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
	if c.watch != nil {
		c.watch.Cancel()
	}
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
	// A watch left idle by a completed wait would otherwise keep its claim
	// on the port past Close. An abandoned wait has already dropped the
	// watch, so a non-nil watch here has no user.
	c.mu.Lock()
	w := c.watch
	c.watch = nil
	c.mu.Unlock()
	if w != nil {
		_ = w.Close()
	}
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

// Drain waits for pending output to be transmitted. satpulsetool gps calls it
// before Close so a final no-response command (e.g. a reset) reaches the
// receiver before the port settings are restored and the port is closed. It
// mirrors WriteThenChangeSpeed: the drain ioctl for all TTYs, plus the
// computed transmit time for non-UART devices, whose adapter buffer the kernel
// cannot observe.
func (c *SerialConn) Drain() error {
	t := c.term()
	if t == nil {
		return nil
	}
	delay := minDelay
	if c.kind != term.DevUART {
		delay += t.TransmitTime(c.lastWriteLen)
	}
	time.Sleep(delay)
	return t.Drain()
}

const readTimeout = time.Millisecond * 100

// minDelay is the minimum settle time before restoring or changing serial
// settings, on top of the computed transmit time for non-UART devices.
const minDelay = time.Millisecond

func openTerm(path string, speed int) (term.Term, error) {
	opts := []term.AttrSetter{
		term.RawMode,
		term.Local,
		term.NoParity,
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
