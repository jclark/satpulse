//go:build !linux

package pps

import "time"

func sleepDuration(d time.Duration) time.Duration {
	return d
}
