package serialpps

import "testing"

// runBubble skips: now reads the system clock through
// GetSystemTimePreciseAsFileTime, which a synctest bubble cannot fake, so the
// loop would predict edges on the real clock while sleeping on the fake one.
// The tests that need the bubble therefore do not run on Windows.
func runBubble(t *testing.T, _ func(*testing.T)) {
	t.Skip("synctest cannot fake the precise Windows clock")
}
