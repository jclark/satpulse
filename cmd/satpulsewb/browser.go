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

func canOpenBrowser(goos string, getenv func(string) string) bool {
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return false
	}
	if goos == "linux" {
		return getenv("DISPLAY") != "" || getenv("WAYLAND_DISPLAY") != ""
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
