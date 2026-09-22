package pps

import (
	"slices"
	"time"
)

const maxClockDiscrepancy = time.Millisecond

// ReconcileTimes returns wall-clock times for the samples in wall, adjusted so
// that their spacing matches the monotonic spacing of mono. time.Now reads the
// wall and monotonic clocks in separate calls with nothing making the pair
// atomic, so a delay between the two leaves that sample's wall reading early
// while its monotonic reading still stands. A timestamp interpolated from such
// a sample keeps its width and slides off the event it brackets, which no
// reported uncertainty can express. Taking the median of the per-sample
// offsets discards the straddled sample and leaves the others where they are.
//
// wall and mono may be the same slice, which is how a caller passes samples
// whose single time.Time carries both readings. The results carry no monotonic
// reading. One straddled sample is tolerated once there are three samples;
// with two there is no majority and the error is split between them.
//
// If the largest and smallest wall-to-monotonic offsets differ by more than
// 1 ms, the samples may span a clock step and the result is nil, false. The
// limit applies to the discrepancy between samples, not their corrections.
// Otherwise ok is true, including for empty slices.
//
// The slices must be the same length, which panics otherwise. Where no mono
// entry carries a monotonic reading, as with a model clock, there is nothing
// to reconcile against and the wall times come back unchanged.
func ReconcileTimes(wall, mono []time.Time) ([]time.Time, bool) {
	if len(wall) != len(mono) {
		panic("wall and mono differ in length")
	}
	if len(wall) == 0 {
		return nil, true
	}
	// Round(0) strips a monotonic reading, so the wall term below compares
	// wall clocks and a value differs from its stripped self only when it has
	// one.
	w0, m0 := wall[0].Round(0), mono[0]
	monotonic := m0 != m0.Round(0)
	// offsets[i] is how far sample i's wall reading sits from its monotonic
	// reading, relative to sample 0.
	offsets := make([]time.Duration, len(wall))
	for i := range wall {
		if (mono[i] != mono[i].Round(0)) != monotonic {
			panic("mono entries disagree on carrying a monotonic reading")
		}
		offsets[i] = wall[i].Round(0).Sub(w0) - mono[i].Sub(m0)
	}
	m, discrepancy := medianAndSpread(offsets)
	if discrepancy > maxClockDiscrepancy {
		return nil, false
	}
	out := make([]time.Time, len(wall))
	for i := range wall {
		out[i] = wall[i].Round(0).Add(m - offsets[i])
	}
	return out, true
}

// medianAndSpread returns the median and the difference between the largest
// and smallest values of ds. It averages the middle two for an even count and
// sorts a copy because the caller's order pairs ds with its samples.
func medianAndSpread(ds []time.Duration) (time.Duration, time.Duration) {
	s := slices.Clone(ds)
	slices.Sort(s)
	lo, hi := s[(len(s)-1)/2], s[len(s)/2]
	return lo + (hi-lo)/2, s[len(s)-1] - s[0]
}
