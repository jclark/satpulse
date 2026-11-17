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
// and reports whether the sleep succeeded. This covers the whole wait when
// it is shorter than the runtime timer's resolution. The deadline is absolute
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
	for {
		err := unix.ClockNanosleep(unix.CLOCK_MONOTONIC, unix.TIMER_ABSTIME, &deadline, nil)
		if err != unix.EINTR {
			return err == nil
		}
	}
}

// tightenTimerSlack narrows the calling thread's timer slack, which the
// kernel adds to every sleep the thread makes: the default 50 us is several
// times the wakeup delay itself. It applies to the thread, so the caller must
// already be locked to one. On success it returns a function that restores
// the previous slack and must be called before unlocking the thread.
func tightenTimerSlack() (func(), error) {
	slack, err := unix.PrctlRetInt(unix.PR_GET_TIMERSLACK, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.Prctl(unix.PR_SET_TIMERSLACK, 1, 0, 0, 0); err != nil {
		return nil, err
	}
	return func() { unix.Prctl(unix.PR_SET_TIMERSLACK, uintptr(slack), 0, 0, 0) }, nil
}
