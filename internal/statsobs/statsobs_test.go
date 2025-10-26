package statsobs

import (
	"testing"

	"github.com/jclark/satpulse/internal/phcsync"
)

func TestAccumPhase(t *testing.T) {
	t.Run("BasicAccumulation", func(t *testing.T) {
		a := accumPhase{}

		a.add(phcsync.SampleOK, 0.1)
		a.add(phcsync.SampleOK, -0.2)
		a.add(phcsync.SampleOK, 0.15)

		if a.n != 3 {
			t.Errorf("n: got %d, want 3", a.n)
		}

		meanAbs := a.meanAbs()
		expectedMeanAbs := (0.1 + 0.2 + 0.15) / 3
		diff := meanAbs - expectedMeanAbs
		if diff < -0.0001 || diff > 0.0001 {
			t.Errorf("meanAbs: got %f, want %f", meanAbs, expectedMeanAbs)
		}

		if a.maxAbs != 0.2 {
			t.Errorf("maxAbs: got %f, want 0.2", a.maxAbs)
		}
	})

	t.Run("CountsMissingAndOutliers", func(t *testing.T) {
		a := accumPhase{}

		a.add(phcsync.SampleOK, 0.1)
		a.add(phcsync.SampleMissing, 0)
		a.add(phcsync.SampleOutlier, 0)
		a.add(phcsync.SampleMissing, 0)

		if a.n != 4 {
			t.Errorf("n: got %d, want 4", a.n)
		}
		if a.nMissing != 2 {
			t.Errorf("nMissing: got %d, want 2", a.nMissing)
		}
		if a.nOutliers != 1 {
			t.Errorf("nOutliers: got %d, want 1", a.nOutliers)
		}
	})
}

func TestAccumFreq(t *testing.T) {
	t.Run("BasicAccumulation", func(t *testing.T) {
		a := accumFreq{}

		a.add(1000)
		a.add(1100)
		a.add(900)

		if a.n != 3 {
			t.Errorf("n: got %d, want 3", a.n)
		}

		mean := a.mean()
		if mean != 1000 {
			t.Errorf("mean: got %f, want 1000", mean)
		}
	})
}
