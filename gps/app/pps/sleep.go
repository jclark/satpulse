//go:build !linux

package pps

import "time"

func sleepDuration(d time.Duration) time.Duration {
	return d
}

// sleepUntil has nothing to do: sleepDuration asked the runtime timer for
// the whole wait, and there is no GPIO source to sleep for.
func sleepUntil(time.Time) bool { return false }
