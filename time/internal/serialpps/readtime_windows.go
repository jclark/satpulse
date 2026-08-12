package serialpps

import "time"

// elapsedSince uses the precise wall readings on Windows. The mono readings
// come from time.Now and are too coarse for modem-state read times.
func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.wall.Sub(start.wall)
}
