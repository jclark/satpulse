package pps

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestReconcileTimes(t *testing.T) {
	// The spacing is one of the four-read catches from a serial PPS poll: two
	// reads of 4.803us and 4.440us separated by a 1.567us gap. The straddle is
	// the larger of the two seen on ttyS0 over eight hours.
	trueOffsets := []time.Duration{0, 4803, 6370, 10810}
	const straddle = 14886 * time.Nanosecond
	tests := []struct {
		name string
		// trueAt is when each sample was really taken, straddle how far that
		// sample's wall read preceded its monotonic read, and expect the
		// wall-clock offset wanted back.
		trueAt   []time.Duration
		straddle []time.Duration
		expect   []time.Duration
	}{
		{
			name:     "no samples",
			trueAt:   nil,
			straddle: nil,
			expect:   nil,
		},
		{
			name:     "clean",
			trueAt:   trueOffsets,
			straddle: []time.Duration{0, 0, 0, 0},
			expect:   trueOffsets,
		},
		{
			name:     "straddle at first sample",
			trueAt:   trueOffsets,
			straddle: []time.Duration{straddle, 0, 0, 0},
			expect:   trueOffsets,
		},
		{
			name:     "straddle at second sample",
			trueAt:   trueOffsets,
			straddle: []time.Duration{0, straddle, 0, 0},
			expect:   trueOffsets,
		},
		{
			name:     "straddle at third sample",
			trueAt:   trueOffsets,
			straddle: []time.Duration{0, 0, straddle, 0},
			expect:   trueOffsets,
		},
		{
			name:     "straddle at last sample",
			trueAt:   trueOffsets,
			straddle: []time.Duration{0, 0, 0, straddle},
			expect:   trueOffsets,
		},
		{
			name:     "three samples still outvote a straddle",
			trueAt:   trueOffsets[:3],
			straddle: []time.Duration{0, straddle, 0},
			expect:   trueOffsets[:3],
		},
		{
			// Two samples have no majority, so the correction splits between
			// them: the spacing comes out right and both land half a straddle
			// early.
			name:     "two samples split the straddle",
			trueAt:   trueOffsets[:2],
			straddle: []time.Duration{0, straddle},
			expect:   []time.Duration{-straddle / 2, trueOffsets[1] - straddle/2},
		},
	}
	// A wall base with no monotonic reading and a monotonic base with one, so
	// that each sample's two readings can be placed independently.
	wallBase := time.Date(2026, 9, 21, 6, 45, 48, 0, time.UTC)
	monoBase := time.Now()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var wall, mono, expect []time.Time
			for i, at := range tc.trueAt {
				wall = append(wall, wallBase.Add(at-tc.straddle[i]))
				mono = append(mono, monoBase.Add(at))
			}
			for _, at := range tc.expect {
				expect = append(expect, wallBase.Add(at))
			}
			got, ok := ReconcileTimes(wall, mono)
			if !ok {
				t.Fatal("reconciliation rejected ordinary readings")
			}
			if !reflect.DeepEqual(got, expect) {
				t.Errorf("got  %v\nwant %v", got, expect)
			}
		})
	}
}

func TestReconcileTimesClockSteps(t *testing.T) {
	wallBase, monoBase := time.Unix(1_700_000_000, 0), time.Now()
	// Try both step directions, each position in or outside the four reads,
	// and both sides of the 1 ms limit. A 2:2 split just above the limit must
	// fail even though each median correction is only about half a millisecond.
	for _, magnitude := range []time.Duration{time.Millisecond - 1, time.Millisecond, time.Millisecond + 1, 100 * time.Millisecond} {
		for _, step := range []time.Duration{-magnitude, magnitude} {
			for split := 0; split <= 4; split++ {
				t.Run(fmt.Sprintf("%s/first_post_step_%d", step, split), func(t *testing.T) {
					var wall, mono []time.Time
					for i := range 4 {
						at := time.Duration(i) * 10 * time.Microsecond
						w := wallBase.Add(at)
						if i >= split {
							w = w.Add(step)
						}
						wall = append(wall, w)
						mono = append(mono, monoBase.Add(at))
					}
					wantOK := split == 0 || split == 4 || magnitude <= time.Millisecond
					got, ok := ReconcileTimes(wall, mono)
					if ok != wantOK {
						t.Fatalf("ok = %v, want %v", ok, wantOK)
					}
					if !ok {
						if got != nil {
							t.Fatalf("rejected result = %v, want nil", got)
						}
						return
					}
					shift := time.Duration(0)
					if split < 2 {
						shift = step
					} else if split == 2 {
						// medianAndSpread rounds the average toward the lower offset.
						shift = step / 2
						if step < 0 && step%2 != 0 {
							shift--
						}
					}
					for i, v := range got {
						want := wallBase.Add(time.Duration(i)*10*time.Microsecond + shift)
						if !v.Equal(want) {
							t.Errorf("sample %d = %v, want %v", i, v, want)
						}
					}
				})
			}
		}
	}
}

func TestReconcileTimesExcessiveDiscrepancy(t *testing.T) {
	wallBase, monoBase := time.Unix(1_700_000_000, 0), time.Now()
	for _, offsets := range [][4]time.Duration{
		{0, -750 * time.Microsecond, 750 * time.Microsecond, 0},
		{-2 * time.Millisecond, 0, 0, 0},
		{0, -2 * time.Millisecond, 0, 0},
		{0, 0, -2 * time.Millisecond, 0},
		{0, 0, 0, -2 * time.Millisecond},
	} {
		var wall, mono []time.Time
		for i, offset := range offsets {
			at := time.Duration(i) * 10 * time.Microsecond
			wall = append(wall, wallBase.Add(at+offset))
			mono = append(mono, monoBase.Add(at))
		}
		if got, ok := ReconcileTimes(wall, mono); ok || got != nil {
			t.Errorf("offsets %v: got %v, %v; want nil, false", offsets, got, ok)
		}
	}
}

func TestReconcileTimesAliased(t *testing.T) {
	// Samples whose single time.Time carries both readings, as now() builds
	// them outside Windows. Real readings carry a nanosecond or two of offset
	// scatter, so the property to check is the invariant rather than equality
	// with the input: each result sits at its monotonic spacing from the
	// first, and none keeps a monotonic reading.
	ts := []time.Time{time.Now(), time.Now(), time.Now(), time.Now()}
	got, ok := ReconcileTimes(ts, ts)
	if !ok {
		t.Fatal("reconciliation rejected ordinary readings")
	}
	if len(got) != len(ts) {
		t.Fatalf("got %d results, want %d", len(got), len(ts))
	}
	expect := make([]time.Time, len(ts))
	for i := range ts {
		expect[i] = got[0].Add(ts[i].Sub(ts[0]))
	}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %v\nwant %v", got, expect)
	}
	for i, v := range got {
		if v != v.Round(0) {
			t.Errorf("result %d kept a monotonic reading", i)
		}
	}
}

func TestReconcileTimesModelClock(t *testing.T) {
	// A model clock supplies one synthetic time.Time for both readings, with
	// no monotonic reading anywhere, so there is nothing to reconcile against
	// and the wall times come back unchanged.
	base := time.Date(2026, 9, 21, 6, 45, 48, 0, time.UTC)
	ts := []time.Time{base, base.Add(4803), base.Add(6370), base.Add(10810)}
	if got, ok := ReconcileTimes(ts, ts); !ok || !reflect.DeepEqual(got, ts) {
		t.Errorf("got %v, %v; want %v, true", got, ok, ts)
	}
}

func TestReconcileTimesPanics(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		wall, mono []time.Time
	}{
		{
			name: "length mismatch",
			wall: []time.Time{now, now},
			mono: []time.Time{now},
		},
		{
			name: "mono entries disagree on carrying a monotonic reading",
			wall: []time.Time{now, now},
			mono: []time.Time{now, now.Round(0)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("expected a panic")
				}
			}()
			ReconcileTimes(tc.wall, tc.mono)
		})
	}
}
