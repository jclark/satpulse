//go:build !windows

package serialpps

import "time"

// now reads the clocks behind Edge: wall locates edges and paces the loop,
// mono serves elapsed-time arithmetic against other time.Now values. Here
// they are one time.Now reading; a platform whose precise wall-clock read
// carries no monotonic reading separates them. It is a variable so the
// synctest tests can substitute a reading of the bubble's clock for a
// platform implementation that reads a real clock outside it.
var now = func() clockReading {
	t := time.Now()
	return clockReading{wall: t, mono: t}
}

// elapsedSince measures elapsed time with the monotonic clock.
func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.mono.Sub(start.mono)
}
