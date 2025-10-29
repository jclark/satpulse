package logobs

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/phcsync"
)

func TestStatsLogObserver(t *testing.T) {
	t.Run("NoLoggingWhenIntervalZero", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		obs := NewStatsLogObserver(lg, 0)

		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOK,
			Offset:    100 * time.Nanosecond,
			Freq:      1000,
			Mode: phcsync.ModeTracking,
		})

		if buf.Len() > 0 {
			t.Error("expected no logging when interval is 0")
		}
	})

	t.Run("ImmediateLoggingWhenIntervalOne", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		obs := NewStatsLogObserver(lg, 1)

		// Test OK sample
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOK,
			Offset:    100 * time.Nanosecond,
			Freq:      1000,
			Mode: phcsync.ModeTracking,
		})

		output := buf.String()
		if !strings.Contains(output, "adjusting clock frequency") {
			t.Error("expected 'adjusting clock frequency' log")
		}
		if !strings.Contains(output, "freq=1000") {
			t.Error("expected freq=1000 in log")
		}

		buf.Reset()

		// Test missing sample
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleMissing,
			Mode: phcsync.ModeTracking,
		})

		output = buf.String()
		if !strings.Contains(output, "missed 1PPS sample") {
			t.Error("expected 'missed 1PPS sample' log")
		}

		buf.Reset()

		// Test outlier sample
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOutlier,
			Offset:    200 * time.Nanosecond,
			Freq:      2000,
			Mode: phcsync.ModeTracking,
		})

		output = buf.String()
		if !strings.Contains(output, "outlier sample") {
			t.Error("expected 'outlier sample' log")
		}
	})

	t.Run("AccumulationAndFlush", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		obs := NewStatsLogObserver(lg, 3) // Accumulate 3 samples

		// Add 3 samples to trigger flush
		for i := 0; i < 3; i++ {
			obs.Sample(phcsync.Sample{
				Kind:      phcsync.SampleOK,
				Offset:    time.Duration(100+i*10) * time.Nanosecond,
				Freq:      float64(1000 + i*100),
				FreqDelta: float64(i * 10),
				Mode: phcsync.ModeTracking,
			})
		}

		output := buf.String()
		if !strings.Contains(output, "summary") {
			t.Error("expected 'summary' log after 3 samples")
		}
		if !strings.Contains(output, "nSecs=3") {
			t.Error("expected nSecs=3 in summary")
		}
		if !strings.Contains(output, "nMissing=0") {
			t.Error("expected nMissing=0 in summary")
		}
		if !strings.Contains(output, "nOutliers=0") {
			t.Error("expected nOutliers=0 in summary")
		}
	})

	t.Run("MixedSampleTypes", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		obs := NewStatsLogObserver(lg, 5)

		// Add different sample types
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOK,
			Offset:    100 * time.Nanosecond,
			Freq:      1000,
			Mode: phcsync.ModeTracking,
		})
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleMissing,
			Mode: phcsync.ModeTracking,
		})
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOutlier,
			Offset:    500 * time.Nanosecond,
			Freq:      1100,
			Mode: phcsync.ModeTracking,
		})
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOK,
			Offset:    150 * time.Nanosecond,
			Freq:      1050,
			Mode: phcsync.ModeTracking,
		})
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOK,
			Offset:    120 * time.Nanosecond,
			Freq:      1025,
			Mode: phcsync.ModeTracking,
		})

		output := buf.String()
		if !strings.Contains(output, "nSecs=5") {
			t.Error("expected nSecs=5")
		}
		if !strings.Contains(output, "nMissing=1") {
			t.Error("expected nMissing=1")
		}
		if !strings.Contains(output, "nOutliers=1") {
			t.Error("expected nOutliers=1")
		}
	})

	t.Run("FlushOnSyncLoss", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		obs := NewStatsLogObserver(lg, 10) // Large interval

		// Add some samples while in sync
		for i := 0; i < 3; i++ {
			obs.Sample(phcsync.Sample{
				Kind:      phcsync.SampleOK,
				Offset:    100 * time.Nanosecond,
				Freq:      1000,
				Mode: phcsync.ModeTracking,
			})
		}

		// Should not flush yet (interval is 10)
		if strings.Contains(buf.String(), "summary") {
			t.Error("should not flush before interval")
		}

		// Lose sync - should trigger flush
		obs.Sample(phcsync.Sample{
			Kind:      phcsync.SampleOK,
			Mode: phcsync.ModeReset,
		})

		output := buf.String()
		if !strings.Contains(output, "summary") {
			t.Error("expected flush when losing sync")
		}
		if !strings.Contains(output, "nSecs=3") {
			t.Error("expected nSecs=3 in partial flush")
		}
	})

	t.Run("ReleaseFlushesPartialStats", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		obs := NewStatsLogObserver(lg, 10)

		// Add partial samples
		for i := 0; i < 3; i++ {
			obs.Sample(phcsync.Sample{
				Kind:      phcsync.SampleOK,
				Offset:    100 * time.Nanosecond,
				Freq:      1000,
				Mode: phcsync.ModeTracking,
			})
		}

		// Release should flush
		obs.Release()

		output := buf.String()
		if !strings.Contains(output, "summary") {
			t.Error("expected flush on Release")
		}
		if !strings.Contains(output, "nSecs=3") {
			t.Error("expected nSecs=3 in Release flush")
		}
	})

	t.Run("NoAccumulationWhenNotInSync", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		obs := NewStatsLogObserver(lg, 3)

		// Add samples while not in sync
		for i := 0; i < 5; i++ {
			obs.Sample(phcsync.Sample{
				Kind:      phcsync.SampleOK,
				Offset:    100 * time.Nanosecond,
				Freq:      1000,
				Mode: phcsync.ModeReset,
			})
		}

		// Should not accumulate or flush
		if buf.Len() > 0 {
			t.Error("should not log when not in sync")
		}
	})

	t.Run("ReopenLogIsNoOp", func(t *testing.T) {
		lg := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		obs := NewStatsLogObserver(lg, 10)

		// Should not panic or error
		obs.ReopenLog()
	})
}