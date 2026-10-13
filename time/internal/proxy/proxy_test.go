package proxy

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/scan"
)

// TestShutdownClientNotReading checks that cancelling ctx stops the connection
// workers even when the client has stopped reading, so the connection writer
// is parked in Write. net.Pipe is unbuffered, so it models a socket whose
// buffers are already full.
func TestShutdownClientNotReading(t *testing.T) {
	tests := []struct {
		name     string
		readOnly bool
	}{
		{name: "read-only", readOnly: true},
		{name: "writable", readOnly: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lg := slog.New(slog.DiscardHandler)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			msgCh := make(chan scan.Packet)
			b := bcast.New(msgCh)
			var bwg sync.WaitGroup
			defer bwg.Wait()
			defer b.Close()
			bwg.Go(func() { b.Run(ctx, lg) })
			stop := make(chan struct{})
			defer close(stop)
			// bcast drops packets until there is a subscriber, so keep sending
			// until the writer has subscribed.
			bwg.Go(func() {
				defer close(msgCh)
				tick := time.NewTicker(time.Millisecond)
				defer tick.Stop()
				for {
					select {
					case <-stop:
						return
					case <-tick.C:
						select {
						case msgCh <- scan.Packet{Data: "ab"}:
						case <-stop:
							return
						}
					}
				}
			})
			server, client := net.Pipe()
			defer client.Close()
			var wg sync.WaitGroup
			handleConn(ctx, lg, &wg, svcConfig{readOnly: tc.readOnly}, server, b, make(gpsio.OutPortLock, 1))
			// Reading one byte of a two-byte packet leaves the writer blocked in Write.
			if _, err := client.Read(make([]byte, 1)); err != nil {
				t.Fatalf("read from proxy connection: %v", err)
			}
			cancel()
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				client.Close()
				<-done
				t.Errorf("connection workers did not exit after cancellation")
			}
		})
	}
}
