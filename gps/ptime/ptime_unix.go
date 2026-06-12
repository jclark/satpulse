//go:build !windows

package ptime

import "golang.org/x/sys/unix"

func TimespecToTime(t unix.Timespec) Time {
	return Time(t.Nano())
}

func (t Time) Timespec() unix.Timespec {
	return unix.NsecToTimespec(int64(t))
}
