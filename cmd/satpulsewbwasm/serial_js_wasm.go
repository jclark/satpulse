package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"syscall/js"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/term"
)

type serialOpener struct {
	serial js.Value
	device string
	mu     sync.Mutex
	speed  int
}

// Open opens a previously granted Web Serial port at its retained speed.
func (o *serialOpener) Open(ctx context.Context, _ *slog.Logger) (session.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o.mu.Lock()
	speed := o.speed
	o.mu.Unlock()
	abort := js.Global().Get("AbortController").New()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			abort.Call("abort")
		case <-done:
		}
	}()
	v, err := await(o.serial.Call("open", o.device, speed, abort.Get("signal")))
	close(done)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	c := &serialConn{value: v, op: o}
	if err := ctx.Err(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// Socket reports that Web Serial is a direct receiver connection.
func (*serialOpener) Socket() bool { return false }

type serialConn struct {
	value   js.Value
	op      *serialOpener
	readBuf []byte
	writeMu sync.Mutex
	mu      sync.Mutex
	stopped bool
	pLog    *gpsio.PacketLog
}

var _ session.Opener = (*serialOpener)(nil)
var _ session.Conn = (*serialConn)(nil)
var _ gpsio.SerialOutPort = (*serialConn)(nil)

// Read adapts chunked browser input and idle timeouts to the packet scanner.
func (c *serialConn) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if c.isStopped() {
		return 0, io.EOF
	}
	if len(c.readBuf) == 0 {
		v, err := await(c.value.Call("read"))
		if err != nil {
			if e, ok := err.(*jsError); ok {
				var flags term.ErrFlags
				switch e.name {
				case "FramingError":
					flags = term.ErrFraming
				case "ParityError":
					flags = term.ErrParity
				case "BreakError":
					flags = term.ErrBreak
				case "BufferOverrunError":
					flags = term.ErrBufOverrun
				}
				if flags != 0 {
					return 0, &gpsio.SerialError{Path: c.LocalAddr(), Flags: flags}
				}
			}
			return 0, err
		}
		if v.IsNull() {
			return 0, readTimeout{}
		}
		if v.Length() == 0 {
			return 0, io.EOF
		}
		c.readBuf = make([]byte, v.Length())
		js.CopyBytesToGo(c.readBuf, v)
	}
	n := copy(b, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

// Write sends bytes and records them in the session packet log.
func (c *serialConn) Write(b []byte) (int, error) {
	return c.write(b, 0, nil)
}

// WritePacket sends bytes with a known packet format for logging.
func (c *serialConn) WritePacket(b []byte, pf gpsprot.PacketFormat) (int, error) {
	return c.write(b, 0, pf)
}

// WriteThenChangeSpeed sends bytes before closing and reopening the browser port.
func (c *serialConn) WriteThenChangeSpeed(b []byte, speed int) (int, error) {
	return c.write(b, speed, nil)
}

func (c *serialConn) write(b []byte, speed int, pf gpsprot.PacketFormat) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isStopped() {
		return 0, net.ErrClosed
	}
	v := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(v, b)
	_, err := await(c.value.Call("write", v))
	if err != nil {
		return 0, err
	}
	if speed != 0 {
		_, err = await(c.value.Call("changeSpeed", speed))
		if err == nil {
			c.op.mu.Lock()
			c.op.speed = speed
			c.op.mu.Unlock()
		} else {
			speed = 0
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pLog != nil {
		c.pLog.LogOutput(time.Now(), b, speed, pf)
	}
	return len(b), err
}

// Stop unblocks Read and detaches output logging before session teardown.
func (c *serialConn) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.stopped = true
	c.value.Call("stop")
	if c.pLog != nil {
		c.pLog.SemiClose()
		c.pLog = nil
	}
}

// Close waits for writes, releases browser stream locks, and closes the port.
func (c *serialConn) Close() error {
	c.Stop()
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := await(c.value.Call("close"))
	return err
}

func (c *serialConn) isStopped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

// SetPacketLog installs the session's outgoing packet logger.
func (c *serialConn) SetPacketLog(pl *gpsio.PacketLog) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pLog = pl
}

// Speed returns the speed retained for subsequent reconnects.
func (c *serialConn) Speed() int {
	c.op.mu.Lock()
	defer c.op.mu.Unlock()
	return c.op.speed
}

// LocalAddr returns the page-local identifier of the granted port.
func (c *serialConn) LocalAddr() string { return c.op.device }

// ReadOnly reports that Web Serial supports receiver writes.
func (*serialConn) ReadOnly() bool { return false }

// Direct reports a hardware serial attachment for probe timing.
func (*serialConn) Direct() bool { return true }

// SetDetected needs no action because browser ports do not restore UART settings.
func (*serialConn) SetDetected() {}

// Buffered returns zero because Web Serial cannot report pending output bytes.
func (*serialConn) Buffered() (int, error) { return 0, nil }

// Drain waits for the adapter's estimated serial transmit time.
func (c *serialConn) Drain() error {
	_, err := await(c.value.Call("drain"))
	return err
}

type readTimeout struct{}

// Error identifies an idle read timeout to the scanner.
func (readTimeout) Error() string { return "serial read timeout" }

// Timeout distinguishes an idle timeout from a terminal read error.
func (readTimeout) Timeout() bool { return true }
