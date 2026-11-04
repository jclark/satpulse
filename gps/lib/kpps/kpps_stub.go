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

// Device describes a kernel PPS device on supported platforms.
type Device struct {
	Path       string
	Name       string
	SourcePath string
	Mode       Mode
}

// ListDevices reports that kernel PPS is not yet supported on this
// platform.
func ListDevices() ([]Device, error) {
	return nil, errNotSupported
}

// DevicePathForTTY reports that kernel PPS is not yet supported on this
// platform.
func DevicePathForTTY(int) (string, error) {
	return "", errNotSupported
}

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
