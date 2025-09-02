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

func extts(lg *slog.Logger, cfg *FlagConfig, errp *error) iter.Seq[Printer] {
	return func(yield func(Printer) bool) {
		lg.Debug("extts mode", "interface", cfg.Interface, "timeout", cfg.Timeout, "pin", cfg.Pin, "chan", cfg.Chan)
		
		// Get PHC index for interface
		phcIndex, err := phc.IfPhcIndex(cfg.Interface)
		if err != nil || phcIndex < 0 {
			*errp = fmt.Errorf("interface %s does not have a PTP hardware clock: %w", cfg.Interface, err)
			return
		}

		// Open PHC device
		phcPath := fmt.Sprintf("/dev/ptp%d", phcIndex)
		clk, err := phc.Open(phcPath)
		if err != nil {
			*errp = fmt.Errorf("failed to open PHC device for interface %s: %w", cfg.Interface, err)
			return
		}
		defer clk.Close()

		// Check if we have pins
		if clk.PinCount() == 0 {
			*errp = fmt.Errorf("interface %s has no software-defined pins", cfg.Interface)
			return
		}

		// Resolve pin name to index if needed
		pinIndex, err := resolvePinIndex(clk, cfg.Pin)
		if err != nil {
			*errp = err
			return
		}

		// Validate channel index
		if err := validateExttsChannel(clk, cfg.Chan); err != nil {
			*errp = err
			return
		}

		// Configure pin for external timestamps
		err = clk.PinSetFunc(uint32(pinIndex), phc.PinFuncExtts, uint32(cfg.Chan))
		if err != nil {
			*errp = fmt.Errorf("failed to configure pin %d for external timestamps: %w", pinIndex, err)
			return
		}

		// Enable external timestamps
		_, err = clk.ExttsEnable(uint32(cfg.Chan), true)
		if err != nil {
			*errp = fmt.Errorf("failed to enable external timestamps on channel %d: %w", cfg.Chan, err)
			return
		}
		defer clk.ExttsEnable(uint32(cfg.Chan), false) // Cleanup on exit

		// Set up context with timeout
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
					// Channel closed, worker done
					if noEventsReceived && cfg.Timeout > 0 {
						*errp = fmt.Errorf("no timestamps received during monitoring period")
					}
					return
				}
				if !event.Stale || cfg.ShowStale {
					noEventsReceived = false
					if !yield(&event) {
						// Consumer stopped iteration
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
		// Poll for available events with timeout
		hasEvents := clk.ExttsAvailable(pollTimeout)
		
		// Check if we should exit
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

		// Read the timestamp
		t, chanIndex, err := clk.ReadExtts()
		tRead := time.Now()
		if err != nil {
			lg.Debug("failed to read external timestamp", "error", err)
			continue
		}

		// Send event (from any channel)
		event := ExttsEvent{
			Timestamp: t,
			TRead:     tRead,
			Chan:      chanIndex,
			Stale:     stale,
		}

		lg.Debug("received timestamp", "timestamp", t, "channel", chanIndex, "stale", stale)
		
		select {
		case eventCh <- event:
			// Event sent successfully
		case <-ctx.Done():
			lg.Debug("context cancelled, stopping worker")
			return
		}
	}
}

