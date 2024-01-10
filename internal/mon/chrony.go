package mon

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/chrony"
	"github.com/jclark/satpulse/internal/ptime"
	"golang.org/x/sys/unix"
)

type NTPSample struct {
	Sys time.Time
	Ref ptime.Time
	Ls  ptime.LeapSecond
}

type NTPServer struct {
	sampleCh chan<- NTPSample
}

func NewNTPServer() (*NTPServer, <-chan NTPSample) {
	sampleCh := make(chan NTPSample, 1)
	return &NTPServer{sampleCh: sampleCh}, sampleCh
}

func (s *NTPServer) Close() {
	close(s.sampleCh)
}

func (s *NTPServer) Sample(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) bool {
	select {
	case s.sampleCh <- NTPSample{Sys: sys, Ref: ref, Ls: ls}:
		return true
	default:
		return false
	}
}

const chronyTimeout = time.Second / 10

// The updating goroutine must wait for a response to each request before sending another request.
func ChronyWorker(ctx context.Context, client *chrony.Client, sampCh <-chan NTPSample, lg *slog.Logger) {
	lg.Debug("the chrony refclock SOCK client worker has started")
	sockExists := true
	for {
		samp, ok := <-sampCh
		if !ok {
			break
		}
		err := client.Send(samp.Sys, samp.Ref, samp.Ls, chronyTimeout)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				if sockExists {
					lg.Info("the chrony refclock SOCK socket does not exist (chrony not running?)", "path", client.RemotePath())
					sockExists = false
				}
			} else if errors.Is(err, os.ErrDeadlineExceeded) && sockExists {
				lg.Info("timeout on chrony refclock SOCK socket", "path", client.RemotePath())
			} else if ctx.Err() == nil {
				lg.Warn("error while sending sample to chrony", "err", err)
			}
		} else {
			if !sockExists {
				lg.Info("the chrony refclock SOCK socket now exists", "path", client.RemotePath())
				sockExists = true
			}
			lg.Debug("successfully sent sample to chrony", "sys", samp.Sys, "ref", samp.Ref)
		}
	}
	lg.Debug("the chrony worker is about to close the chrony refclock SOCK client")
	err := client.Close()
	if err != nil {
		lg.Warn("error while closing the chrony refclock SOCK client", "err", err)
	}
	lg.Debug("the chrony refclock SOCK client worker is about to exit")
}
