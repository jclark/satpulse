package pollsim

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

var testLog = slog.New(slog.DiscardHandler)

// TestSimulateQuiet checks the default host: after acquisition nearly every
// pulse is forwarded at query resolution, no forwarded edge lies outside
// its bracket, and tracking is never lost.
func TestSimulateQuiet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sim.Duration = 300
	st, err := Simulate(cfg, testLog, func(e EdgeRecord) {
		if e.Err < -e.Uncertainty[1] || e.Err > e.Uncertainty[0] {
			t.Errorf("edge error %g outside uncertainty %v", e.Err, e.Uncertainty)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(st)
	if st.Lost != 0 || st.Acquisitions != 1 {
		t.Errorf("acquisitions = %d lost = %d, want one acquisition and no loss", st.Acquisitions, st.Lost)
	}
	if st.Forwarded < st.Pulses-15 {
		t.Errorf("forwarded %d of %d pulses, want all but acquisition and a few misses", st.Forwarded, st.Pulses)
	}
	if st.Wrong != 0 {
		t.Errorf("%d forwarded edges wrong by more than the limit, want none", st.Wrong)
	}
	if st.ErrP90 > 200e-6 || st.LongestGap > 3 {
		t.Errorf("errP90 = %v longestGap = %v, want query resolution and no gap over 3 s", st.ErrP90, st.LongestGap)
	}
	if st.Queries > 20*int(cfg.Sim.Duration) {
		t.Errorf("%d queries over %g s, want a few per second", st.Queries, cfg.Sim.Duration)
	}
}

// TestSimulateStalls replays the shape of the 2026-09-20 incident with
// stalls placed where the loop is exposed. The first two cause missed
// windows; the third stretches the catching bracket to tens of milliseconds.
// That catch is anomalous and too wide to correct prediction. Nothing wrong
// is forwarded, and the next pulse after the last stall is caught normally.
func TestSimulateStalls(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sim.Duration = 180
	cfg.Poll.PreWarm = 0.05
	cfg.Fault.Stall = []StallConfig{{At: 120.9998, Duration: 2.4e-3}, {At: 122.98, Duration: 90e-3}, {At: 123.9999, Duration: 50e-3}}
	st, err := Simulate(cfg, testLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(st)
	if st.Anomalous != 1 || st.TrackMisses != 2 {
		t.Errorf("anomalous = %d trackMisses = %d, want 1 and 2", st.Anomalous, st.TrackMisses)
	}
	if st.Wrong != 0 {
		t.Errorf("%d forwarded edges wrong by more than the limit, want none", st.Wrong)
	}
	if st.LongestGap > 3.001 || st.Lost != 0 {
		t.Errorf("longestGap = %v s lost = %d, want a gap of at most three pulses without reacquisition", st.LongestGap, st.Lost)
	}
}

// TestSimulateOutage checks that a long outage returns the loop to
// acquisition and that forwarding resumes soon after the pulse does.
func TestSimulateOutage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sim.Duration = 200
	cfg.Fault.Outage = []OutageConfig{{Start: 100, Duration: 30}}
	st, err := Simulate(cfg, testLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(st)
	if st.Lost != 1 || st.Acquisitions != 2 {
		t.Errorf("lost = %d acquisitions = %d, want one loss and a reacquisition", st.Lost, st.Acquisitions)
	}
	if st.LongestGap > 45 {
		t.Errorf("longest gap = %v s, want forwarding back within 15 s of the pulse returning", st.LongestGap)
	}
	if st.Wrong != 0 {
		t.Errorf("%d forwarded edges wrong by more than the limit, want none", st.Wrong)
	}
}

// TestSimulateLinuxUART models a fast UART on Linux: 10 us queries and
// timer sleeps truncated to a millisecond, so the loop is query-paced.
func TestSimulateLinuxUART(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sim.Duration = 300
	cfg.Host.Query = QueryConfig{Duration: 10e-6, Jitter: 2e-6, Idle: IdleConfig{Factor: 1}}
	cfg.Host.Timer = TimerConfig{Resolution: 1e-3, Overshoot: 50e-6, OvershootJitter: 30e-6}
	st, err := Simulate(cfg, testLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(st)
	if st.Lost != 0 || st.Wrong != 0 {
		t.Errorf("lost = %d wrong = %d, want 0 and 0", st.Lost, st.Wrong)
	}
	if st.ErrP90 > 50e-6 {
		t.Errorf("errP90 = %v, want query resolution", st.ErrP90)
	}
}

func TestPulseOnJitter(t *testing.T) {
	s := newSim(DefaultConfig())
	s.jitter[0], s.jitter[1], s.jitter[2] = -2*time.Millisecond, -2*time.Millisecond, 2*time.Millisecond
	for _, tc := range []struct {
		at time.Duration
		on bool
	}{
		{0, true},
		{98 * time.Millisecond, false},
		{998*time.Millisecond - 1, false},
		{998 * time.Millisecond, true},
		{1098*time.Millisecond - 1, true},
		{1098 * time.Millisecond, false},
		{2 * time.Second, false},
		{2002 * time.Millisecond, true},
		{2102 * time.Millisecond, false},
	} {
		if got := s.pulseOn(tc.at); got != tc.on {
			t.Errorf("pulseOn(%v) = %v, want %v", tc.at, got, tc.on)
		}
	}
}

func TestSimulateJitter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sim.Duration = 60
	cfg.Pulse.Jitter = 2e-3
	cfg.Host.Query = QueryConfig{Duration: 10e-6, Idle: IdleConfig{Factor: 1}}
	st, err := Simulate(cfg, testLog, func(e EdgeRecord) {
		if e.Err < -e.Uncertainty[1] || e.Err > e.Uncertainty[0] {
			t.Errorf("edge at %g: error %g outside uncertainty %v", e.T, e.Err, e.Uncertainty)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Forwarded == 0 || st.Wrong != 0 {
		t.Errorf("forwarded = %d wrong = %d, want forwarded edges and none wrong", st.Forwarded, st.Wrong)
	}
}

func TestOverlappingStalls(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stalls []StallConfig
		end    time.Duration
	}{
		{"overlap", []StallConfig{{Duration: .010}, {At: .005, Duration: .010}}, 15 * time.Millisecond},
		{"contained", []StallConfig{{Duration: .010}, {At: .005, Duration: .002}}, 10 * time.Millisecond},
		{"adjacent", []StallConfig{{Duration: .010}, {At: .010, Duration: .005}}, 15 * time.Millisecond},
		{"chain", []StallConfig{{Duration: .010}, {At: .005, Duration: .010}, {At: .012, Duration: .008}}, 20 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Fault.Stall = tc.stalls
			s := newSim(cfg)
			s.run(time.Millisecond)
			if want := tc.end + time.Millisecond; s.now != want {
				t.Errorf("run: now = %v, want %v", s.now, want)
			}
			if s.work != time.Millisecond {
				t.Errorf("work = %v, want 1 ms: stalls do not consume CPU", s.work)
			}
			s = newSim(cfg)
			if _, err := s.wait(context.Background(), simBase.Add(time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			if s.now != tc.end || s.work != 0 {
				t.Errorf("wait: now = %v work = %v, want %v and 0", s.now, s.work, tc.end)
			}
		})
	}
}

func TestFaultBursts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sim.Duration = 5
	cfg.Fault.Stalls = []StallBurst{{Start: 1, Duration: 2, Rate: 20, Min: .001, Max: .010}}
	a := newSim(cfg)
	cfg.Fault.Stalls = nil
	cfg.Fault.Slows = []SlowBurst{{Start: 1, Duration: 2, Rate: 20, Min: .001, Max: .010, Factor: 3}}
	b := newSim(cfg)
	if len(a.stalls) == 0 || len(a.stalls) != len(b.slows) {
		t.Fatalf("stalls = %d slows = %d, want equal nonzero counts", len(a.stalls), len(b.slows))
	}
	for i, st := range a.stalls {
		if st.at < time.Second || st.at >= 3*time.Second || st.dur < time.Millisecond || st.dur > 10*time.Millisecond {
			t.Errorf("stall %d outside configured bounds: %+v", i, st)
		}
		if sl := b.slows[i]; sl.from != st.at || sl.to != st.at+st.dur || sl.factor != 3 {
			t.Errorf("slow %d = %+v, want same timing as stall %+v and factor 3", i, sl, st)
		}
	}
	cfg.Fault.Slows[0].Factor = 1
	if s := newSim(cfg); len(s.slows) != 0 {
		t.Error("factor 1 generated slow periods")
	}
}

func BenchmarkSimulatePreWarm(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Sim.Duration = 20
	cfg.Poll.PreWarm = .05
	cfg.Host.Query.Idle = IdleConfig{After: .02, Factor: 4, Recover: .04}
	for range b.N {
		if _, err := Simulate(cfg, testLog, nil); err != nil {
			b.Fatal(err)
		}
	}
}
