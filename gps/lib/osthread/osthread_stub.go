//go:build !linux

package osthread

import (
	"errors"
	"fmt"
	"time"
)

var errNotSupported = fmt.Errorf("OS thread tuning: %w", errors.ErrUnsupported)

// SetAffinity reports that pinning a thread to a CPU is not supported on
// this platform.
func SetAffinity(int) error {
	return errNotSupported
}

// SetFIFO reports that real-time scheduling is not supported on this
// platform.
func SetFIFO(int) error {
	return errNotSupported
}

// TightenTimerSlack does nothing: Linux is the only platform here that
// exposes timer slack.
func TightenTimerSlack() (restore func(), err error) {
	return func() {}, nil
}

// SleepUntil does not sleep on this platform, and reports that it did not.
func SleepUntil(time.Time) bool {
	return false
}
