//go:build !windows

package serialpps

import "time"

// now reads the clocks behind Edge: wall locates edges and paces the loop,
// mono serves elapsed-time arithmetic against other time.Now values. Here
// they are one time.Now reading; Windows separates them. It is a variable
// so the synctest tests can substitute a reading of the bubble's clock for
// a platform implementation that reads a real clock outside it.
var now = func() (wall, mono time.Time) {
	t := time.Now()
	return t, t
}
