//go:build !linux

package pps

import "time"

func sleepDuration(d time.Duration) time.Duration {
	return d
}

// sleepRemainder has nothing to do: sleepDuration asked the runtime timer for
// the whole wait.
func sleepRemainder(time.Time) {}

// tightenTimerSlack has nothing to do: Linux is the only platform here that
// exposes the knob.
func tightenTimerSlack() error { return nil }
