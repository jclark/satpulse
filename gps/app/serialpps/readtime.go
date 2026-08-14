//go:build !windows

package serialpps

import "time"

// now reads the clocks behind Edge: wall locates edges, mono paces the loop
// and serves elapsed-time arithmetic against other time.Now values. Here
// they are one time.Now reading; a platform whose precise wall-clock read
// carries no monotonic reading separates them.
func now() clockReading {
	t := time.Now()
	return clockReading{wall: t, mono: t}
}

// elapsedSince measures elapsed time with the monotonic clock.
func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.mono.Sub(start.mono)
}
