package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

func newTestServer(token string) *server {
	hub := newSSEHub()
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	sess := session.New(lg, hub, session.Options{})
	return newServer(context.Background(), sess, hub, token, gpsreg.VendorUnknown)
}

func TestAuth(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		url        string
		expectCode int
	}{
		{name: "no token", token: "secret", url: "/api/state", expectCode: http.StatusUnauthorized},
		{name: "wrong token", token: "secret", url: "/api/state?t=wrong", expectCode: http.StatusUnauthorized},
		{name: "good token", token: "secret", url: "/api/state?t=secret", expectCode: http.StatusOK},
		{name: "auth disabled", token: "", url: "/api/state", expectCode: http.StatusOK},
		{name: "static needs no token", token: "secret", url: "/", expectCode: http.StatusOK},
		{name: "sse needs token", token: "secret", url: "/sse", expectCode: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(tc.token)
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, httptest.NewRequest("GET", tc.url, nil))
			if w.Code != tc.expectCode {
				t.Errorf("got %d want %d", w.Code, tc.expectCode)
			}
		})
	}
}

func (s *server) post(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	s.seatMu.Lock()
	seat := s.seat
	s.seatMu.Unlock()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return s.rawPost(path+sep+"seat="+seat, body)
}

func (s *server) rawPost(path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	s.mux.ServeHTTP(w, r)
	return w
}

func (s *server) claimTestSeat(t *testing.T, query string) string {
	t.Helper()
	w := s.rawPost("/api/seat"+query, `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("claim seat: status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Seat string `json:"seat"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode seat: %v", err)
	}
	if len(resp.Seat) != 32 {
		t.Fatalf("seat length %d want 32", len(resp.Seat))
	}
	return resp.Seat
}

func TestSeatClaimGuards(t *testing.T) {
	s := newTestServer("secret")
	w := s.rawPost("/api/seat", `{}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("claim without token: got %d want %d", w.Code, http.StatusUnauthorized)
	}
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/seat?t=secret", strings.NewReader(`{}`)))
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("claim without JSON: got %d want %d", w.Code, http.StatusUnsupportedMediaType)
	}
	s.claimTestSeat(t, "?t=secret")
}

// TestRequireJSON checks the CSRF guard: a POST without a JSON
// Content-Type is rejected before reaching the handler.
func TestRequireJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expectCode  int
	}{
		{name: "no content type", contentType: "", expectCode: http.StatusUnsupportedMediaType},
		{name: "form content type", contentType: "application/x-www-form-urlencoded", expectCode: http.StatusUnsupportedMediaType},
		{name: "text content type", contentType: "text/plain", expectCode: http.StatusUnsupportedMediaType},
		{name: "json", contentType: "application/json", expectCode: http.StatusBadRequest}, // empty device
		{name: "json with charset", contentType: "application/json; charset=utf-8", expectCode: http.StatusBadRequest},
	}
	s := newTestServer("")
	seat := s.claimTestSeat(t, "")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/connect?seat="+seat, strings.NewReader(`{"device":""}`))
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			s.mux.ServeHTTP(w, r)
			if w.Code != tc.expectCode {
				t.Errorf("got %d want %d", w.Code, tc.expectCode)
			}
		})
	}
}

func TestEndpoints(t *testing.T) {
	nmea := "$GPGGA,000000.00,0000.000,N,00000.000,E,1,05,1.0,0.0,M,0.0,M,,"
	nmea = fmt.Sprintf("%s*%02X\r\n", nmea, nmeamsg.Checksum([]byte(nmea[1:])))
	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		expectCode int
		expectBody string // trimmed; empty means don't check
	}{
		{name: "state", method: "GET", url: "/api/state", expectCode: 200, expectBody: `"disconnected"`},
		{name: "receiver", method: "GET", url: "/api/receiver", expectCode: 200, expectBody: `{"ok":false}`},
		{name: "speed", method: "GET", url: "/api/speed", expectCode: 200, expectBody: `0`},
		{name: "corrections", method: "GET", url: "/api/corrections", expectCode: 200, expectBody: `{"state":"stopped"}`},
		{name: "read config not connected", method: "POST", url: "/api/config/read", body: `{}`,
			expectCode: 409, expectBody: `{"error":"not connected"}`},
		{name: "apply config not connected", method: "POST", url: "/api/config/apply", body: `{}`,
			expectCode: 409, expectBody: `{"error":"not connected"}`},
		{name: "connect empty device", method: "POST", url: "/api/connect", body: `{"device":"","speed":9600}`,
			expectCode: 400},
		{name: "corrections start no host", method: "POST", url: "/api/corrections/start", body: `{"mode":"tcp"}`,
			expectCode: 409},
		{name: "bad json", method: "POST", url: "/api/signals", body: `{`,
			expectCode: 400},
		{name: "signals", method: "POST", url: "/api/signals", body: `{"gnss":["GPS"]}`,
			expectCode: 200},
		{name: "decode nmea", method: "POST", url: "/api/decode-packet",
			body:       fmt.Sprintf(`{"data":%q,"hex":false,"out":false}`, nmea),
			expectCode: 200},
		{name: "decode garbage", method: "POST", url: "/api/decode-packet", body: `{"data":"zzzz","hex":false,"out":false}`,
			expectCode: 200, expectBody: `null`},
		{name: "decode bad hex", method: "POST", url: "/api/decode-packet", body: `{"data":"zz","hex":true,"out":false}`,
			expectCode: 400},
		{name: "ecef to llh", method: "POST", url: "/api/geo/ecef-to-llh", body: `{"x":6378137,"y":0,"z":0}`,
			expectCode: 200, expectBody: `{"lat":0,"lon":0,"height":0}`},
		{name: "llh to ecef", method: "POST", url: "/api/geo/llh-to-ecef", body: `{"lat":0,"lon":0,"height":0}`,
			expectCode: 200, expectBody: `[6378137,0,0]`},
		{name: "check on earth", method: "POST", url: "/api/geo/check-on-earth", body: `{"x":6378137,"y":0,"z":0}`,
			expectCode: 200, expectBody: `true`},
		{name: "check off earth", method: "POST", url: "/api/geo/check-on-earth", body: `{"x":0,"y":0,"z":0}`,
			expectCode: 200, expectBody: `false`},
		{name: "vel ned no position", method: "POST", url: "/api/geo/vel-ned-to-ecef", body: `{"n":1,"e":0,"d":0}`,
			expectCode: 200, expectBody: `null`},
	}
	s := newTestServer("")
	s.claimTestSeat(t, "")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tc.method == "GET" {
				w = httptest.NewRecorder()
				s.mux.ServeHTTP(w, httptest.NewRequest("GET", tc.url, nil))
			} else {
				w = s.post(t, tc.url, tc.body)
			}
			if w.Code != tc.expectCode {
				t.Fatalf("got status %d want %d (body %s)", w.Code, tc.expectCode, w.Body.String())
			}
			if tc.expectBody != "" {
				got := strings.TrimSpace(w.Body.String())
				if !reflect.DeepEqual(got, tc.expectBody) {
					t.Errorf("got  %s\nwant %s", got, tc.expectBody)
				}
			}
		})
	}
}

// TestSSEPriming checks that a new SSE subscriber is primed with the
// latest sticky events before receiving live ones.
func TestSSEPriming(t *testing.T) {
	s := newTestServer("")
	seat := s.claimTestSeat(t, "")
	s.hub.Emit(session.Event{Name: session.EventState, Data: session.StateConnected})
	s.hub.Emit(session.Event{Name: session.EventSpeed, Data: 9600})
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/sse?seat="+seat, nil).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.mux.ServeHTTP(w, r)
		close(done)
	}()
	// The prime is written before the handler blocks on the event
	// channel; cancelling the request then ends the stream.
	cancel()
	<-done
	expect := "event: gps:state\ndata: \"connected\"\n\nevent: gps:speed\ndata: 9600\n\n"
	if got := w.Body.String(); got != expect {
		t.Errorf("got  %q\nwant %q", got, expect)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("got Content-Type %q want text/event-stream", ct)
	}
}

func TestSeatLifecycle(t *testing.T) {
	s := newTestServer("")
	old := s.claimTestSeat(t, "")
	if w := s.rawPost("/api/signals?seat="+old, `{"gnss":["GPS"]}`); w.Code != http.StatusOK {
		t.Fatalf("current-seat POST: got %d want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/sse?seat="+old, nil).WithContext(ctx))
		close(done)
	}()
	waitForClients(t, s.hub, 1)
	current := s.claimTestSeat(t, "")
	if current == old {
		t.Fatal("second claim returned the old seat")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("old stream did not close on takeover")
	}
	if got, want := w.Body.String(), "event: takeover\ndata: {}\n\n"; got != want {
		t.Errorf("old stream got %q want %q", got, want)
	}
	if w := s.rawPost("/api/signals?seat="+old, `{"gnss":["GPS"]}`); w.Code != http.StatusGone {
		t.Errorf("old-seat POST: got %d want %d", w.Code, http.StatusGone)
	}
	if w := s.rawPost("/api/signals", `{"gnss":["GPS"]}`); w.Code != http.StatusGone {
		t.Errorf("missing-seat POST: got %d want %d", w.Code, http.StatusGone)
	}
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/sse?seat="+old, nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("old-seat SSE: got %d want %d", w.Code, http.StatusNoContent)
	}
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/sse", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing-seat SSE: got %d want %d", w.Code, http.StatusBadRequest)
	}
	ctx, cancel = context.WithCancel(context.Background())
	w = httptest.NewRecorder()
	done = make(chan struct{})
	go func() {
		s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/sse?seat="+current, nil).WithContext(ctx))
		close(done)
	}()
	waitForClients(t, s.hub, 1)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("current stream did not close after cancellation")
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("current-seat SSE Content-Type %q", ct)
	}
}

func waitForClients(t *testing.T, h *sseHub, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		got := len(h.clients)
		h.mu.Unlock()
		if got == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("SSE client count did not reach %d", n)
}
