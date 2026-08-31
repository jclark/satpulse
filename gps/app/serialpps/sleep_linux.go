package serialpps

import "time"

// Linux's epoll_pwait timeout has whole-millisecond resolution. The Go
// runtime rounds a sub-millisecond timer up to one millisecond and a
// fractional-millisecond timer up to the next millisecond, so let state reads
// pace the remainder instead of asking the runtime to oversleep it. See
// https://go.dev/issue/53824.
func sleepDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Millisecond)
}
