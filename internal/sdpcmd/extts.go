package sdpcmd

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"os"
	"strconv"
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

func extts(lg *slog.Logger, cfg *FlagConfig) iter.Seq[Printer] {
	return func(yield func(Printer) bool) {
		lg.Debug("extts mode", "interface", cfg.Interface, "timeout", cfg.Timeout, "pin", cfg.Pin, "chan", cfg.Chan)
		
		// Get PHC index for interface
		phcIndex, err := phc.IfPhcIndex(cfg.Interface)
		if err != nil || phcIndex < 0 {
			lg.Error("interface does not have a PTP hardware clock", "interface", cfg.Interface, "error", err)
			return
		}

		// Open PHC device
		phcPath := fmt.Sprintf("/dev/ptp%d", phcIndex)
		clk, err := phc.Open(phcPath)
		if err != nil {
			lg.Error("failed to open PHC device", "path", phcPath, "error", err)
			return
		}
		defer clk.Close()

		// Check if we have pins
		if clk.PinCount() == 0 {
			lg.Error("interface has no software-defined pins", "interface", cfg.Interface)
			return
		}

		// Resolve pin name to index if needed
		pinIndex, err := resolvePinIndex(clk, cfg.Pin)
		if err != nil {
			lg.Error("pin not found", "pin", cfg.Pin, "interface", cfg.Interface, "error", err)
			return
		}

		// Validate channel index
		if int(cfg.Chan) >= clk.ExttsChanCount() {
			lg.Error("channel out of range", "channel", cfg.Chan, "max", clk.ExttsChanCount()-1)
			return
		}

		// Configure pin for external timestamps
		err = clk.PinSetFunc(pinIndex, phc.PinFuncExtts, cfg.Chan)
		if err != nil {
			lg.Error("failed to configure pin", "pin", pinIndex, "error", err)
			return
		}

		// Enable external timestamps
		_, err = clk.ExttsEnable(cfg.Chan, true)
		if err != nil {
			lg.Error("failed to enable external timestamps", "channel", cfg.Chan, "error", err)
			return
		}
		defer clk.ExttsEnable(cfg.Chan, false) // Cleanup on exit

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
						lg.Info("no timestamps received during monitoring period")
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

// resolvePinIndex converts a pin name or index string to a pin index
func resolvePinIndex(clk *phc.Clock, pin string) (uint32, error) {
	// Try to parse as integer first
	if index, err := strconv.Atoi(pin); err == nil {
		if index < 0 || index >= clk.PinCount() {
			return 0, fmt.Errorf("pin index %d out of range (0-%d)", index, clk.PinCount()-1)
		}
		return uint32(index), nil
	}

	// Not a number, treat as pin name
	for i := 0; i < clk.PinCount(); i++ {
		desc, err := clk.PinGetFunc(uint32(i))
		if err != nil {
			continue
		}
		if string(desc.Name[:]) == pin || stripNulls(string(desc.Name[:])) == pin {
			return uint32(i), nil
		}
	}

	return 0, fmt.Errorf("pin name not found")
}

// stripNulls removes null bytes from a string (for C-style strings)
func stripNulls(s string) string {
	for i, c := range s {
		if c == 0 {
			return s[:i]
		}
	}
	return s
}