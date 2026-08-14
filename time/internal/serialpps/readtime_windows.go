package serialpps

import "time"

// elapsedSince measures from the wall readings on Windows, because that is
// where the platform's most precise system-clock read appears and time.Now's
// quantum is too coarse for modem-state read times. Where now returns one
// reading for both fields the two forms coincide.
func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.wall.Sub(start.wall)
}
