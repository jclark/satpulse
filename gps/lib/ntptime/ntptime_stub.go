//go:build !linux

package ntptime

// Get returns the current NTP state of the system clock.
// On non-Linux platforms, this function is not implemented.
func Get() (*State, error) {
	return nil, ErrorNotSupported
}