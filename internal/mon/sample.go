package mon

import (
	"math"
	"slices"

	"github.com/jclark/satpulse/internal/circbuf"
	"github.com/jclark/satpulse/internal/ptime"
)

type sampleData struct {
	off  float64
	kind SampleKind
}

type sampleState struct {
	invalidWeight   float64 // Tracks recent invalid samples with decay
	goodSampleCount int     // Tracks consecutive good samples when in noSync state
	emaOffset       float64 // Exponential moving average of valid offsets
	buf             *circbuf.Buffer[sampleData]
	era             ptime.Era
}

func newSampleState(n int) *sampleState {
	return &sampleState{
		buf: circbuf.New[sampleData](n),
	}
}

type syncConfig struct {
	maxOffset        float64 // Maximum offset (in seconds) to be considered in sync
	minGood          int     // Minimum number of consecutive OK samples, within maxOffset, to be in sync
	maxConsecInvalid int     // Maximum consecutive invalid samples before exiting sync
	emaDecayFactor   float64 // Decay factor applied to count of consecutive invalid samples
	emaAlpha         float64 // Smoothing factor for calculating offset EMA, used for exiting sync
	madMultiple      float64
	madMinSamples    int
}

var defaultSyncConfig = syncConfig{
	maxOffset:        50e-9, // this is usually overridden by the ClockAccuracy in the MonitorConfig
	minGood:          4,
	maxConsecInvalid: 10,
	emaDecayFactor:   0.5,
	emaAlpha:         0.08, // corresponds to 24 effective samples
	madMultiple:      25,
	madMinSamples:    10,
}

func (ss *sampleState) sample(kind SampleKind, offSecs float64, era ptime.Era, state SyncState, cfg *syncConfig) {
	if era != ss.era {
		ss.era = era
		ss.buf.Clear()
		ss.emaOffset = 0
	}
	ss.buf.Append(sampleData{off: offSecs, kind: kind})

	// Handle invalid samples (missing or outlier)
	if kind == SampleMissing || kind == SampleOutlier {
		ss.invalidWeight += 1.0
	}

	// Apply decay on valid samples
	if kind == SampleOK {
		ss.invalidWeight *= cfg.emaDecayFactor
		ss.emaOffset = cfg.emaAlpha*math.Abs(offSecs) + (1-cfg.emaAlpha)*ss.emaOffset
	}

	// Track good samples only when out of sync
	if state == NoSync {
		if kind == SampleOK && math.Abs(offSecs) <= cfg.maxOffset {
			ss.goodSampleCount++
		} else {
			ss.goodSampleCount = 0
		}
	}
}

func (ss *sampleState) nextSyncState(currentState SyncState, cfg *syncConfig) SyncState {
	if currentState == NoSync {
		if ss.goodSampleCount >= cfg.minGood {
			return InSync
		}
	} else {
		if ss.invalidWeight > float64(cfg.maxConsecInvalid) || ss.emaOffset > cfg.maxOffset {
			return NoSync
		}
	}
	return currentState
}

func (ss *sampleState) resetForSyncChange(newState SyncState) {
	if newState == InSync {
		ss.invalidWeight = 0
		ss.emaOffset = ss.emaGoodSamples()
	} else {
		ss.goodSampleCount = 0
	}
}

func (ss *sampleState) emaGoodSamples() float64 {
	if ss.goodSampleCount == 0 {
		return 0
	}
	// TODO Compute an EMA from all the good samples
	// Using the last is good enough for now
	return math.Abs(ss.buf.Last(0).off)
}

func (ss *sampleState) madIsOutlier(offSecs float64, cfg *syncConfig) bool {
	// Idea is that if it is not enough to take the state out of sync,
	// then it should not be considered as an outlier.
	// This should be a quick check that succeeds most of the time,
	// to avoid running complex MAD logic on every sample.
	if math.Abs(offSecs) < cfg.maxOffset {
		return false
	}
	n, min, max := ss.mad(cfg.madMultiple)
	return n >= cfg.madMinSamples && (offSecs < min || offSecs > max)
}

func (ss *sampleState) mad(k float64) (n int, min, max float64) {
	d := make([]float64, 0, ss.buf.Len())
	nMissing := 0
	ss.buf.Iterate(func(i int, s sampleData) bool {
		if s.kind == SampleMissing {
			nMissing++
		} else {
			d = append(d, s.off)
		}
		return true
	})
	if len(d) == 0 {
		return 0, math.NaN(), math.NaN()
	}
	center := sortMedian(d)
	for i, v := range d {
		d[i] = math.Abs(v - center)
	}
	mad := sortMedian(d) * k
	return len(d), center - mad, center + mad
}

func sortMedian(d []float64) float64 {
	slices.Sort(d)
	n := len(d)
	mid := n / 2
	if n%2 != 0 {
		// eg for len 1, we want d[0]; for len 5, we want d[2]
		return d[mid]
	}
	if n == 0 {
		return math.NaN()
	}
	// eg. for len 2, mid will be 1 and we want the average of d[0] and d[1]
	return (d[mid-1] + d[mid]) / 2
}
