package sdpcmd

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
)

// ExttsEvent represents a timestamp event (for -i mode)
type ExttsEvent struct {
	Timestamp ptime.Time `json:"timestamp"` // PTP time
	TRead     time.Time  `json:"tRead"`     // System time when we read the event
	Chan      uint32     `json:"chan"`      // channel index
	Stale     bool       `json:"stale,omitempty"` // true if timestamp was buffered before monitoring started
}

// Compile-time assertion that ExttsEvent implements Printer
var _ Printer = (*ExttsEvent)(nil)

// Print outputs the timestamp in human-readable format
func (e *ExttsEvent) Print(f *os.File) {
	fmt.Fprintln(f, e.Timestamp)
}

// extts implements --extts/-i mode
func extts(lg *slog.Logger, cfg *FlagConfig, errp *error) iter.Seq[Printer] {
	return func(yield func(Printer) bool) {
		lg.Debug("extts mode", "interface", cfg.Interface, "timeout", cfg.Timeout, "pin", cfg.Pin, "chan", cfg.Chan)
		
		clk, pinIndex, err := phcOpen(cfg.Interface, cfg.Pin)
		if err != nil {
			*errp = err
			return
		}
		defer clk.Close()

		chanIndex, err := validateExttsChannel(clk, cfg.Chan)
		if err != nil {
			*errp = err
			return
		}

		err = clk.PinSetFunc(pinIndex, phc.PinFuncExtts, chanIndex)
		if err != nil {
			*errp = fmt.Errorf("failed to configure pin %d for external timestamps: %w", pinIndex, err)
			return
		}

		_, err = clk.ExttsEnable(chanIndex, true)
		if err != nil {
			*errp = fmt.Errorf("failed to enable external timestamps on channel %d: %w", cfg.Chan, err)
			return
		}
		defer clk.ExttsEnable(chanIndex, false) // Cleanup on exit

		ctx := context.Background()
		if cfg.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
		}

		// Log configuration
		lg.Info("monitoring external timestamps", 
			"interface", cfg.Interface,
			"pin", pinIndex,
			"channel", cfg.Chan,
			"timeout", cfg.Timeout,
			"showStale", cfg.ShowStale)

		// Start worker goroutine
		eventCh := make(chan ExttsEvent, 10)
		go exttsWorker(ctx, lg, clk, eventCh)

		noEventsReceived := true
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					if noEventsReceived && cfg.Timeout > 0 {
						*errp = fmt.Errorf("no timestamps received during monitoring period")
					}
					return
				}
				if !event.Stale || cfg.ShowStale {
					noEventsReceived = false
					if !yield(&event) {
						return
					}
				}
			case <-ctx.Done():
				// Timeout or cancellation - wait for worker to close channel
				continue
			}
		}
	}
}

// exttsWorker reads external timestamps from the PHC device
func exttsWorker(ctx context.Context, lg *slog.Logger, clk *phc.Clock, eventCh chan<- ExttsEvent) {
	defer close(eventCh)
	
	const pollTimeout = 100 * time.Millisecond
	stale := true

	for {
		hasEvents := clk.ExttsAvailable(pollTimeout)
		
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		if !hasEvents {
			// After a timeout with no events, we know future events are fresh
			stale = false
			continue
		}

		t, chanIndex, err := clk.ReadExtts()
		tRead := time.Now()
		if err != nil {
			lg.Debug("failed to read external timestamp", "error", err)
			continue
		}

		event := ExttsEvent{
			Timestamp: t,
			TRead:     tRead,
			Chan:      chanIndex,
			Stale:     stale,
		}

		lg.Debug("received timestamp", "timestamp", t, "channel", chanIndex, "stale", stale)
		
		select {
		case eventCh <- event:
		case <-ctx.Done():
			lg.Debug("context cancelled, stopping worker")
			return
		}
	}
}

// validateExttsChannel validates a channel for external timestamp use
func validateExttsChannel(clk *phc.Clock, channel int) (uint32, error) {
	if channel < 0 {
		return 0, fmt.Errorf("channel index cannot be negative (got %d)", channel)
	}
	maxChannel := clk.ExttsChanCount() - 1
	if channel > maxChannel {
		if maxChannel < 0 {
			return 0, fmt.Errorf("this PHC has no external timestamp channels")
		}
		return 0, fmt.Errorf("channel index %d exceeds maximum (%d) for external timestamps", channel, maxChannel)
	}
	return uint32(channel), nil
}
