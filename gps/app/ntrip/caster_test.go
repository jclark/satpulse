package ntrip

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/scan"
)

// testRTCMData is a syntactically-valid RTCM packet with checksum.
// Tests use it to confirm that bytes flow through unchanged.
const testRTCMData = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

// fixture wraps the runtime state needed to drive a caster in tests.
// It exposes a packet input channel and a TCP listener address.
type fixture struct {
	t        *testing.T
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	pktCh    chan scan.Packet
	addr     string
	server   *http.Server
	listener net.Listener
}

// newFixture starts a bcast goroutine and an http.Server backed by a
// caster built from cfg.  The fixture takes ownership of pktCh; tests
// send scan.Packet values to drive the stream.
func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("invalid test config: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &fixture{t: t, cancel: cancel, pktCh: make(chan scan.Packet)}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := bcast.New((<-chan scan.Packet)(f.pktCh))
	f.wg.Go(func() { b.Run(ctx, lg) })
	build := StreamRecordBuilder(&cfg.SharedStreamConfig, nil, nil, "", 0, len(cfg.Users) > 0, nil)
	c := newCaster(lg, cfg, "0.0.0", b, build)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	f.listener = listener
	f.addr = listener.Addr().String()
	f.server = c.newServer()
	f.wg.Go(func() {
		_ = f.server.Serve(listener)
	})
	t.Cleanup(func() {
		_ = f.server.Shutdown(context.Background())
		cancel()
		close(f.pktCh)
		b.Close()
		f.wg.Wait()
	})
	return f
}

// dial opens a TCP connection to the caster.
func (f *fixture) dial() net.Conn {
	f.t.Helper()
	conn, err := net.Dial("tcp", f.addr)
	if err != nil {
		f.t.Fatalf("dial: %v", err)
	}
	return conn
}

// send delivers a single RTCM packet through the bcast.  Returns
// once the caster's handler has read the packet (or after the test
// times out).
func (f *fixture) send(pkt scan.Packet) {
	f.t.Helper()
	select {
	case f.pktCh <- pkt:
	case <-time.After(2 * time.Second):
		f.t.Fatal("timed out sending packet to bcast")
	}
}

// rtcmPacket builds a test scan.Packet carrying RTCM bytes.
func rtcmPacket(data string) scan.Packet {
	return scan.Packet{
		Format:        rtcm.PacketFormat,
		Data:          data,
		ChecksumValid: true,
	}
}

// readResponse reads a complete HTTP response (status line, headers,
// body up to Content-Length).  For chunked or unbounded streams,
// readResponseStatus is more appropriate.
func readResponse(t *testing.T, conn net.Conn) *http.Response {
	t.Helper()
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	return resp
}

// readLine reads a CRLF-terminated line from r.
func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	return line
}

// readN reads exactly n bytes from r.
func readN(t *testing.T, r io.Reader, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	return buf
}

// basicAuthHeader returns the value of an Authorization: Basic header.
func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestSourceTableV1(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}, {Name: "BKK2"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.0\r\n\r\n")
	r := bufio.NewReader(conn)
	status := readLine(t, r)
	if status != "SOURCETABLE 200 OK\r\n" {
		t.Fatalf("status %q", status)
	}
	headers := map[string]string{}
	for {
		line := readLine(t, r)
		if line == "\r\n" {
			break
		}
		k, v, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ": ")
		if !ok {
			t.Fatalf("bad header line %q", line)
		}
		headers[k] = v
	}
	if got := headers["Content-Type"]; got != "text/plain" {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.HasPrefix(headers["Server"], "NTRIP satpulse") { // wire convention: all-caps prefix
		t.Errorf("Server = %q", headers["Server"])
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.HasSuffix(body, []byte("ENDSOURCETABLE\r\n")) {
		t.Errorf("body missing ENDSOURCETABLE: %q", body)
	}
	for _, name := range []string{"BKK", "BKK2"} {
		if !bytes.Contains(body, []byte("STR;"+name+";")) {
			t.Errorf("body missing mountpoint %q: %q", name, body)
		}
	}
}

func TestSourceTableV2(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nConnection: close\r\n\r\n")
	resp := readResponse(t, conn)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "gnss/sourcetable" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Ntrip-Version"); got != "Ntrip/2.0" {
		t.Errorf("Ntrip-Version = %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Contains(body, []byte("STR;BKK;")) {
		t.Errorf("body missing mountpoint: %q", body)
	}
	if !bytes.HasSuffix(body, []byte("ENDSOURCETABLE\r\n")) {
		t.Errorf("body missing ENDSOURCETABLE: %q", body)
	}
}

func TestSourceTableV2WithQueryString(t *testing.T) {
	// v2 spec requires casters that do not implement filtering to
	// respond to ?match=/?filter= as if no query string was supplied.
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET /?filter=foo HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nConnection: close\r\n\r\n")
	resp := readResponse(t, conn)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Contains(body, []byte("STR;BKK;")) {
		t.Errorf("body missing mountpoint: %q", body)
	}
}

func TestStreamV1(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET /BKK HTTP/1.0\r\n\r\n")
	r := bufio.NewReader(conn)
	status := readLine(t, r)
	if status != "ICY 200 OK\r\n" {
		t.Fatalf("status %q", status)
	}
	f.send(rtcmPacket(testRTCMData))
	got := readN(t, r, len(testRTCMData))
	if string(got) != testRTCMData {
		t.Errorf("got  %x\nwant %x", got, testRTCMData)
	}
}

func TestStreamV2(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET /BKK HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nConnection: close\r\n\r\n")
	r := bufio.NewReader(conn)
	status := readLine(t, r)
	if !strings.HasPrefix(status, "HTTP/1.1 200") {
		t.Fatalf("status %q", status)
	}
	headers := map[string]string{}
	for {
		line := readLine(t, r)
		if line == "\r\n" {
			break
		}
		k, v, _ := strings.Cut(strings.TrimRight(line, "\r\n"), ": ")
		headers[k] = v
	}
	if got := headers["Content-Type"]; got != "gnss/data" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := headers["Transfer-Encoding"]; got != "chunked" {
		t.Errorf("Transfer-Encoding = %q", got)
	}
	if got := headers["Ntrip-Version"]; got != "Ntrip/2.0" {
		t.Errorf("Ntrip-Version = %q", got)
	}
	f.send(rtcmPacket(testRTCMData))
	// Consume one chunked frame.
	got, err := readChunk(r)
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if string(got) != testRTCMData {
		t.Errorf("got  %x\nwant %x", got, testRTCMData)
	}
}

// readChunk reads one chunked-encoding frame from r.
func readChunk(r *bufio.Reader) ([]byte, error) {
	sizeLine, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var size int
	if _, err := fmt.Sscanf(sizeLine, "%x", &size); err != nil {
		return nil, fmt.Errorf("bad chunk size %q: %w", sizeLine, err)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	if _, err := r.Discard(2); err != nil {
		return nil, err
	}
	return buf, nil
}

func TestStreamFiltersNonRTCM(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET /BKK HTTP/1.0\r\n\r\n")
	r := bufio.NewReader(conn)
	if line := readLine(t, r); line != "ICY 200 OK\r\n" {
		t.Fatalf("status %q", line)
	}
	// Bad checksum: must be dropped.
	f.send(scan.Packet{Format: rtcm.PacketFormat, Data: testRTCMData, ChecksumValid: false})
	// Good packet: must arrive.
	f.send(rtcmPacket(testRTCMData))
	got := readN(t, r, len(testRTCMData))
	if string(got) != testRTCMData {
		t.Errorf("got  %x\nwant %x", got, testRTCMData)
	}
}

func TestAuthRequired(t *testing.T) {
	cfg := Config{
		Users:      []UserConfig{{Username: "rover1", Password: "secret"}},
		Mountpoint: []MountConfig{{Name: "BKK"}},
	}
	tests := []struct {
		name           string
		authHeader     string
		expectStatus   int
		expectAuthHdr  string
		expectStreamOK bool
	}{
		{name: "missing creds", expectStatus: http.StatusUnauthorized, expectAuthHdr: `Basic realm="/BKK"`},
		{name: "bad password", authHeader: basicAuthHeader("rover1", "wrong"), expectStatus: http.StatusUnauthorized, expectAuthHdr: `Basic realm="/BKK"`},
		{name: "unknown user", authHeader: basicAuthHeader("ghost", "x"), expectStatus: http.StatusUnauthorized, expectAuthHdr: `Basic realm="/BKK"`},
		{name: "good creds", authHeader: basicAuthHeader("rover1", "secret"), expectStreamOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, cfg)
			conn := f.dial()
			defer conn.Close()
			req := "GET /BKK HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nConnection: close\r\n"
			if tc.authHeader != "" {
				req += "Authorization: " + tc.authHeader + "\r\n"
			}
			req += "\r\n"
			fmt.Fprint(conn, req)
			if tc.expectStreamOK {
				r := bufio.NewReader(conn)
				if line := readLine(t, r); !strings.HasPrefix(line, "HTTP/1.1 200") {
					t.Fatalf("expected 200, got %q", line)
				}
				return
			}
			resp := readResponse(t, conn)
			if resp.StatusCode != tc.expectStatus {
				t.Fatalf("status %d", resp.StatusCode)
			}
			if got := resp.Header.Get("WWW-Authenticate"); got != tc.expectAuthHdr {
				t.Errorf("WWW-Authenticate = %q", got)
			}
		})
	}
}

func TestPerMountpointUserRestriction(t *testing.T) {
	f := newFixture(t, Config{
		Users: []UserConfig{
			{Username: "rover1", Password: "p1"},
			{Username: "rover2", Password: "p2"},
		},
		Mountpoint: []MountConfig{{
			Name:  "BKK",
			Users: []string{"rover1"},
		}},
	})
	tests := []struct {
		user, pass    string
		expectAllowed bool
	}{
		{"rover1", "p1", true},
		{"rover2", "p2", false},
	}
	for _, tc := range tests {
		t.Run(tc.user, func(t *testing.T) {
			conn := f.dial()
			defer conn.Close()
			fmt.Fprintf(conn,
				"GET /BKK HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nAuthorization: %s\r\nConnection: close\r\n\r\n",
				basicAuthHeader(tc.user, tc.pass))
			r := bufio.NewReader(conn)
			line := readLine(t, r)
			if tc.expectAllowed {
				if !strings.HasPrefix(line, "HTTP/1.1 200") {
					t.Fatalf("expected 200, got %q", line)
				}
			} else {
				if !strings.HasPrefix(line, "HTTP/1.1 401") {
					t.Fatalf("expected 401, got %q", line)
				}
			}
		})
	}
}

func TestAnonymousAccess(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET /BKK HTTP/1.0\r\n\r\n")
	r := bufio.NewReader(conn)
	if line := readLine(t, r); line != "ICY 200 OK\r\n" {
		t.Fatalf("expected ICY 200 OK, got %q", line)
	}
	f.send(rtcmPacket(testRTCMData))
	got := readN(t, r, len(testRTCMData))
	if string(got) != testRTCMData {
		t.Errorf("got  %x\nwant %x", got, testRTCMData)
	}
}

func TestUnknownMountpointV1ReturnsSourcetable(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET /XYZ HTTP/1.0\r\n\r\n")
	r := bufio.NewReader(conn)
	if line := readLine(t, r); line != "SOURCETABLE 200 OK\r\n" {
		t.Fatalf("expected SOURCETABLE, got %q", line)
	}
}

func TestUnknownMountpointV2Returns404(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	defer conn.Close()
	fmt.Fprint(conn, "GET /XYZ HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nConnection: close\r\n\r\n")
	resp := readResponse(t, conn)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d", resp.StatusCode)
	}
}

func TestClientDisconnectCleansUp(t *testing.T) {
	f := newFixture(t, Config{
		Mountpoint: []MountConfig{{Name: "BKK"}},
	})
	conn := f.dial()
	fmt.Fprint(conn, "GET /BKK HTTP/1.0\r\n\r\n")
	r := bufio.NewReader(conn)
	if line := readLine(t, r); line != "ICY 200 OK\r\n" {
		t.Fatalf("status %q", line)
	}
	conn.Close()
	// Sending packets after the client disconnects should not block
	// the caster: streamLoop must observe the write error and unsub.
	for range 10 {
		select {
		case f.pktCh <- rtcmPacket(testRTCMData):
		case <-time.After(2 * time.Second):
			t.Fatal("packet send blocked after client disconnect")
		}
	}
}
