package gpsio

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/scan"
)

type OutPort interface {
	io.Writer
	Buffered() (int, error)
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
}

// timeoutError implements scan.TimeoutError
var _ scan.TimeoutError = timeoutError{}

// TermError implements scan.TemporaryError
var _ scan.TemporaryError = TermError{}

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
