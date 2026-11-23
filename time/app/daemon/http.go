package daemon

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/time/internal/promobs"
	"github.com/jclark/satpulse/time/internal/proxy"
	"github.com/jclark/satpulse/time/internal/sseobs"
	"github.com/jclark/satpulse/time/internal/web"
	"github.com/jclark/satpulse/time/lib/sse"
)

type HTTPConfig struct {
	Listen string `toml:"listen"`
	PProf  bool   `toml:"pprof"`
	// These default to true but use *bool because table array items
	// cannot have pre-filled defaults.
	GUI      *bool `toml:"gui"`      // Serve graphical user interface
	Metrics  *bool `toml:"metrics"`  // Serve Prometheus metrics endpoint
	Position *bool `toml:"position"` // Serve current position endpoint
}

// gui gives the value of the GUI option, defaulting to true if not set.
func (hc HTTPConfig) gui() bool {
	if hc.GUI == nil {
		return true
	}
	return *hc.GUI
}

// metrics gives the value of the Metrics option, defaulting to true if not set.
func (hc HTTPConfig) metrics() bool {
	if hc.Metrics == nil {
		return true
	}
	return *hc.Metrics
}

// position gives the value of the Position option, defaulting to true if not set.
func (hc HTTPConfig) position() bool {
	if hc.Position == nil {
		return true
	}
	return *hc.Position
}

const gracefulShutdownTimeout = 1 * time.Second

func registerPprofHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func startHTTP(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg []HTTPConfig, b *bcast.Bcast[sseobs.TimedEvent], sseObs *sseobs.SSEObserver, promObs *promobs.PrometheusObserver, posObs *positionObserver) error {
	if len(cfg) == 0 {
		return nil
	}
	for _, c := range cfg {
		if c.Listen == "" {
			return configErrorf("must specify listen option for each HTTP element")
		}
		// Validate that at least one endpoint is enabled
		if !c.PProf && !c.gui() && !c.metrics() && !c.position() {
			return configErrorf("HTTP endpoint %s must enable at least one of: pprof, gui, metrics, position", c.Listen)
		}
	}

	listenCfg := net.ListenConfig{}
	listeners := make([]net.Listener, len(cfg))
	for i, c := range cfg {
		listen, err := listenCfg.Listen(ctx, "tcp", c.Listen)
		if err != nil {
			return err
		}
		listeners[i] = listen
	}

	fileServer := http.FileServer(http.FS(web.Content()))
	servers := make([]*http.Server, len(listeners))
	for i, listener := range listeners {
		listener := listener
		mux := http.NewServeMux()
		if cfg[i].PProf {
			registerPprofHandlers(mux)
		}
		// Only register GUI routes if enabled for this endpoint
		if cfg[i].gui() {
			mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
				sseHandleRequest(ctx, lg, w, r, b, sseObs)
			})
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				fileServer.ServeHTTP(w, r)
			})
		}

		// Only register metrics endpoint if enabled for this endpoint
		if cfg[i].metrics() {
			mux.Handle("/metrics", promObs.Handler())
		}
		if cfg[i].position() {
			mux.HandleFunc("/position", positionHandler(posObs))
		}

		// XXX we should supply an error logger that wraps lg
		server := &http.Server{Handler: mux}
		servers[i] = server
		wg.Go(func() {
			lg.Debug("HTTP server listening", "addr", listener.Addr())
			if err := server.Serve(listener); err != http.ErrServerClosed {
				lg.Error("HTTP serve error", "err", err)
			}
			lg.Debug("HTTP server about to exit", "addr", listener.Addr())
		})
	}
	wg.Go(func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		lg.Debug("initiating HTTP server shutdown")
		for _, server := range servers {
			if err := server.Shutdown(shutdownCtx); err != nil {
				lg.Error("Server shutdown error", "err", err)
			}
		}
		lg.Debug("about to exit HTTP server shutdown goroutine")
	})

	return nil
}

// maxSSELag is how long after the data in an SSE event was produced the
// client's connection must have accepted the event. A client that stops
// reading, or reads more slowly than events are produced, is
// disconnected once it falls this far behind, so that it cannot make the
// event bcast queue grow without limit; its EventSource then reconnects
// and is primed again.
const maxSSELag = 10 * time.Second

func sseHandleRequest(ctx context.Context, lg *slog.Logger, w http.ResponseWriter, r *http.Request, b *bcast.Bcast[sseobs.TimedEvent], sseObs *sseobs.SSEObserver) {
	lg = lg.With("remoteAddr", r.RemoteAddr)
	defer lg.Debug("about to exit HTTP SSE request handler")
	lg.Debug("starting to handle HTTP SSE request")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	rc := http.NewResponseController(w)
	// Subscribe before snapshotting the init events: any broadcast event
	// that fires after Subscribe will be queued for us, so a mode change
	// racing with our connect cannot be lost between snapshot and live
	// delivery.
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)
	// The init events describe the current state, so they are due now.
	// They are flushed even if there are none, so the client gets the
	// response headers at once.
	if !writeSSE(lg, w, rc, time.Time{}, sseObs.InitEvents()...) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				lg.Debug("broadcast channel to SSE request handler closed")
				return
			}
			lg.Debug("received a broadcast SSE event", "event", event.Event)
			if !writeSSE(lg, w, rc, event.T, event.Event) {
				return
			}
		case <-r.Context().Done():
			lg.Debug("client closed HTTP connection")
			return
		}
	}
}

// writeSSE writes evs to the client, skipping zero events, and flushes,
// giving up if the client has not accepted them by maxSSELag after t,
// the time of the data they carry; a zero t means now. It reports whether
// they were written.
func writeSSE(lg *slog.Logger, w http.ResponseWriter, rc *http.ResponseController, t time.Time, evs ...sse.Event) bool {
	if t.IsZero() {
		t = time.Now()
	}
	err := rc.SetWriteDeadline(t.Add(maxSSELag))
	for _, ev := range evs {
		if err != nil {
			break
		}
		if !ev.IsZero() {
			_, err = w.Write([]byte(ev.Format()))
		}
	}
	if err == nil {
		err = rc.Flush()
	}
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		lg.Info("SSE client is not keeping up, disconnecting", "maxLag", maxSSELag)
	} else if !proxy.IsNormalConnClose(err) {
		lg.Error("error writing HTTP response", "err", err)
	}
	return false
}
