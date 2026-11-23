package pps

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/jclark/satpulse/gps/lib/gpiomem"
	"github.com/jclark/satpulse/gps/lib/osthread"
)

// GPIOConfig controls how a GPIO is polled for PPS edges.
type GPIOConfig struct {
	// CPU, if non-nil, is the CPU the poller's thread is pinned to. It
	// should not be a CPU that handles interrupts, above all the GPIO's
	// own, whose handling interrupts the reads that bracket the edge.
	CPU *int
	// Priority, if nonzero, is the SCHED_FIFO priority, from 1 to 99, at
	// which the poller's thread runs, so that its short polling windows
	// preempt ordinary work.
	Priority int
}

// DetectGPIO polls the GPIO called name, such as GPIO18, for the rising
// edges of its PPS pulses and sends a candidate for each until ctx is done
// or polling fails. The GPIO is read through gpiomem, so it must already be
// an input. The poller runs on its own OS thread, pinned and prioritized as
// cfg says; the thread is discarded when polling ends, so its settings never
// reach the rest of the program. Cancellation takes effect at the next
// poll, at most one period away. If stats is non-nil, it records polling
// statistics.
func DetectGPIO(ctx context.Context, lg *slog.Logger, name string, cfg GPIOConfig, ceCh chan<- CandidateEdge, stats *PollStats) error {
	pin, err := gpiomem.Open(name)
	if err != nil {
		return err
	}
	defer pin.Close()
	lg.Info("GPIO PPS polling", "gpio", name, "device", pin.Device())
	errCh := make(chan error, 1)
	go func() {
		// The goroutine exits with its thread still locked, so the runtime
		// discards the thread rather than reusing it with these settings.
		runtime.LockOSThread()
		if err := setupThread(cfg); err != nil {
			errCh <- err
			return
		}
		errCh <- Poll(ctx, lg, gpioReader{pin}, gpioPollParams, ceCh, stats)
	}()
	return <-errCh
}

// setupThread applies cfg to the calling OS thread.
func setupThread(cfg GPIOConfig) error {
	if cfg.CPU != nil {
		if err := osthread.SetAffinity(*cfg.CPU); err != nil {
			return fmt.Errorf("pinning GPIO poller to CPU %d: %w", *cfg.CPU, err)
		}
	}
	if cfg.Priority != 0 {
		if err := osthread.SetFIFO(cfg.Priority); err != nil {
			return fmt.Errorf("setting GPIO poller SCHED_FIFO priority %d: %w", cfg.Priority, err)
		}
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

// gpioSleepQuantum is the resolution of gpioWait's sleeps that need not be
// precise. A scheduled time nearer than this is not slept to, and reported
// as not waited for, so that the reads pace the loop below it, as with
// waitUntil's millisecond truncation on the runtime's timers.
const gpioSleepQuantum = 10 * time.Microsecond

// gpioWait sleeps until t with clock_nanosleep on the calling thread, so that
// the kernel wakes the thread directly at the deadline, with its scheduling
// priority, rather than through the runtime's timer and scheduler. A wait
// that need not be precise ends up to gpioSleepQuantum early. ctx is
// consulted only around the sleep, which is at most a period long. A failed
// sleep is an error: with no runtime timer behind it, the poller would
// otherwise read continuously.
func gpioWait(ctx context.Context, t time.Time, precise bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !precise {
		now := time.Now()
		d := t.Sub(now).Truncate(gpioSleepQuantum)
		if d <= 0 {
			return false, nil
		}
		t = now.Add(d)
	}
	slept, err := sleepUntil(t)
	if err != nil {
		return false, fmt.Errorf("sleeping until the next GPIO poll: %w", err)
	}
	return slept, ctx.Err()
}
