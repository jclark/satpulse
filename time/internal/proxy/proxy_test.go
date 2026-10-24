package proxy

import (
	"context"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
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
						case msgCh <- scan.Packet{Data: "ab", TRead: time.Now()}:
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

// TestConnQueue checks which packets connQueue passes to a writer that
// reads only after every input packet has been sent, so connQueue must
// queue packets without the writer. The test runs in a synctest bubble,
// so packets age on the bubble's clock, and a connQueue that blocks on
// the writer fails the test with a deadlock.
func TestConnQueue(t *testing.T) {
	ubx := packetFormat(t, gpsreg.TagUBX)
	nmea := packetFormat(t, gpsreg.TagNMEA)
	type input struct {
		after time.Duration // how long to wait before sending pkt
		age   time.Duration // how old pkt is when it is sent
		pkt   scan.Packet
	}
	tests := []struct {
		name   string
		tag    gpsprot.Tag
		input  []input
		expect []string
	}{
		{
			name: "packets are queued while the writer is stalled",
			input: []input{
				{pkt: scan.Packet{Data: "a"}},
				{after: 500 * time.Millisecond, pkt: scan.Packet{Data: "b"}},
				{after: 500 * time.Millisecond, pkt: scan.Packet{Data: "c"}},
			},
			expect: []string{"a", "b", "c"},
		},
		{
			name: "packets older than maxPacketAge are dropped",
			input: []input{
				{pkt: scan.Packet{Data: "a"}},
				{after: 900 * time.Millisecond, pkt: scan.Packet{Data: "b"}},
				{after: 900 * time.Millisecond, pkt: scan.Packet{Data: "c"}},
				{after: 900 * time.Millisecond, pkt: scan.Packet{Data: "d"}},
				{after: 900 * time.Millisecond, pkt: scan.Packet{Data: "e"}},
			},
			expect: []string{"c", "d", "e"},
		},
		{
			name: "the newest packet is kept however old",
			input: []input{
				{age: 3 * time.Second, pkt: scan.Packet{Data: "a"}},
			},
			expect: []string{"a"},
		},
		{
			name: "an old packet is dropped when a newer one arrives",
			input: []input{
				{age: 3 * time.Second, pkt: scan.Packet{Data: "a"}},
				{pkt: scan.Packet{Data: "b"}},
			},
			expect: []string{"b"},
		},
		{
			name: "empty packets are not forwarded",
			input: []input{
				{pkt: scan.Packet{}},
				{pkt: scan.Packet{Data: "a"}},
			},
			expect: []string{"a"},
		},
		{
			name: "all packets without a protocol",
			input: []input{
				{pkt: scan.Packet{Format: ubx, ChecksumValid: true, Data: "a"}},
				{pkt: scan.Packet{Format: nmea, ChecksumValid: false, Data: "b"}},
				{pkt: scan.Packet{Data: "c"}},
			},
			expect: []string{"a", "b", "c"},
		},
		{
			name: "valid packets of the protocol",
			tag:  gpsreg.TagUBX,
			input: []input{
				{pkt: scan.Packet{Format: ubx, ChecksumValid: true, Data: "a"}},
				{pkt: scan.Packet{Format: nmea, ChecksumValid: true, Data: "b"}},
				{pkt: scan.Packet{Format: ubx, ChecksumValid: false, Data: "c"}},
				{pkt: scan.Packet{Data: "d"}},
			},
			expect: []string{"a"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ch := make(chan scan.Packet)
				writerCh := make(chan scan.Packet)
				go connQueue(ctx, slog.New(slog.DiscardHandler), svcConfig{tag: tc.tag}, ch, writerCh)
				for _, in := range tc.input {
					time.Sleep(in.after)
					pkt := in.pkt
					pkt.TRead = time.Now().Add(-in.age)
					ch <- pkt
				}
				if got := queued(writerCh); !reflect.DeepEqual(got, tc.expect) {
					t.Errorf("got  %q\nwant %q", got, tc.expect)
				}
			})
		})
	}
}

// queued returns the data of the packets that connQueue offers on
// writerCh until it has none left. It must be called in a synctest bubble.
func queued(writerCh <-chan scan.Packet) []string {
	var got []string
	for {
		synctest.Wait()
		select {
		case pkt := <-writerCh:
			got = append(got, pkt.Data)
		default:
			return got
		}
	}
}

func packetFormat(t *testing.T, tag gpsprot.Tag) gpsprot.PacketFormat {
	for _, f := range gpsreg.CreatePacketFormats(nil) {
		if f.Tag() == tag {
			return f
		}
	}
	t.Fatalf("no packet format with tag %v", tag)
	return nil
}
