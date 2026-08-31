//go:build !linux

package serialpps

import "time"

func sleepDuration(d time.Duration) time.Duration {
	return d
}
