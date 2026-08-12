//go:build !windows

package serialpps

import "time"

// elapsedSince measures elapsed time with the monotonic clock.
func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.mono.Sub(start.mono)
}
