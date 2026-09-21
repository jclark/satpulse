package pps

import (
	"time"

	"golang.org/x/sys/unix"
)

// Linux's epoll_pwait timeout has whole-millisecond resolution. The Go
// runtime rounds a sub-millisecond timer up to one millisecond and a
// fractional-millisecond timer up to the next millisecond, so ask it only for
// the whole milliseconds and leave the rest to sleepRemainder, or to the
// state reads where the wait is not precise. See https://go.dev/issue/53824.
func sleepDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Millisecond)
}

// sleepRemainder sleeps until t, the part of a wait sleepDuration truncated,
// and reports whether it had to. That is the whole wait when the wait is
// shorter than the runtime timer's resolution. The deadline is absolute
// because the runtime's preemption signal interrupts the sleep, and the Go
// monotonic clock is CLOCK_MONOTONIC, so resuming needs no arithmetic.
func sleepRemainder(t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return false
	}
	var ts unix.Timespec
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts) != nil {
		return false
	}
	deadline := unix.NsecToTimespec(unix.TimespecToNsec(ts) + int64(d))
	for unix.ClockNanosleep(unix.CLOCK_MONOTONIC, unix.TIMER_ABSTIME, &deadline, nil) == unix.EINTR {
	}
	return true
}

// tightenTimerSlack narrows the calling thread's timer slack, which the
// kernel adds to every sleep the thread makes: the default 50 us is several
// times the wakeup delay itself. It applies to the thread, so the caller must
// already be locked to one.
func tightenTimerSlack() error {
	return unix.Prctl(unix.PR_SET_TIMERSLACK, 1, 0, 0, 0)
}
