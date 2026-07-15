package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/serialenum"
)

// server adapts a session to HTTP: session methods become POST
// endpoints, snapshots GET endpoints, and events an SSE stream.
type server struct {
	ctx     context.Context // run context; SSE streams end when it is done
	sess    *session.Session
	hub     *sseHub
	lg      *slog.Logger
	token   string        // empty means no auth
	vendor  gpsreg.Vendor // --vendor: the vendor for every connect
	msgDirs []string      // message-file library search path
	mux     *http.ServeMux
	files   http.Handler // static SPA assets, served under /
	seatMu  sync.Mutex
	seat    string
	// Single-use launch token, minted only when a browser is auto-opened on a
	// platform whose argv leaks the URL. It has its own lock, separate from the
	// seat: the two protect unrelated state.
	singleMu   sync.Mutex
	single     string // empty means none minted
	singleExp  time.Time
	singleUsed bool
}

func newServer(ctx context.Context, sess *session.Session, hub *sseHub, lg *slog.Logger, token string, vendor gpsreg.Vendor, msgDirs []string) *server {
	s := &server{ctx: ctx, sess: sess, hub: hub, lg: lg, token: token, vendor: vendor, msgDirs: msgDirs, mux: http.NewServeMux(), files: http.FileServer(http.FS(webContent()))}
	// Seed the writer broadcast with a no-holder value: after a restart with
	// the token disabled, an old tab's EventSource reconnects and re-primes
	// from this hub, and must learn that its previous seat is stale (flipping
	// read-only) rather than keep rendering writable against the empty seat,
	// which 410s every writer POST. A first claim overwrites the seed, which
	// never becomes the current seat.
	hub.broadcastWriter(randHex())
	// get: token-checked. reader: token-checked and requires a JSON
	// Content-Type, which blocks cross-site CSRF (see requireJSON); for POSTs
	// that only read session state, plus the seat claim itself. writer: like
	// reader but also requires the current seat, for the mutating session
	// operations. Claiming the seat is the one mutating POST without a seat.
	get := s.auth
	reader := func(h http.HandlerFunc) http.HandlerFunc { return s.auth(requireJSON(h)) }
	writer := func(h http.HandlerFunc) http.HandlerFunc { return s.auth(s.requireSeat(requireJSON(h))) }
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("GET /sse", get(s.handleSSE))
	s.mux.HandleFunc("POST /api/seat", reader(s.handleSeat))
	s.mux.HandleFunc("GET /api/state", get(s.handleState))
	s.mux.HandleFunc("GET /api/receiver", get(s.handleReceiver))
	s.mux.HandleFunc("GET /api/speed", get(s.handleSpeed))
	s.mux.HandleFunc("GET /api/corrections", get(s.handleCorrState))
	s.mux.HandleFunc("GET /api/ports", get(s.handlePorts))
	s.mux.HandleFunc("POST /api/connect", writer(s.handleConnect))
	s.mux.HandleFunc("POST /api/disconnect", writer(s.handleDisconnect))
	s.mux.HandleFunc("POST /api/config/read", writer(s.handleReadConfig))
	s.mux.HandleFunc("POST /api/config/apply", writer(s.handleApplyConfig))
	s.mux.HandleFunc("POST /api/signals", reader(s.handleSignals))
	s.mux.HandleFunc("POST /api/corrections/start", writer(s.handleCorrStart))
	s.mux.HandleFunc("POST /api/corrections/stop", writer(s.handleCorrStop))
	s.mux.HandleFunc("GET /api/msgfile/catalog", get(s.handleMsgCatalog))
	s.mux.HandleFunc("POST /api/msgfile/select", writer(s.handleMsgSelect))
	s.mux.HandleFunc("POST /api/msgfile/send", writer(s.handleMsgSend))
	s.mux.HandleFunc("POST /api/msgfile/cancel", writer(s.handleMsgCancel))
	s.mux.HandleFunc("POST /api/decode-packet", reader(s.handleDecodePacket))
	s.mux.HandleFunc("POST /api/geo/ecef-to-llh", reader(s.handleECEFtoLLH))
	s.mux.HandleFunc("POST /api/geo/llh-to-ecef", reader(s.handleLLHtoECEF))
	s.mux.HandleFunc("POST /api/geo/check-on-earth", reader(s.handleCheckOnEarth))
	s.mux.HandleFunc("POST /api/geo/vel-ned-to-ecef", reader(s.handleVelNEDtoECEF))
	s.mux.HandleFunc("POST /api/geo/vel-ecef-to-ned", reader(s.handleVelECEFtoNED))
	return s
}

// newSingleUseToken creates and registers a single-use launch token. On
// argv-leaky platforms the browser-launch URL carries this instead of the
// real token; its first presentation on GET / redirects to the real-token
// URL, and any later use is an error, so a stolen argv value is caught.
func (s *server) newSingleUseToken() string {
	token := newToken()
	s.singleMu.Lock()
	s.single = token
	s.singleExp = time.Now().Add(60 * time.Second)
	s.singleUsed = false
	s.singleMu.Unlock()
	return token
}

type launchResult int

const (
	launchMiss launchResult = iota // not the single-use token; serve the SPA
	launchOK                       // valid first use; redirect to the real token
	launchUsed                     // presented after redemption (theft alarm)
	launchExpired
)

// handleRoot serves the SPA, intercepting only GET / with a t query that
// matches a minted single-use launch token; every other request (a real
// token, no token, an asset path) falls through to the file server, which
// ignores the query exactly as before.
func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		if t := r.URL.Query().Get("t"); t != "" {
			switch s.redeemSingleUse(t) {
			case launchOK:
				http.Redirect(w, r, "/?t="+s.token, http.StatusFound)
				return
			case launchUsed:
				s.lg.Warn("single-use launch token presented again after use", "remote", r.RemoteAddr)
				launchErrorPage(w)
				return
			case launchExpired:
				s.lg.Warn("single-use launch token used after expiry", "remote", r.RemoteAddr)
				launchErrorPage(w)
				return
			}
		}
	}
	s.files.ServeHTTP(w, r)
}

// redeemSingleUse checks t against the single-use launch token with a
// constant-time compare and, on a valid first use, marks it used. A miss
// (none minted, or t is some other value such as the real token) falls
// through to the SPA.
func (s *server) redeemSingleUse(t string) launchResult {
	s.singleMu.Lock()
	defer s.singleMu.Unlock()
	if s.single == "" || subtle.ConstantTimeCompare([]byte(t), []byte(s.single)) != 1 {
		return launchMiss
	}
	if s.singleUsed {
		return launchUsed
	}
	if !time.Now().Before(s.singleExp) {
		return launchExpired
	}
	s.singleUsed = true
	return launchOK
}

// launchErrorPage tells the person who followed a stale browser-launch
// link what happened. Plain text, not the JSON error shape: the reader is
// a human in a browser, not the SPA.
func launchErrorPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusGone)
	io.WriteString(w, "This launch link has already been used or has expired. Restart satpulsewb for a fresh one.\n")
}

func (s *server) requireSeat(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.seatMu.Lock()
		current := r.URL.Query().Get("seat") == s.seat && s.seat != ""
		s.seatMu.Unlock()
		if !current {
			writeError(w, http.StatusGone, errors.New("workbench seat is no longer current"))
			return
		}
		h(w, r)
	}
}

// auth wraps a handler with the access-token check. The token rides a
// query parameter rather than a header because EventSource cannot set
// headers. Static assets are served without auth: only the API and the
// event stream are protected.
func (s *server) auth(h http.HandlerFunc) http.HandlerFunc {
	if s.token == "" {
		return h
	}
	tok := []byte(s.token)
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), tok) != 1 {
			writeError(w, http.StatusUnauthorized, errors.New("missing or invalid access token"))
			return
		}
		h(w, r)
	}
}

// requireJSON rejects a state-changing request whose body is not
// declared application/json. This is the CSRF guard: a cross-site page
// can issue "simple" POSTs (form/text bodies) without a CORS preflight,
// but setting Content-Type to application/json forces a preflight the
// browser blocks, since the server sends no CORS headers. It matters
// even with the token disabled (-L), where auth is a no-op.
func requireJSON(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, errors.New("Content-Type must be application/json"))
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// readJSON decodes the request body into v, writing a 400 response and
// returning false on failure.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

// sessionError maps a session method failure onto the response. The
// errors are state preconditions ("not connected") or operation
// failures, both conflicts with the current session state.
func sessionError(w http.ResponseWriter, err error) {
	writeError(w, http.StatusConflict, err)
}

// handleSeat claims the write seat, unconditionally: it mints a fresh value,
// makes it current, returns it to the claimant, and broadcasts it so every
// window learns who now holds the seat. Newest claim wins. The seat check on
// writer POSTs is a freshness check, not authentication - every window that
// can claim is equally trusted.
func (s *server) handleSeat(w http.ResponseWriter, _ *http.Request) {
	seat := randHex()
	s.seatMu.Lock()
	s.seat = seat
	s.hub.broadcastWriter(seat)
	s.seatMu.Unlock()
	writeJSON(w, map[string]string{"seat": seat})
}

// randHex returns a 128-bit crypto-random value as hex, used for the seat
// values. rand.Read cannot fail (it crashes the program if the platform
// source of randomness does).
func randHex() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *server) handleSSE(w http.ResponseWriter, r *http.Request) {
	c, prime := s.hub.subscribe(r.URL.Query().Get("stream") == "packets")
	defer s.hub.unsubscribe(c)
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for _, e := range prime {
		if _, err := io.WriteString(w, e.Format()); err != nil {
			return
		}
	}
	flusher.Flush()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-r.Context().Done():
			return
		case <-c.dead:
			// Fell behind and overflowed; ending the response makes the
			// browser reconnect and re-prime from the cache.
			return
		case e := <-c.ch:
			if _, err := io.WriteString(w, e.Format()); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *server) handleState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.sess.State())
}

func (s *server) handleReceiver(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.sess.Receiver())
}

func (s *server) handleSpeed(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.sess.Speed())
}

func (s *server) handleCorrState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.sess.CorrectionsState())
}

func (s *server) handlePorts(w http.ResponseWriter, _ *http.Request) {
	ports, err := serialenum.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if ports == nil {
		ports = []serialenum.Port{}
	}
	writeJSON(w, ports)
}

func (s *server) handleConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device string `json:"device"`
		Speed  int    `json:"speed"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Device == "" {
		writeError(w, http.StatusBadRequest, errors.New("device is required"))
		return
	}
	if err := s.sess.Connect(session.SerialOpener{Device: req.Device, Speed: req.Speed}, s.vendor); err != nil {
		sessionError(w, err)
		return
	}
	writeJSON(w, struct{}{})
}

func (s *server) handleDisconnect(w http.ResponseWriter, _ *http.Request) {
	s.sess.Disconnect()
	writeJSON(w, struct{}{})
}

func (s *server) handleReadConfig(w http.ResponseWriter, _ *http.Request) {
	// s.ctx, not the request context: the configure run must finish (or
	// stop only on server shutdown) even if the browser reloads or the
	// HTTP connection drops mid-operation, so the receiver is never left
	// half-configured.
	props, err := s.sess.ReadConfig(s.ctx)
	if err != nil {
		sessionError(w, err)
		return
	}
	writeJSON(w, props)
}

func (s *server) handleApplyConfig(w http.ResponseWriter, r *http.Request) {
	target := gpsprot.NewConfigTarget()
	if !readJSON(w, r, target) {
		return
	}
	// s.ctx, not the request context: see handleReadConfig.
	if err := s.sess.ApplyConfig(s.ctx, target); err != nil {
		sessionError(w, err)
		return
	}
	writeJSON(w, struct{}{})
}

func (s *server) handleSignals(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GNSS gpsprot.GNSSSet `json:"gnss"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	writeJSON(w, s.sess.SignalCatalog(req.GNSS))
}

func (s *server) handleCorrStart(w http.ResponseWriter, r *http.Request) {
	var src session.CorrectionSource
	if !readJSON(w, r, &src) {
		return
	}
	if err := s.sess.StartCorrections(src); err != nil {
		sessionError(w, err)
		return
	}
	writeJSON(w, struct{}{})
}

func (s *server) handleCorrStop(w http.ResponseWriter, _ *http.Request) {
	if err := s.sess.StopCorrections(); err != nil {
		sessionError(w, err)
		return
	}
	writeJSON(w, struct{}{})
}

func (s *server) handleDecodePacket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data string `json:"data"`
		Hex  bool   `json:"hex"`
		Out  bool   `json:"out"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	b := []byte(req.Data)
	if req.Hex {
		var err error
		if b, err = hex.DecodeString(req.Data); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid hex: %w", err))
			return
		}
	}
	rslt, err := session.DecodePacket(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), b, req.Out)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, rslt)
}

// llh is the JSON shape for geodetic coordinates: latitude and
// longitude in degrees, height above the WGS84 ellipsoid in meters.
type llh struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Height float64 `json:"height"`
}

type xyz struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

func (s *server) handleECEFtoLLH(w http.ResponseWriter, r *http.Request) {
	var req xyz
	if !readJSON(w, r, &req) {
		return
	}
	v, err := geopos.WGS84.ECEFtoLLH(geopos.ECEF{req.X, req.Y, req.Z})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, llh{Lat: v.Lat, Lon: v.Lon, Height: v.Height})
}

func (s *server) handleLLHtoECEF(w http.ResponseWriter, r *http.Request) {
	var req llh
	if !readJSON(w, r, &req) {
		return
	}
	writeJSON(w, [3]float64(geopos.WGS84.LLHtoECEF(geopos.LLH{Lat: req.Lat, Lon: req.Lon, Height: req.Height})))
}

func (s *server) handleCheckOnEarth(w http.ResponseWriter, r *http.Request) {
	var req xyz
	if !readJSON(w, r, &req) {
		return
	}
	writeJSON(w, geopos.ECEF{req.X, req.Y, req.Z}.CheckOnEarth() == nil)
}

func (s *server) handleVelNEDtoECEF(w http.ResponseWriter, r *http.Request) {
	var req struct {
		N float64 `json:"n"`
		E float64 `json:"e"`
		D float64 `json:"d"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	writeJSON(w, s.sess.VelNEDtoECEF(req.N, req.E, req.D))
}

func (s *server) handleVelECEFtoNED(w http.ResponseWriter, r *http.Request) {
	var req xyz
	if !readJSON(w, r, &req) {
		return
	}
	writeJSON(w, s.sess.VelECEFtoNED(req.X, req.Y, req.Z))
}
