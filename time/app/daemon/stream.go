package daemon

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/jclark/satpulse/gps/app/stream"
)

// startStream spawns the stream pull goroutine.  It is a no-op when
// setup is nil.  A non-cancel error from Setup.Run is logged but
// does not cancel the daemon: time/PHC sync is independent of
// corrections, and the daemon should continue degraded rather than
// tear down on a correction-side fault.
func startStream(ctx context.Context, lg *slog.Logger,
	wg *sync.WaitGroup, setup *stream.PullSetup) {
	if setup == nil {
		return
	}
	addr := setup.Addr()
	onState := func(st stream.State, err error) {
		switch st {
		case stream.Connecting:
			lg.Info("stream pull connecting", "addr", addr)
		case stream.Connected:
			lg.Info("stream pull connected", "addr", addr)
		case stream.Reconnecting:
			lg.Warn("stream pull reconnecting", "addr", addr, "err", err)
		}
	}
	wg.Go(func() {
		err := setup.Run(ctx, lg, onState)
		if err != nil && !errors.Is(err, context.Canceled) {
			lg.Error("stream pull exited with error", "addr", addr, "err", err)
		}
	})
}
