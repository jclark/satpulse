package phcsync

import (
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/phctime"
)

func TestMADWindow(t *testing.T) {
	t.Run("BasicAddAndCount", func(t *testing.T) {
		w := newMADWindow(5)

		if w.Len() != 0 {
			t.Errorf("Len on empty: got %d, want 0", w.Len())
		}
		if w.OutlierCount() != 0 {
			t.Errorf("OutlierCount on empty: got %d, want 0", w.OutlierCount())
		}

		w.Add(10 * time.Nanosecond)
		w.Add(20 * time.Nanosecond)
		w.Add(30 * time.Nanosecond)

		if w.Len() != 3 {
			t.Errorf("Len after 3 adds: got %d, want 3", w.Len())
		}
		if w.OutlierCount() != 0 {
			t.Errorf("OutlierCount with no outliers: got %d, want 0", w.OutlierCount())
		}
	})

	t.Run("SetLastOutlier", func(t *testing.T) {
		w := newMADWindow(5)

		w.Add(10 * time.Nanosecond)
		w.Add(20 * time.Nanosecond)
		w.SetLastOutlier()
		w.Add(30 * time.Nanosecond)
		w.SetLastOutlier()

		if w.OutlierCount() != 2 {
			t.Errorf("OutlierCount: got %d, want 2", w.OutlierCount())
		}
	})

	t.Run("OutlierEviction", func(t *testing.T) {
		w := newMADWindow(3)

		// Add 3 samples, mark second and third as outliers
		w.Add(10 * time.Nanosecond)
		w.Add(20 * time.Nanosecond)
		w.SetLastOutlier()
		w.Add(30 * time.Nanosecond)
		w.SetLastOutlier()

		if w.OutlierCount() != 2 {
			t.Errorf("OutlierCount before eviction: got %d, want 2", w.OutlierCount())
		}

		// Add another sample, evicts first (non-outlier)
		w.Add(40 * time.Nanosecond)
		if w.OutlierCount() != 2 {
			t.Errorf("OutlierCount after evicting non-outlier: got %d, want 2", w.OutlierCount())
		}

		// Add another sample, evicts second (outlier)
		w.Add(50 * time.Nanosecond)
		if w.OutlierCount() != 1 {
			t.Errorf("OutlierCount after evicting outlier: got %d, want 1", w.OutlierCount())
		}

		// Add another sample, evicts third (outlier)
		w.Add(60 * time.Nanosecond)
		if w.OutlierCount() != 0 {
			t.Errorf("OutlierCount after evicting all outliers: got %d, want 0", w.OutlierCount())
		}
	})

	t.Run("MedianComputation", func(t *testing.T) {
		w := newMADWindow(5)

		w.Add(10 * time.Nanosecond)
		w.Add(30 * time.Nanosecond)
		w.Add(20 * time.Nanosecond)

		median := w.Median()
		if median != 20*time.Nanosecond {
			t.Errorf("Median: got %v, want 20ns", median)
		}
	})

	t.Run("SetLastOutlierOnEmpty", func(t *testing.T) {
		w := newMADWindow(3)
		// Should not panic
		w.SetLastOutlier()
		if w.OutlierCount() != 0 {
			t.Errorf("OutlierCount on empty after SetLastOutlier: got %d, want 0", w.OutlierCount())
		}
	})

	t.Run("OutlierRatio", func(t *testing.T) {
		w := newMADWindow(10)

		// Add 10 samples, mark 3 as outliers
		for i := range 10 {
			w.Add(time.Duration(i) * time.Nanosecond)
			if i == 2 || i == 5 || i == 8 {
				w.SetLastOutlier()
			}
		}

		ratio := float64(w.OutlierCount()) / float64(w.Len())
		if ratio != 0.3 {
			t.Errorf("Outlier ratio: got %v, want 0.3", ratio)
		}
	})
}

// corrBuffer is a TimeMsgBuffer whose post-pulse correction is awaited until
// corr is set.
type corrBuffer struct {
	corr *time.Duration
}

func (b *corrBuffer) GetPostTimeMessages(n int) (ptime.Time, []time.Time, func(time.Duration) bool) {
	return 0, nil, nil
}

func (b *corrBuffer) GetPulseCorrection(refTime ptime.Time) (time.Duration, bool) {
	if b.corr == nil {
		return 0, false
	}
	return *b.corr, true
}

func (b *corrBuffer) WaitForPulseCorrection(refTime ptime.Time) bool { return true }

// TestHeldPulseTReadWall checks that the sample for a pulse held for its
// correction carries that pulse's TReadWall, even when a trailing edge, which
// is ignored, arrives before the sample.
func TestHeldPulseTReadWall(t *testing.T) {
	corr := 3 * time.Nanosecond
	lead := PulseEdge{
		Timestamp: phctime.Time{T: ptime.Unix(100, 20), Era: 1},
		TReadMono: phctime.Sample{PHC: phctime.Time{T: ptime.Unix(100, 1020), Era: 1}, Sys: time.Unix(5, 2000)},
		TReadWall: phctime.Sample{PHC: phctime.Time{T: ptime.Unix(100, 1100), Era: 1}, Sys: time.Unix(1_700_000_000, 0)},
	}
	trail := PulseEdge{
		Timestamp: phctime.Time{T: ptime.Unix(100, 100_000_020), Era: 1},
		TReadMono: phctime.Sample{PHC: phctime.Time{T: ptime.Unix(100, 100_001_020), Era: 1}, Sys: time.Unix(5, 100_002_000)},
		TReadWall: phctime.Sample{PHC: phctime.Time{T: ptime.Unix(100, 100_001_100), Era: 1}, Sys: time.Unix(1_700_000_000, 100_000_000)},
	}
	tests := []struct {
		name    string
		timeout bool // the correction never arrives, so Tick releases the pulse
		expect  *Sample
	}{
		{
			name: "Correction",
			expect: &Sample{Kind: SampleOK, Ref: ptime.Unix(100, -3), Offset: 23, Era: 1, EdgeIndex: 2,
				Sys: time.Unix(5, 1000), SysOffset: lead.TReadWall},
		},
		{
			name:    "Timeout",
			timeout: true,
			expect: &Sample{Kind: SampleOK, Ref: ptime.Unix(100, 0), Offset: 20, Era: 1, EdgeIndex: 2,
				Sys: time.Unix(5, 1000), SysOffset: lead.TReadWall},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := &corrBuffer{}
			last := &Sample{Kind: SampleOK, Ref: ptime.Unix(99, 0), Era: 1, EdgeIndex: 0}
			g := newTrackingSampleGenerator(defaultTrackingConfig(), PulseType{EdgesPerPulse: 2, PulseWidth: 100 * time.Millisecond},
				last, 0, 1e8, buf, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if s := g.pulseEdgeSample(lead, 2); s != nil {
				t.Fatalf("leading edge not held: %+v", s)
			}
			if s := g.pulseEdgeSample(trail, 3); s != nil {
				t.Fatalf("trailing edge not ignored: %+v", s)
			}
			var got *Sample
			if tc.timeout {
				got = g.tickSample(time.Unix(6, 0))
			} else {
				buf.corr = &corr
				got = g.timeMessageSample()
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}
