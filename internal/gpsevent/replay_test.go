package gpsevent

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/timemsg"
)

func TestReplayFast(t *testing.T) {
	sampler := &replaySampler{t: t}
	warnCountHandler := NewWarnCountHandler(slog.NewTextHandler(io.Discard, nil))
	lg := slog.New(warnCountHandler)
	// Create mock clock
	clock := &mockClock{}
	// Create controller
	cfg := phcsync.DefaultConfig()
	ctrl, err := phcsync.NewController(clock, sampler, nil, cfg, ptime.LeapSecond2016(), 2, lg)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}
	defer ctrl.Close()
	// Create time message buffer
	timeMsgBuffer := timemsg.NewBuffer(lg, 5*time.Second, ptime.LeapSecond2016(), gpsprot.GPS)
	ctrl.SetTimeMsgBuffer(timeMsgBuffer)
	// Replay file
	err = ReplayFile("testdata/fast.jsonl", ctrl, timeMsgBuffer, nil)
	if err != nil {
		t.Fatalf("error replaying: %v", err)
	}
	// Verify reset mode behavior
	if sampler.sampleCount == 0 {
		t.Errorf("no samples emitted during replay")
	}
	// Reset mode should produce at least one sample before transitioning
	if sampler.sampleCount < 1 {
		t.Errorf("expected at least 1 sample from reset mode, got %d", sampler.sampleCount)
	}
	t.Logf("Replayed fast.jsonl: %d samples, %d warnings, mode=%s",
		sampler.sampleCount, warnCountHandler.WarnCount(), ctrl.Mode())
}

// mockClock implements phcsync.Clock for testing
type mockClock struct {
	freqOffset float64
}

func (m *mockClock) SetFreqOffset(offset float64) error {
	m.freqOffset = offset
	return nil
}

func (m *mockClock) FreqOffset() (float64, error) {
	return m.freqOffset, nil
}

func (m *mockClock) MaxFreqOffset() float64 {
	return 62500000.0 // 62.5 million PPB = 62500 PPM
}

func (m *mockClock) AdjTime(d time.Duration) (ptime.Era, error) {
	return ptime.Era(1), nil
}

// replaySampler implements phcsync.Sampler for testing
type replaySampler struct {
	t           *testing.T
	ref         ptime.Time
	sampleCount int
}

func (s *replaySampler) Sample(data phcsync.Sample) {
	if data.Kind == phcsync.SampleMissing {
		return
	}
	if data.Ref == s.ref {
		s.t.Errorf("duplicate sample at %v", data.Ref)
	}
	s.ref = data.Ref
	s.sampleCount++
}
