package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

func newTestServer(token string) *server {
	hub := newSSEHub()
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	sess := session.New(lg, hub, session.Options{})
	return newServer(context.Background(), sess, hub, token)
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
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
	return w
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
		{name: "connect empty device", method: "POST", url: "/api/connect", body: `{"device":"","speed":9600,"vendor":""}`,
			expectCode: 400},
		{name: "connect bad vendor", method: "POST", url: "/api/connect", body: `{"device":"/dev/x","speed":9600,"vendor":"nonesuch"}`,
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
	s.hub.Emit(session.Event{Name: session.EventState, Data: session.StateConnected})
	s.hub.Emit(session.Event{Name: session.EventSpeed, Data: 9600})
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/sse", nil).WithContext(ctx)
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
