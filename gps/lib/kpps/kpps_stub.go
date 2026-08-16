//go:build !linux

package kpps

import (
	"errors"
	"fmt"
	"time"
)

var errNotSupported = fmt.Errorf("kernel PPS: %w", errors.ErrUnsupported)

// Source represents a kernel PPS source on supported platforms.
type Source struct{}

// Open reports that kernel PPS is not yet supported on this platform.
func Open(string) (*Source, error) {
	return nil, errNotSupported
}

// GetCap reports that kernel PPS is not yet supported on this platform.
func (*Source) GetCap() (Mode, error) {
	return 0, errNotSupported
}

// Fetch reports that kernel PPS is not yet supported on this platform.
func (*Source) Fetch(Info, time.Duration) (Info, error) {
	return Info{}, errNotSupported
}

// Close reports that kernel PPS is not yet supported on this platform.
func (*Source) Close() error {
	return errNotSupported
}
