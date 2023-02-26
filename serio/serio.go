package serio

import (
	"context"
	"io"
	"time"

	"github.com/jclark/gps2phc/logctx"
	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/term"
)

const readTimeout = (time.Second * 11) / 10
const scanBufSize = 16

func OpenTerm(path string) (*term.Term, error) {
	t, err := term.Open(path, term.RawMode, term.Local, term.NoFlowControl, term.ReadTimeout(readTimeout))
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

func StartScan(ctx context.Context, r io.Reader) <-chan scan.Frame {
	scanner := scan.New(r, scanBufSize)
	c := make(chan scan.Frame, 1) // XXX think about the buffering
	go ScanWorker(ctx, scanner, c)
	return c
}

func ScanWorker(ctx context.Context, p *scan.Scanner, c chan scan.Frame) {
	lg := logctx.FromContext(ctx)
	lg.Debug("scanWorkerStarted")
	defer func() {
		close(c)
		lg.Debug("scanWorkerDone")
	}()
	for {
		f, err := p.Scan(ctx)
		c <- f
		if err != nil && err != io.EOF {
			if ctx.Err() == nil {
				logctx.FromContext(ctx).Error("readError", err)
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
		logctx.FromContext(ctx).Debug("draining")
		c <- Drain(ctx, p, nBytes)
		logctx.FromContext(ctx).Debug("writeAsyncDone")
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
		lg.Debug("drainFailed", "bytesInBuffer", n, "sleepTime", totalSlept)
	}

	return nil
}
