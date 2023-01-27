package serio

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/jclark/gps2phc/scan"
	"github.com/pkg/term"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

const readTimeout = (time.Second * 11) / 10
const maxWriteLen = 4096

func Open(path string) (*term.Term, error) {
	t, err := term.Open(path, term.RawMode, term.FlowControl(term.NONE), term.ReadTimeout(readTimeout))
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

func ReadStart(ctx context.Context, scanner *scan.Scanner) chan scan.Frame {
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

func WriteAsync(ctx context.Context, w *term.Term, frames [][]byte) <-chan error {
	c := make(chan error, 1)
	go func() {
		for _, frame := range frames {
			select {
			case <-ctx.Done():
				c <- ctx.Err()
				return
			default:
			}
			_, err := Write(ctx, w, frame)
			if err != nil {
				c <- err
				return
			}
		}
		c <- Drain(ctx, w)
		slog.FromContext(ctx).Debug("writeAsyncDone")
	}()
	return c
}

func Write(ctx context.Context, w *term.Term, buf []byte) (int, error) {
	total := 0
	lg := slog.FromContext(ctx)
	for len(buf) > 0 {

		// Semantics of Unix write and Go Write are not the same:
		// Unix can write less than requested amount without its being an error.
		wBuf := buf
		if len(buf) > maxWriteLen {
			wBuf = wBuf[0:maxWriteLen]
		}
		n, err := w.Write(wBuf)
		if err == io.ErrShortWrite && n > 0 {
			err = nil
		}
		if err == nil {
			lg.Debug("ialWrite", "n", n)
			total += n
			buf = buf[n:]
		} else if !errors.Is(err, unix.EINTR) {
			return total, err
		}
	}
	return total, nil
}

func Drain(tx context.Context, w *term.Term) error {
	lg := slog.FromContext(tx)
	for {
		select {
		case <-tx.Done():
			return tx.Err()
		default:
		}
		n, err := w.Buffered()
		if err != nil || n == 0 {
			break
		}
		lg.Debug("ialBufferedBytes", "n", n)
		time.Sleep(time.Microsecond * 10)
		n, _ = w.Buffered()
		lg.Debug("drainBufferedBytes", "n", n)
	}
	return nil
}

const (
	DevUnknown = iota
	DevUART
	DevUSB
	DevUSBtoUART
	DevBT
)

func DevType(path string) int {
	s := unix.Stat_t{}
	err := unix.Stat(path, &s)
	if err != nil {
		return DevUnknown
	}
	// See https://www.kernel.org/doc/html/latest/admin-guide/devices.html
	switch unix.Major(s.Dev) {
	case 4, 5:
		if unix.Minor(s.Dev) >= 64 { // ttyS0, /dev/ttycua0
			return DevUART
		}
	case 166, 167: // USB ACM "modem" /dev/ttyACM0
		return DevUSB
	case 188, 189: // USB ial converter /dev/ttyUSB0
		return DevUSBtoUART
	case 204, 205: // low-density ial port (Raspberry Pi uses /dev/ttyAMA0)
		return DevUART
	case 216, 217: // Bluetooth RFCOMM /dev/rfcomm0
		return DevBT
	}
	return DevUnknown
}
