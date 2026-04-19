package phcsample

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

type capturingSampler struct {
	calls int
}

func (s *capturingSampler) NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time) {
	s.calls++
}

// TestGeneratorWarmUpReturnsErrNotReady confirms that Generate returns
// ErrNotReady while phcWindow is stubbed and that no Sampler call is
// issued on the error path.
func TestGeneratorWarmUpReturnsErrNotReady(t *testing.T) {
	smp := &capturingSampler{}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := NewGenerator(DefaultConfig(), smp, 1, lg)

	g.MsgUTCTime(utcBaseTime, utcBaseTime.Add(100*time.Millisecond), ptime.LeapSecondNone)
	g.Pulse(PulseEdge{})

	off, err := g.Generate(0, utcBaseTime)
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v, want ErrNotReady", err)
	}
	if off != 0 {
		t.Errorf("offset = %v, want 0", off)
	}
	if smp.calls != 0 {
		t.Errorf("Sampler called %d times on error path; want 0", smp.calls)
	}
}
