package serio

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/term"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

const readTimeout = (time.Second * 11) / 10
const maxWriteLen = 4096
const scanBufSize = 16

type Port struct {
	term.Term
}

func Open(path string) (*Port, error) {
	p := &Port{}
	err := p.Term.Init(path, term.RawMode, term.Local, term.NoFlowControl, term.ReadTimeout(readTimeout))
	if err != nil {
		return nil, err
	}
	err = p.Flush()
	if err != nil {
		p.Restore()
		p.Close()
		return nil, err
	}
	return p, nil
}

func (p *Port) StartRead(ctx context.Context) chan scan.Frame {
	scanner := scan.New(&p.Term, scanBufSize)
	c := make(chan scan.Frame, 1) // XXX think about the buffering
	go readWorker(ctx, scanner, c)
	return c
}

func readWorker(ctx context.Context, p *scan.Scanner, c chan scan.Frame) {
	slog.FromContext(ctx).Debug("readWorkerStarted")
	defer close(c)
	for {
		f, err := p.Scan(ctx)
		c <- f
		if err != nil && err != io.EOF {
			if ctx.Err() == nil {
				slog.FromContext(ctx).Error("readError", err)
			}
			break
		}
	}
}

func (p *Port) WriteAsync(ctx context.Context, frames [][]byte) <-chan error {
	c := make(chan error, 1)
	go func() {
		for _, frame := range frames {
			select {
			case <-ctx.Done():
				c <- ctx.Err()
				return
			default:
			}
			_, err := p.Write(ctx, frame)
			if err != nil {
				c <- err
				return
			}
		}
		//slog.FromContext(ctx).Debug("draining")
		//c <- p.Drain(ctx)
		c <- nil
		slog.FromContext(ctx).Debug("writeAsyncDone")
	}()
	return c
}

func (p *Port) Write(ctx context.Context, buf []byte) (int, error) {
	total := 0
	lg := slog.FromContext(ctx)
	for len(buf) > 0 {

		// Semantics of Unix write and Go Write are not the same:
		// Unix can write less than requested amount without its being an error.
		wBuf := buf
		if len(buf) > maxWriteLen {
			wBuf = wBuf[0:maxWriteLen]
		}
		n, err := p.Term.Write(wBuf)
		if err == io.ErrShortWrite && n > 0 {
			err = nil
		}
		if err == nil {
			lg.Debug("serialWrite", "n", n)
			total += n
			buf = buf[n:]
		} else if !errors.Is(err, unix.EINTR) {
			return total, err
		}
	}
	return total, nil
}

func (p *Port) Drain(tx context.Context) error {
	lg := slog.FromContext(tx)
	for {
		select {
		case <-tx.Done():
			return tx.Err()
		default:
		}
		n, err := p.Term.Buffered()
		if err != nil || n == 0 {
			break
		}
		lg.Debug("serialBufferedBytes", "n", n)
		time.Sleep(time.Microsecond * 10)
		n, _ = p.Term.Buffered()
		lg.Debug("drainBufferedBytes", "n", n)
	}
	return nil
}
