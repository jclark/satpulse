//go:build !linux || !arm64 || baremetal

package gpiomem

import (
	"errors"
	"fmt"
)

var errNotSupported = fmt.Errorf("gpiomem: %w", errors.ErrUnsupported)

// Pin is a mapped GPIO whose level can be read on supported platforms.
type Pin struct{}

// Open reports that reading GPIO registers is not supported on this
// platform.
func Open(string) (*Pin, error) {
	return nil, errNotSupported
}

// High is never called: Open returns no Pin on this platform.
func (*Pin) High() bool {
	panic("gpiomem: not supported on this platform")
}

// Device is never called: Open returns no Pin on this platform.
func (*Pin) Device() string {
	panic("gpiomem: not supported on this platform")
}

// Close reports that reading GPIO registers is not supported on this
// platform.
func (*Pin) Close() error {
	return errNotSupported
}
