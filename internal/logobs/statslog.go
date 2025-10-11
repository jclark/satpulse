// Package logobs provides observability through logging.
package logobs

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
)

// StatsLogObserver accumulates statistics over a configurable interval
// and logs summaries via slog.
type StatsLogObserver struct {
	obs.DefaultObserver
	lg           *slog.Logger
	interval     int
	accumPhase   accumPhase
	accumFreq    accumFreq
	accumFreqDelta accumFreq
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
		if o.accumPhase.n > 0 {
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
	o.accumPhase.add(data.Kind, data.Offset.Seconds())
	o.accumFreq.add(data.Freq)
	o.accumFreqDelta.add(data.FreqDelta)

	// Flush when we reach the interval
	if o.accumPhase.n == o.interval {
		o.flush()
	}
}

// flush logs the accumulated statistics and resets the accumulators
func (o *StatsLogObserver) flush() {
	o.lg.Info("summary",
		"absOffMax", fmt.Sprintf("%.0f", o.accumPhase.maxAbs*1e9),
		"absOffMean", fmt.Sprintf("%.1f", o.accumPhase.meanAbs()*1e9),
		"offRMS", fmt.Sprintf("%.1f", o.accumPhase.rms()*1e9),
		"freqMean", fmt.Sprintf("%.0f", o.accumFreq.mean()),
		"freqStdDev", fmt.Sprintf("%.0f", o.accumFreq.stddev()),
		"freqDeltaStdDev", fmt.Sprintf("%.1f", o.accumFreqDelta.stddev()),
		"nSecs", o.accumPhase.n,
		"nMissing", o.accumPhase.nMissing,
		"nOutliers", o.accumPhase.nOutliers)
	o.accumPhase = accumPhase{}
	o.accumFreq = accumFreq{}
	o.accumFreqDelta = accumFreq{}
}

// Release releases any resources and flushes pending stats
func (o *StatsLogObserver) Release() {
	// Flush any remaining accumulated stats
	if o.accumPhase.n > 0 {
		o.flush()
	}
}


// accumPhase accumulates phase/offset statistics
type accumPhase struct {
	sumAbs     float64
	sumSquares float64
	maxAbs     float64
	n          int
	nMissing   int
	nOutliers  int
}

func (a *accumPhase) add(kind mon.SampleKind, v float64) {
	a.n++
	switch kind {
	case mon.SampleOK:
		av := math.Abs(v)
		a.sumAbs += av
		a.sumSquares += v * v
		a.maxAbs = math.Max(a.maxAbs, av)
	case mon.SampleMissing:
		a.nMissing++
	case mon.SampleOutlier:
		a.nOutliers++
	}
}

func (a *accumPhase) meanAbs() float64 {
	if a.n == 0 {
		return 0
	}
	return a.sumAbs / float64(a.n)
}

func (a *accumPhase) rms() float64 {
	if a.n == 0 {
		return 0
	}
	return math.Sqrt(a.sumSquares / float64(a.n))
}

// accumFreq accumulates frequency statistics
type accumFreq struct {
	sum                float64
	sumMeanDiffSquares float64
	n                  int
}

func (a *accumFreq) add(v float64) {
	a.n++
	a.sum += v
	md := v - a.sum/float64(a.n)
	a.sumMeanDiffSquares += md * md
}

func (a *accumFreq) mean() float64 {
	if a.n == 0 {
		return 0
	}
	return a.sum / float64(a.n)
}

func (a *accumFreq) stddev() float64 {
	if a.n == 0 {
		return 0
	}
	return math.Sqrt(a.sumMeanDiffSquares / float64(a.n))
}