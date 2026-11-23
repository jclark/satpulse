//go:build !baremetal

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

// SetTimerSlack sets the calling OS thread's timer slack, which the kernel
// adds to every sleep the thread makes so that wakeups can be coalesced,
// and returns the previous slack. A slack of zero restores the thread's
// default, which it inherited from the thread that created it, normally
// 50 us. slack must not be negative. A real-time thread has no timer slack,
// and the kernel ignores the setting for it.
func SetTimerSlack(slack time.Duration) (prev time.Duration, err error) {
	if slack < 0 {
		panic("osthread: negative timer slack")
	}
	if time.Duration(uintptr(slack)) != slack {
		return 0, fmt.Errorf("timer slack %v: too large for this platform", slack)
	}
	ns, err := unix.PrctlRetInt(unix.PR_GET_TIMERSLACK, 0, 0, 0, 0)
	if err != nil {
		return 0, err
	}
	if err := unix.Prctl(unix.PR_SET_TIMERSLACK, uintptr(slack), 0, 0, 0); err != nil {
		return 0, err
	}
	// The slack is an unsigned long, which PrctlRetInt returns as an int.
	return time.Duration(uint(ns)), nil
}
