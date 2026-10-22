//go:build (unix || windows) && !baremetal

package gpsio

import (
	"os"
	"os/signal"
	"syscall"
)

// Use SIGHUP as a signal to reopen the log file (e.g. after log rotation)
func notifyReopen(c chan<- os.Signal) {
	signal.Notify(c, syscall.SIGHUP)
}
