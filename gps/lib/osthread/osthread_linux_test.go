package osthread

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// onThread runs f on a goroutine locked to its own OS thread and waits for
// it. The goroutine exits without unlocking, so the runtime discards the
// thread and the test's changes to it go with it.
func onThread(f func()) {
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		f()
	}()
	<-done
}

func TestSetAffinity(t *testing.T) {
	onThread(func() {
		var allowed unix.CPUSet
		if err := unix.SchedGetaffinity(0, &allowed); err != nil {
			t.Errorf("SchedGetaffinity: %v", err)
			return
		}
		cpu := 0
		for !allowed.IsSet(cpu) {
			cpu++
		}
		if err := SetAffinity(cpu); err != nil {
			t.Errorf("SetAffinity(%d): %v", cpu, err)
			return
		}
		var got unix.CPUSet
		if err := unix.SchedGetaffinity(0, &got); err != nil {
			t.Errorf("SchedGetaffinity: %v", err)
			return
		}
		if got.Count() != 1 || !got.IsSet(cpu) {
			t.Errorf("affinity has %d CPUs, CPU %d set %v; want only CPU %d", got.Count(), cpu, got.IsSet(cpu), cpu)
		}
	})
}

func TestSetAffinityOutOfRange(t *testing.T) {
	onThread(func() {
		if err := SetAffinity(1024); err == nil {
			t.Errorf("SetAffinity(1024): expected error")
		}
	})
}

func TestSetFIFO(t *testing.T) {
	var err error
	onThread(func() {
		if err = SetFIFO(1); err != nil {
			return
		}
		attr, err := unix.SchedGetAttr(0, 0)
		if err != nil {
			t.Errorf("SchedGetAttr: %v", err)
			return
		}
		if attr.Policy != unix.SCHED_FIFO || attr.Priority != 1 {
			t.Errorf("got policy %d priority %d; want SCHED_FIFO priority 1", attr.Policy, attr.Priority)
		}
	})
	if errors.Is(err, unix.EPERM) {
		t.Skip("SetFIFO needs CAP_SYS_NICE or an RLIMIT_RTPRIO limit")
	}
	if err != nil {
		t.Fatalf("SetFIFO(1): %v", err)
	}
}

func TestSetTimerSlack(t *testing.T) {
	onThread(func() {
		before, err := unix.PrctlRetInt(unix.PR_GET_TIMERSLACK, 0, 0, 0, 0)
		if err != nil {
			t.Errorf("PR_GET_TIMERSLACK: %v", err)
			return
		}
		prev, err := SetTimerSlack(time.Nanosecond)
		if err != nil {
			t.Errorf("SetTimerSlack: %v", err)
			return
		}
		if prev != time.Duration(before) {
			t.Errorf("previous slack is %v; want %v", prev, time.Duration(before))
		}
		if slack, _ := unix.PrctlRetInt(unix.PR_GET_TIMERSLACK, 0, 0, 0, 0); slack != 1 {
			t.Errorf("slack is %d ns; want 1", slack)
		}
		if _, err := SetTimerSlack(prev); err != nil {
			t.Errorf("SetTimerSlack(%v): %v", prev, err)
		}
		if slack, _ := unix.PrctlRetInt(unix.PR_GET_TIMERSLACK, 0, 0, 0, 0); slack != before {
			t.Errorf("slack after restore is %d ns; want %d", slack, before)
		}
	})
}
