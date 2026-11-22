package phcsync

import (
	"testing"
	"time"
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
