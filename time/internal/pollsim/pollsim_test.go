package pollsim

import (
	"log/slog"
	"testing"
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
// stalls placed where the loop is exposed: one inside the bracket around an
// edge, one long enough in the pre-warm busy-wait to push the window open
// past the pulse, and one inside the window that stretches the catching
// bracket to tens of milliseconds. Caught stalls are flagged as anomalous
// but still correct prediction, changing where later stalls land. Nothing
// wrong is forwarded, and forwarding is back within a few seconds.
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
	if st.LongestGap > 5 || st.Lost != 0 {
		t.Errorf("longestGap = %v s lost = %d, want forwarding back within a few seconds without reacquisition", st.LongestGap, st.Lost)
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
