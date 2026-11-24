package pps

import (
	"time"

	"golang.org/x/sys/unix"
)

// Linux's epoll_pwait timeout has whole-millisecond resolution. The Go
// runtime rounds a sub-millisecond timer up to one millisecond and a
// fractional-millisecond timer up to the next millisecond, so ask it only for
// the whole milliseconds and leave the rest to sleepUntil, or to the
// state reads where the wait is not precise. See https://go.dev/issue/53824.
func sleepDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Millisecond)
}

// sleepUntil sleeps the calling OS thread until t with clock_nanosleep, so
// that the kernel wakes the thread itself rather than through the runtime's
// timers, and reports whether it slept: false, with no error, if t has
// passed. waitUntil uses it for the part of a wait sleepDuration
// truncated, which is the whole wait when it is shorter than the runtime
// timer's resolution, and the GPIO source for every wait. The deadline is
// absolute because the runtime's preemption signal interrupts the sleep, and
// the Go monotonic clock is CLOCK_MONOTONIC, so resuming needs no arithmetic.
func sleepUntil(t time.Time) (bool, error) {
	// The clock is read before d is measured, so that a delay between the
	// two readings makes the deadline early rather than late.
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return false, err
	}
	d := time.Until(t)
	if d <= 0 {
		return false, nil
	}
	deadline := unix.NsecToTimespec(unix.TimespecToNsec(ts) + int64(d))
	for {
		err := unix.ClockNanosleep(unix.CLOCK_MONOTONIC, unix.TIMER_ABSTIME, &deadline, nil)
		if err != unix.EINTR {
			return err == nil, err
		}
	}
}
