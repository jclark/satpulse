package serialpps

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

// settleCapture records the window attribute of the "serial PPS settled"
// debug line, so tests can check where in the descent the latch fired. Read
// it only after Poll has returned.
type settleCapture struct {
	slog.Handler
	window time.Duration
}

func (h *settleCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *settleCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "serial PPS settled" {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "window" {
				if d, ok := a.Value.Any().(time.Duration); ok {
					h.window = d
				}
			}
			return true
		})
	}
	return nil
}

// fakePulse simulates a receiver pulsing at 1 Hz from epoch on, observed
// through a modem-state query that blocks for callDur. The pin reads
// deasserted (in pulse) for width after each pulse's leading edge. Pulses
// with index in [offFrom, offTo) are suppressed (offTo 0 means none), and
// every lateEvery-th pulse is delivered late by late (lateEvery 0 means
// none), modelling a delivery tail. A nonzero wakeJitter delays any query
// that follows an idle gap, alternating between the full amount and an
// eighth of it, modelling the sleep overshoot observed inside the daemon:
// queries after a sleep run late by a varying amount, back-to-back queries
// do not. A nonzero stall delays the single first query at or after
// stallAfter (relative to epoch) by that much, stretching one bracket --
// the noise event that made a latch comparing consecutive brackets misfire
// in the daemon. A nonzero slowCallDur replaces callDur from slowFrom until
// slowTo, modelling a transient run of slow queries. A nonzero stateRefresh
// exposes pulse-state changes only on that time grid, and edgeCallDur replaces
// callDur for the one query that first observes each leading edge, modelling a
// coarse status-delivery bracket around otherwise fast cached queries. calls
// counts the state queries.
type fakePulse struct {
	epoch          time.Time
	width          time.Duration
	callDur        time.Duration
	offFrom, offTo int
	lateEvery      int
	late           time.Duration
	wakeJitter     time.Duration
	stallAfter     time.Duration
	stall          time.Duration
	slowFrom       time.Duration
	slowTo         time.Duration
	slowCallDur    time.Duration
	stateRefresh   time.Duration
	edgeCallDur    time.Duration
	stalled        bool
	haveState      bool
	lastState      gpsio.ModemControlPinState
	lastEnd        time.Time
	seq            uint32
	calls          atomic.Int64
}

func (f *fakePulse) ModemControlPinState() (gpsio.ModemControlPinState, error) {
	f.calls.Add(1)
	if f.wakeJitter > 0 && !f.lastEnd.IsZero() && time.Since(f.lastEnd) > 0 {
		if f.seq++; f.seq%2 == 0 {
			time.Sleep(f.wakeJitter)
		} else {
			time.Sleep(f.wakeJitter / 8)
		}
	}
	if f.stall > 0 && !f.stalled && time.Since(f.epoch) >= f.stallAfter {
		f.stalled = true
		time.Sleep(f.stall)
	}
	defer func() { f.lastEnd = time.Now() }()
	callDur := f.callDur
	since := time.Since(f.epoch)
	if f.slowCallDur > 0 && since >= f.slowFrom && since < f.slowTo {
		callDur = f.slowCallDur
	}
	if state := f.state(since); f.edgeCallDur > 0 && f.haveState &&
		f.lastState.Asserted(gpsio.ModemCTS) && !state.Asserted(gpsio.ModemCTS) {
		callDur = f.edgeCallDur
	}
	time.Sleep(callDur)
	state := f.state(time.Since(f.epoch))
	f.lastState = state
	f.haveState = true
	return state, nil
}

func (f *fakePulse) state(since time.Duration) gpsio.ModemControlPinState {
	if f.stateRefresh > 0 && since >= 0 {
		since = since.Truncate(f.stateRefresh)
	}
	n := int(since / period)
	off := since % period
	if f.lateEvery > 0 && n%f.lateEvery == 0 {
		off -= f.late
	}
	if since >= 0 && off >= 0 && off < f.width && !(f.offTo > 0 && n >= f.offFrom && n < f.offTo) {
		return 0
	}
	return gpsio.ModemControlPinState(1 << gpsio.ModemCTS)
}

func TestPoll(t *testing.T) {
	tests := []struct {
		name             string
		epochOffset      time.Duration // pulse 0's leading edge relative to start
		callDur          time.Duration
		expectFirstPulse int // settling length bounds, in pulses
		expectLastPulse  int
		expectTol        time.Duration // per-edge timestamp error bound
	}{
		{name: "slow query (FT232R class)", epochOffset: 350 * time.Millisecond, callDur: 2 * time.Millisecond,
			expectFirstPulse: 3, expectLastPulse: 12, expectTol: 3 * time.Millisecond},
		{name: "fast query (spacing floor binds)", epochOffset: 350 * time.Millisecond, callDur: 20 * time.Microsecond,
			expectFirstPulse: 9, expectLastPulse: 18, expectTol: 100 * time.Microsecond},
		{name: "cold start inside pulse", epochOffset: -20 * time.Millisecond, callDur: 20 * time.Microsecond,
			expectFirstPulse: 9, expectLastPulse: 18, expectTol: 100 * time.Microsecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runBubble(t, func(t *testing.T) {
				f := &fakePulse{epoch: time.Now().Add(tc.epochOffset), width: 100 * time.Millisecond, callDur: tc.callDur}
				ctx, cancel := context.WithCancel(context.Background())
				candidates := make(chan CandidateEdge)
				errCh := make(chan error, 1)
				go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
				var got []CandidateEdge
				sawUnsettled := false
				for len(got) < 3 {
					candidate := <-candidates
					if !candidate.Settled {
						sawUnsettled = true
						continue
					}
					got = append(got, candidate)
				}
				cancel()
				if err := <-errCh; err != context.Canceled {
					t.Fatalf("Poll error = %v, want context.Canceled", err)
				}
				if !sawUnsettled {
					t.Error("Poll did not report any candidates before settling")
				}
				for i, e := range got {
					if e.Uncertainty <= 0 {
						t.Errorf("candidate %d uncertainty = %v, want positive", i, e.Uncertainty)
					}
					if !e.TRead.After(e.Timestamp) {
						t.Errorf("candidate %d read time %v is not after timestamp %v", i, e.TRead, e.Timestamp)
					}
					since := e.Timestamp.Sub(f.epoch)
					pulse := pulseIndex(e.Timestamp, f.epoch)
					if err := since - time.Duration(pulse)*period; err < -tc.expectTol || err > tc.expectTol {
						t.Errorf("edge %d at %v: error %v from pulse %d, want within %v", i, e.Timestamp, err, pulse, tc.expectTol)
					}
					if i == 0 && (pulse < tc.expectFirstPulse || pulse > tc.expectLastPulse) {
						t.Errorf("first published edge is pulse %d, want settling to end between pulses %d and %d",
							pulse, tc.expectFirstPulse, tc.expectLastPulse)
					}
					if i > 0 {
						d := e.Timestamp.Sub(got[i-1].Timestamp)
						if d < period-2*tc.expectTol || d > period+2*tc.expectTol {
							t.Errorf("edge %d follows edge %d by %v, want ~%v", i, i-1, d, period)
						}
					}
				}
			})
		})
	}
}

// TestPollSettlesWithCoarseStateRefresh exercises the former fixed point: the
// ordinary cached query takes only 5 us, but a state refresh stretches each
// catching bracket to about 2 ms. The old window-driven acquisition stalled
// above minSpacing while every caught window remained sleep-paced.
func TestPollSettlesWithCoarseStateRefresh(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{
			epoch:        time.Now().Add(350 * time.Millisecond),
			width:        100 * time.Millisecond,
			callDur:      5 * time.Microsecond,
			stateRefresh: 2 * time.Millisecond,
			edgeCallDur:  4 * time.Millisecond,
		}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		deadline := time.After(20 * period)
		settled := 0
		timedOut := false
		for settled < 3 && !timedOut {
			select {
			case candidate := <-candidates:
				if candidate.Settled {
					settled++
				}
			case <-deadline:
				timedOut = true
			}
		}
		cancel()
		if err := <-errCh; err != context.Canceled {
			t.Fatalf("Poll error = %v, want context.Canceled", err)
		}
		if timedOut {
			t.Fatal("Poll did not settle with coarse modem-state refreshes")
		}
	})
}

func TestPollMissedPulseKeepsLatch(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 17}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		var logs bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
		go func() { errCh <- Poll(ctx, lg, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		seen := make(map[int]bool)
		for pulse := 0; pulse < 18; {
			pulse = pulseIndex(nextSettled(candidates).Timestamp, f.epoch)
			seen[pulse] = true
		}
		cancel()
		<-errCh
		if seen[16] {
			t.Error("edge published for suppressed pulse 16")
		}
		if !seen[15] || !seen[17] {
			t.Errorf("pulses seen = %v, want 15 and 17 published around the missed pulse", seen)
		}
		status := logs.String()
		if !strings.Contains(status, `msg="serial PPS track status" reason=miss`) {
			t.Errorf("logs %q do not report the missed pulse at info level", status)
		}
		for _, field := range []string{"polls=", "reduction=", "shrinkAfter=", "spacing=", "window=", "bracket=", "misses=1"} {
			if !strings.Contains(status, field) {
				t.Errorf("track status %q does not contain %q", status, field)
			}
		}
	})
}

func TestPollOutageResettles(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 31}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		var first int
		for first <= 15 {
			first = pulseIndex(nextSettled(candidates).Timestamp, f.epoch)
		}
		cancel()
		<-errCh
		if first < f.offTo+9 || first > f.offTo+18 {
			t.Errorf("first edge after outage is pulse %d, want a fresh settle between pulses %d and %d",
				first, f.offTo+9, f.offTo+18)
		}
	})
}

// TestPollShrinksToFloor checks that proportional startup reductions bring
// tracking down to a handful of state queries per pulse within a few minutes.
func TestPollShrinksToFloor(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		for pulseIndex(nextSettled(candidates).Timestamp, f.epoch) < 200 {
		}
		start := f.calls.Load()
		for i := 0; i < 50; i++ {
			nextSettled(candidates)
		}
		perPulse := (f.calls.Load() - start) / 50
		cancel()
		<-errCh
		if perPulse > 6 {
			t.Errorf("steady state costs %d queries per pulse, want at most 6", perPulse)
		}
	})
}

// TestPollLearnsDeliveryTail checks that a recurring 1 ms delivery delay,
// which the settled window is initially shrunk too far to cover, is learned
// as equilibrium growth: after the window has grown back, nearly every pulse
// is caught again.
func TestPollLearnsDeliveryTail(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 100 * time.Microsecond, lateEvery: 5, late: time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		seen := make(map[int]bool)
		for last := 0; last < 500; {
			last = pulseIndex(nextSettled(candidates).Timestamp, f.epoch)
			seen[last] = true
		}
		cancel()
		<-errCh
		missed := 0
		for p := 400; p < 500; p++ {
			if !seen[p] {
				missed++
			}
		}
		if missed > 5 {
			t.Errorf("%d of pulses 400-499 missed, want the window grown to cover the delivery tail", missed)
		}
	})
}

// TestPollSettlesDespiteSleepJitter reproduces the daemon's sleep-overshoot
// regime: wakeups after an idle gap run up to ~0.9 ms late, and one poll
// mid-settling stalls outright, stretching its bracket -- the noise the
// former bracket-comparison latch settled on, publishing millisecond-class
// samples from a still-wide window. Settling must ignore bracket noise and
// wait until the queries pace the loop, where the jitter vanishes and
// edges are located to the query time. The stall is timed to hit the
// bracket of the pulse-4 catch, mid-halving.
func TestPollSettlesDespiteSleepJitter(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 100 * time.Microsecond, wakeJitter: 900 * time.Microsecond,
			stallAfter: 3999 * time.Millisecond, stall: 3 * time.Millisecond}
		capture := &settleCapture{Handler: slog.DiscardHandler}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, slog.New(capture), f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		var got []CandidateEdge
		for len(got) < 20 {
			got = append(got, nextSettled(candidates))
		}
		cancel()
		<-errCh
		if first := pulseIndex(got[0].Timestamp, f.epoch); first > 15 {
			t.Errorf("first edge published at pulse %d, want settling despite the jitter plateau", first)
		}
		// Settling in the jitter plateau leaves the window at 15.625ms or
		// wider; the query-paced floor is reached at 3.9ms.
		if capture.window == 0 || capture.window > 8*time.Millisecond {
			t.Errorf("settled at window %v, want the latch to hold out until the queries pace the loop", capture.window)
		}
		for i, e := range got {
			pulse := pulseIndex(e.Timestamp, f.epoch)
			if i > 0 {
				prev := pulseIndex(got[i-1].Timestamp, f.epoch)
				if pulse > prev+2 {
					t.Errorf("edge %d is pulse %d after pulse %d, want convergence misses to be isolated", i, pulse, prev)
				}
			}
			if err := e.Timestamp.Sub(f.epoch) - time.Duration(pulse)*period; err < -500*time.Microsecond || err > 500*time.Microsecond {
				t.Errorf("edge %d at pulse %d: error %v, want within 500µs of the query-time floor", i, pulse, err)
			}
		}
	})
}

// TestPollConfirmsQueryPacing checks that a single query slowdown does not
// open the publishing gate. The slowdown covers the catch at the 15.625 ms
// window, where its 400 us queries outlast the 244 us target. Normal 20 us
// queries resume at the next pulse, so settling must continue until the
// 50 us spacing floor is reached.
func TestPollConfirmsQueryPacing(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{
			epoch:       time.Now().Add(350 * time.Millisecond),
			width:       100 * time.Millisecond,
			callDur:     20 * time.Microsecond,
			slowFrom:    6*time.Second - 10*time.Millisecond,
			slowTo:      6*time.Second + 10*time.Millisecond,
			slowCallDur: 400 * time.Microsecond,
		}
		capture := &settleCapture{Handler: slog.DiscardHandler}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, slog.New(capture), f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		for range 3 {
			nextSettled(candidates)
		}
		cancel()
		<-errCh
		if capture.window == 0 || capture.window >= 15*time.Millisecond {
			t.Errorf("settled at window %v, want the one-window query slowdown suppressed", capture.window)
		}
	})
}

// TestPollNarrowPulse checks that a pulse narrower than the cold-start
// spacing (Septentrio's 5 ms default) is acquired by the phase sweep at the
// cap and then tracked normally, since the settled spacing is below the
// width.
func TestPollNarrowPulse(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 5 * time.Millisecond,
			callDur: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		var got []CandidateEdge
		for len(got) < 3 {
			got = append(got, nextSettled(candidates))
		}
		cancel()
		<-errCh
		if first := pulseIndex(got[0].Timestamp, f.epoch); first > 40 {
			t.Errorf("first edge published at pulse %d, want acquisition well before pulse 40", first)
		}
		for i, e := range got {
			pulse := pulseIndex(e.Timestamp, f.epoch)
			if err := e.Timestamp.Sub(f.epoch) - time.Duration(pulse)*period; err < -3*time.Millisecond || err > 3*time.Millisecond {
				t.Errorf("edge %d at %v: error %v from pulse %d, want within 3ms", i, e.Timestamp, err, pulse)
			}
		}
	})
}

func TestClassify(t *testing.T) {
	base := time.Unix(1_000, 0)
	asserted := gpsio.ModemControlPinState(1 << gpsio.ModemCTS)
	tests := []struct {
		name       string
		curState   gpsio.ModemControlPinState
		curAt      time.Duration
		deadline   time.Duration
		wantEdgeAt time.Duration
		wantMissed bool
	}{
		{
			name:       "transition before deadline",
			curAt:      4 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantEdgeAt: 2 * time.Millisecond,
		},
		{
			name:       "transition crossing deadline",
			curAt:      12 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantEdgeAt: 6 * time.Millisecond,
		},
		{
			name:     "no transition before deadline",
			curState: asserted,
			curAt:    4 * time.Millisecond,
			deadline: 5 * time.Millisecond,
		},
		{
			name:       "no transition reaching deadline",
			curState:   asserted,
			curAt:      5 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "no transition crossing deadline",
			curState:   asserted,
			curAt:      6 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "bracket spanning a period",
			curAt:      1100 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantMissed: true,
		},
	}
	// The mono readings are skewed from the stamp readings so a midpoint or a
	// deadline comparison taken from the wrong clock is caught. deadline is
	// on the mono timeline, as Poll's is; the "reaching deadline" case
	// straddles the two, so comparing it against stamp would report a miss
	// one poll late.
	const monoSkew = time.Millisecond
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prevAt := clockReading{stamp: base, mono: base.Add(monoSkew)}
			curAt := clockReading{stamp: base.Add(tc.curAt), mono: base.Add(tc.curAt + monoSkew)}
			prev := reading{state: asserted, poll: poll{start: prevAt, end: prevAt}}
			cur := reading{state: tc.curState, poll: poll{start: curAt, end: curAt}}
			edge, missed := classify(prev, cur, Wiring{Pin: gpsio.ModemCTS}, base.Add(tc.deadline+monoSkew))
			if missed != tc.wantMissed {
				t.Errorf("missed = %v, want %v", missed, tc.wantMissed)
			}
			if tc.wantEdgeAt == 0 {
				if !edge.stamp.IsZero() {
					t.Errorf("edge = %v, want zero", edge)
				}
			} else if want := base.Add(tc.wantEdgeAt); !edge.stamp.Equal(want) {
				t.Errorf("edge stamp = %v, want %v", edge.stamp, want)
			} else if !edge.mono.Equal(want.Add(monoSkew)) {
				t.Errorf("edge mono = %v, want %v", edge.mono, want.Add(monoSkew))
			}
		})
	}
}

func TestHalfCeil(t *testing.T) {
	for d, want := range map[time.Duration]time.Duration{
		4 * time.Nanosecond: 2 * time.Nanosecond,
		5 * time.Nanosecond: 3 * time.Nanosecond,
	} {
		if got := halfCeil(d); got != want {
			t.Errorf("halfCeil(%v) = %v, want %v", d, got, want)
		}
	}
}

func TestClockReadingElapsedSinceUsesStamp(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	start := clockReading{stamp: base, mono: base}
	end := clockReading{stamp: base.Add(2 * time.Millisecond), mono: base.Add(10 * time.Millisecond)}
	if got := end.elapsedSince(start); got != 2*time.Millisecond {
		t.Errorf("elapsedSince = %v, want 2ms from stamp readings", got)
	}
}

type errPin struct{ err error }

func (p errPin) ModemControlPinState() (gpsio.ModemControlPinState, error) { return 0, p.err }

func TestPollReaderError(t *testing.T) {
	e := errors.New("query failed")
	if err := Poll(context.Background(), testLog, errPin{err: e}, Wiring{Pin: gpsio.ModemCTS}, nil, nil); err != e {
		t.Fatalf("Poll error = %v, want %v", err, e)
	}
}

// pulseIndex is the index of the pulse nearest t, counting from epoch.
func pulseIndex(t, epoch time.Time) int {
	return int((t.Sub(epoch) + period/2) / period)
}

func nextSettled(candidates <-chan CandidateEdge) CandidateEdge {
	for {
		candidate := <-candidates
		if candidate.Settled {
			return candidate
		}
	}
}
