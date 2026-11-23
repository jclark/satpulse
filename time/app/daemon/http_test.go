package daemon

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/sseobs"
	"github.com/jclark/satpulse/time/lib/sse"
)

func TestConfigHTTPFeatureChecks(t *testing.T) {
	tests := []struct {
		name           string
		http           []HTTPConfig
		wantGUI        bool
		wantMetrics    bool
		wantPosition   bool
		wantSatellites bool
	}{
		{
			name: "none",
		},
		{
			name:           "defaults",
			http:           []HTTPConfig{{Listen: "127.0.0.1:0"}},
			wantGUI:        true,
			wantMetrics:    true,
			wantPosition:   true,
			wantSatellites: true,
		},
		{
			name: "metrics only",
			http: []HTTPConfig{{
				Listen:   "127.0.0.1:0",
				GUI:      httpBool(false),
				Metrics:  httpBool(true),
				Position: httpBool(false),
			}},
			wantMetrics:    true,
			wantSatellites: true,
		},
		{
			name: "position only",
			http: []HTTPConfig{{
				Listen:   "127.0.0.1:0",
				GUI:      httpBool(false),
				Metrics:  httpBool(false),
				Position: httpBool(true),
			}},
			wantPosition: true,
		},
		{
			name: "pprof only",
			http: []HTTPConfig{{
				Listen:   "127.0.0.1:0",
				PProf:    true,
				GUI:      httpBool(false),
				Metrics:  httpBool(false),
				Position: httpBool(false),
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{HTTP: tc.http}
			if got := cfg.anyHTTP(HTTPConfig.gui); got != tc.wantGUI {
				t.Errorf("anyHTTP(HTTPConfig.gui) = %t, want %t", got, tc.wantGUI)
			}
			if got := cfg.anyHTTP(HTTPConfig.metrics); got != tc.wantMetrics {
				t.Errorf("anyHTTP(HTTPConfig.metrics) = %t, want %t", got, tc.wantMetrics)
			}
			if got := cfg.anyHTTP(HTTPConfig.position); got != tc.wantPosition {
				t.Errorf("anyHTTP(HTTPConfig.position) = %t, want %t", got, tc.wantPosition)
			}
			if got := cfg.httpWantsSatellites(); got != tc.wantSatellites {
				t.Errorf("httpWantsSatellites() = %t, want %t", got, tc.wantSatellites)
			}
		})
	}
}

func TestHTTPObserverFactoriesUseFeatureChecks(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{HTTP: []HTTPConfig{{
		Listen:   "127.0.0.1:0",
		GUI:      httpBool(false),
		Metrics:  httpBool(true),
		Position: httpBool(false),
	}}}
	if obs := newSSEObserver(cfg, nil, lg, nil); obs != nil {
		t.Fatalf("newSSEObserver() = %v, want nil", obs)
	}
	if obs := newPrometheusObserver(cfg); obs == nil {
		t.Fatalf("newPrometheusObserver() = nil")
	}
	if obs := newPositionObserver(cfg); obs != nil {
		t.Fatalf("newPositionObserver() = %v, want nil", obs)
	}

	cfg = &Config{HTTP: []HTTPConfig{{
		Listen:   "127.0.0.1:0",
		GUI:      httpBool(true),
		Metrics:  httpBool(false),
		Position: httpBool(false),
	}}}
	sseCh := make(chan sseobs.TimedEvent, 1)
	obs := newSSEObserver(cfg, sseCh, lg, nil)
	if obs == nil {
		t.Fatalf("newSSEObserver() = nil")
	}
	obs.Release()
	if obs := newPrometheusObserver(cfg); obs != nil {
		t.Fatalf("newPrometheusObserver() = %v, want nil", obs)
	}
	if obs := newPositionObserver(cfg); obs != nil {
		t.Fatalf("newPositionObserver() = %v, want nil", obs)
	}
}

func httpBool(v bool) *bool { return &v }

// syncBuffer collects log output written by the handler's goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestSSEDisconnectsLaggingClient checks that an SSE client that reads
// too slowly is disconnected, with a message at Info naming it, and that
// an event with no time is delivered. Such a client is handed events
// whose data is already more than maxSSELag old, so the write fails at
// once.
func TestSSEDisconnectsLaggingClient(t *testing.T) {
	tests := []struct {
		name             string
		age              time.Duration // how old each event's data is; 0 means it has no time
		expectDisconnect bool
	}{
		{
			name:             "events too old",
			age:              2 * maxSSELag,
			expectDisconnect: true,
		},
		{
			name: "events without a time",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs := &syncBuffer{}
			lg := slog.New(slog.NewTextHandler(logs, nil))
			ctx, cancel := context.WithCancel(context.Background())
			evCh := make(chan sseobs.TimedEvent)
			b := bcast.New((<-chan sseobs.TimedEvent)(evCh))
			obs := sseobs.New(make(chan sseobs.TimedEvent, 1), ptime.LeapSecond{}, lg, nil)
			// This gives the handler an init event to prime the client with.
			obs.ModeChanged(phcsync.ModeInvalid, phcsync.ModeReset, time.Time{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sseHandleRequest(ctx, lg, w, r, b, obs)
			}))
			defer srv.Close()
			ev, err := sse.Make("test", "x")
			if err != nil {
				t.Fatalf("sse.Make: %v", err)
			}
			stop := make(chan struct{})
			var wg sync.WaitGroup
			wg.Go(func() { b.Run(ctx, lg) })
			// The bcast drops events until the handler subscribes, so keep sending.
			wg.Go(func() {
				defer close(evCh)
				for {
					te := sseobs.TimedEvent{Event: ev}
					if tc.age > 0 {
						te.T = time.Now().Add(-tc.age)
					}
					select {
					case evCh <- te:
					case <-stop:
						return
					}
					select {
					case <-stop:
						return
					case <-time.After(time.Millisecond):
					}
				}
			})
			defer func() {
				close(stop)
				cancel()
				b.Close()
				wg.Wait()
			}()
			conn, err := net.Dial("tcp", srv.Listener.Addr().String())
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close()
			fmt.Fprint(conn, "GET /sse HTTP/1.1\r\nHost: x\r\n\r\n")
			want := `level=INFO msg="SSE client is not keeping up, disconnecting" remoteAddr=` + conn.LocalAddr().String()
			if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatalf("SetReadDeadline: %v", err)
			}
			if tc.expectDisconnect {
				// io.Copy returns nil at EOF, when the server closes the connection.
				if _, err := io.Copy(io.Discard, conn); err != nil {
					t.Fatalf("server did not close the connection: %v", err)
				}
			} else {
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						t.Fatalf("reading events: %v", err)
					}
					if line == "event: test\n" {
						break
					}
				}
			}
			out := logs.String()
			if strings.Contains(out, want) != tc.expectDisconnect || strings.Contains(out, "level=ERROR") {
				t.Errorf("logs:\n%s\nwant disconnect message %v and no errors", out, tc.expectDisconnect)
			}
		})
	}
}

// TestSSESendsHeadersWithoutInitEvents checks that the response headers
// reach the client at once even when there are no init events to send.
func TestSSESendsHeadersWithoutInitEvents(t *testing.T) {
	lg := slog.New(slog.DiscardHandler)
	ctx, cancel := context.WithCancel(context.Background())
	evCh := make(chan sseobs.TimedEvent)
	b := bcast.New((<-chan sseobs.TimedEvent)(evCh))
	obs := sseobs.New(make(chan sseobs.TimedEvent, 1), ptime.LeapSecond{}, lg, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseHandleRequest(ctx, lg, w, r, b, obs)
	}))
	defer srv.Close()
	var wg sync.WaitGroup
	wg.Go(func() { b.Run(ctx, lg) })
	defer func() {
		close(evCh)
		cancel()
		b.Close()
		wg.Wait()
	}()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "GET /sse HTTP/1.1\r\nHost: x\r\n\r\n")
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("no response headers: %v", err)
	}
	if !strings.HasPrefix(status, "HTTP/1.1 200") {
		t.Errorf("status %q", status)
	}
}

// pipeListener is a net.Listener that hands out the connections sent
// on conns, so that an http.Server can serve a net.Pipe.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

// TestSSEClientStops checks that a write to an SSE client that has
// stopped reading fails once the event's data is maxSSELag old, so the
// client is disconnected. net.Pipe is unbuffered, so it models a socket
// whose buffers are already full.
func TestSSEClientStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := &syncBuffer{}
		lg := slog.New(slog.NewTextHandler(logs, nil))
		ctx := t.Context()
		evCh := make(chan sseobs.TimedEvent)
		b := bcast.New((<-chan sseobs.TimedEvent)(evCh))
		obs := sseobs.New(make(chan sseobs.TimedEvent, 1), ptime.LeapSecond{}, lg, nil)
		server, client := net.Pipe()
		l := &pipeListener{conns: make(chan net.Conn, 1), done: make(chan struct{})}
		l.conns <- server
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sseHandleRequest(ctx, lg, w, r, b, obs)
		})}
		var wg sync.WaitGroup
		defer wg.Wait()
		defer close(evCh)
		defer b.Close()
		defer srv.Close()
		defer client.Close()
		wg.Go(func() { b.Run(ctx, lg) })
		wg.Go(func() { srv.Serve(l) })
		fmt.Fprint(client, "GET /sse HTTP/1.1\r\nHost: x\r\n\r\n")
		r := bufio.NewReader(client)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("reading response headers: %v", err)
			}
			if line == "\r\n" {
				break
			}
		}
		synctest.Wait()
		ev, err := sse.Make("test", "x")
		if err != nil {
			t.Fatalf("sse.Make: %v", err)
		}
		// The event's data is already half maxSSELag old, so the deadline
		// is half maxSSELag away.
		evCh <- sseobs.TimedEvent{Event: ev, T: time.Now().Add(-maxSSELag / 2)}
		const want = `level=INFO msg="SSE client is not keeping up, disconnecting"`
		time.Sleep(maxSSELag/2 - time.Millisecond)
		synctest.Wait()
		if strings.Contains(logs.String(), want) {
			t.Fatalf("client disconnected before the deadline")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if out := logs.String(); !strings.Contains(out, want) || strings.Contains(out, "level=ERROR") {
			t.Errorf("logs:\n%s\nwant a line starting %s and no errors", out, want)
		}
	})
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }
