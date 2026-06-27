package phcsample

import (
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/lib/ntime"
	"github.com/jclark/satpulse/time/phctime"
)

// The simulated timelines: pulse k occurs at TAI taiBase+k seconds, at
// monotonic time monoBaseTime+k seconds, when the free-running PHC
// reads phcBase+k seconds. The PHC is read readLag after the edge, and
// the time message for second k is read msgDelay after the pulse.
const (
	taiBase = ptime.Time(1_800_000_000 * int64(time.Second))
	phcBase = ptime.Time(1000 * int64(time.Second))
)

const (
	readLag  = time.Millisecond
	msgDelay = 150 * time.Millisecond // matches DefaultConfig().ExpectedDelay
)

type recordingSampler struct {
	phcs    []ntime.Time
	offsets []float64
	leaps   []ptime.LeapSecondKind
}

func (r *recordingSampler) PHCSample(phc ntime.Time, offset float64, leap ptime.LeapSecondKind) {
	r.phcs = append(r.phcs, phc)
	r.offsets = append(r.offsets, offset)
	r.leaps = append(r.leaps, leap)
}

func testGenerator(cfg Config) (*Generator, *recordingSampler) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := NewGenerator(cfg, 1, lg)
	rec := &recordingSampler{}
	g.SetSampler(rec)
	return g, rec
}

func pulseAt(g *Generator, k int) {
	phc := phcBase.Add(time.Duration(k) * time.Second)
	g.Pulse(
		phctime.Time{T: phc},
		phctime.Sample{
			PHC: phctime.Time{T: phc.Add(readLag)},
			Sys: monoBaseTime.Add(time.Duration(k)*time.Second + readLag),
		},
	)
}

func msgAt(g *Generator, k int, taiShift time.Duration, leap ptime.LeapSecondKind) {
	g.MsgTAITime(
		taiBase.Add(time.Duration(k)*time.Second+taiShift),
		monoBaseTime.Add(time.Duration(k)*time.Second+msgDelay),
		leap,
	)
}

func TestGeneratorEmitsPerEdge(t *testing.T) {
	g, rec := testGenerator(DefaultConfig())
	// Warm-up: nothing may be emitted until the wallClock fit spans
	// MinMsgSpan (2.9 s), which happens at the message for second 3.
	for k := range 3 {
		pulseAt(g, k)
		msgAt(g, k, 0, ptime.LeapSecondNone)
		if len(rec.phcs) != 0 {
			t.Fatalf("emitted %d samples during warm-up at second %d", len(rec.phcs), k)
		}
	}
	pulseAt(g, 3)
	msgAt(g, 3, 0, ptime.LeapSecondNone)
	// All four buffered edges become labellable at once.
	wantOffset := taiBase.Sub(phcBase).Seconds()
	expectPhcs := []ntime.Time{
		ntime.Time(phcBase),
		ntime.Time(phcBase.Add(1 * time.Second)),
		ntime.Time(phcBase.Add(2 * time.Second)),
		ntime.Time(phcBase.Add(3 * time.Second)),
	}
	if !reflect.DeepEqual(rec.phcs, expectPhcs) {
		t.Fatalf("phcs after warm-up:\ngot  %v\nwant %v", rec.phcs, expectPhcs)
	}
	for i, off := range rec.offsets {
		if off != wantOffset {
			t.Errorf("offset[%d] = %v, want %v", i, off, wantOffset)
		}
	}
	// Steady state: each new edge is emitted exactly once, directly at
	// the pulse (the fit extrapolates forward within MaxMsgGap), and
	// never re-emitted when the message for its second arrives.
	pulseAt(g, 4)
	if len(rec.phcs) != 5 {
		t.Fatalf("emitted %d samples after pulse 4, want 5", len(rec.phcs))
	}
	if rec.phcs[4] != ntime.Time(phcBase.Add(4*time.Second)) || rec.offsets[4] != wantOffset {
		t.Errorf("sample 4 = (%v, %v), want (%v, %v)", rec.phcs[4], rec.offsets[4], ntime.Time(phcBase.Add(4*time.Second)), wantOffset)
	}
	msgAt(g, 4, 0, ptime.LeapSecondPositive)
	if len(rec.phcs) != 5 {
		t.Fatalf("edge 4 re-emitted after its message arrived")
	}
	// The leap state delivered with the messages rides along on the
	// next emission.
	pulseAt(g, 5)
	if len(rec.phcs) != 6 {
		t.Fatalf("emitted %d samples after pulse 5, want 6", len(rec.phcs))
	}
	if rec.leaps[5] != ptime.LeapSecondPositive {
		t.Errorf("leap[5] = %v, want positive", rec.leaps[5])
	}
}

func TestGeneratorDropsEdgesFarFromSecond(t *testing.T) {
	g, rec := testGenerator(DefaultConfig())
	// Shift all message labels by 450 ms: every edge's predicted label
	// is then 450 ms from an integer second, beyond the default
	// EdgeSecondTolerance of 0.4, so nothing may be emitted.
	for k := range 6 {
		pulseAt(g, k)
		msgAt(g, k, 450*time.Millisecond, ptime.LeapSecondNone)
	}
	if len(rec.phcs) != 0 {
		t.Fatalf("emitted %d samples from edges far from integer seconds", len(rec.phcs))
	}
}

type fixedCorrector struct {
	corr float64
}

func (c fixedCorrector) GetTAIPulseCorrection(_ ptime.Time) (float64, bool) {
	return c.corr, true
}

func TestGeneratorSawtoothCorrection(t *testing.T) {
	tests := []struct {
		name   string
		ignore bool
		corr   float64 // true-time ns
		expect float64 // expected offset delta vs uncorrected, in seconds
	}{
		{name: "positive correction", corr: 12.5, expect: -12.5e-9},
		{name: "negative correction", corr: -8.25, expect: 8.25e-9},
		{name: "ignored", ignore: true, corr: 12.5, expect: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.IgnoreSawtoothCorrection = tc.ignore
			g, rec := testGenerator(cfg)
			g.SetPulseCorrector(fixedCorrector{corr: tc.corr})
			for k := range 4 {
				pulseAt(g, k)
				msgAt(g, k, 0, ptime.LeapSecondNone)
			}
			if len(rec.offsets) == 0 {
				t.Fatalf("no samples emitted")
			}
			want := taiBase.Sub(phcBase).Seconds() + tc.expect
			if got := rec.offsets[0]; got != want {
				t.Errorf("offset = %v, want %v", got, want)
			}
		})
	}
}

func TestGeneratorNewInstanceKeepsWiring(t *testing.T) {
	g, rec := testGenerator(DefaultConfig())
	g.SetPulseCorrector(fixedCorrector{})
	n := g.NewInstance()
	if n.sampler != Sampler(rec) || n.pc == nil || n.edgesPerPulse != g.edgesPerPulse {
		t.Errorf("NewInstance lost wiring: sampler %v pc %v edgesPerPulse %d", n.sampler, n.pc, n.edgesPerPulse)
	}
	if len(n.wc.points) != 0 || len(n.win.buf) != 0 {
		t.Errorf("NewInstance kept state: %d points, %d edges", len(n.wc.points), len(n.win.buf))
	}
}
