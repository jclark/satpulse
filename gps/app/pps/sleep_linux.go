package pps

import "time"

// Linux's epoll_pwait timeout has whole-millisecond resolution. The Go
// runtime rounds a sub-millisecond timer up to one millisecond and a
// fractional-millisecond timer up to the next millisecond, so ask it only for
// the whole milliseconds and leave the rest to osthread.SleepUntil, or to the
// state reads where the wait is not precise. See https://go.dev/issue/53824.
func sleepDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Millisecond)
}
