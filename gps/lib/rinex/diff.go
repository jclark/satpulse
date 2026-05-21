package rinex

import (
	"fmt"
	"math"
	"slices"

	"github.com/jclark/satpulse/gps/lib/opt"
)

// Tolerances controls observation value comparison.
type Tolerances struct {
	PR  float64
	CP  float64
	Do  float64
	CN0 float64
}

// DiffReporter receives observation differences.
type DiffReporter interface {
	Diff(t Time, sat SatelliteID, sig SignalID, a, b *SignalValues) error
}

// DiffErrorReporter receives non-fatal input problems found while diffing.
type DiffErrorReporter interface {
	Duplicate(side int, index, prevIndex int) error
	Unordered(side int, index int) error
}

type obsKey struct {
	sat SatelliteID
	sig SignalID
}

type obsIndex struct {
	epochs map[Time]map[obsKey]int
	times  []Time
	obs    []SignalObservation
}

// DiffObservations compares two observation streams and reports each difference.
func DiffObservations(a, b []SignalObservation, tol Tolerances, r DiffReporter, er DiffErrorReporter) (int, error) {
	if err := validateTolerances(tol); err != nil {
		return 0, err
	}
	ai, err := indexObservations(0, a, er)
	if err != nil {
		return 0, err
	}
	bi, err := indexObservations(1, b, er)
	if err != nil {
		return 0, err
	}
	n, err := reportDiffs(ai, bi, tol, r)
	return n, err
}

// DiffSignal compares two signal observations and returns the differing values.
func DiffSignal(a, b *SignalValues, tol Tolerances) (aRet, bRet *SignalValues) {
	if a == nil || b == nil {
		return a, b
	}
	aRet = &SignalValues{}
	bRet = &SignalValues{}
	compareValue(&aRet.Frq, &bRet.Frq, a.Frq.IsSet(), b.Frq.IsSet(), a.Frq.Get(), b.Frq.Get())
	compareFloat64(&aRet.PR, &bRet.PR, a.PR.IsSet(), b.PR.IsSet(), a.PR.Get(), b.PR.Get(), tol.PR)
	compareFloat64(&aRet.CP, &bRet.CP, a.CP.IsSet(), b.CP.IsSet(), a.CP.Get(), b.CP.Get(), tol.CP)
	compareFloat64(&aRet.Do, &bRet.Do, a.Do.IsSet(), b.Do.IsSet(), a.Do.Get(), b.Do.Get(), tol.Do)
	compareFloat32(&aRet.CN0, &bRet.CN0, a.CN0.IsSet(), b.CN0.IsSet(), a.CN0.Get(), b.CN0.Get(), tol.CN0)
	compareValue(&aRet.LLI, &bRet.LLI, a.LLI.IsSet(), b.LLI.IsSet(), a.LLI.Get(), b.LLI.Get())
	compareValue(&aRet.SSI, &bRet.SSI, a.SSI.IsSet(), b.SSI.IsSet(), a.SSI.Get(), b.SSI.Get())
	return aRet, bRet
}

func indexObservations(side int, obs []SignalObservation, er DiffErrorReporter) (obsIndex, error) {
	idx := obsIndex{epochs: make(map[Time]map[obsKey]int), obs: obs}
	var lastT Time
	haveLast := false
	for i, o := range obs {
		if haveLast && o.T < lastT {
			if er != nil {
				if err := er.Unordered(side, i); err != nil {
					return idx, err
				}
			}
		}
		if _, ok := idx.epochs[o.T]; !ok {
			idx.epochs[o.T] = make(map[obsKey]int)
			idx.times = append(idx.times, o.T)
		}
		k := obsKey{sat: o.Sat, sig: o.Sig}
		if prev, ok := idx.epochs[o.T][k]; ok {
			if er != nil {
				if err := er.Duplicate(side, i, prev); err != nil {
					return idx, err
				}
			}
		} else {
			idx.epochs[o.T][k] = i
		}
		lastT = o.T
		haveLast = true
	}
	return idx, nil
}

func reportDiffs(a, b obsIndex, tol Tolerances, r DiffReporter) (int, error) {
	n := 0
	for _, t := range diffTimes(a, b) {
		keys := diffKeys(a.epochs[t], b.epochs[t])
		for _, k := range keys {
			af, bf := compareAt(t, k, a, b, tol)
			if af != nil && bf != nil && af.IsZero() && bf.IsZero() {
				continue
			}
			if err := r.Diff(t, k.sat, k.sig, af, bf); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

func diffTimes(a, b obsIndex) []Time {
	seen := make(map[Time]bool)
	out := make([]Time, 0, len(a.times)+len(b.times))
	for _, t := range a.times {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range b.times {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}

func diffKeys(a, b map[obsKey]int) []obsKey {
	seen := make(map[obsKey]bool)
	out := make([]obsKey, 0, len(a)+len(b))
	for k := range a {
		seen[k] = true
		out = append(out, k)
	}
	for k := range b {
		if !seen[k] {
			out = append(out, k)
		}
	}
	slices.SortFunc(out, compareObsKey)
	return out
}

func compareAt(t Time, k obsKey, ai, bi obsIndex, tol Tolerances) (*SignalValues, *SignalValues) {
	an, aok := ai.epochs[t][k]
	bn, bok := bi.epochs[t][k]
	var av *SignalValues
	var bv *SignalValues
	if aok {
		av = &ai.obs[an].SignalValues
	}
	if bok {
		bv = &bi.obs[bn].SignalValues
	}
	return DiffSignal(av, bv, tol)
}

func validateTolerances(tol Tolerances) error {
	if tol.PR < 0 || tol.CP < 0 || tol.Do < 0 || tol.CN0 < 0 {
		return fmt.Errorf("tolerances must be non-negative")
	}
	return nil
}

func compareObsKey(a, b obsKey) int {
	if a.sat < b.sat {
		return -1
	}
	if a.sat > b.sat {
		return 1
	}
	if a.sig < b.sig {
		return -1
	}
	if a.sig > b.sig {
		return 1
	}
	return 0
}

func compareValue[T comparable](a, b *opt.Val[T], as, bs bool, av, bv T) {
	if as == bs && (!as || av == bv) {
		return
	}
	if as {
		a.Set(av)
	}
	if bs {
		b.Set(bv)
	}
}

func compareFloat64(a, b *opt.Val[float64], as, bs bool, av, bv, tol float64) {
	if as == bs && (!as || math.Abs(av-bv) <= tol) {
		return
	}
	if as {
		a.Set(av)
	}
	if bs {
		b.Set(bv)
	}
}

func compareFloat32(a, b *opt.Val[float32], as, bs bool, av, bv float32, tol float64) {
	if as == bs && (!as || math.Abs(float64(av-bv)) <= tol) {
		return
	}
	if as {
		a.Set(av)
	}
	if bs {
		b.Set(bv)
	}
}
