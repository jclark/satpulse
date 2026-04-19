package phcsample

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

// TestGeneratorWarmUpReturnsErrNotReady confirms that Generate returns
// ErrNotReady while phcWindow is stubbed.
func TestGeneratorWarmUpReturnsErrNotReady(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := NewGenerator(DefaultConfig(), 1, lg)

	g.MsgUTCTime(utcBaseTime, utcBaseTime.Add(100*time.Millisecond), ptime.LeapSecondNone)
	g.Pulse(PulseEdge{})

	off, err := g.Generate(0, utcBaseTime)
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v, want ErrNotReady", err)
	}
	if off != 0 {
		t.Errorf("offset = %v, want 0", off)
	}
}
