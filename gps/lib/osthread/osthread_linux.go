package osthread

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// SetAffinity pins the calling OS thread to CPU cpu, which must not be
// negative.
func SetAffinity(cpu int) error {
	if cpu < 0 {
		panic("osthread: negative CPU")
	}
	var set unix.CPUSet
	set.Set(cpu)
	if set.Count() == 0 {
		return fmt.Errorf("CPU %d: beyond the CPUs a CPU set can hold", cpu)
	}
	return unix.SchedSetaffinity(0, &set)
}

// SetFIFO gives the calling OS thread the SCHED_FIFO real-time policy at
// priority, which must be from 1 to 99, so that it preempts threads of
// normal priority when it wakes. It needs CAP_SYS_NICE or an RLIMIT_RTPRIO
// limit allowing the priority.
func SetFIFO(priority int) error {
	if priority < 1 || priority > 99 {
		panic("osthread: SCHED_FIFO priority not from 1 to 99")
	}
	attr := unix.SchedAttr{Size: unix.SizeofSchedAttr, Policy: unix.SCHED_FIFO, Priority: uint32(priority)}
	return unix.SchedSetAttr(0, &attr, 0)
}

// TightenTimerSlack narrows the calling OS thread's timer slack, which the
// kernel adds to every sleep the thread makes: the default 50 us is several
// times the wakeup delay itself. On success it returns a function that
// restores the previous slack, which must be called before the thread is
// unlocked.
func TightenTimerSlack() (restore func(), err error) {
	slack, err := unix.PrctlRetInt(unix.PR_GET_TIMERSLACK, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.Prctl(unix.PR_SET_TIMERSLACK, 1, 0, 0, 0); err != nil {
		return nil, err
	}
	return func() { unix.Prctl(unix.PR_SET_TIMERSLACK, uintptr(slack), 0, 0, 0) }, nil
}

// SleepUntil sleeps the calling OS thread until t with clock_nanosleep, so
// that the kernel wakes the thread directly rather than through the
// runtime's timers, and reports whether the sleep succeeded. It returns
// false at once if t has passed. The deadline is absolute because the
// runtime's preemption signal interrupts the sleep, and the Go monotonic
// clock is CLOCK_MONOTONIC, so resuming needs no arithmetic.
func SleepUntil(t time.Time) bool {
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
