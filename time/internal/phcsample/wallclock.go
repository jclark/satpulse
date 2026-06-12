package phcsample

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/lib/ntime"
)

// errStale indicates a wallClock query is more than MaxMsgGap past the
// most recent observation. labelEdges uses this as a stop-iterating
// signal: later edges in chronological order are also stale.
var errStale = errors.New("phcsample: wallClock query beyond max message gap")

// maxBackwardCoverage is how far before the earliest retained message
// read-time the wallClock fit is allowed to extrapolate. The fit only
// needs to identify the reference second of a pulse edge, so a small bounded
// backward extrapolation is acceptable and avoids startup sensitivity
// to whether the first actual message delay lands slightly above or
// below ExpectedDelay.
const maxBackwardCoverage = 500 * time.Millisecond

// wallClock maps monotonic time to an integer-second reference time
// from the message-time stream. The reference domain is whatever the
// feeding sink delivers (TAI in PHC-base-clock mode). Internally it
// maintains a sliding linear regression over
// (tRead - expectedDelay, ref) pairs.
//
// Design note on absolute delay. wallClock does not attempt to measure
// the absolute pulse-to-message delay from its inputs. tRead carries a
// monotonic-clock reading and ref is a wall-clock value; their raw
// difference folds in the current CLOCK_REALTIME error, which is
// precisely what phcsample exists to fix. An OLS fit through
// (tRead, ref) absorbs any constant delay into the intercept, so no
// absolute-delay quantity can be recovered from the fit either.
// Consequently the configured gates validate only quantities that are
// meaningful purely from the message stream: enough data, data span,
// extrapolation staleness, fitted rate, and scatter around the fit.
type wallClock struct {
	expectedDelay time.Duration
	msgWindow     time.Duration
	maxGap        time.Duration
	minSpan       time.Duration
	rateLimit     float64
	timingVar     time.Duration

	points []wallPoint

	fitValid bool
	anchorX  time.Time
	anchorY  ntime.Time
	a        time.Duration
	b        float64
}

type wallPoint struct {
	tRead time.Time
	ref   ntime.Time
}

func newWallClock(cfg *Config) *wallClock {
	return &wallClock{
		expectedDelay: ptime.Seconds(cfg.ExpectedDelay),
		msgWindow:     time.Duration(cfg.MsgWindow) * time.Second,
		maxGap:        ptime.Seconds(cfg.MaxMsgGap),
		minSpan:       ptime.Seconds(cfg.MinMsgSpan),
		rateLimit:     cfg.ClockRateLimit,
		timingVar:     ptime.Seconds(cfg.MsgTimingVariation),
	}
}

// Add observes a message-time sample. tRead is the monotonic read time
// (with ReadDelay already subtracted by timemsg.Buffer); ref is the
// ms-rounded reference time reported for that message. Observations
// older than MsgWindow are discarded.
func (c *wallClock) Add(tRead time.Time, ref ntime.Time) {
	c.points = append(c.points, wallPoint{tRead: tRead, ref: ref})
	cutoff := tRead.Add(-c.msgWindow)
	drop := 0
	for drop < len(c.points) && c.points[drop].tRead.Before(cutoff) {
		drop++
	}
	if drop > 0 {
		c.points = append(c.points[:0], c.points[drop:]...)
	}
	c.fitValid = false
}

// predictRef returns the fit-predicted reference time (unrounded) at
// the given monotonic instant. On failure it returns an error
// describing which gate rejected the fit; the sentinel ErrNotReady is
// returned when there isn't yet enough observation to answer (too few
// points, or observations span too little real time), which callers
// treat as a quiet transient state. labelEdges applies
// cfg.EdgeSecondTolerance to the unrounded prediction before rounding.
func (c *wallClock) predictRef(mono time.Time) (ntime.Time, error) {
	if len(c.points) < 2 {
		return 0, ErrNotReady
	}
	span := c.points[len(c.points)-1].tRead.Sub(c.points[0].tRead)
	if span < c.minSpan {
		return 0, ErrNotReady
	}
	last := c.points[len(c.points)-1].tRead
	if gap := mono.Sub(last); gap > c.maxGap {
		return 0, fmt.Errorf("phcsample: wallClock query %v past most recent message (limit %v): %w",
			gap, c.maxGap, errStale)
	}
	// Backward coverage: reject queries that fall too far before the
	// earliest retained observation's read-time. A small bounded
	// backward extrapolation is fine for assigning a pulse edge to the
	// correct UTC second, but the fit must not reach arbitrarily far
	// back into stale pulse history after recovery from a message gap.
	// Return ErrNotReady rather than errStale: errStale is reserved
	// for forward-uncovered queries and drives mapEdgesToUTC's
	// stop-iterating behaviour, which is not what we want here.
	firstCoveredMono := c.points[0].tRead.Add(-maxBackwardCoverage)
	if mono.Before(firstCoveredMono) {
		return 0, ErrNotReady
	}
	if !c.fitValid {
		c.refit()
	}
	if math.Abs(c.b-1) > c.rateLimit {
		return 0, fmt.Errorf("phcsample: wallClock fitted rate mismatch %.4f exceeds limit %.4f",
			c.b-1, c.rateLimit)
	}
	if medAbs := c.medianAbsResidual(); medAbs > c.timingVar {
		return 0, fmt.Errorf("phcsample: wallClock median residual %v exceeds limit %v",
			medAbs, c.timingVar)
	}
	dx := mono.Sub(c.anchorX)
	offset := c.a + time.Duration(c.b*float64(dx))
	return c.anchorY.Add(offset), nil
}

func (c *wallClock) refit() {
	c.fitValid = true
	n := len(c.points)
	c.anchorX = c.points[0].tRead.Add(-c.expectedDelay)
	c.anchorY = c.points[0].ref

	var sumX, sumY time.Duration
	xs := make([]time.Duration, n)
	ys := make([]time.Duration, n)
	for i, p := range c.points {
		xs[i] = p.tRead.Add(-c.expectedDelay).Sub(c.anchorX)
		ys[i] = p.ref.Sub(c.anchorY)
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX := sumX / time.Duration(n)
	meanY := sumY / time.Duration(n)

	var sxy, sxx float64
	for i := range xs {
		dx := float64(xs[i] - meanX)
		dy := float64(ys[i] - meanY)
		sxy += dx * dy
		sxx += dx * dx
	}
	if sxx == 0 {
		c.b = 1
		c.a = meanY - meanX
		return
	}
	c.b = sxy / sxx
	c.a = meanY - time.Duration(c.b*float64(meanX))
}

func (c *wallClock) medianAbsResidual() time.Duration {
	n := len(c.points)
	absRes := make([]time.Duration, n)
	for i, p := range c.points {
		x := p.tRead.Add(-c.expectedDelay).Sub(c.anchorX)
		y := p.ref.Sub(c.anchorY)
		pred := c.a + time.Duration(c.b*float64(x))
		r := y - pred
		if r < 0 {
			r = -r
		}
		absRes[i] = r
	}
	slices.Sort(absRes)
	if n%2 == 1 {
		return absRes[n/2]
	}
	return (absRes[n/2-1] + absRes[n/2]) / 2
}
