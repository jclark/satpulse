//go:build !linux

package pps

import "time"

func sleepDuration(d time.Duration) time.Duration {
	return d
}

// sleepRemainder has nothing to do: sleepDuration asked the runtime timer for
// the whole wait.
func sleepRemainder(time.Time) bool { return false }
