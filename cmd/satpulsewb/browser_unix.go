//go:build linux || freebsd

package main

import (
	"os"
	"os/exec"
)

func hasGraphicalSession() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// launchBrowser opens the URL with xdg-open. xdg-open, and then the
// browser it spawns, hold the URL in world-readable process argv
// (/proc/PID/cmdline on Linux, kern.proc.args on FreeBSD), which is why
// the opened URL must carry a single-use launch token that is worthless
// once used.
func launchBrowser(url string) error {
	return exec.Command("xdg-open", url).Run()
}

// launchBrowserLeaksURL reports whether launchBrowser exposes the URL to
// other local users. True on Linux and FreeBSD: xdg-open and the browser
// carry the URL in world-readable argv.
const launchBrowserLeaksURL = true
