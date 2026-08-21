//go:build !windows

package serialpps

import "time"

// now reads the clocks used by the poller. Here one time.Now reading is both
// the measurement stamp and the monotonic pacing coordinate; a platform whose
// precise wall-clock read carries no monotonic reading separates them.
func now() clockReading {
	t := time.Now()
	return clockReading{stamp: t, mono: t}
}
