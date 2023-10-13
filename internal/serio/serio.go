package serio

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jclark/gps4ptp/internal/logctx"
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
	path  string
	Flags term.ErrorFlags
}

// TermError implements scan.TemporaryError

var _ scan.TemporaryError = TermError{}

func (e TermError) Error() string {
	return e.path + ": " + e.Flags.String()
}

func (e TermError) Frame() bool {
	return e.Flags&term.FrameError != 0
}

func (e TermError) Temporary() bool {
	return true
}

func (r *termReader) Read(p []byte) (n int, err error) {
	n, err = r.t.Read(p)
	if err == nil {
		if flags := r.t.GetErrorFlags(); flags != 0 {
			err = TermError{path: r.t.Path(), Flags: flags}
		} else if n == 0 {
			err = timeoutError{path: r.t.Path()}
		}
	}
	return
}

func ScanWorker(ctx context.Context, p *scan.Scanner, c chan scan.Frame) {
	lg := logctx.FromContext(ctx)
	lg.Debug("the scan worker goroutine has started")
	defer func() {
		close(c)
		lg.Debug("the scan worker goroutine is about to exit")
	}()
	for {
		f, err := p.Scan(ctx)
		c <- f
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
}

func WriteAsync(ctx context.Context, p OutPort, frames [][]byte) <-chan error {
	c := make(chan error, 1)
	go func() {
		nBytes := 0
		for _, frame := range frames {
			select {
			case <-ctx.Done():
				c <- ctx.Err()
				return
			default:
			}
			_, err := p.Write(frame)
			if err != nil {
				c <- err
				return
			}
			nBytes += len(frame)
		}
		logctx.FromContext(ctx).Debug("about to drain the serial port")
		c <- Drain(ctx, p, nBytes)
		logctx.FromContext(ctx).Debug("about to exit the asynchronous write goroutine")
	}()
	return c
}

func Drain(ctx context.Context, p OutPort, nBytesWritten int) error {
	lg := logctx.FromContext(ctx)
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
