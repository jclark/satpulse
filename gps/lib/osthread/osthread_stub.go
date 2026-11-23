//go:build !linux || baremetal

package osthread

import (
	"errors"
	"fmt"
	"time"
)

var errNotSupported = fmt.Errorf("OS thread settings: %w", errors.ErrUnsupported)

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

// SetTimerSlack reports that timer slack is not supported on this platform.
func SetTimerSlack(time.Duration) (time.Duration, error) {
	return 0, errNotSupported
}
