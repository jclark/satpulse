package phcsample

import (
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

func TestFitAndEvaluateSmoothPhaseFalseAnchorsNewestEntry(t *testing.T) {
	entries := []calibEntry{
		{X: 0, Y: utcBaseTime},
		{X: float64(time.Second), Y: utcBaseTime.Add(time.Second)},
		{X: float64(2 * time.Second), Y: utcBaseTime.Add(3 * time.Second)},
	}

	got, err := fitAndEvaluate(
		entries,
		0,
		ptime.Time(2*time.Second),
		utcBaseTime,
		3*time.Second,
		false,
	)
	if err != nil {
		t.Fatalf("fitAndEvaluate(..., smoothPhase=false) err = %v", err)
	}
	if math.Abs(got-3.0) > 1e-12 {
		t.Fatalf("offset = %.12f, want 3.0", got)
	}
}

func TestFitAndEvaluateSmoothPhaseTrueMatchesLegacyWindowOLS(t *testing.T) {
	entries := []calibEntry{
		{X: 0, Y: utcBaseTime},
		{X: float64(time.Second), Y: utcBaseTime.Add(time.Second)},
		{X: float64(2 * time.Second), Y: utcBaseTime.Add(3 * time.Second)},
	}

	phcQuery := ptime.Time(2500 * time.Millisecond)
	got, err := fitAndEvaluate(entries, 0, phcQuery, utcBaseTime, 3*time.Second, true)
	if err != nil {
		t.Fatalf("fitAndEvaluate(..., smoothPhase=true) err = %v", err)
	}
	want, err := legacyFitAndEvaluate(entries, 0, phcQuery, utcBaseTime, 3*time.Second)
	if err != nil {
		t.Fatalf("legacyFitAndEvaluate err = %v", err)
	}
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("offset = %.12f, want legacy OLS %.12f", got, want)
	}

	anchored, err := fitAndEvaluate(entries, 0, phcQuery, utcBaseTime, 3*time.Second, false)
	if err != nil {
		t.Fatalf("fitAndEvaluate(..., smoothPhase=false) err = %v", err)
	}
	if math.Abs(anchored-got) < 1e-12 {
		t.Fatalf("anchored and smoothed offsets unexpectedly match: %.12f", got)
	}
}

func legacyFitAndEvaluate(entries []calibEntry, basePHC ptime.Time, phc ptime.Time, sys time.Time, maxExtrap time.Duration) (float64, error) {
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
	yQIntNs := int64(yQueryNs)
	yQFracNs := yQueryNs - float64(yQIntNs)
	totalDur := yRef.Sub(sys) + time.Duration(yQIntNs)
	return totalDur.Seconds() + yQFracNs*1e-9, nil
}
