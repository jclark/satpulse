package rinex

import (
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

type recordSink struct {
	meta    []Metadata
	obs     []SignalObservation
	flushed bool
}

func (s *recordSink) Metadata(m Metadata) error {
	s.meta = append(s.meta, m)
	return nil
}

func (s *recordSink) Observation(obs SignalObservation) error {
	s.obs = append(s.obs, obs)
	return nil
}

func (s *recordSink) Flush() error {
	s.flushed = true
	return nil
}

func TestNewDecimationSinkRejectsInvalidInterval(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"subsecond", 500 * time.Millisecond, "at least 1 second"},
		{"subtick", time.Second + time.Nanosecond, "multiple of 100ns"},
		{"not day divisor", 7 * time.Second, "divide one GPS day"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDecimationSink(&recordSink{}, tt.d)
			if err == nil {
				t.Fatal("NewDecimationSink succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecimationSinkEmitsRoundedGridEpochs(t *testing.T) {
	dst := &recordSink{}
	sink, err := NewDecimationSink(dst, 5*time.Second)
	if err != nil {
		t.Fatalf("NewDecimationSink: %v", err)
	}
	meta := Metadata{}
	meta.Marker.Name = "MARK"
	if err := sink.Metadata(meta); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	obs := []SignalObservation{
		{T: mustTime(t, "2025-07-01T00:00:00.0490000"), Sat: "G03", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
		{T: mustTime(t, "2025-07-01T00:00:00.0510000"), Sat: "G03", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(2.0)}},
		{T: mustTime(t, "2025-07-01T00:00:04.9510000"), Sat: "G03", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(3.0)}},
		{T: mustTime(t, "2025-07-01T00:00:05.0510000"), Sat: "G03", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(4.0)}},
	}
	for _, o := range obs {
		if err := sink.Observation(o); err != nil {
			t.Fatalf("Observation: %v", err)
		}
	}
	if err := sink.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(dst.meta) != 1 || dst.meta[0].Marker.Name != "MARK" {
		t.Fatalf("metadata = %#v", dst.meta)
	}
	if len(dst.obs) != 2 {
		t.Fatalf("len obs = %d, want 2: %#v", len(dst.obs), dst.obs)
	}
	if dst.obs[0].T != obs[0].T || dst.obs[1].T != obs[2].T {
		t.Fatalf("observation times = %v, %v", dst.obs[0].T, dst.obs[1].T)
	}
	if !dst.flushed {
		t.Fatal("Flush did not reach wrapped sink")
	}
}

func TestDecimationSinkCarriesSkippedLLI(t *testing.T) {
	dst := &recordSink{}
	sink, err := NewDecimationSink(dst, 10*time.Second)
	if err != nil {
		t.Fatalf("NewDecimationSink: %v", err)
	}
	obs := []SignalObservation{
		{T: mustTime(t, "2025-07-01T00:00:01.0000000"), Sat: "G03", Sig: "1C", SignalValues: SignalValues{LLI: opt.Make(LLILostLock)}},
		{T: mustTime(t, "2025-07-01T00:00:02.0000000"), Sat: "G03", Sig: "1C", SignalValues: SignalValues{LLI: opt.Make(LLIHalfCycleAmbiguity)}},
		{T: mustTime(t, "2025-07-01T00:00:03.0000000"), Sat: "G03", Sig: "2S", SignalValues: SignalValues{LLI: opt.Make(LLILostLock)}},
		{T: mustTime(t, "2025-07-01T00:00:10.0000000"), Sat: "G03", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
		{T: mustTime(t, "2025-07-01T00:00:10.0000000"), Sat: "G03", Sig: "2S", SignalValues: SignalValues{PR: opt.Make(2.0), LLI: opt.Make(LLIBOCTracking)}},
	}
	for _, o := range obs {
		if err := sink.Observation(o); err != nil {
			t.Fatalf("Observation: %v", err)
		}
	}
	if len(dst.obs) != 2 {
		t.Fatalf("len obs = %d, want 2: %#v", len(dst.obs), dst.obs)
	}
	if !dst.obs[0].LLI.IsSet() || dst.obs[0].LLI.Get() != LLILostLock|LLIHalfCycleAmbiguity {
		t.Errorf("G03 1C LLI = %v, want 3", dst.obs[0].LLI)
	}
	if !dst.obs[1].LLI.IsSet() || dst.obs[1].LLI.Get() != LLILostLock|LLIBOCTracking {
		t.Errorf("G03 2S LLI = %v, want 5", dst.obs[1].LLI)
	}
}
