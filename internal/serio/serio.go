package serio

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/term"
)

const readTimeout = time.Millisecond * 100

func OpenTerm(path string, speed *int) (*term.Term, error) {
	opts := []term.AttrSetter{
		term.RawMode,
		term.Local,
		term.NoFlowControl,
		term.ReadTimeout(readTimeout),
	}
	if speed != nil {
		if !term.IsValidSpeed(*speed) {
			return nil, fmt.Errorf("non-standard serial speed %d is not supported", *speed)
		}
		opts = append(opts, term.Speed(*speed))
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

const scanBufSize = 16

func NewScanner(t *term.Term) *scan.Scanner {
	return scan.New(&termReader{t: t}, scanBufSize)
}

// termReader adjusts the error handling of the underlying term.Term to better match scan.Scanner
type termReader struct {
	t *term.Term
}

type timeoutError struct {
	path string
}

// timeoutError implements scan.TimeoutError
var _ scan.TimeoutError = timeoutError{}

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

// TermError implements scan.TemporaryError
var _ scan.TemporaryError = TermError{}

func (e TermError) Error() string {
	return e.path + ": serial errors:" + e.counts.String()
}

func (e TermError) FramingErrs() int {
	return int(e.counts.FrameErrs)
}

func (e TermError) Temporary() bool {
	return true
}

func (r *termReader) Read(p []byte) (n int, err error) {
	n, err = r.t.Read(p)
	if err == nil {
		if errCounts := r.t.GetErrorCounts(); !errCounts.IsZero() {
			err = TermError{path: r.t.Path(), counts: errCounts}
		} else if n == 0 {
			err = timeoutError{path: r.t.Path()}
		}
	}
	return
}

func ScanWorker(ctx context.Context, lg *slog.Logger, scanner *scan.Scanner, ch chan scan.Packet) {
	lg.Debug("the scan worker goroutine has started")
	defer func() {
		close(ch)
		lg.Debug("the scan worker goroutine is about to exit")
	}()
	for {
		pkt, err := scanner.Scan(ctx)
		ch <- pkt
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				lg.Error("read error while scanning", "error", err)
			}
			break
		}
	}
}

type OutPort interface {
	io.Writer
	Buffered() (int, error)
	TransmitTime(nBytes int) time.Duration
}

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
