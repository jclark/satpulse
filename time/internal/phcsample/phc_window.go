package phcsample

import (
	"errors"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

// minFitEntries is the minimum number of admitted calibration entries
// required for an OLS fit to produce a meaningful result. Below this
// phcWindow surfaces ErrNotReady.
const minFitEntries = 3

// errExtrapolation indicates phc is too far past the last admitted
// entry's PHC position for the fit to be trusted.
var errExtrapolation = errors.New("phcsample: phc beyond extrapolation range")

// phcWindow holds recent pulse edges, labels them via the wallClock
// at query time, admits those that pass pre-admission filtering, fits
// the PHC-to-UTC regression, and combines with the cross-sample's sys
// reading to produce the refclock offset.
type phcWindow struct {
	cfg           *Config
	edgesPerPulse int
	buf           []pulseEdge
}

func newPhcWindow(cfg *Config, edgesPerPulse int) *phcWindow {
	return &phcWindow{
		cfg:           cfg,
		edgesPerPulse: edgesPerPulse,
	}
}

// Pulse records a pulse edge for later processing. The buffer is
// bounded to PulseWindow * edgesPerPulse; the oldest entry is dropped
// when capacity is exceeded.
func (w *phcWindow) Pulse(edge pulseEdge) {
	w.buf = append(w.buf, edge)
	maxLen := w.cfg.PulseWindow * w.edgesPerPulse
	if maxLen > 0 && len(w.buf) > maxLen {
		keep := len(w.buf) - maxLen
		w.buf = append(w.buf[:0], w.buf[keep:]...)
	}
}

// Reset clears recorded edges. Used on leap transition in phase 2.
func (w *phcWindow) Reset() {
	w.buf = nil
}

// TrueTimeOffset returns the refclock offset in seconds:
//
//	offset = true_time_at(phc) - sys
//
// A positive value means true time is ahead of sys; the system clock
// is behind real time and needs to advance by this amount. Returns
// ErrNotReady while the pipeline does not yet have enough admissible
// data, and other sentinels (errExtrapolation, wallClock gates) for
// less routine failures.
func (w *phcWindow) TrueTimeOffset(phc ptime.Time, sys time.Time, wc *wallClock, po pulseCorrector, lg *slog.Logger) (float64, error) {
	if len(w.buf) == 0 {
		return 0, ErrNotReady
	}
	timing, medianInterval, err := timingEdges(w.buf, w.edgesPerPulse, w.cfg)
	if err != nil {
		return 0, err
	}
	basePHC := w.buf[0].Timestamp.T
	entries, err := mapEdgesToUTC(timing, medianInterval, basePHC, wc, po, w.cfg.EdgeSecondTolerance)
	if err != nil {
		return 0, err
	}
	return fitAndEvaluate(entries, basePHC, phc, sys, w.cfg.maxExtrapolation())
}

// timingEdges runs pre-admission stride filtering and (in dual-edge
// mode) polarity selection. Returns the chosen polarity's surviving
// edges in chronological order, the median same-polarity interval for
// PHC-to-real scaling downstream, and any error.
func timingEdges(buf []pulseEdge, edgesPerPulse int, cfg *Config) ([]pulseEdge, time.Duration, error) {
	discThresh := time.Duration(cfg.DiscontinuityThreshold * float64(time.Second))
	if edgesPerPulse == 1 {
		raw, medianInterval := consistentEdges(buf, cfg.PulseVariation, discThresh)
		edges := removeZeroEdges(raw)
		if len(edges) == 0 || medianInterval <= 0 {
			return nil, 0, ErrNotReady
		}
		return edges, medianInterval, nil
	}
	a, b := splitAlternating(buf)
	ea, ma := consistentEdges(a, cfg.PulseVariation, discThresh)
	eb, mb := consistentEdges(b, cfg.PulseVariation, discThresh)
	return selectTimingStream(ea, eb, ma, mb, cfg.PulseWidthDetectLimit)
}

// consistentEdges takes a chronological polarity stream and returns a
// same-length slice where rejected edges are replaced by the pulseEdge
// zero value, along with the stream's median interval. Admitted edges
// keep their original indices so callers pairing two polarity streams
// can assume positional alignment. See "consistentEdges (6a)" in
// plan/phc-sample.md for the algorithm.
func consistentEdges(stream []pulseEdge, tolPPB float64, discThresh time.Duration) ([]pulseEdge, time.Duration) {
	n := len(stream)
	out := make([]pulseEdge, n)
	if n < 2 {
		return out, 0
	}
	start := 0
	for start < n-1 {
		intervals := make([]time.Duration, n-1-start)
		for i := range intervals {
			intervals[i] = stream[start+i+1].Timestamp.T.Sub(stream[start+i].Timestamp.T)
		}
		med := medianDuration(intervals)
		if med <= 0 {
			return out, 0
		}
		if idx, ok := firstDiscontinuity(intervals, med, discThresh); ok {
			// Entries up to and including start+idx stay zero in
			// out; restart from the post-discontinuity suffix.
			start += idx + 1
			continue
		}
		flagged := make([]bool, len(intervals))
		for i, iv := range intervals {
			devPPB := math.Abs(float64(iv-med)) / float64(med) * 1e9
			if devPPB > tolPPB {
				flagged[i] = true
			}
		}
		k := len(intervals) + 1
		for j := range k {
			var reject bool
			switch {
			case j == 0:
				reject = flagged[0]
			case j == k-1:
				reject = flagged[len(flagged)-1]
			default:
				reject = flagged[j-1] && flagged[j]
			}
			if !reject {
				out[start+j] = stream[start+j]
			}
		}
		return out, med
	}
	return out, 0
}

// firstDiscontinuity returns the index of the first interval whose
// absolute deviation from the median is at least discThresh and whose
// neighbouring intervals are each below that threshold. This flags
// PHC discontinuities (forward missing-pulse gaps, and forward or
// backward PHC steps injected by another process) so consistentEdges
// can restart from the post-discontinuity suffix. At stream boundaries
// only the available neighbour is required to be below threshold. The
// boolean is false when no such interval exists.
func firstDiscontinuity(intervals []time.Duration, med time.Duration, discThresh time.Duration) (int, bool) {
	farFromMed := func(iv time.Duration) bool {
		d := iv - med
		if d < 0 {
			d = -d
		}
		return d >= discThresh
	}
	for i, iv := range intervals {
		if !farFromMed(iv) {
			continue
		}
		if i > 0 && farFromMed(intervals[i-1]) {
			continue
		}
		if i+1 < len(intervals) && farFromMed(intervals[i+1]) {
			continue
		}
		return i, true
	}
	return 0, false
}

func medianDuration(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	sorted := slices.Clone(ds)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// splitAlternating splits buf into even-index and odd-index streams,
// preserving chronological order inside each. In dual-edge mode this
// separates the two polarities.
func splitAlternating(buf []pulseEdge) ([]pulseEdge, []pulseEdge) {
	a := make([]pulseEdge, 0, (len(buf)+1)/2)
	b := make([]pulseEdge, 0, len(buf)/2)
	for i, e := range buf {
		if i%2 == 0 {
			a = append(a, e)
		} else {
			b = append(b, e)
		}
	}
	return a, b
}

func removeZeroEdges(in []pulseEdge) []pulseEdge {
	out := make([]pulseEdge, 0, len(in))
	for _, e := range in {
		if !e.IsZero() {
			out = append(out, e)
		}
	}
	return out
}

// selectTimingStream picks the rising-edge polarity stream using the
// short/long pattern inherited from phcsync's
// filterEdgeListsByPulseWidth. The ambiguous ~50%-duty branch is
// deliberately rejected; satpulse is what configures the receiver, so
// V1 requires the pulse width to be far from 0.5 s.
func selectTimingStream(a, b []pulseEdge, aMed, bMed time.Duration, pulseWidthLimit float64) ([]pulseEdge, time.Duration, error) {
	if aMed <= 0 || bMed <= 0 {
		return nil, 0, ErrNotReady
	}
	pulseWidthPHC, ok := crossPolarityGap(a, b)
	if !ok {
		return nil, 0, ErrNotReady
	}
	avgMedian := (aMed + bMed) / 2
	pulseWidth := float64(pulseWidthPHC) * (float64(time.Second) / float64(avgMedian))
	switch {
	case pulseWidth <= pulseWidthLimit*float64(time.Second):
		// Short gap from a b-edge to the next a-edge: b is the
		// rising polarity (top-of-second).
		edges := removeZeroEdges(b)
		if len(edges) == 0 {
			return nil, 0, ErrNotReady
		}
		return edges, bMed, nil
	case (1-pulseWidthLimit)*float64(time.Second) <= pulseWidth:
		// Long gap from a b-edge to the next a-edge: a is the
		// rising polarity.
		edges := removeZeroEdges(a)
		if len(edges) == 0 {
			return nil, 0, ErrNotReady
		}
		return edges, aMed, nil
	default:
		return nil, 0, ErrNotReady
	}
}

// crossPolarityGap averages the PHC interval from each b-edge to the
// following a-edge, skipping pairs where either side is the zero
// sentinel. Returns ok=false when no admissible pair is available.
// Matches the "gap from polarity B to the next polarity A" semantic
// the selectTimingStream short/long branches are written against.
func crossPolarityGap(a, b []pulseEdge) (time.Duration, bool) {
	var total time.Duration
	var n int
	for i := 0; i+1 < len(a) && i < len(b); i++ {
		if b[i].IsZero() || a[i+1].IsZero() {
			continue
		}
		total += a[i+1].Timestamp.T.Sub(b[i].Timestamp.T)
		n++
	}
	if n == 0 {
		return 0, false
	}
	return total / time.Duration(n), true
}

// calibEntry is one point on the PHC-to-true-time ruler. X is PHC
// nanoseconds relative to the per-call basePHC anchor, shifted by the
// PHC-scaled pulse-offset correction (phase 2) so the ruler mark lands
// at the exact top-of-second rather than at the physical edge; Y is
// the integer-second UTC label of that top-of-second.
type calibEntry struct {
	X float64
	Y time.Time
}

// mapEdgesToUTC labels each admitted edge with an integer-second UTC
// via the wallClock fit. Edges whose predicted UTC is too far from an
// integer second are dropped (EdgeSecondTolerance). wallClock errors
// propagate: ErrNotReady surfaces immediately, errStale stops
// iteration (later edges are also stale) but returns the admitted
// prefix, others abort with the error.
func mapEdgesToUTC(edges []pulseEdge, medianInterval time.Duration, basePHC ptime.Time, wc *wallClock, po pulseCorrector, edgeSecTol float64) ([]calibEntry, error) {
	if medianInterval <= 0 {
		return nil, ErrNotReady
	}
	tolNs := int64(edgeSecTol * float64(time.Second))
	entries := make([]calibEntry, 0, len(edges))
	scale := float64(time.Second) / float64(medianInterval)
	for _, edge := range edges {
		phcDelta := edge.TRead.PHC.T.Sub(edge.Timestamp.T)
		realDelta := time.Duration(float64(phcDelta) * scale)
		edgeMono := edge.TRead.Sys.Add(-realDelta)

		pred, err := wc.predictUTC(edgeMono)
		if err != nil {
			if errors.Is(err, ErrNotReady) {
				return nil, ErrNotReady
			}
			if errors.Is(err, errStale) {
				break
			}
			return nil, err
		}

		rounded := pred.Round(time.Second)
		r := pred.Sub(rounded)
		if r < 0 {
			r = -r
		}
		if int64(r) > tolNs {
			continue
		}

		var pulseOffsetPHCNs float64
		if po != nil {
			if v, ok := po.GetUTCPulseCorrection(rounded); ok {
				// PulseOffset is true-time ns: true_second = pulse_time + PulseOffset,
				// so the physical edge sits PulseOffset true-time ns before the top
				// of second. Convert into PHC ns (scale = realPerPHC, so PHC ns per
				// real ns = 1/scale = medianInterval/Second) and add to the edge's
				// PHC coordinate, shifting the ruler mark from the physical edge
				// to the top-of-second that Y labels.
				pulseOffsetPHCNs = v / scale
			}
		}

		x := float64(edge.Timestamp.T.Sub(basePHC).Nanoseconds()) + pulseOffsetPHCNs
		entries = append(entries, calibEntry{X: x, Y: rounded})
	}
	if len(entries) < minFitEntries {
		return nil, ErrNotReady
	}
	return entries, nil
}

// fitAndEvaluate fits a plain OLS line to the calibration entries and
// evaluates it at phc, subtracting sys to yield the refclock offset in
// seconds. The entire pipeline after mapEdgesToUTC stays in float64
// nanoseconds so the phase-2 sub-ns pulse-offset correction is carried
// through to the returned seconds value.
func fitAndEvaluate(entries []calibEntry, basePHC ptime.Time, phc ptime.Time, sys time.Time, maxExtrap time.Duration) (float64, error) {
	if len(entries) < minFitEntries {
		return 0, ErrNotReady
	}
	xQuery := float64(phc.Sub(basePHC).Nanoseconds())
	if xQuery-entries[len(entries)-1].X > float64(maxExtrap.Nanoseconds()) {
		return 0, errExtrapolation
	}
	yRef := entries[0].Y

	n := float64(len(entries))
	ys := make([]float64, len(entries))
	var sumX, sumY float64
	for i, e := range entries {
		ys[i] = float64(e.Y.Sub(yRef).Nanoseconds())
		sumX += e.X
		sumY += ys[i]
	}
	meanX := sumX / n
	meanY := sumY / n

	var sxy, sxx float64
	for i, e := range entries {
		dx := e.X - meanX
		dy := ys[i] - meanY
		sxy += dx * dy
		sxx += dx * dx
	}
	var slope, intercept float64
	if sxx == 0 {
		slope = 1
		intercept = meanY - meanX
	} else {
		slope = sxy / sxx
		intercept = meanY - slope*meanX
	}
	yQueryNs := intercept + slope*xQuery
	// Keep the big (yRef - sys) delta as an int64 Duration until the
	// very end, then convert via Duration.Seconds() (which splits
	// sec/nsec internally). This matches how the refclock-sample
	// consumer sees offsets in production where sys and true time
	// differ by at most a few seconds, and is the only way to stay
	// below the float64 nanosecond quantization floor (~240 ns at
	// 1.5e9-second offsets) that simulations deliberately exercise.
	// Sub-ns precision from the phase-2 pulse-offset correction
	// survives via yQFracNs.
	yQIntNs := int64(yQueryNs)
	yQFracNs := yQueryNs - float64(yQIntNs)
	totalDur := yRef.Sub(sys) + time.Duration(yQIntNs)
	return totalDur.Seconds() + yQFracNs*1e-9, nil
}
