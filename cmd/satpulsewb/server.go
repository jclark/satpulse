package main

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/serialenum"
)

// server adapts a session to HTTP: session methods become POST
// endpoints, snapshots GET endpoints, and events an SSE stream.
type server struct {
	ctx   context.Context // run context; SSE streams end when it is done
	sess  *session.Session
	hub   *sseHub
	token string // empty means no auth
	mux   *http.ServeMux
}

func newServer(ctx context.Context, sess *session.Session, hub *sseHub, token string) *server {
	s := &server{ctx: ctx, sess: sess, hub: hub, token: token, mux: http.NewServeMux()}
	// get: token-checked. post: token-checked and requires a JSON
	// Content-Type, which blocks cross-site CSRF (see requireJSON).
	get := s.auth
	post := func(h http.HandlerFunc) http.HandlerFunc { return s.auth(requireJSON(h)) }
	s.mux.Handle("/", http.FileServer(http.FS(webContent())))
	s.mux.HandleFunc("GET /sse", get(s.handleSSE))
	s.mux.HandleFunc("GET /api/state", get(s.handleState))
	s.mux.HandleFunc("GET /api/receiver", get(s.handleReceiver))
	s.mux.HandleFunc("GET /api/speed", get(s.handleSpeed))
	s.mux.HandleFunc("GET /api/corrections", get(s.handleCorrState))
	s.mux.HandleFunc("GET /api/ports", get(s.handlePorts))
	s.mux.HandleFunc("GET /api/vendors", get(s.handleVendors))
	s.mux.HandleFunc("POST /api/connect", post(s.handleConnect))
	s.mux.HandleFunc("POST /api/disconnect", post(s.handleDisconnect))
	s.mux.HandleFunc("POST /api/config/read", post(s.handleReadConfig))
	s.mux.HandleFunc("POST /api/config/apply", post(s.handleApplyConfig))
	s.mux.HandleFunc("POST /api/signals", post(s.handleSignals))
	s.mux.HandleFunc("POST /api/corrections/start", post(s.handleCorrStart))
	s.mux.HandleFunc("POST /api/corrections/stop", post(s.handleCorrStop))
	s.mux.HandleFunc("POST /api/decode-packet", post(s.handleDecodePacket))
	s.mux.HandleFunc("POST /api/geo/ecef-to-llh", post(s.handleECEFtoLLH))
	s.mux.HandleFunc("POST /api/geo/llh-to-ecef", post(s.handleLLHtoECEF))
	s.mux.HandleFunc("POST /api/geo/check-on-earth", post(s.handleCheckOnEarth))
	s.mux.HandleFunc("POST /api/geo/vel-ned-to-ecef", post(s.handleVelNEDtoECEF))
	s.mux.HandleFunc("POST /api/geo/vel-ecef-to-ned", post(s.handleVelECEFtoNED))
	return s
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
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
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

func (s *server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	c, prime := s.hub.subscribe(r.URL.Query().Get("stream") == "packets")
	defer s.hub.unsubscribe(c)
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

func (s *server) handleVendors(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, gpsreg.VendorNames())
}

func (s *server) handleConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device string `json:"device"`
		Speed  int    `json:"speed"`
		Vendor string `json:"vendor"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Device == "" {
		writeError(w, http.StatusBadRequest, errors.New("device is required"))
		return
	}
	vendor, err := gpsreg.ParseVendor(req.Vendor)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.sess.Connect(session.SerialOpener{Device: req.Device, Speed: req.Speed}, vendor); err != nil {
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
