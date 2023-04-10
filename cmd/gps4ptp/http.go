package main

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jclark/gps4ptp/internal/bcast"
	"golang.org/x/exp/slog"
)

//go:embed "index.html"
var htmlRoot []byte

type HTTPConfig struct {
	Listen string `toml:"listen"`
}

type SSEEvent struct {
	Event string
	Data  string
}

const gracefulShutdownTimeout = 1 * time.Second

func startHTTP(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg []HTTPConfig, b *bcast.Bcast[SSEEvent]) error {
	if len(cfg) == 0 {
		return nil
	}
	for _, c := range cfg {
		if c.Listen == "" {
			return errors.New("must specify listen option for each HTTP element")
		}
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(htmlRoot)
		if err != nil {
			lg.Error("error writing HTTP response for /", err)
		}
	})

	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		sseHandleRequest(ctx, lg, wg, w, r, b)
	})
	listenCfg := net.ListenConfig{}

	listeners := make([]net.Listener, len(cfg))
	for i, c := range cfg {
		listen, err := listenCfg.Listen(ctx, "tcp", c.Listen)
		if err != nil {
			return err
		}
		listeners[i] = listen
	}
	// XXX we should supply an error logger that wraps lg
	server := &http.Server{Handler: mux}

	for _, listener := range listeners {
		wg.Add(1)

		go func(listener net.Listener) {
			defer wg.Done()
			lg.Debug("HTTP server listening", "addr", listener.Addr())
			if err := server.Serve(listener); err != http.ErrServerClosed {
				lg.Error("HTTP serve error", err)
			}
			lg.Debug("HTTP server about to exit", "addr", listener.Addr())
		}(listener)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		lg.Debug("initiating HTTP server shutdown")
		if err := server.Shutdown(shutdownCtx); err != nil {
			lg.Error("Server shutdown error", err)
		}
		lg.Debug("about to exit HTTP server shutdown goroutine")
	}()

	return nil
}

func sseHandleRequest(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, w http.ResponseWriter, r *http.Request, b *bcast.Bcast[SSEEvent]) {
	defer lg.Debug("about to exit HTTP SSE request handler")
	lg.Debug("starting to handle HTTP SSE request")
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher := w.(http.Flusher)
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
			_, err := w.Write(([]byte)(formatSSEEvent(event)))
			if err != nil {
				lg.Error("error writing HTTP response", err)
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			lg.Debug("client closed HTTP connection")
			return
		}
	}

}

func formatSSEEvent(event SSEEvent) string {
	data := formatSSEData(event.Data) + "\n"
	if event.Event != "" {
		return "event: " + event.Event + "\n" + data
	}
	return data
}

func formatSSEData(data string) string {
	if !strings.ContainsAny(data, "\r\n") {
		return "data: " + data + "\n"
	}

	var lines []string

	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		lines = append(lines, "data: "+scanner.Text()+"\n")
	}
	if err := scanner.Err(); err != nil {
		panic(fmt.Errorf("failed to scan data: %v", err))
	}

	return strings.Join(lines, "")
}
