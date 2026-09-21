package pps

import (
	"time"

	"golang.org/x/sys/unix"
)

// Linux's epoll_pwait timeout has whole-millisecond resolution. The Go
// runtime rounds a sub-millisecond timer up to one millisecond and a
// fractional-millisecond timer up to the next millisecond, so ask it only for
// the whole milliseconds. A wait shorter than that is not slept at all:
// state reads pace it. See https://go.dev/issue/53824.
func sleepDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Millisecond)
}

// sleepRemainder sleeps out the part of the wait sleepDuration truncated, so
// that a wait ends at its deadline rather than up to a millisecond before it.
// The deadline is absolute because the runtime's preemption signal interrupts
// the sleep, and the Go monotonic clock is CLOCK_MONOTONIC, so resuming after
// the interruption needs no arithmetic.
func sleepRemainder(t time.Time) {
	d := time.Until(t)
	if d <= 0 {
		return
	}
	var ts unix.Timespec
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts) != nil {
		return
	}
	deadline := unix.NsecToTimespec(unix.TimespecToNsec(ts) + int64(d))
	for unix.ClockNanosleep(unix.CLOCK_MONOTONIC, unix.TIMER_ABSTIME, &deadline, nil) == unix.EINTR {
	}
}

// tightenTimerSlack narrows the calling thread's timer slack, which the
// kernel adds to every sleep the thread makes: the default 50 us is several
// times the wakeup delay itself. It applies to the thread, so the caller must
// already be locked to one.
func tightenTimerSlack() error {
	return unix.Prctl(unix.PR_SET_TIMERSLACK, 1, 0, 0, 0)
}
