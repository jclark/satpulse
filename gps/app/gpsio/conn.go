package gpsio

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/term"
	"github.com/jclark/satpulse/gps/scan"
)

// SerialError is the concrete error type returned by SerialConn.Read when
// the underlying device reports a serial fault (framing, parity, overrun,
// break, or buffer overrun). It is an alias for term.Error; callers that
// need to assert the contract between Read errors and higher-level
// interfaces (e.g. gpscfg.SerialError) can do so via *SerialError without
// depending on the term package directly.
type SerialError = term.Error

// ModemControlPin identifies a modem control input pin.
type ModemControlPin = term.ModemControlPin

// ModemControlPinChange reports one observed transition of a modem control input.
type ModemControlPinChange = term.ModemControlPinChange

// ErrUnavailable reports that a PPS detection method exists on the platform
// but is unavailable for the particular device or driver.
var ErrUnavailable = term.ErrUnavailable

const (
	// ModemCTS is clear to send.
	ModemCTS = term.ModemCTS
	// ModemDCD is data carrier detect.
	ModemDCD = term.ModemDCD
	// ModemDSR is data set ready.
	ModemDSR = term.ModemDSR
	// ModemRI is ring indicator.
	ModemRI = term.ModemRI
)

// ModemControlPinState is the set of asserted modem control input pins.
type ModemControlPinState = term.ModemControlPinState

// PPSMethod identifies how serial PPS edges are detected: adaptive polling
// of the modem control pin state or blocking on the platform's modem-status
// wait primitive. The zero value is unspecified, which callers interpret as
// automatic selection.
type PPSMethod int

const (
	PPSMethodPoll PPSMethod = iota + 1
	PPSMethodWait
)

// String returns the method's name, or a placeholder when it has none.
func (m PPSMethod) String() string {
	switch m {
	case PPSMethodPoll:
		return "poll"
	case PPSMethodWait:
		return "wait"
	}
	return fmt.Sprintf("PPSMethod(%d)", int(m))
}

// ParsePPSMethod parses the name of a PPS detection method. Unspecified
// has no name: it is expressed by omitting the option or key.
func ParsePPSMethod(s string) (PPSMethod, error) {
	switch s {
	case "poll":
		return PPSMethodPoll, nil
	case "wait":
		return PPSMethodWait, nil
	}
	return 0, fmt.Errorf("invalid PPS method %q: must be poll or wait", s)
}

// MarshalText implements encoding.TextMarshaler.
func (m PPSMethod) MarshalText() ([]byte, error) {
	if m < PPSMethodPoll || m > PPSMethodWait {
		return nil, fmt.Errorf("PPS method %v has no text form", m)
	}
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *PPSMethod) UnmarshalText(text []byte) error {
	v, err := ParsePPSMethod(string(text))
	if err != nil {
		return err
	}
	*m = v
	return nil
}

type OutPort interface {
	io.Writer
	Buffered() (int, error)
	// ReadOnly reports whether writes to the port are rejected.
	// Callers that would otherwise probe (e.g. gpscfg.Configure)
	// should fall through to a listen-only path when true.
	ReadOnly() bool
	// Direct reports whether the port is known to be a live hardware
	// attachment that will produce data continuously (e.g. a
	// classified UART or USB serial receiver). When false, callers
	// must not assume prompt input: the port could be a FIFO, a
	// socket, a pseudo-terminal, or any char device we couldn't
	// classify further. Used to decide whether to fail fast on a
	// silent port.
	Direct() bool
}

// OutPortLock coordinates exclusive write access to an OutPort.
// Acquire the port by receiving from the channel; release by sending it back.
type OutPortLock chan OutPort

// NewOutPortLock creates a new OutPortLock containing the given port.
func NewOutPortLock(port OutPort) OutPortLock {
	ch := make(OutPortLock, 1)
	ch <- port
	return ch
}

type SerialOutPort interface {
	OutPort
	WriteThenChangeSpeed(p []byte, speed int) (int, error)
}

type Conn interface {
	io.Reader
	io.Closer
	OutPort
	Stop()
	LocalAddr() string
	// Drain blocks until pending output has been transmitted. It is a
	// no-op on connections with no serial output buffer.
	Drain() error
}

const scanBufSize = 16

func Scan(ctx context.Context, lg *slog.Logger, conn Conn, ch chan<- scan.Packet, pLog *PacketLog, pktFormats []gpsprot.PacketFormat) {
	lg.Debug("the scan worker goroutine has started")
	defer func() {
		close(ch)
		if pLog != nil {
			pLog.SemiClose()
		}
		lg.Debug("the scan worker goroutine is about to exit")
	}()
	scanner := scan.New(conn, scanBufSize, pktFormats)
	go func() {
		<-ctx.Done()
		conn.Stop()
	}()
	for {
		pkt, err := scanner.Scan()
		ch <- pkt
		if pLog != nil {
			pLog.LogInput(pkt)
		}
		if err != nil {
			if err != io.EOF {
				lg.Error("read error while scanning", "error", err)
			}
			break
		}
	}
}

// Drain waits for the serial port to drain.
// Not sure if this is a good idea.
func Drain(ctx context.Context, lg *slog.Logger, p OutPort, nBytesWritten int) error {
	n, err := p.Buffered()
	if err != nil {
		return err
	}
	sleepTime := (time.Duration(nBytesWritten) * time.Second) / 10000
	totalSlept := time.Duration(0)
	for n > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		time.Sleep(sleepTime)
		totalSlept += sleepTime
		nPrev := n
		n, err = p.Buffered()
		if err != nil {
			return err
		}
		// give up if we are not making progress or if we have slept long enough
		if n >= nPrev {
			break
		}
		sleepTime *= 2
		if sleepTime > time.Second/5 {
			break
		}
	}
	if n > 0 {
		lg.Debug("failed to drain the serial port", "bytesInBuffer", n, "sleepTime", totalSlept)
	}

	return nil
}
