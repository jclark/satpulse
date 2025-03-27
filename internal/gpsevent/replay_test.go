package gpsevent

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
)

func TestReplayFast(t *testing.T) {
	testReplay(t, "testdata/fast.jsonl", phc.DriverBothEdges, 34)
}

func testReplay(t *testing.T, fn string, phcFlags phc.DriverFlags, expectedSampleCount int) {
	sampler := replaySampler{t: t}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))

	err := ReplayFile(fn, phcFlags, &sampler, ptime.LeapSecond2016(), lg)
	if err != nil {
		t.Fatalf("error replaying %s: %v", fn, err)
	}
	if sampler.sampleCount != expectedSampleCount {
		t.Errorf("too few samples emitted: got %d, want %d", sampler.sampleCount, expectedSampleCount)
	}
}

type replaySampler struct {
	t           *testing.T
	ref         ptime.Time
	sampleCount int
}

func (s *replaySampler) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	if ref == s.ref {
		s.t.Errorf("duplicate sample at %v", ref)
	}
	s.ref = ref
	s.sampleCount++
}
