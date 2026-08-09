package serialpps

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

var testLog = slog.New(slog.DiscardHandler)

func TestClassifyReading(t *testing.T) {
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
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := reading{state: asserted, at: base}
			cur := reading{state: tc.curState, at: base.Add(tc.curAt)}
			edge, missed := classifyReading(prev, cur, Wiring{Pin: gpsio.ModemCTS}, base.Add(tc.deadline))
			if missed != tc.wantMissed {
				t.Errorf("missed = %v, want %v", missed, tc.wantMissed)
			}
			if tc.wantEdgeAt == 0 {
				if !edge.IsZero() {
					t.Errorf("edge = %v, want zero", edge)
				}
			} else if want := base.Add(tc.wantEdgeAt); !edge.Equal(want) {
				t.Errorf("edge = %v, want %v", edge, want)
			}
		})
	}
}

func TestGenerator(t *testing.T) {
	msgUTC := time.Unix(1_000, 0).UTC()
	msgRead := time.Unix(900, 125_000_000)
	edge := time.Unix(900, 1_000_000)
	tests := []struct {
		name string
		msg  bool
		age  time.Duration
		leap ptime.LeapSecondKind
		ok   bool
	}{
		{name: "identifies second", msg: true, leap: ptime.LeapSecondNone, ok: true},
		{name: "positive leap passthrough", msg: true, leap: ptime.LeapSecondPositive, ok: true},
		{name: "negative leap passthrough", msg: true, leap: ptime.LeapSecondNegative, ok: true},
		{name: "message exactly three seconds old", msg: true, age: 3 * time.Second, ok: true},
		{name: "stale message", msg: true, age: 3*time.Second + time.Nanosecond},
		{name: "no message"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator()
			tEdge := edge
			utc := msgUTC
			read := msgRead
			if tc.age != 0 {
				read = tEdge.Add(-tc.age)
				utc = read.Add(msgUTC.Sub(msgRead))
			}
			if tc.msg {
				g.MsgUTCTime(utc, read, tc.leap)
			}
			sample, ok := g.Edge(Edge{T: tEdge})
			if ok != tc.ok {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			wantRef := time.Unix(1_000, 0).UTC()
			if !sample.Reference.Equal(wantRef) {
				t.Errorf("reference = %v, want %v", sample.Reference, wantRef)
			}
			if !sample.System.Equal(tEdge) {
				t.Errorf("system = %v, want %v", sample.System, tEdge)
			}
			if sample.Leap != tc.leap {
				t.Errorf("leap = %v, want %v", sample.Leap, tc.leap)
			}
		})
	}
}

func TestGeneratorKeepsNewestMessage(t *testing.T) {
	g := NewGenerator()
	newRead := time.Unix(100, 100_000_000)
	g.MsgUTCTime(time.Unix(200, 0), newRead, ptime.LeapSecondPositive)
	g.MsgUTCTime(time.Unix(300, 0), newRead.Add(-time.Second), ptime.LeapSecondNegative)
	sample, ok := g.Edge(Edge{T: time.Unix(100, 0)})
	if !ok {
		t.Fatal("Edge returned no sample")
	}
	if !sample.Reference.Equal(time.Unix(200, 0)) || sample.Leap != ptime.LeapSecondPositive {
		t.Fatalf("sample = %+v, want newest message reference and leap", sample)
	}
}

func TestGeneratorLeapCrossing(t *testing.T) {
	tests := []struct {
		name       string
		utc        time.Time
		leap       ptime.LeapSecondKind
		elapsed    time.Duration // edge.T - tRead
		expectRef  time.Time
		expectLeap ptime.LeapSecondKind
		expectOK   bool
	}{
		{name: "pulse before positive leap", utc: time.Unix(86_398, 500_000_000), leap: ptime.LeapSecondPositive,
			elapsed: 900 * time.Millisecond, expectRef: time.Unix(86_399, 0), expectLeap: ptime.LeapSecondPositive, expectOK: true},
		{name: "inserted second pulse yields no sample", utc: time.Unix(86_399, 500_000_000), leap: ptime.LeapSecondPositive,
			elapsed: 900 * time.Millisecond},
		{name: "first pulse after positive leap", utc: time.Unix(86_399, 500_000_000), leap: ptime.LeapSecondPositive,
			elapsed: 1900 * time.Millisecond, expectRef: time.Unix(86_400, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
		{name: "second pulse after positive leap", utc: time.Unix(86_399, 500_000_000), leap: ptime.LeapSecondPositive,
			elapsed: 2900 * time.Millisecond, expectRef: time.Unix(86_401, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
		{name: "pulse before negative leap", utc: time.Unix(86_397, 500_000_000), leap: ptime.LeapSecondNegative,
			elapsed: 900 * time.Millisecond, expectRef: time.Unix(86_398, 0), expectLeap: ptime.LeapSecondNegative, expectOK: true},
		{name: "first pulse after negative leap", utc: time.Unix(86_398, 500_000_000), leap: ptime.LeapSecondNegative,
			elapsed: 900 * time.Millisecond, expectRef: time.Unix(86_400, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator()
			read := tc.utc.Add(100 * time.Millisecond)
			g.MsgUTCTime(tc.utc, read, tc.leap)
			sample, ok := g.Edge(Edge{T: read.Add(tc.elapsed)})
			if ok != tc.expectOK {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.expectOK)
			}
			if !ok {
				return
			}
			if !sample.Reference.Equal(tc.expectRef) || sample.Leap != tc.expectLeap {
				t.Errorf("sample = %+v, want reference %v and leap %v", sample, tc.expectRef, tc.expectLeap)
			}
		})
	}
}

// fakePulse simulates a receiver pulsing at 1 Hz from epoch on, observed
// through a modem-state query that blocks for callDur. The pin reads
// deasserted (in pulse) for width after each pulse's leading edge. Pulses
// with index in [offFrom, offTo) are suppressed (offTo 0 means none), and
// every lateEvery-th pulse is delivered late by late (lateEvery 0 means
// none), modelling a delivery tail. calls counts the state queries.
type fakePulse struct {
	epoch          time.Time
	width          time.Duration
	callDur        time.Duration
	offFrom, offTo int
	lateEvery      int
	late           time.Duration
	calls          atomic.Int64
}

func (f *fakePulse) ModemControlPinState() (gpsio.ModemControlPinState, error) {
	f.calls.Add(1)
	time.Sleep(f.callDur)
	since := time.Since(f.epoch)
	n := int(since / pulsePeriod)
	off := since % pulsePeriod
	if f.lateEvery > 0 && n%f.lateEvery == 0 {
		off -= f.late
	}
	if since >= 0 && off >= 0 && off < f.width && !(f.offTo > 0 && n >= f.offFrom && n < f.offTo) {
		return 0, nil
	}
	return gpsio.ModemControlPinState(1 << gpsio.ModemCTS), nil
}

// pulseIndex is the index of the pulse nearest t, counting from epoch.
func pulseIndex(t, epoch time.Time) int {
	return int((t.Sub(epoch) + pulsePeriod/2) / pulsePeriod)
}

func TestPoll(t *testing.T) {
	tests := []struct {
		name             string
		epochOffset      time.Duration // pulse 0's leading edge relative to start
		callDur          time.Duration
		expectFirstPulse int           // settling length bounds, in pulses
		expectLastPulse  int
		expectTol        time.Duration // per-edge timestamp error bound
	}{
		{name: "slow query (FT232R class)", epochOffset: 350 * time.Millisecond, callDur: 2 * time.Millisecond,
			expectFirstPulse: 4, expectLastPulse: 12, expectTol: 3 * time.Millisecond},
		{name: "fast query (spacing floor binds)", epochOffset: 350 * time.Millisecond, callDur: 20 * time.Microsecond,
			expectFirstPulse: 9, expectLastPulse: 18, expectTol: 100 * time.Microsecond},
		{name: "cold start inside pulse", epochOffset: -20 * time.Millisecond, callDur: 20 * time.Microsecond,
			expectFirstPulse: 9, expectLastPulse: 18, expectTol: 100 * time.Microsecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := &fakePulse{epoch: time.Now().Add(tc.epochOffset), width: 100 * time.Millisecond, callDur: tc.callDur}
				ctx, cancel := context.WithCancel(context.Background())
				edges := make(chan Edge)
				errCh := make(chan error, 1)
				go func() { errCh <- Poll(ctx, f, Wiring{Pin: gpsio.ModemCTS}, edges, testLog) }()
				var got []Edge
				for len(got) < 3 {
					got = append(got, <-edges)
				}
				cancel()
				if err := <-errCh; err != context.Canceled {
					t.Fatalf("Poll error = %v, want context.Canceled", err)
				}
				for i, e := range got {
					since := e.T.Sub(f.epoch)
					pulse := pulseIndex(e.T, f.epoch)
					if err := since - time.Duration(pulse)*pulsePeriod; err < -tc.expectTol || err > tc.expectTol {
						t.Errorf("edge %d at %v: error %v from pulse %d, want within %v", i, e.T, err, pulse, tc.expectTol)
					}
					if i == 0 && (pulse < tc.expectFirstPulse || pulse > tc.expectLastPulse) {
						t.Errorf("first published edge is pulse %d, want settling to end between pulses %d and %d",
							pulse, tc.expectFirstPulse, tc.expectLastPulse)
					}
					if i > 0 {
						d := e.T.Sub(got[i-1].T)
						if d < pulsePeriod-2*tc.expectTol || d > pulsePeriod+2*tc.expectTol {
							t.Errorf("edge %d follows edge %d by %v, want ~%v", i, i-1, d, pulsePeriod)
						}
					}
				}
			})
		})
	}
}

func TestPollMissedPulseKeepsLatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 17}
		ctx, cancel := context.WithCancel(context.Background())
		edges := make(chan Edge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, f, Wiring{Pin: gpsio.ModemCTS}, edges, testLog) }()
		seen := make(map[int]bool)
		for pulse := 0; pulse < 18; {
			pulse = pulseIndex((<-edges).T, f.epoch)
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
	})
}

func TestPollOutageResettles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 31}
		ctx, cancel := context.WithCancel(context.Background())
		edges := make(chan Edge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, f, Wiring{Pin: gpsio.ModemCTS}, edges, testLog) }()
		var first int
		for first <= 15 {
			first = pulseIndex((<-edges).T, f.epoch)
		}
		cancel()
		<-errCh
		if first < f.offTo+9 || first > f.offTo+18 {
			t.Errorf("first edge after outage is pulse %d, want a fresh settle between pulses %d and %d",
				first, f.offTo+9, f.offTo+18)
		}
	})
}

// TestPollShrinksToFloor checks that the additive shrink walks the settled
// window down until steady state costs only a handful of state queries per
// pulse. The descent is one bracket gap per shrinkAfter catches from a
// settled window of about pollsPerWindow gaps, so it needs several hundred
// simulated pulses to reach the floor.
func TestPollShrinksToFloor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		edges := make(chan Edge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, f, Wiring{Pin: gpsio.ModemCTS}, edges, testLog) }()
		for pulseIndex((<-edges).T, f.epoch) < 900 {
		}
		start := f.calls.Load()
		for i := 0; i < 50; i++ {
			<-edges
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
	synctest.Test(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 100 * time.Microsecond, lateEvery: 5, late: time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		edges := make(chan Edge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, f, Wiring{Pin: gpsio.ModemCTS}, edges, testLog) }()
		seen := make(map[int]bool)
		for last := 0; last < 500; {
			last = pulseIndex((<-edges).T, f.epoch)
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

// TestPollNarrowPulse checks that a pulse narrower than the cold-start
// spacing (Septentrio's 5 ms default) is acquired by the phase sweep at the
// cap and then tracked normally, since the settled spacing is below the
// width.
func TestPollNarrowPulse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 5 * time.Millisecond,
			callDur: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		edges := make(chan Edge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, f, Wiring{Pin: gpsio.ModemCTS}, edges, testLog) }()
		var got []Edge
		for len(got) < 3 {
			got = append(got, <-edges)
		}
		cancel()
		<-errCh
		if first := pulseIndex(got[0].T, f.epoch); first > 40 {
			t.Errorf("first edge published at pulse %d, want acquisition well before pulse 40", first)
		}
		for i, e := range got {
			pulse := pulseIndex(e.T, f.epoch)
			if err := e.T.Sub(f.epoch) - time.Duration(pulse)*pulsePeriod; err < -3*time.Millisecond || err > 3*time.Millisecond {
				t.Errorf("edge %d at %v: error %v from pulse %d, want within 3ms", i, e.T, err, pulse)
			}
		}
	})
}

type errPin struct{ err error }

func (p errPin) ModemControlPinState() (gpsio.ModemControlPinState, error) { return 0, p.err }

func TestPollReaderError(t *testing.T) {
	e := errors.New("query failed")
	if err := Poll(context.Background(), errPin{err: e}, Wiring{Pin: gpsio.ModemCTS}, nil, testLog); err != e {
		t.Fatalf("Poll error = %v, want %v", err, e)
	}
}
