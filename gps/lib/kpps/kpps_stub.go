//go:build !linux

package kpps

import "time"

// Source represents a kernel PPS source on supported platforms.
type Source struct{}

// Open reports that kernel PPS is not yet supported on this platform.
func Open(string) (*Source, error) {
	return nil, ErrNotSupported
}

// GetCap reports that kernel PPS is not yet supported on this platform.
func (*Source) GetCap() (Mode, error) {
	return 0, ErrNotSupported
}

// Fetch reports that kernel PPS is not yet supported on this platform.
func (*Source) Fetch(time.Duration) (Info, error) {
	return Info{}, ErrNotSupported
}

// Close reports that kernel PPS is not yet supported on this platform.
func (*Source) Close() error {
	return ErrNotSupported
}
