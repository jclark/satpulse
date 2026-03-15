package daemon

import (
	"context"
	_ "embed"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/time/internal/promobs"
	"github.com/jclark/satpulse/time/lib/sse"
	"github.com/jclark/satpulse/time/internal/sseobs"
	"github.com/jclark/satpulse/web"
)

type HTTPConfig struct {
	Listen   string `toml:"listen"`
	PProf    bool   `toml:"pprof"`
	GUI      *bool  `toml:"gui"`      // Serve graphical user interface
	Metrics  *bool  `toml:"metrics"`  // Serve Prometheus metrics endpoint
	Position *bool  `toml:"position"` // Serve current position endpoint
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

func startHTTP(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg []HTTPConfig, b *bcast.Bcast[sse.Event], sseObs *sseobs.SSEObserver, promObs *promobs.PrometheusObserver, posObs *positionObserver) error {
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
				sseHandleRequest(ctx, lg, w, r, b, sseObs.InitEvent())
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

func sseHandleRequest(ctx context.Context, lg *slog.Logger, w http.ResponseWriter, r *http.Request, b *bcast.Bcast[sse.Event], initEvent sse.Event) {
	defer lg.Debug("about to exit HTTP SSE request handler")
	lg.Debug("starting to handle HTTP SSE request")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher := w.(http.Flusher)
	if !initEvent.IsZero() {
		_, err := w.Write(([]byte)(initEvent.Format()))
		if err != nil {
			lg.Error("error writing HTTP response", "err", err)
			return
		}
	}
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				lg.Debug("broadcast channel to SSE request handler closed")
				return
			}
			lg.Debug("received a broadcast SSE event", "event", event)
			// XXX: handle error
			_, err := w.Write(([]byte)(event.Format()))
			if err != nil {
				lg.Error("error writing HTTP response", "err", err)
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			lg.Debug("client closed HTTP connection")
			return
		}
	}

}
