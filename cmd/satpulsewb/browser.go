package main

import (
	"log/slog"
	"net"
	"os"
	"runtime"
	"strconv"
)

func openBrowser(lg *slog.Logger, ln net.Listener, token string) {
	if !canOpenBrowser(runtime.GOOS, os.Getenv) {
		return
	}
	u := loopbackURL(ln.Addr().(*net.TCPAddr).Port, token)
	lg.Debug("opening browser", "url", u)
	go func() {
		if err := launchBrowser(u); err != nil {
			lg.Debug("could not open browser", "url", u, "err", err)
		}
	}()
}

// canOpenBrowser reports whether this looks like a local desktop
// session on a platform where launching a browser is safe: not over
// SSH, and not Linux, where the launched browser's command line
// (token included) would be readable by other users via /proc.
func canOpenBrowser(goos string, getenv func(string) string) bool {
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return false
	}
	return goos == "darwin" || goos == "windows"
}

func loopbackURL(port int, token string) string {
	u := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/"
	if token != "" {
		u += "?t=" + token
	}
	return u
}
