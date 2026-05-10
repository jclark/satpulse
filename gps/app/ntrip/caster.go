package ntrip

import (
	"bufio"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
)

// Start validates cfg, opens a TCP listener, and starts goroutines
// that accept and serve Ntrip client connections.  It returns once
// the listener is open; serving continues until ctx is cancelled.
//
// version is the satpulse version (e.g. from cmd.Version()), used in
// the Server response header.  Empty is allowed.
//
// buildSTR is the closure returned by StreamRecordBuilder.  Start
// calls it once per mountpoint at startup, prepends "STR;<name>;" to
// each result, and stores the resulting line-prefixed strings in
// caster runtime state to be served as part of the source-table
// response.  After startup the closure is not retained.
func Start(ctx context.Context, lg *slog.Logger,
	wg *sync.WaitGroup, cfg Config, version string,
	b *bcast.Bcast[scan.Packet],
	buildSTR func(sc *StreamConfig, name string) string,
) error {
	c := newCaster(lg, cfg, version, b, buildSTR)
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.listen())
	if err != nil {
		return err
	}
	server := c.newServer()
	wg.Go(func() {
		lg.Debug("Ntrip caster listening", "addr", listener.Addr())
		if err := server.Serve(listener); err != http.ErrServerClosed {
			lg.Error("Ntrip serve error", "err", err)
		}
		lg.Debug("Ntrip caster about to exit", "addr", listener.Addr())
	})
	wg.Go(func() {
		<-ctx.Done()
		lg.Debug("initiating Ntrip caster shutdown")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			lg.Error("Ntrip shutdown error", "err", err)
		}
	})
	return nil
}

// gracefulShutdownTimeout bounds how long server.Shutdown waits for
// in-flight handlers to return, so a stuck streamLoop (e.g. blocked
// on a slow client's Write) cannot hang daemon shutdown.
const gracefulShutdownTimeout = 1 * time.Second

// caster is the runtime state of the Ntrip caster.
type caster struct {
	lg          *slog.Logger
	bcast       *bcast.Bcast[scan.Packet]
	mounts      map[string]*mount
	users       map[string]string // username -> password
	sourceTable string
	serverHdr   string
}

// mount is a configured mountpoint at runtime.
type mount struct {
	name    string
	allowed map[string]struct{} // nil means all defined users (or anonymous)
}

// buildServerHdr builds the Server response header value.  The
// "NTRIP" prefix uses the all-caps wire convention from Appendix B
// of the v1 spec.  If version is empty (development build), the
// header omits the "/<version>" suffix.
func buildServerHdr(version string) string {
	if version == "" {
		return "NTRIP satpulse"
	}
	return "NTRIP satpulse/" + version
}

func newCaster(lg *slog.Logger, cfg Config, version string,
	b *bcast.Bcast[scan.Packet],
	buildSTR func(sc *StreamConfig, name string) string,
) *caster {
	users := make(map[string]string, len(cfg.Users))
	for _, u := range cfg.Users {
		users[u.Username] = u.Password
	}
	mounts := make(map[string]*mount, len(cfg.Mountpoint))
	var sb strings.Builder
	for i := range cfg.Mountpoint {
		mc := &cfg.Mountpoint[i]
		fields := buildSTR(&mc.StreamConfig, mc.Name)
		var allowed map[string]struct{}
		if len(mc.Users) > 0 {
			allowed = make(map[string]struct{}, len(mc.Users))
			for _, u := range mc.Users {
				allowed[u] = struct{}{}
			}
		}
		mounts[mc.Name] = &mount{
			name:    mc.Name,
			allowed: allowed,
		}
		sb.WriteString("STR;" + mc.Name + ";" + fields + "\r\n")
	}
	sb.WriteString("ENDSOURCETABLE\r\n")
	return &caster{
		lg:          lg,
		bcast:       b,
		mounts:      mounts,
		users:       users,
		sourceTable: sb.String(),
		serverHdr:   buildServerHdr(version),
	}
}

// newServer constructs a configured *http.Server backing the caster.
// It disables HTTP/2 (needed for hijacking) and routes all requests
// through serveHTTP.
func (c *caster) newServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", c.serveHTTP)
	return &http.Server{
		Handler:      mux,
		ErrorLog:     slogErrorLog(c.lg),
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
	}
}

// serveHTTP dispatches an incoming request: source table for /, or
// stream/auth handling for /<mountpoint>.
func (c *caster) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	v2 := isNtripV2(r)
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		c.serveSourceTable(w, v2)
		return
	}
	m, ok := c.mounts[path]
	if !ok {
		if v2 {
			http.NotFound(w, r)
			return
		}
		c.serveSourceTable(w, false)
		return
	}
	if !c.checkAuth(r, m) {
		c.sendUnauthorized(w, m, v2)
		return
	}
	c.serveStream(w, r, m, v2)
}

// isNtripV2 returns true if the request claims Ntrip v2 via header.
func isNtripV2(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Ntrip-Version"), "Ntrip/2.0")
}

// checkAuth returns true if the request is authorized for mount m.
// If no users are defined globally, any request is allowed.
// Otherwise basic auth must match a defined user, and that user must
// be in m.allowed (when m.allowed restricts).
func (c *caster) checkAuth(r *http.Request, m *mount) bool {
	if len(c.users) == 0 {
		return true
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	want, found := c.users[user]
	if !found {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(pass), []byte(want)) != 1 {
		return false
	}
	if m.allowed != nil {
		if _, ok := m.allowed[user]; !ok {
			return false
		}
	}
	return true
}

// sendUnauthorized writes a 401 response with WWW-Authenticate.
func (c *caster) sendUnauthorized(w http.ResponseWriter, m *mount, v2 bool) {
	w.Header().Set("WWW-Authenticate", `Basic realm="/`+m.name+`"`)
	if v2 {
		w.Header().Set("Ntrip-Version", "Ntrip/2.0")
	}
	w.Header().Set("Server", c.serverHdr)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// serveSourceTable writes the source table to w.  v1 hijacks the
// connection because the status line "SOURCETABLE 200 OK" is not a
// valid HTTP status line.
func (c *caster) serveSourceTable(w http.ResponseWriter, v2 bool) {
	body := c.sourceTable
	if v2 {
		h := w.Header()
		h.Set("Content-Type", "gnss/sourcetable")
		h.Set("Ntrip-Version", "Ntrip/2.0")
		h.Set("Ntrip-Flags", "")
		h.Set("Server", c.serverHdr)
		h.Set("Connection", "close")
		h.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	conn, bw, err := hj.Hijack()
	if err != nil {
		c.lg.Error("hijack failed", "err", err)
		return
	}
	defer conn.Close()
	fmt.Fprintf(bw, "SOURCETABLE 200 OK\r\n")
	fmt.Fprintf(bw, "Server: %s\r\n", c.serverHdr)
	fmt.Fprintf(bw, "Content-Type: text/plain\r\n")
	fmt.Fprintf(bw, "Content-Length: %d\r\n", len(body))
	fmt.Fprintf(bw, "\r\n")
	_, _ = bw.WriteString(body)
	_ = bw.Flush()
}

// serveStream handles a successful GET /<mountpoint> request.
func (c *caster) serveStream(w http.ResponseWriter, r *http.Request, _ *mount, v2 bool) {
	if v2 {
		c.serveStreamV2(w, r)
	} else {
		c.serveStreamV1(w, r)
	}
}

// serveStreamV1 hijacks the connection and writes the 12-byte
// "ICY 200 OK\r\n" status line followed by the raw RTCM byte stream.
func (c *caster) serveStreamV1(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		c.lg.Error("hijack failed", "err", err)
		return
	}
	defer conn.Close()
	bw := brw.Writer
	if _, err := bw.WriteString("ICY 200 OK\r\n"); err != nil {
		return
	}
	if err := bw.Flush(); err != nil {
		return
	}
	c.streamLoop(r.Context(), bufWriter{bw})
}

// serveStreamV2 streams via the http.ResponseWriter using chunked
// encoding (Go's default since Content-Length is unknown).
func (c *caster) serveStreamV2(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "gnss/data")
	h.Set("Cache-Control", "no-store, no-cache, max-age=0")
	h.Set("Pragma", "no-cache")
	h.Set("Connection", "close")
	h.Set("Server", c.serverHdr)
	h.Set("Ntrip-Version", "Ntrip/2.0")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		return
	}
	c.streamLoop(r.Context(), respWriter{w, rc})
}

// streamWriter is the writer abstraction used by streamLoop so the
// same loop serves v1 (raw conn) and v2 (chunked HTTP) paths.
type streamWriter interface {
	io.Writer
	flush() error
}

type bufWriter struct{ w *bufio.Writer }

func (b bufWriter) Write(p []byte) (int, error) { return b.w.Write(p) }
func (b bufWriter) flush() error                { return b.w.Flush() }

type respWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (r respWriter) Write(p []byte) (int, error) { return r.w.Write(p) }
func (r respWriter) flush() error                { return r.rc.Flush() }

// streamLoop subscribes to the packet bcast and writes RTCM packets
// to w until ctx is cancelled, the bcast closes, or a write fails.
// Mirrors proxy.connWriteWorker in time/internal/proxy.
func (c *caster) streamLoop(ctx context.Context, w streamWriter) {
	defer c.lg.Debug("about to exit ntrip stream worker")
	ch := c.bcast.Subscribe()
	defer c.bcast.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case pkt, ok := <-ch:
			if !ok {
				return
			}
			if !pkt.HasTag(gpsreg.TagRTCM) || !pkt.ChecksumValid {
				continue
			}
			if _, err := w.Write([]byte(pkt.Data)); err != nil {
				logConnErr(c.lg, "error writing to ntrip stream", err)
				return
			}
			if err := w.flush(); err != nil {
				logConnErr(c.lg, "error flushing ntrip stream", err)
				return
			}
		}
	}
}

// logConnErr logs a connection error unless it's a normal close
// (net.ErrClosed).  Matches proxy.logConnErr.
func logConnErr(lg *slog.Logger, msg string, err error) {
	if !errors.Is(err, net.ErrClosed) {
		lg.Error(msg, "err", err)
	}
}

// slogErrorLog adapts an slog.Logger for use as http.Server.ErrorLog.
func slogErrorLog(lg *slog.Logger) *log.Logger {
	return slog.NewLogLogger(lg.Handler(), slog.LevelError)
}

