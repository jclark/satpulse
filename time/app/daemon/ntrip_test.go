package daemon

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/ntrip"
	"github.com/jclark/satpulse/gps/gpsprot"
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
	if err := startNtrip(ctx, lg, &wg, cfg, nil, b, nil); err != nil {
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

func TestStartNtripSourceTableUsesConfigSupport(t *testing.T) {
	on := true
	cfg := &Config{
		GPS: GPSConfig{Config: true, RTCMOutput: &on},
		Ntrip: ntrip.Config{
			Listen:     freeListenAddr(t),
			Mountpoint: []ntrip.MountConfig{{Name: "BKK"}},
		},
	}
	props := new(gpsprot.ConfigProps)
	props.SetSignalsEnabled(gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA,
		gpsprot.SigQZSSL1CA,
		gpsprot.SigQZSSL2C,
	))
	gcfg := &gpscfg.Result{
		ConfigProps: props,
		ConfigSupport: gpsprot.ConfigSupportRTCMMSM4 |
			gpsprot.ConfigSupportRTCMMSM7 |
			gpsprot.ConfigSupportRTCMQZSS,
	}
	ctx, cancel := context.WithCancel(context.Background())
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	var wg sync.WaitGroup
	pktCh := make(chan scan.Packet)
	b := bcast.New((<-chan scan.Packet)(pktCh))
	wg.Go(func() { b.Run(ctx, lg) })
	defer func() {
		cancel()
		close(pktCh)
		b.Close()
		wg.Wait()
	}()
	if err := startNtrip(ctx, lg, &wg, cfg, gcfg, b, nil); err != nil {
		t.Fatalf("startNtrip: %v", err)
	}
	conn, err := net.Dial("tcp", cfg.Ntrip.Listen)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nConnection: close\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	want := "STR;BKK;BKK;RTCM 3.3;1005(1),1074(1),1114(1);2;GPS+QZSS;"
	if !strings.Contains(string(body), want) {
		t.Fatalf("source table missing %q: %s", want, body)
	}
}

func TestRTCMOutputSupport(t *testing.T) {
	on := true
	off := false
	base := &Config{GPS: GPSConfig{Config: true, RTCMOutput: &on}}
	tests := []struct {
		name string
		cfg  *Config
		gcfg *gpscfg.Result
		want gpsprot.ConfigSupportFlags
	}{
		{
			name: "msm and qzss support",
			cfg:  base,
			gcfg: &gpscfg.Result{
				ConfigSupport: gpsprot.ConfigSupportRTCMMSM4 |
					gpsprot.ConfigSupportRTCMMSM7 |
					gpsprot.ConfigSupportRTCMQZSS |
					gpsprot.ConfigSupportRaw,
			},
			want: gpsprot.ConfigSupportRTCMMSM4 |
				gpsprot.ConfigSupportRTCMMSM7 |
				gpsprot.ConfigSupportRTCMQZSS,
		},
		{
			name: "no support",
			cfg:  base,
			gcfg: &gpscfg.Result{},
			want: 0,
		},
		{
			name: "rtcm output off",
			cfg:  &Config{GPS: GPSConfig{Config: true, RTCMOutput: &off}},
			gcfg: &gpscfg.Result{ConfigSupport: gpsprot.ConfigSupportRTCMMSM7},
			want: 0,
		},
		{
			name: "no probe result",
			cfg:  base,
			gcfg: nil,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rtcmOutputSupport(tt.cfg, tt.gcfg); got != tt.want {
				t.Errorf("rtcmOutputSupport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func freeListenAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().String()
}
