// Package logobs provides observability through logging.
package logobs

import (
	"log/slog"

	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/statsobs"
)

// StatsLogObserver accumulates statistics over a configurable interval
// and logs summaries via slog.
type StatsLogObserver struct {
	obs.DefaultObserver
	statsobs.StatsObserver
	lg       *slog.Logger
	interval int
}

// NewStatsLogObserver creates a new StatsLogObserver that logs statistics
// at the specified interval (in seconds). If interval <= 0, no logging occurs.
func NewStatsLogObserver(lg *slog.Logger, interval int) *StatsLogObserver {
	return &StatsLogObserver{
		lg:       lg,
		interval: interval,
	}
}

// Sample implements phcsync.Sampler
func (o *StatsLogObserver) Sample(data phcsync.Sample) {
	if o.interval <= 0 {
		return
	}

	// Only accumulate when in sync
	if !data.Mode.InSync() {
		// Flush any partial stats when losing sync
		if o.StatsObserver.HasSamples() {
			o.flush()
		}
		return
	}

	// For interval == 1, log immediately instead of accumulating
	if o.interval == 1 {
		switch data.Kind {
		case phcsync.SampleOK:
			o.lg.Info("adjusting clock frequency",
				"off", data.Offset,
				"freq", data.Freq)
		case phcsync.SampleMissing:
			o.lg.Info("missed 1PPS sample")
		case phcsync.SampleOutlier:
			o.lg.Info("outlier sample",
				"off", data.Offset,
				"freq", data.Freq)
		}
		return
	}

	// Accumulate statistics via embedded observer
	o.StatsObserver.Sample(data)

	// Flush when we reach the interval
	if o.StatsObserver.NSamples() == o.interval {
		o.flush()
	}
}

// flush logs the accumulated statistics and resets the accumulators
func (o *StatsLogObserver) flush() {
	s := o.StatsObserver.Stats()
	o.lg.Info("summary", s.LogArgs()...)
	o.StatsObserver.Reset()
}

// Release releases any resources and flushes pending stats
func (o *StatsLogObserver) Release() {
	// Flush any remaining accumulated stats
	if o.StatsObserver.HasSamples() {
		o.flush()
	}
}