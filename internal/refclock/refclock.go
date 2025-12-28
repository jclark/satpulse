package refclock

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
	"golang.org/x/sys/unix"
)

type RefClock interface {
	io.Closer
	Sample(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) error
}

type SockRefClock interface {
	RefClock
	RemotePath() string
}

type ProxyRefClock struct {
	sampleCh chan<- RefClockSample
}

type RefClockSample struct {
	Sys time.Time
	Ref ptime.Time
	Ls  ptime.LeapSecond
}

func NewProxyRefClock() (*ProxyRefClock, <-chan RefClockSample) {
	sampleCh := make(chan RefClockSample, 1)
	return &ProxyRefClock{sampleCh: sampleCh}, sampleCh
}

func (rc *ProxyRefClock) Close() {
	close(rc.sampleCh)
}

func (rc *ProxyRefClock) Sample(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) error {
	select {
	case rc.sampleCh <- RefClockSample{Sys: sys, Ref: ref, Ls: ls}:
		return nil
	default:
		return errors.New("RefClockWorker is not ready for a sample")
	}
}

func RefClockWorker(rc RefClock, sampCh <-chan RefClockSample, lg *slog.Logger) {
	lg.Debug("the refclock worker has started")
	for {
		samp, ok := <-sampCh
		if !ok {
			break
		}
		err := rc.Sample(samp.Sys, samp.Ref, samp.Ls)
		if err != nil {
			lg.Info("error while sending refclock sample", "err", err)
		}
	}
	lg.Debug("the refclock worker is about to close the  refclock")
	err := rc.Close()
	if err != nil {
		lg.Warn("error while closing refclock", "err", err)
	}
	lg.Debug("the refclock worker is about to exit")
}

// LoggingSockRefClock does error handling for a refclock that sends to a socket that may not exist.
// The error handling consists of logging changes in when the socket exists.
type LoggingSockRefClock struct {
	sockOK bool
	sock   SockRefClock
	lg     *slog.Logger
}

func NewLoggingSockRefClock(lg *slog.Logger, sock SockRefClock) *LoggingSockRefClock {
	return &LoggingSockRefClock{lg: lg, sock: sock, sockOK: true}
}

func (rc *LoggingSockRefClock) Close() error {
	return rc.sock.Close()
}

func (rc *LoggingSockRefClock) Sample(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) error {
	err := rc.sock.Sample(sys, ref, ls)
	lg := rc.lg
	path := rc.sock.RemotePath()
	if err != nil {
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ECONNREFUSED) {
			if rc.sockOK {
				lg.Info("the refclock socket is not ready", "path", path)
				rc.sockOK = false
			}
		} else if errors.Is(err, os.ErrDeadlineExceeded) && rc.sockOK {
			lg.Info("timeout sending to refclock socket", "path", path)
		} else {
			return err
		}
	} else {
		if !rc.sockOK {
			lg.Info("the refclock socket has become ready", "path", path)
			rc.sockOK = true
		}
		lg.Debug("successfully sent sample to refclock socket", "path", path, "sys", sys, "ref", ref)
	}
	return nil
}
