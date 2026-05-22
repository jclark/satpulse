package ntrip

import (
	"bufio"
	"fmt"
	"net/http"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// authTOML mirrors the daemon's top-level shape for tests that parse
// a full TOML snippet covering both top-level [[user]] credentials
// and the [ntrip] caster configuration.  It avoids depending on the
// daemon package while still letting test cases be expressed as TOML
// source.
type authTOML struct {
	Users []authUser `toml:"user"`
	Ntrip Config     `toml:"ntrip"`
}

type authUser struct {
	Name     string `toml:"name"`
	Password string `toml:"password"`
}

// TestAuth verifies authentication decisions for the Ntrip caster.
// Each case supplies a TOML configuration plus a target mountpoint
// and a list of credentials with expected HTTP status codes.  The
// test parses the TOML, spins up a caster fixture, and issues one
// GET per credential.  Anonymous attempts (empty user) skip the
// Authorization header entirely.
func TestAuth(t *testing.T) {
	type cred struct {
		user, pass   string
		expectStatus int
	}
	tests := []struct {
		name       string
		toml       string
		mountpoint string
		creds      []cred
	}{
		{
			name: "anonymous mountpoint accepts anyone",
			toml: `
[[ntrip.mountpoint]]
name = "OPEN"
`,
			mountpoint: "OPEN",
			creds: []cred{
				{expectStatus: http.StatusOK},
				{user: "ghost", pass: "x", expectStatus: http.StatusOK},
			},
		},
		{
			name: "auth.anyUser accepts any valid user",
			toml: `
[[user]]
name = "rover1"
password = "p1"
[[user]]
name = "rover2"
password = "p2"

[[ntrip.mountpoint]]
name = "ANY"
auth.anyUser = true
`,
			mountpoint: "ANY",
			creds: []cred{
				{user: "rover1", pass: "p1", expectStatus: http.StatusOK},
				{user: "rover2", pass: "p2", expectStatus: http.StatusOK},
				{user: "rover1", pass: "wrong", expectStatus: http.StatusUnauthorized},
				{user: "ghost", pass: "x", expectStatus: http.StatusUnauthorized},
				{expectStatus: http.StatusUnauthorized},
			},
		},
		{
			name: "auth.users restricts to listed users",
			toml: `
[[user]]
name = "rover1"
password = "p1"
[[user]]
name = "rover2"
password = "p2"

[[ntrip.mountpoint]]
name = "BKK"
auth.users = ["rover1"]
`,
			mountpoint: "BKK",
			creds: []cred{
				{user: "rover1", pass: "p1", expectStatus: http.StatusOK},
				{user: "rover2", pass: "p2", expectStatus: http.StatusUnauthorized},
				{user: "rover1", pass: "wrong", expectStatus: http.StatusUnauthorized},
				{expectStatus: http.StatusUnauthorized},
			},
		},
		{
			name: "auth = {} is a closed mountpoint",
			toml: `
[[user]]
name = "rover1"
password = "p1"

[[ntrip.mountpoint]]
name = "CLOSED"
auth = {}
`,
			mountpoint: "CLOSED",
			creds: []cred{
				{user: "rover1", pass: "p1", expectStatus: http.StatusUnauthorized},
				{expectStatus: http.StatusUnauthorized},
			},
		},
		{
			name: "auth.users = [] is a closed mountpoint",
			toml: `
[[user]]
name = "rover1"
password = "p1"

[[ntrip.mountpoint]]
name = "CLOSED"
auth.users = []
`,
			mountpoint: "CLOSED",
			creds: []cred{
				{user: "rover1", pass: "p1", expectStatus: http.StatusUnauthorized},
			},
		},
		{
			name: "open and authed mountpoints coexist",
			toml: `
[[user]]
name = "rover1"
password = "p1"

[[ntrip.mountpoint]]
name = "OPEN"

[[ntrip.mountpoint]]
name = "PRIV"
auth.users = ["rover1"]
`,
			mountpoint: "OPEN",
			creds: []cred{
				{expectStatus: http.StatusOK},
				{user: "rover1", pass: "p1", expectStatus: http.StatusOK},
			},
		},
		{
			name: "open and authed mountpoints coexist, hit authed",
			toml: `
[[user]]
name = "rover1"
password = "p1"

[[ntrip.mountpoint]]
name = "OPEN"

[[ntrip.mountpoint]]
name = "PRIV"
auth.users = ["rover1"]
`,
			mountpoint: "PRIV",
			creds: []cred{
				{user: "rover1", pass: "p1", expectStatus: http.StatusOK},
				{expectStatus: http.StatusUnauthorized},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg authTOML
			if err := toml.Unmarshal([]byte(tc.toml), &cfg); err != nil {
				t.Fatalf("toml.Unmarshal: %v", err)
			}
			users := make(map[string]string, len(cfg.Users))
			for _, u := range cfg.Users {
				users[u.Name] = u.Password
			}
			f := newFixture(t, cfg.Ntrip, users)
			for _, c := range tc.creds {
				name := c.user
				if name == "" {
					name = "anonymous"
				}
				t.Run(name, func(t *testing.T) {
					status, authHdr := authRequest(t, f, tc.mountpoint, c.user, c.pass)
					if status != c.expectStatus {
						t.Errorf("status = %d, want %d", status, c.expectStatus)
					}
					if c.expectStatus == http.StatusUnauthorized {
						want := `Basic realm="/` + tc.mountpoint + `"`
						if authHdr != want {
							t.Errorf("WWW-Authenticate = %q, want %q", authHdr, want)
						}
					}
				})
			}
		})
	}
}

// authRequest issues a GET /<mountpoint> request with optional basic
// auth credentials (empty user means no Authorization header) and
// returns the response status code and any WWW-Authenticate header
// value.  The request claims Ntrip/2.0 so the response framing is
// standard HTTP, suitable for http.ReadResponse.
//
// Successful (200) responses begin streaming chunked RTCM data with
// no Content-Length; on success we close the connection immediately
// after reading headers rather than calling resp.Body.Close(), which
// would otherwise block trying to drain the open chunked body.
func authRequest(t *testing.T, f *fixture, mountpoint, user, pass string) (int, string) {
	t.Helper()
	conn := f.dial()
	defer conn.Close()
	req := "GET /" + mountpoint + " HTTP/1.1\r\nHost: x\r\nNtrip-Version: Ntrip/2.0\r\nConnection: close\r\n"
	if user != "" {
		req += "Authorization: " + basicAuthHeader(user, pass) + "\r\n"
	}
	req += "\r\n"
	if _, err := fmt.Fprint(conn, req); err != nil {
		t.Fatalf("write request: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	return resp.StatusCode, resp.Header.Get("WWW-Authenticate")
}
