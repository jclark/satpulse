//go:build !linux

package wakeup

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// LatencyResolution is the smallest positive time.Duration. Wakeup latency
// limiting is not yet supported on this platform.
const LatencyResolution = time.Nanosecond

var errNotSupported = fmt.Errorf("wakeup latency limiting: %w", errors.ErrUnsupported)

// RequestLatencyLimit reports that wakeup latency limiting is not yet
// supported on this platform.
func RequestLatencyLimit(time.Duration) (io.Closer, error) {
	return nil, errNotSupported
}
