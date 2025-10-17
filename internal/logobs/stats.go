package logobs

import (
	"fmt"
	"math"

	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
)

// Stats contains accumulated statistics
type Stats struct {
	AbsOffMax       float64 // nanoseconds
	AbsOffMean      float64 // nanoseconds
	OffRMS          float64 // nanoseconds
	FreqMean        float64 // ppb
	FreqStdDev      float64 // ppb
	FreqDeltaStdDev float64 // ppb
	NSamples        int
	NMissing        int
	NOutliers       int
}

// LogArgs returns name/value pairs suitable for slog.Logger.Info()
func (s Stats) LogArgs() []any {
	return []any{
		"absOffMax", fmt.Sprintf("%.0f", s.AbsOffMax),
		"absOffMean", fmt.Sprintf("%.1f", s.AbsOffMean),
		"offRMS", fmt.Sprintf("%.1f", s.OffRMS),
		"freqMean", fmt.Sprintf("%.0f", s.FreqMean),
		"freqStdDev", fmt.Sprintf("%.0f", s.FreqStdDev),
		"freqDeltaStdDev", fmt.Sprintf("%.1f", s.FreqDeltaStdDev),
		"nSecs", s.NSamples,
		"nMissing", s.NMissing,
		"nOutliers", s.NOutliers,
	}
}

// StatsObserver accumulates statistics without logging
type StatsObserver struct {
	obs.DefaultObserver
	accum statsAccum
}

// NewStatsObserver creates a new StatsObserver
func NewStatsObserver() *StatsObserver {
	return &StatsObserver{}
}

// Sample implements mon.Sampler
func (o *StatsObserver) Sample(data mon.SampleData) {
	// Only accumulate when in sync
	if data.SyncState != mon.InSync {
		return
	}
	o.accum.add(data.Kind, data.Offset.Seconds(), data.Freq, data.FreqDelta)
}

// Stats returns accumulated statistics
func (o *StatsObserver) Stats() Stats {
	return o.accum.stats()
}

// statsAccum wraps the three internal accumulators
type statsAccum struct {
	phase     accumPhase
	freq      accumFreq
	freqDelta accumFreq
}

func (a *statsAccum) add(kind mon.SampleKind, offset float64, freq float64, freqDelta float64) {
	a.phase.add(kind, offset)
	a.freq.add(freq)
	a.freqDelta.add(freqDelta)
}

func (a *statsAccum) stats() Stats {
	return Stats{
		AbsOffMax:       a.phase.maxAbs * 1e9,
		AbsOffMean:      a.phase.meanAbs() * 1e9,
		OffRMS:          a.phase.rms() * 1e9,
		FreqMean:        a.freq.mean(),
		FreqStdDev:      a.freq.stddev(),
		FreqDeltaStdDev: a.freqDelta.stddev(),
		NSamples:        a.phase.n,
		NMissing:        a.phase.nMissing,
		NOutliers:       a.phase.nOutliers,
	}
}

func (a *statsAccum) reset() {
	a.phase = accumPhase{}
	a.freq = accumFreq{}
	a.freqDelta = accumFreq{}
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
