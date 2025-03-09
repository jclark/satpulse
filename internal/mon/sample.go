package mon

import (
	"math"
	"slices"

	"github.com/jclark/satpulse/internal/ptime"
)

type sampleKind int

const (
	sampleMissing sampleKind = iota
	sampleOK
	sampleOutlier
)

type sampleData struct {
	off  float64
	kind sampleKind
}

type sampleState struct {
	invalidWeight   float64 // Tracks recent invalid samples with decay
	goodSampleCount int     // Tracks consecutive good samples when in noSync state
	emaOffset       float64 // Exponential moving average of valid offsets
	win             window[sampleData]
	era             ptime.Era
}

func newSampleState(n int) *sampleState {
	return &sampleState{
		win: *newWindow[sampleData](n),
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
	minOutlier       float64 // Minimum offset to be considered an outlier in seconds
}

var defaultSyncConfig = syncConfig{
	maxOffset:        50e-9,
	minGood:          4,
	maxConsecInvalid: 10,
	emaDecayFactor:   0.5,
	emaAlpha:         0.08, // corresponds to 24 effective samples
	madMultiple:      25,
	madMinSamples:    10,
	minOutlier:       50e-9,
}

func (ss *sampleState) sample(kind sampleKind, offSecs float64, era ptime.Era, state syncState, cfg *syncConfig) {
	if era != ss.era {
		ss.era = era
		ss.win.clear()
		ss.emaOffset = 0
	}
	ss.win.append(sampleData{off: offSecs, kind: kind})

	// Handle invalid samples (missing or outlier)
	if kind == sampleMissing || kind == sampleOutlier {
		ss.invalidWeight += 1.0
	}

	// Apply decay on valid samples
	if kind == sampleOK {
		ss.invalidWeight *= cfg.emaDecayFactor
		ss.emaOffset = cfg.emaAlpha*math.Abs(offSecs) + (1-cfg.emaAlpha)*ss.emaOffset
	}

	// Track good samples only when out of sync
	if state == noSync {
		if kind == sampleOK && math.Abs(offSecs) <= cfg.maxOffset {
			ss.goodSampleCount++
		} else {
			ss.goodSampleCount = 0
		}
	}
}

func (ss *sampleState) nextSyncState(currentState syncState, cfg *syncConfig) syncState {
	if currentState == noSync {
		if ss.goodSampleCount >= cfg.minGood {
			return inSync
		}
	} else {
		if ss.invalidWeight > float64(cfg.maxConsecInvalid) || ss.emaOffset > cfg.maxOffset {
			return noSync
		}
	}
	return currentState
}

func (ss *sampleState) resetForSyncChange(newState syncState) {
	if newState == inSync {
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
	return math.Abs(ss.win.last(0).off)
}

func (ss *sampleState) madIsOutlier(offSecs float64, cfg *syncConfig) bool {
	// this should be a quick check that succeeds most of the time
	// to avoid running complex MAD logic on every sample
	if math.Abs(offSecs) < cfg.minOutlier {
		return false
	}
	n, min, max := ss.mad(cfg.madMultiple)
	return n >= cfg.madMinSamples && (offSecs < min || offSecs > max)
}

func (ss *sampleState) mad(k float64) (n int, min, max float64) {
	d := make([]float64, 0, ss.win.length())
	nMissing := 0
	ss.win.iterate(func(i int, s sampleData) bool {
		if s.kind == sampleMissing {
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
