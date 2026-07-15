package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/msgfile"
)

func newTestServer(token string) *server {
	return newTestServerFull(token, gpsreg.VendorUnknown, nil)
}

func newTestServerFull(token string, vendor gpsreg.Vendor, msgDirs []string) *server {
	hub := newSSEHub()
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	sess := session.New(lg, hub, session.Options{})
	return newServer(context.Background(), sess, hub, lg, token, vendor, msgDirs)
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
		{name: "unknown json field", method: "POST", url: "/api/signals", body: `{"gnss":["GPS"],"unknown":true}`,
			expectCode: 400},
		{name: "multiple json values", method: "POST", url: "/api/signals", body: `{"gnss":["GPS"]} {}`,
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
		{name: "msg send not connected", method: "POST", url: "/api/msgfile/send",
			body: `{"tag":"x","port":"","save":false}`, expectCode: 409, expectBody: `{"error":"not connected"}`},
		{name: "msg cancel not sending", method: "POST", url: "/api/msgfile/cancel", body: `{}`,
			expectCode: 409, expectBody: `{"error":"not sending"}`},
		{name: "msg select unknown name", method: "POST", url: "/api/msgfile/select",
			body: `{"vendor":"no-such-vendor","file":"a"}`, expectCode: 400},
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

// TestSingleUseLaunch covers the single-use browser-launch token: a fresh
// one redirects to the real-token URL and burns; a second use and an
// expired one get the human error page, not a redirect; it never
// authenticates the API; and GET / with the real token or none still
// serves the SPA.
func TestSingleUseLaunch(t *testing.T) {
	const tok = "multiuse"

	s := newTestServer(tok)
	launch := s.newSingleUseToken()
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/?t="+launch, nil))
	if w.Code != http.StatusFound {
		t.Fatalf("first use: got %d want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/?t="+tok {
		t.Errorf("redirect Location %q want %q", loc, "/?t="+tok)
	}
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/?t="+launch, nil))
	if w.Code != http.StatusGone {
		t.Errorf("second use: got %d want %d (want error page, not redirect)", w.Code, http.StatusGone)
	}

	s = newTestServer(tok)
	launch = s.newSingleUseToken()
	s.singleMu.Lock()
	s.singleExp = time.Now().Add(-time.Second)
	s.singleMu.Unlock()
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/?t="+launch, nil))
	if w.Code != http.StatusGone {
		t.Errorf("expired: got %d want %d", w.Code, http.StatusGone)
	}

	s = newTestServer(tok)
	launch = s.newSingleUseToken()
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/state?t="+launch, nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("single-use token on API: got %d want %d", w.Code, http.StatusUnauthorized)
	}
	for _, url := range []string{"/", "/?t=" + tok} {
		w = httptest.NewRecorder()
		s.mux.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		if w.Code != http.StatusOK {
			t.Errorf("GET %q: got %d want %d", url, w.Code, http.StatusOK)
		}
	}
}

// TestSSEPriming checks that a new SSE subscriber is primed with the
// latest sticky events before receiving live ones.
func TestSSEPriming(t *testing.T) {
	s := newTestServer("")
	// The claim broadcasts the seat value, which primes ahead of the session
	// sticky events. SSE is seat-free, so the stream URL carries no seat.
	seat := s.claimTestSeat(t, "")
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
	expect := mustMake(t, "writer", map[string]string{"seat": seat}).Format() +
		"event: gps:state\ndata: \"connected\"\n\nevent: gps:speed\ndata: 9600\n\n"
	if got := w.Body.String(); got != expect {
		t.Errorf("got  %q\nwant %q", got, expect)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("got Content-Type %q want text/event-stream", ct)
	}
}

func TestMsgDirs(t *testing.T) {
	t.Run("environment", func(t *testing.T) {
		dirs := []string{"/one", "/two"}
		t.Setenv("SATPULSE_GPSMSG_PATH", strings.Join(dirs, string(os.PathListSeparator)))
		if got := msgDirs(); !reflect.DeepEqual(got, dirs) {
			t.Errorf("got  %+v\nwant %+v", got, dirs)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		sys := []string{"/sys/one", "/sys/two"}
		expect := append([]string{filepath.Join("/cfg", "satpulse", "gpsmsg")}, sys...)
		if got := defaultDirs("/cfg", sys); !reflect.DeepEqual(got, expect) {
			t.Errorf("got  %+v\nwant %+v", got, expect)
		}
	})
	t.Run("system dirs are absolute", func(t *testing.T) {
		for _, dir := range systemDirs() {
			if !filepath.IsAbs(dir) {
				t.Errorf("systemDirs entry %q is not absolute", dir)
			}
		}
	})
}

// TestMsgFileCatalog covers the message-file library endpoints: the
// catalog lists the names on the search path and preselects the
// session vendor, a name selects and returns its tags, a traversal
// name is rejected, and a malformed library file is a server error.
func TestMsgFileCatalog(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "u-blox"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "u-blox", "a.toml")
	if err := os.WriteFile(path,
		[]byte("[[nmea]]\ntext = \"PUBX,04\"\ntag = \"poll\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newTestServerFull("", gpsreg.VendorUblox, []string{dir})
	s.claimTestSeat(t, "") // select is writer-gated; s.post carries the seat

	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/msgfile/catalog", nil))
	if w.Code != 200 {
		t.Fatalf("catalog status %d", w.Code)
	}
	var cat msgCatalog
	if err := json.Unmarshal(w.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	expect := msgCatalog{
		Names:     []msgfile.Entry{{Name: msgfile.Name{Vendor: "u-blox", File: "a"}, Path: path}},
		Preselect: "u-blox",
	}
	if !reflect.DeepEqual(cat, expect) {
		t.Fatalf("got  %+v\nwant %+v", cat, expect)
	}

	w = s.post(t, "/api/msgfile/select", `{"vendor":"u-blox","file":"a"}`)
	if w.Code != 200 {
		t.Fatalf("select status %d: %s", w.Code, w.Body)
	}
	var res msgFileResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) != 1 || res.Tags[0].Tag != "poll" {
		t.Fatalf("unexpected select tags: %+v", res.Tags)
	}

	w = s.post(t, "/api/msgfile/select", `{"vendor":"u-blox","file":"../../u-blox/a"}`)
	if w.Code != 400 {
		t.Errorf("traversal select status = %d, want 400", w.Code)
	}

	if err := os.WriteFile(filepath.Join(dir, "u-blox", "bad.toml"), []byte("[[nmea]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = s.post(t, "/api/msgfile/select", `{"vendor":"u-blox","file":"bad"}`)
	if w.Code != 500 {
		t.Errorf("malformed select status = %d, want 500", w.Code)
	}
}

func TestSeatLifecycle(t *testing.T) {
	s := newTestServer("")
	old := s.claimTestSeat(t, "")
	// A writer POST with the current seat is accepted; a reader POST needs no
	// seat at all.
	if w := s.rawPost("/api/disconnect?seat="+old, `{}`); w.Code != http.StatusOK {
		t.Fatalf("current-seat writer POST: got %d want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if w := s.rawPost("/api/signals", `{"gnss":["GPS"]}`); w.Code != http.StatusOK {
		t.Fatalf("seat-free reader POST: got %d want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	// SSE opens without a seat and is primed with the current seat value.
	ctx, cancel := context.WithCancel(context.Background())
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/sse", nil).WithContext(ctx))
		close(done)
	}()
	waitForClients(t, s.hub, 1)
	// A second claim supersedes the first, without closing the open stream.
	current := s.claimTestSeat(t, "")
	if current == old {
		t.Fatal("second claim did not regenerate the seat")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not end after cancellation")
	}
	if body := w.Body.String(); !strings.Contains(body, old) {
		t.Errorf("stream not primed with the seat value: %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("SSE Content-Type %q want text/event-stream", ct)
	}
	// A writer POST carrying the superseded seat, or none, is refused with 410.
	if w := s.rawPost("/api/disconnect?seat="+old, `{}`); w.Code != http.StatusGone {
		t.Errorf("superseded-seat writer POST: got %d want %d", w.Code, http.StatusGone)
	}
	if w := s.rawPost("/api/disconnect", `{}`); w.Code != http.StatusGone {
		t.Errorf("missing-seat writer POST: got %d want %d", w.Code, http.StatusGone)
	}
	// The new seat works.
	if w := s.rawPost("/api/disconnect?seat="+current, `{}`); w.Code != http.StatusOK {
		t.Errorf("new-seat writer POST: got %d want %d", w.Code, http.StatusOK)
	}
}

// TestWriterSeed checks that a fresh server with no claim primes a no-holder
// writer value (so a tab reconnecting after a restart learns its old seat is
// stale) and that a claim then replaces it.
func TestWriterSeed(t *testing.T) {
	s := newTestServer("")
	seed := grabWriterSeat(t, s)
	if len(seed) != 32 {
		t.Fatalf("seed value length %d want 32", len(seed))
	}
	seat := s.claimTestSeat(t, "")
	if seat == seed {
		t.Fatal("claim did not replace the seed value")
	}
	if got := grabWriterSeat(t, s); got != seat {
		t.Errorf("primed value %q want %q", got, seat)
	}
}

// grabWriterSeat opens a seat-free SSE stream, ends it after the prime, and
// returns the seat value carried by the primed writer event.
func grabWriterSeat(t *testing.T, s *server) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.mux.ServeHTTP(w, httptest.NewRequest("GET", "/sse", nil).WithContext(ctx))
		close(done)
	}()
	cancel()
	<-done
	_, rest, ok := strings.Cut(w.Body.String(), "event: writer\ndata: ")
	if !ok {
		t.Fatalf("no writer event primed: %q", w.Body.String())
	}
	var p struct {
		Seat string `json:"seat"`
	}
	if err := json.Unmarshal([]byte(rest[:strings.Index(rest, "\n")]), &p); err != nil {
		t.Fatalf("decode writer event: %v", err)
	}
	return p.Seat
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
