package gpsevent

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
)

func TestReplayFast(t *testing.T) {
	testReplay(t, "testdata/fast.jsonl", phc.DriverBothEdges, 36, 0)
}

func TestReplayNMEA(t *testing.T) {
	testReplay(t, "testdata/nmea.jsonl", phc.DriverOneEdge|phc.DriverPoll4Hz, 360, 0)
}

func testReplay(t *testing.T, fn string, phcFlags phc.DriverFlags, expectedSampleCount int, expectedWarnCount int) {
	sampler := replaySampler{t: t}
	warnCountHandler := NewWarnCountHandler(slog.NewTextHandler(io.Discard, nil))
	lg := slog.New(warnCountHandler)

	err := ReplayFile(fn, phcFlags, &sampler, ptime.LeapSecond2016(), nil, lg)
	if err != nil {
		t.Fatalf("error replaying %s: %v", fn, err)
	}
	if sampler.sampleCount != expectedSampleCount {
		t.Errorf("wrong number of samples emitted: got %d, want %d", sampler.sampleCount, expectedSampleCount)
	}
	if warnCountHandler.WarnCount() != expectedWarnCount {
		t.Errorf("wrong number of warnings: got %d, want %d", warnCountHandler.WarnCount(), expectedWarnCount)
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
