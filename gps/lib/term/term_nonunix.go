//go:build !unix || baremetal

package term

import "sort"

// baudRates lists the standard speeds without termios encodings.
var baudRates = []int{
	50, 75, 110, 134, 150, 200, 300, 600, 1200, 1800,
	2400, 4800, 9600, 19200, 38400, 57600, 115200,
	230400, 460800, 921600,
}

// IsValidSpeed reports whether speed is a standard serial rate.
// It does not imply that serial terminals are available on this platform.
func IsValidSpeed(speed int) bool {
	i := sort.SearchInts(baudRates, speed)
	return i < len(baudRates) && baudRates[i] == speed
}
