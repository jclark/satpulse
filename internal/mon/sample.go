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

type sampleWindow struct {
	window[sampleData]
	Era ptime.Era
}

func newSampleWindow(n int) *sampleWindow {
	return &sampleWindow{
		window: *newWindow[sampleData](n),
	}
}

func (w *sampleWindow) append(kind sampleKind, off float64, era ptime.Era) {
	if era != w.Era {
		w.Era = era
		w.window.clear()
	}
	w.window.append(sampleData{off: off, kind: kind})
}

type sampleConfig struct {
	maxOffset float64
	// minimum number of OK samples in window, within maxOffset, to be in sync
	minGood int
	// if we get more than maxConsecInvalid consecutive invalid samples (before minGood), we are out of sync
	// invalid sample here means missing or outlier (something not fed to the servo) rather than a large offset
	maxConsecInvalid int
	// if we get more than maxTotalInvalid invalid samples (before minGood), we are out of sync
	maxTotalInvalid int
	madMultiple     float64
	madMinSamples   int
	// minimum offset to be considered an outlier in seconds
	minOutlier float64
}

var defaultSampleConfig = sampleConfig{
	// For sync, require that the maximum absolute offset is less than 50ns.
	// Reasoning is we are claiming 100ns accuracy, and we need to budget for other sources of error,
	// specifically errors in GPS signal
	// any offset > maxOffset, that is not an outlier, makes us out of sync
	maxOffset:        50e-9,
	minGood:          4,
	maxConsecInvalid: 7,
	maxTotalInvalid:  12,
	// Stable32 uses 5 here, but outliers for GPS are usually quite extreme compared to the normal offsets which are usually <30ns
	// If it's too low, then during settling phase things can be incorrectly marked as outliers
	madMultiple:   25,
	madMinSamples: 10,
	// if this offset isn't bad enough to take use out of sync,
	// then there's no need to consider it as an outlier
	minOutlier: 50e-9,
}

func (w *sampleWindow) nextSyncState(curState syncState, cfg *sampleConfig) syncState {
	if curState == noSync {
		if w.enterSync(cfg) {
			return inSync
		}
	} else {
		if !w.remainInSync(cfg) {
			return noSync
		}
	}
	return curState
}

func (w *sampleWindow) enterSync(cfg *sampleConfig) bool {
	nGood := 0
	w.iterate(func(i int, sample sampleData) bool {
		if sample.kind != sampleOK || math.Abs(sample.off) > cfg.maxOffset {
			return false
		}
		nGood++
		return nGood >= cfg.minGood
	})
	return nGood >= cfg.minGood
}

func (w *sampleWindow) remainInSync(cfg *sampleConfig) bool {
	remain := false
	nGood := 0
	nConsecInvalid := 0
	nTotalInvalid := 0
	w.iterate(func(i int, sample sampleData) bool {
		switch sample.kind {
		case sampleOK:
			if math.Abs(sample.off) > cfg.maxOffset {
				return false
			}
			nGood++
			nConsecInvalid = 0
			if nGood >= cfg.minGood {
				remain = true
				return false
			}
		default:
			nConsecInvalid++
			nTotalInvalid++
			if nConsecInvalid > cfg.maxConsecInvalid || nTotalInvalid > cfg.maxTotalInvalid {
				return false
			}
		}
		return true
	})
	return remain
}

func (w *sampleWindow) madIsOutlier(offSecs float64, cfg *sampleConfig) bool {
	// this should be a quick check that succeeds most of the time
	// to avoid running complex MAD logic on every sample
	if math.Abs(offSecs) < cfg.minOutlier {
		return false
	}
	n, min, max := w.mad(cfg.madMultiple)
	return n >= cfg.madMinSamples && (offSecs < min || offSecs > max)
}

func (w *sampleWindow) mad(k float64) (n int, min, max float64) {
	d := make([]float64, 0, w.length())
	nMissing := 0
	w.iterate(func(i int, s sampleData) bool {
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
