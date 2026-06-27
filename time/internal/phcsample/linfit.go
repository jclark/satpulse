package phcsample

// This file is the ordinary-least-squares core shared by the package's
// fits. It is deliberately self-contained (nothing phcsample-specific)
// so it can be promoted to a library package if a consumer outside
// phcsample appears.

// value constrains the coordinate types of a linear fit.
type value interface{ ~int64 | ~float64 }

// fittedLine is a line y = intercept + slope*x fitted over points
// (X, Y). The X parameter does not appear in the representation; it
// records which x-domain the line was fitted over, so lineAt cannot
// evaluate it against a different domain.
type fittedLine[X, Y value] struct {
	slope     float64 // change in Y per unit X
	intercept Y
}

// fitLine computes the ordinary-least-squares fittedLine over the
// points (xs[i], ys[i]). The slices must have equal length. ok is
// false for degenerate input: fewer than two points, or no x spread.
// The arithmetic is float64 throughout; for an int64-based Y the
// intercept is truncated once here, not on each evaluation. Callers
// are expected to supply anchored deltas, keeping magnitudes within
// float64's exact-integer range.
func fitLine[X, Y value](xs []X, ys []Y) (l fittedLine[X, Y], ok bool) {
	n := len(xs)
	if n < 2 {
		return fittedLine[X, Y]{}, false
	}
	var sumX, sumY float64
	for i := range n {
		sumX += float64(xs[i])
		sumY += float64(ys[i])
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)
	var sxy, sxx float64
	for i := range n {
		dx := float64(xs[i]) - meanX
		dy := float64(ys[i]) - meanY
		sxy += dx * dy
		sxx += dx * dx
	}
	if sxx == 0 {
		return fittedLine[X, Y]{}, false
	}
	slope := sxy / sxx
	return fittedLine[X, Y]{slope: slope, intercept: Y(meanY - slope*meanX)}, true
}

// lineAt evaluates l at x. For an int64-based Y the slope term is
// truncated, matching ordinary Go conversion semantics.
func lineAt[X, Y value](l fittedLine[X, Y], x X) Y {
	return l.intercept + Y(l.slope*float64(x))
}

// lineResidual returns y - lineAt(l, x).
func lineResidual[X, Y value](l fittedLine[X, Y], x X, y Y) Y {
	return y - lineAt(l, x)
}
