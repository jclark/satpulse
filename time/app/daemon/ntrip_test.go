package daemon

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/ntrip"
	"github.com/jclark/satpulse/gps/scan"
)

// TestStartNtripNilGcfg verifies that startNtrip does not panic when
// gcfg is nil (e.g. ErrNotDetected on a non-PHC daemon run).
func TestStartNtripNilGcfg(t *testing.T) {
	cfg := &Config{
		Ntrip: ntrip.Config{
			Listen:     "127.0.0.1:0",
			Mountpoint: []ntrip.MountConfig{{Name: "BKK"}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	var wg sync.WaitGroup
	pktCh := make(chan scan.Packet)
	b := bcast.New((<-chan scan.Packet)(pktCh))
	wg.Go(func() { b.Run(ctx, lg) })
	if err := startNtrip(ctx, lg, &wg, cfg, nil, b); err != nil {
		cancel()
		close(pktCh)
		b.Close()
		wg.Wait()
		t.Fatalf("startNtrip: %v", err)
	}
	cancel()
	close(pktCh)
	b.Close()
	wg.Wait()
}
