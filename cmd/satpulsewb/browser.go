package main

import (
	"log/slog"
	"net"
	"os"
	"strconv"
)

func openBrowser(lg *slog.Logger, ln net.Listener, token string) {
	u := loopbackURL(ln.Addr().(*net.TCPAddr).Port, token)
	lg.Debug("opening browser", "url", u)
	go func() {
		if err := launchBrowser(u); err != nil {
			lg.Debug("could not open browser", "url", u, "err", err)
		}
	}()
}

// canOpenBrowser reports whether this looks like a local desktop session
// where launching a browser is appropriate.
func canOpenBrowser() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	return hasGraphicalSession()
}

func loopbackURL(port int, token string) string {
	u := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/"
	if token != "" {
		u += "?t=" + token
	}
	return u
}
