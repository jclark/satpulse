package pps

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/jclark/satpulse/gps/lib/gpiomem"
	"github.com/jclark/satpulse/gps/ptime"
	"golang.org/x/sys/unix"
)

// DetectGPIO polls the GPIO pin gpio for the rising edges of its PPS pulses
// and sends a candidate for each until ctx is done or polling fails. The
// poller runs on its own OS thread, pinned to cfg.CPU and at SCHED_FIFO
// priority cfg.Priority when those are set; the thread is discarded when
// polling ends, so those settings never reach the rest of the program.
// Edges bracketed more widely than cfg.MaxBracket are reported unsettled.
// Cancellation takes effect at the next poll, at most one period away. If
// stats is non-nil, it records polling statistics.
func DetectGPIO(ctx context.Context, lg *slog.Logger, gpio int, cfg GPIOConfig, ceCh chan<- CandidateEdge, stats *PollStats) error {
	pin, err := gpiomem.Open(gpio)
	if err != nil {
		return err
	}
	defer pin.Close()
	lg.Info("GPIO PPS polling", "gpio", gpio, "device", pin.Device())
	errCh := make(chan error, 1)
	go func() {
		// The goroutine exits with its thread still locked, so the runtime
		// discards the thread rather than reusing it with the affinity and
		// scheduling set here.
		runtime.LockOSThread()
		if err := setupThread(cfg); err != nil {
			errCh <- err
			return
		}
		params := gpioPollParams
		params.MaxBracket = ptime.Seconds(cfg.MaxBracket)
		errCh <- Poll(ctx, lg, gpioReader{pin}, params, ceCh, stats)
	}()
	return <-errCh
}

// setupThread applies cfg to the calling thread and minimizes its timer
// slack, so that the poller's sleeps end when scheduled rather than up to
// the default 50 us later.
func setupThread(cfg GPIOConfig) error {
	if cfg.CPU != nil {
		var set unix.CPUSet
		set.Set(*cfg.CPU)
		if err := unix.SchedSetaffinity(0, &set); err != nil {
			return fmt.Errorf("pinning GPIO poller to CPU %d: %w", *cfg.CPU, err)
		}
	}
	if cfg.Priority != 0 {
		attr := unix.SchedAttr{Size: unix.SizeofSchedAttr, Policy: unix.SCHED_FIFO, Priority: uint32(cfg.Priority)}
		if err := unix.SchedSetAttr(0, &attr, 0); err != nil {
			return fmt.Errorf("setting GPIO poller SCHED_FIFO priority %d: %w", cfg.Priority, err)
		}
	}
	if err := unix.Prctl(unix.PR_SET_TIMERSLACK, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("setting GPIO poller timer slack: %w", err)
	}
	return nil
}

// gpioReader reads a GPIO for Poll; the pulse is active-high.
type gpioReader struct {
	pin *gpiomem.Pin
}

func (r gpioReader) InPulse() (bool, error) {
	return r.pin.High(), nil
}

// gpioPollParams suits register reads of about a microsecond: dense polls
// from the start, spacing down to the read time, and sleeps fine enough to
// be worth taking between such polls.
var gpioPollParams = PollParams{
	InitialPolls: 10000,
	MinSpacing:   time.Microsecond,
	Wait:         gpioWait,
}

// gpioSleepQuantum is the resolution of gpioWait's sleeps. A scheduled time
// nearer than this is not slept to, and reported as not waited for, so that
// the queries pace the loop below it, as with waitUntil's millisecond
// truncation on the runtime's timers.
const gpioSleepQuantum = 10 * time.Microsecond

// gpioWait sleeps until t with clock_nanosleep on the calling thread, so
// that the kernel wakes the thread directly at the deadline, with its
// scheduling priority, rather than through the runtime's timer and
// scheduler. ctx is consulted only around the sleep, which is at most a
// period long.
func gpioWait(ctx context.Context, t time.Time) (bool, error) {
	d := time.Until(t).Truncate(gpioSleepQuantum)
	if d <= 0 {
		return false, ctx.Err()
	}
	var now unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &now); err != nil {
		return false, err
	}
	deadline := unix.NsecToTimespec(now.Nano() + d.Nanoseconds())
	for {
		err := unix.ClockNanosleep(unix.CLOCK_MONOTONIC, unix.TIMER_ABSTIME, &deadline, nil)
		if err == nil {
			return true, ctx.Err()
		}
		if err != unix.EINTR {
			return false, err
		}
	}
}
