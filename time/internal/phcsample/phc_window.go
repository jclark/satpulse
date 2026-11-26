package phcsample

import (
	"errors"
	"math"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/lib/ntime"
)

// phcWindow holds recent pulse edges, labels them via the wallClock
// at query time, and turns each edge that passes pre-admission
// filtering into its own PHC-referenced refclock sample.
type phcWindow struct {
	cfg           *Config
	edgesPerPulse int
	buf           []pulseEdge
	lastEmitted   ptime.Time // PHC timestamp of the newest edge already returned by Samples
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

// refSample is one PHC-referenced refclock sample: the PHC timestamp
// of a pulse edge and the offset (true time - PHC reading) in seconds
// at that instant.
type refSample struct {
	phc    ptime.Time
	offset float64
}

// Samples labels admitted edges via the wallClock and returns one
// refSample per edge not returned by a previous call, oldest first.
// Returns ErrNotReady while the pipeline does not yet have enough
// admissible data, and other sentinels (wallClock gates) for less
// routine failures; an empty slice with a nil error means the pipeline
// is healthy but has no new labelled edge yet.
func (w *phcWindow) Samples(wc *wallClock, po pulseCorrector) ([]refSample, error) {
	if len(w.buf) == 0 {
		return nil, ErrNotReady
	}
	timing, medianInterval, err := timingEdges(w.buf, w.edgesPerPulse, w.cfg)
	if err != nil {
		return nil, err
	}
	return w.labelEdges(timing, medianInterval, wc, po)
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
// can assume positional alignment.
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

// labelEdges labels each not-yet-emitted admitted edge with an
// integer-second TAI label via the wallClock fit and turns it into a
// refSample. Edges whose predicted label is too far from an integer
// second are dropped (EdgeSecondTolerance). wallClock errors
// propagate: ErrNotReady surfaces immediately, errStale stops
// iteration (later edges are also stale) but returns the admitted
// prefix, others abort with the error.
func (w *phcWindow) labelEdges(edges []pulseEdge, medianInterval time.Duration, wc *wallClock, po pulseCorrector) ([]refSample, error) {
	if medianInterval <= 0 {
		return nil, ErrNotReady
	}
	tolNs := int64(w.cfg.EdgeSecondTolerance * float64(time.Second))
	var samples []refSample
	scale := float64(time.Second) / float64(medianInterval)
	for _, edge := range edges {
		if edge.Timestamp.T <= w.lastEmitted {
			continue
		}
		phcDelta := edge.TRead.PHC.T.Sub(edge.Timestamp.T)
		realDelta := time.Duration(float64(phcDelta) * scale)
		edgeMono := edge.TRead.Sys.Add(-realDelta)

		pred, err := wc.predictRef(edgeMono)
		if err != nil {
			if errors.Is(err, ErrNotReady) {
				return nil, ErrNotReady
			}
			if errors.Is(err, errStale) {
				break
			}
			return nil, err
		}

		ref := pred.Round(time.Second)
		r := pred.Sub(ref)
		if r < 0 {
			r = -r
		}
		if int64(r) > tolNs {
			continue
		}

		var corrNs float64
		if po != nil {
			corrNs, _ = po.GetTAIPulseCorrection(ptime.Time(ref))
		}

		// offset = true time - PHC reading at the edge. The label
		// marks the top-of-second; the physical edge sits corrNs
		// true-time ns before it (true_second = pulse_time +
		// correction), so the true time at the edge is ref - corrNs.
		// The int64 Duration carries the (possibly huge) PHC-to-TAI
		// distance exactly; converting to float64 seconds at the end
		// matches the double the SOCK protocol carries anyway.
		d := ref.Sub(ntime.Time(edge.Timestamp.T))
		samples = append(samples, refSample{phc: edge.Timestamp.T, offset: d.Seconds() - corrNs*1e-9})
		w.lastEmitted = edge.Timestamp.T
	}
	return samples, nil
}
