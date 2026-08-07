package serialpps

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

type testChangeWaiter struct {
	state gpsio.ModemControlLineState
	next  chan gpsio.ModemControlLineState
}

func (w *testChangeWaiter) ModemControlLineState() (gpsio.ModemControlLineState, error) {
	return w.state, nil
}

func (w *testChangeWaiter) CanWaitModemControlLineChange() bool { return true }

func (w *testChangeWaiter) WaitModemControlLineChange(gpsio.ModemControlLine) (time.Time, error) {
	w.state = <-w.next
	return time.Now(), nil
}

func TestWait(t *testing.T) {
	asserted := gpsio.ModemControlLineState(1 << gpsio.ModemCTS)
	w := &testChangeWaiter{state: asserted, next: make(chan gpsio.ModemControlLineState, 3)}
	w.next <- asserted
	w.next <- 0
	edges := make(chan Edge, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Wait(ctx, w, Wiring{Line: gpsio.ModemCTS}, edges) }()
	select {
	case edge := <-edges:
		if edge.T.IsZero() {
			t.Fatal("Wait emitted a zero timestamp")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not emit the deasserting edge")
	}
	cancel()
	w.next <- asserted
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
}

// testFallbackWaiter reports the wait capability but fails every wait with
// ErrUnsupported, as a tty driver without TIOCMIWAIT does; it cancels the
// context on the first wait so that the polling fallback returns promptly.
type testFallbackWaiter struct {
	canWait bool
	waits   int
	cancel  context.CancelFunc
}

func (w *testFallbackWaiter) ModemControlLineState() (gpsio.ModemControlLineState, error) {
	return 0, nil
}

func (w *testFallbackWaiter) CanWaitModemControlLineChange() bool { return w.canWait }

func (w *testFallbackWaiter) WaitModemControlLineChange(gpsio.ModemControlLine) (time.Time, error) {
	w.waits++
	w.cancel()
	return time.Time{}, errors.ErrUnsupported
}

func TestDetectFallsBackToPolling(t *testing.T) {
	tests := []struct {
		name        string
		canWait     bool
		expectWaits int
	}{
		{name: "unsupported wait falls back", canWait: true, expectWaits: 1},
		{name: "no capability polls directly", canWait: false, expectWaits: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &testFallbackWaiter{canWait: tc.canWait, cancel: cancel}
			if !tc.canWait {
				cancel()
			}
			err := Detect(ctx, slog.New(slog.DiscardHandler), w, Wiring{Line: gpsio.ModemCTS}, make(chan Edge, 1))
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Detect error = %v, want context.Canceled", err)
			}
			if w.waits != tc.expectWaits {
				t.Errorf("waits = %d, want %d", w.waits, tc.expectWaits)
			}
		})
	}
}

func TestClassifyReading(t *testing.T) {
	base := time.Unix(1_000, 0)
	asserted := gpsio.ModemControlLineState(1 << gpsio.ModemCTS)
	tests := []struct {
		name       string
		curState   gpsio.ModemControlLineState
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := reading{state: asserted, at: base}
			cur := reading{state: tc.curState, at: base.Add(tc.curAt)}
			edge, missed := classifyReading(prev, cur, Wiring{Line: gpsio.ModemCTS}, base.Add(tc.deadline))
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
			if want := wantRef.Sub(tEdge).Seconds(); sample.Offset != want {
				t.Errorf("offset = %v, want %v", sample.Offset, want)
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

func TestGeneratorSuppressesLeapAcrossDayBoundary(t *testing.T) {
	g := NewGenerator()
	utc := time.Unix(86_399, 500_000_000).UTC()
	read := time.Unix(86_399, 600_000_000)
	g.MsgUTCTime(utc, read, ptime.LeapSecondPositive)
	sample, ok := g.Edge(Edge{T: read.Add(900 * time.Millisecond)})
	if !ok {
		t.Fatal("Edge returned no sample")
	}
	if !sample.Reference.Equal(time.Unix(86_400, 0)) || sample.Leap != ptime.LeapSecondNone {
		t.Fatalf("sample = %+v, want midnight reference and no leap", sample)
	}
}
