// Package logobs provides observability through logging.
package logobs

import (
	"log/slog"

	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
)

// StatsLogObserver accumulates statistics over a configurable interval
// and logs summaries via slog.
type StatsLogObserver struct {
	obs.DefaultObserver
	lg       *slog.Logger
	interval int
	accum    statsAccum
}

// NewStatsLogObserver creates a new StatsLogObserver that logs statistics
// at the specified interval (in seconds). If interval <= 0, no logging occurs.
func NewStatsLogObserver(lg *slog.Logger, interval int) *StatsLogObserver {
	return &StatsLogObserver{
		lg:       lg,
		interval: interval,
	}
}

// Sample implements mon.Sampler
func (o *StatsLogObserver) Sample(data mon.SampleData) {
	if o.interval <= 0 {
		return
	}

	// Only accumulate when servo is locked
	if data.SyncState != mon.InSync {
		// Flush any partial stats when losing sync
		if o.accum.phase.n > 0 {
			o.flush()
		}
		return
	}

	// For interval == 1, log immediately instead of accumulating
	if o.interval == 1 {
		switch data.Kind {
		case mon.SampleOK:
			o.lg.Info("adjusting clock frequency",
				"off", data.Offset,
				"freq", data.Freq)
		case mon.SampleMissing:
			o.lg.Info("missed 1PPS sample")
		case mon.SampleOutlier:
			o.lg.Info("outlier sample",
				"off", data.Offset,
				"freq", data.Freq)
		}
		return
	}

	// Accumulate statistics
	o.accum.add(data.Kind, data.Offset.Seconds(), data.Freq, data.FreqDelta)

	// Flush when we reach the interval
	if o.accum.phase.n == o.interval {
		o.flush()
	}
}

// flush logs the accumulated statistics and resets the accumulators
func (o *StatsLogObserver) flush() {
	s := o.accum.stats()
	o.lg.Info("summary", s.LogArgs()...)
	o.accum.reset()
}

// Release releases any resources and flushes pending stats
func (o *StatsLogObserver) Release() {
	// Flush any remaining accumulated stats
	if o.accum.phase.n > 0 {
		o.flush()
	}
}