package serialpps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

var testLog = slog.New(slog.DiscardHandler)

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

type testChangeWaiter struct {
	next chan testWaitResult
	// entered, when non-nil, reports that a wait is under way, so that a
	// test can cancel while the wait is blocked rather than before it.
	entered chan struct{}
}

type testWaitResult struct {
	change gpsio.ModemControlPinChange
	missed int
	err    error
}

func (w *testChangeWaiter) ModemControlPinState() (gpsio.ModemControlPinState, error) {
	return 0, nil
}

func (w *testChangeWaiter) WaitModemControlPinChange(ctx context.Context, _ gpsio.ModemControlPin, _ gpsio.PPSMethod) (gpsio.ModemControlPinChange, int, error) {
	if w.entered != nil {
		w.entered <- struct{}{}
	}
	select {
	case r := <-w.next:
		return r.change, r.missed, r.err
	case <-ctx.Done():
		return gpsio.ModemControlPinChange{}, 0, ctx.Err()
	}
}

func TestWait(t *testing.T) {
	mono := time.Now()
	wall := mono.Add(time.Millisecond)
	w := &testChangeWaiter{next: make(chan testWaitResult, 3)}
	// An asserted transition is not a leading pulse edge and must not be
	// published. The following deasserted transition is published even when
	// the backend reports missed transitions.
	w.next <- testWaitResult{change: gpsio.ModemControlPinChange{Wall: wall.Add(-time.Second), Mono: mono.Add(-time.Second), Asserted: true}}
	w.next <- testWaitResult{change: gpsio.ModemControlPinChange{Wall: wall, Mono: mono}, missed: 2}
	candidates := make(chan CandidateEdge, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	var logs bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	go func() { errCh <- Wait(ctx, lg, w, Wiring{Pin: gpsio.ModemCTS}, gpsio.PPSMethodWait, candidates) }()
	select {
	case candidate := <-candidates:
		if candidate.Wall != wall || candidate.Mono != mono {
			t.Fatalf("Wait edge = %+v, want supplied wakeup times", candidate.Edge)
		}
		if candidate.Uncertainty != 0 {
			t.Errorf("Wait uncertainty = %v, want no polling-bracket uncertainty", candidate.Uncertainty)
		}
		if !candidate.Settled {
			t.Error("Wait candidate is unsettled")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not emit the deasserting edge")
	}
	if !strings.Contains(logs.String(), "serial PPS transitions not observed") || !strings.Contains(logs.String(), "atLeast=2") {
		t.Errorf("logs %q do not report the missed transitions", logs.String())
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
}

func TestWaitContextCancellation(t *testing.T) {
	w := &testChangeWaiter{next: make(chan testWaitResult), entered: make(chan struct{}, 1)}
	candidates := make(chan CandidateEdge, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Wait(ctx, testLog, w, Wiring{Pin: gpsio.ModemCTS}, gpsio.PPSMethodWait, candidates) }()
	<-w.entered
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
	if n := len(candidates); n != 0 {
		t.Fatalf("Wait emitted %d candidates after cancellation, want none", n)
	}
}

// testFallbackWaiter fails every wait with err; it cancels the context on
// the first poll so that a run that reaches the polling fallback returns
// promptly.
type testFallbackWaiter struct {
	err             error
	successfulWaits int
	methods         []gpsio.PPSMethod
	cancel          context.CancelFunc
}

func (w *testFallbackWaiter) ModemControlPinState() (gpsio.ModemControlPinState, error) {
	w.cancel()
	return 0, nil
}

func (w *testFallbackWaiter) WaitModemControlPinChange(_ context.Context, _ gpsio.ModemControlPin, method gpsio.PPSMethod) (gpsio.ModemControlPinChange, int, error) {
	w.methods = append(w.methods, method)
	if w.successfulWaits > 0 {
		w.successfulWaits--
		return gpsio.ModemControlPinChange{Asserted: true}, 0, nil
	}
	return gpsio.ModemControlPinChange{}, 0, w.err
}

// testPoller is a StateReader without the wait capability.
type testPoller struct {
	cancel context.CancelFunc
}

func (p *testPoller) ModemControlPinState() (gpsio.ModemControlPinState, error) {
	p.cancel()
	return 0, nil
}

func TestDetectMethodSelection(t *testing.T) {
	errUnsup := fmt.Errorf("no capability: %w", errors.ErrUnsupported)
	errUnavailable := fmt.Errorf("driver cannot wait: %w", gpsio.ErrUnavailable)
	errDriver := errors.New("inappropriate ioctl for device")
	tests := []struct {
		name            string
		method          gpsio.PPSMethod
		waitErr         error
		successfulWaits int
		expectMethods   []gpsio.PPSMethod
		expectSelected  []gpsio.PPSMethod
		expectErr       error
		expectPolled    bool
		expectLog       string
		expectNoLog     string
	}{
		{
			name: "auto skips unsupported methods quietly", waitErr: errUnsup,
			expectMethods:  []gpsio.PPSMethod{gpsio.PPSMethodKernel, gpsio.PPSMethodWait},
			expectSelected: []gpsio.PPSMethod{gpsio.PPSMethodKernel, gpsio.PPSMethodWait, gpsio.PPSMethodPoll},
			expectErr:      context.Canceled, expectPolled: true,
			expectLog: "serial PPS method unavailable", expectNoLog: "level=WARN",
		},
		{
			name: "auto warns when the method is unavailable", waitErr: errUnavailable,
			expectMethods:  []gpsio.PPSMethod{gpsio.PPSMethodKernel, gpsio.PPSMethodWait},
			expectSelected: []gpsio.PPSMethod{gpsio.PPSMethodKernel, gpsio.PPSMethodWait, gpsio.PPSMethodPoll},
			expectErr:      context.Canceled, expectPolled: true,
			expectLog: "level=WARN msg=\"serial PPS method unavailable; falling back\"",
		},
		{
			name: "auto returns an ordinary failure after a successful wait", waitErr: errDriver, successfulWaits: 1,
			expectMethods:  []gpsio.PPSMethod{gpsio.PPSMethodKernel, gpsio.PPSMethodKernel},
			expectSelected: []gpsio.PPSMethod{gpsio.PPSMethodKernel},
			expectErr:      errDriver, expectPolled: false,
			expectNoLog: "level=WARN",
		},
		{
			name: "forced poll never waits", method: gpsio.PPSMethodPoll,
			expectSelected: []gpsio.PPSMethod{gpsio.PPSMethodPoll},
			expectErr:      context.Canceled, expectPolled: true,
		},
		{
			name: "forced kernel returns failure", method: gpsio.PPSMethodKernel, waitErr: errDriver,
			expectMethods:  []gpsio.PPSMethod{gpsio.PPSMethodKernel},
			expectSelected: []gpsio.PPSMethod{gpsio.PPSMethodKernel},
			expectErr:      errDriver, expectNoLog: "level=WARN",
		},
		{
			name: "forced wait returns unsupported", method: gpsio.PPSMethodWait, waitErr: errUnsup,
			expectMethods:  []gpsio.PPSMethod{gpsio.PPSMethodWait},
			expectSelected: []gpsio.PPSMethod{gpsio.PPSMethodWait},
			expectErr:      errors.ErrUnsupported,
		},
		{
			name: "forced wait returns unavailable", method: gpsio.PPSMethodWait, waitErr: errUnavailable,
			expectMethods:  []gpsio.PPSMethod{gpsio.PPSMethodWait},
			expectSelected: []gpsio.PPSMethod{gpsio.PPSMethodWait},
			expectErr:      gpsio.ErrUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &testFallbackWaiter{err: tc.waitErr, successfulWaits: tc.successfulWaits, cancel: cancel}
			stats := new(PollStats)
			var logs bytes.Buffer
			lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			err := Detect(ctx, lg, w, Wiring{Pin: gpsio.ModemCTS}, tc.method, make(chan CandidateEdge, 1), stats)
			if !errors.Is(err, tc.expectErr) {
				t.Errorf("Detect error = %v, want %v", err, tc.expectErr)
			}
			if !reflect.DeepEqual(w.methods, tc.expectMethods) {
				t.Errorf("methods tried = %v, want %v", w.methods, tc.expectMethods)
			}
			if stats.started != tc.expectPolled {
				t.Errorf("polled = %v, want %v", stats.started, tc.expectPolled)
			}
			selectionCount := strings.Count(logs.String(), `msg="serial PPS method selected"`)
			if selectionCount != len(tc.expectSelected) {
				t.Errorf("method-selection log count = %d, want %d; logs: %q", selectionCount, len(tc.expectSelected), logs.String())
			}
			for _, method := range tc.expectSelected {
				entry := fmt.Sprintf(`msg="serial PPS method selected" method=%s`, method)
				if strings.Count(logs.String(), entry) != 1 {
					t.Errorf("logs %q do not report selecting %v exactly once", logs.String(), method)
				}
			}
			if tc.expectLog != "" && !strings.Contains(logs.String(), tc.expectLog) {
				t.Errorf("logs %q do not contain %q", logs.String(), tc.expectLog)
			}
			if tc.expectNoLog != "" && strings.Contains(logs.String(), tc.expectNoLog) {
				t.Errorf("logs %q unexpectedly contain %q", logs.String(), tc.expectNoLog)
			}
		})
	}
}

func TestDetectWithoutWaiterPolls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stats := new(PollStats)
	var logs bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	err := Detect(ctx, lg, &testPoller{cancel: cancel}, Wiring{Pin: gpsio.ModemCTS}, 0, make(chan CandidateEdge, 1), stats)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Detect error = %v, want context.Canceled", err)
	}
	if !stats.started {
		t.Error("Detect did not fall through to polling")
	}
	if got := strings.Count(logs.String(), `msg="serial PPS method selected" method=poll`); got != 1 {
		t.Errorf("poll-selection log count = %d, want 1; logs: %q", got, logs.String())
	}
	if strings.Contains(logs.String(), "method=wait") {
		t.Errorf("logs report selecting unavailable wait method: %q", logs.String())
	}
}

func TestDetectForcedWaitWithoutWaiter(t *testing.T) {
	for _, method := range []gpsio.PPSMethod{gpsio.PPSMethodWait, gpsio.PPSMethodKernel} {
		t.Run(method.String(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := Detect(ctx, testLog, &testPoller{cancel: cancel}, Wiring{Pin: gpsio.ModemCTS}, method, make(chan CandidateEdge, 1), nil)
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("Detect error = %v, want errors.ErrUnsupported", err)
			}
		})
	}
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
	// The mono readings are skewed from the wall readings so a midpoint or a
	// deadline comparison taken from the wrong clock is caught. deadline is
	// on the mono timeline, as Poll's is; the "reaching deadline" case
	// straddles the two, so comparing it against wall would report a miss
	// one poll late.
	const monoSkew = time.Millisecond
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prevAt := clockReading{wall: base, mono: base.Add(monoSkew)}
			curAt := clockReading{wall: base.Add(tc.curAt), mono: base.Add(tc.curAt + monoSkew)}
			prev := reading{state: asserted, poll: poll{start: prevAt, end: prevAt}}
			cur := reading{state: tc.curState, poll: poll{start: curAt, end: curAt}}
			edge, missed := classify(prev, cur, Wiring{Pin: gpsio.ModemCTS}, base.Add(tc.deadline+monoSkew))
			if missed != tc.wantMissed {
				t.Errorf("missed = %v, want %v", missed, tc.wantMissed)
			}
			if tc.wantEdgeAt == 0 {
				if !edge.Wall.IsZero() {
					t.Errorf("edge = %v, want zero", edge)
				}
			} else if want := base.Add(tc.wantEdgeAt); !edge.Wall.Equal(want) {
				t.Errorf("edge.Wall = %v, want %v", edge.Wall, want)
			} else if !edge.Mono.Equal(want.Add(monoSkew)) {
				t.Errorf("edge.Mono = %v, want %v", edge.Mono, want.Add(monoSkew))
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
			g := NewGenerator(DefaultConfig())
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
			// The wall reading is skewed from the mono reading so a
			// generator using the wrong one for either role is caught.
			wall := tEdge.Add(2 * time.Millisecond)
			sample, ok := g.Sample(Edge{Wall: wall, Mono: tEdge})
			if ok != tc.ok {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			wantRef := time.Unix(1_000, 0).UTC()
			if !sample.Ref.Equal(wantRef) {
				t.Errorf("reference = %v, want %v", sample.Ref, wantRef)
			}
			if !sample.Sys.Equal(wall) {
				t.Errorf("system = %v, want %v", sample.Sys, wall)
			}
			if sample.Leap != tc.leap {
				t.Errorf("leap = %v, want %v", sample.Leap, tc.leap)
			}
		})
	}
}

func TestGeneratorKeepsNewestMessage(t *testing.T) {
	g := NewGenerator(DefaultConfig())
	newRead := time.Unix(100, 100_000_000)
	g.MsgUTCTime(time.Unix(200, 0), newRead, ptime.LeapSecondPositive)
	g.MsgUTCTime(time.Unix(300, 0), newRead.Add(-time.Second), ptime.LeapSecondNegative)
	sample, ok := g.Sample(Edge{Wall: time.Unix(100, 0), Mono: time.Unix(100, 0)})
	if !ok {
		t.Fatal("Edge returned no sample")
	}
	if !sample.Ref.Equal(time.Unix(200, 0)) || sample.Leap != ptime.LeapSecondPositive {
		t.Fatalf("sample = %+v, want newest message reference and leap", sample)
	}
}

func TestGeneratorDelayBounds(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name  string
		delay time.Duration
		ok    bool
	}{
		{name: "at negative uncertainty bound", delay: -seconds(cfg.DelayUncertainty), ok: true},
		{name: "below negative uncertainty bound", delay: -seconds(cfg.DelayUncertainty) - time.Nanosecond},
		{name: "zero delay", delay: 0, ok: true},
		{name: "below maximum delay", delay: seconds(cfg.MaxDelay) - time.Nanosecond, ok: true},
		{name: "at maximum delay", delay: seconds(cfg.MaxDelay)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator(cfg)
			utc := time.Unix(1_000, 0).UTC()
			tRead := time.Unix(900, 0)
			g.MsgUTCTime(utc, tRead, ptime.LeapSecondNone)
			at := tRead.Add(-tc.delay)
			sample, ok := g.Sample(Edge{Wall: at, Mono: at})
			if ok != tc.ok {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.ok)
			}
			if ok && !sample.Ref.Equal(utc) {
				t.Errorf("reference = %v, want %v", sample.Ref, utc)
			}
		})
	}
}

func TestGeneratorLeapCrossing(t *testing.T) {
	tests := []struct {
		name       string
		utc        time.Time
		leap       ptime.LeapSecondKind
		elapsed    time.Duration // edge.Wall - tRead
		expectRef  time.Time
		expectLeap ptime.LeapSecondKind
		expectOK   bool
	}{
		{name: "pulse before positive leap", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: -125 * time.Millisecond, expectRef: time.Unix(86_399, 0), expectLeap: ptime.LeapSecondPositive, expectOK: true},
		{name: "inserted second pulse yields no sample", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: 875 * time.Millisecond},
		{name: "first pulse after positive leap", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: 1875 * time.Millisecond, expectRef: time.Unix(86_400, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
		{name: "second pulse after positive leap", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: 2875 * time.Millisecond, expectRef: time.Unix(86_401, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
		{name: "pulse before negative leap", utc: time.Unix(86_398, 0), leap: ptime.LeapSecondNegative,
			elapsed: -125 * time.Millisecond, expectRef: time.Unix(86_398, 0), expectLeap: ptime.LeapSecondNegative, expectOK: true},
		{name: "first pulse after negative leap", utc: time.Unix(86_398, 0), leap: ptime.LeapSecondNegative,
			elapsed: 875 * time.Millisecond, expectRef: time.Unix(86_400, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator(DefaultConfig())
			read := time.Unix(1_000_000, 125_000_000)
			g.MsgUTCTime(tc.utc, read, tc.leap)
			at := read.Add(tc.elapsed)
			sample, ok := g.Sample(Edge{Wall: at, Mono: at})
			if ok != tc.expectOK {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.expectOK)
			}
			if !ok {
				return
			}
			if !sample.Ref.Equal(tc.expectRef) || sample.Leap != tc.expectLeap {
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
// none), modelling a delivery tail. A nonzero wakeJitter delays any query
// that follows an idle gap, alternating between the full amount and an
// eighth of it, modelling the sleep overshoot observed inside the daemon:
// queries after a sleep run late by a varying amount, back-to-back queries
// do not. A nonzero stall delays the single first query at or after
// stallAfter (relative to epoch) by that much, stretching one bracket --
// the noise event that made a latch comparing consecutive brackets misfire
// in the daemon. A nonzero slowCallDur replaces callDur from slowFrom until
// slowTo, modelling a transient run of slow queries. calls counts the state
// queries.
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
	stalled        bool
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
	time.Sleep(callDur)
	since = time.Since(f.epoch)
	n := int(since / period)
	off := since % period
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

func TestPollStatsSummary(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	newReading := func(offset time.Duration) clockReading {
		return clockReading{wall: base.Add(offset), mono: base.Add(offset)}
	}
	polls := []poll{
		{start: newReading(0), end: newReading(time.Millisecond)},
		{start: newReading(11 * time.Millisecond), end: newReading(13 * time.Millisecond)},
		{start: newReading(33 * time.Millisecond), end: newReading(36 * time.Millisecond)},
		{start: newReading(66 * time.Millisecond), end: newReading(70 * time.Millisecond)},
		{start: newReading(110 * time.Millisecond), end: newReading(115 * time.Millisecond)},
	}
	stats := new(PollStats)
	stats.addPoll(polls[0], nil)
	for i := 1; i < len(polls); i++ {
		stats.addPoll(polls[i], &polls[i-1])
	}
	stats.addWindow(false, false)
	stats.addWindow(true, false)
	stats.addWindow(true, true)

	got := stats.summary()
	want := pollStatsSummary{
		Windows:      3,
		Edges:        2,
		SettledEdges: 1,
		PollDuration: durationStats{
			Count: 5, Sampled: 5, Min: time.Millisecond, Median: 3 * time.Millisecond,
			Mean: 3 * time.Millisecond, P90: 5 * time.Millisecond, Max: 5 * time.Millisecond,
		},
		PollGap: durationStats{
			Count: 4, Sampled: 4, Min: 10 * time.Millisecond, Median: 30 * time.Millisecond,
			Mean: 25 * time.Millisecond, P90: 40 * time.Millisecond, Max: 40 * time.Millisecond,
		},
	}
	if got != want {
		t.Errorf("Summary() = %+v, want %+v", got, want)
	}
}

func TestPollStatsTimingSamplesAreBounded(t *testing.T) {
	stats := new(PollStats)
	base := time.Unix(1_700_000_000, 0)
	for i := range pollStatsSampleLimit + 1 {
		start := clockReading{wall: base, mono: base}
		end := clockReading{wall: base.Add(time.Duration(i) + 1), mono: base.Add(time.Duration(i) + 1)}
		stats.addPoll(poll{start: start, end: end}, nil)
	}
	summary := stats.summary().PollDuration
	if summary.Count != pollStatsSampleLimit+1 || summary.Sampled != pollStatsSampleLimit {
		t.Errorf("duration counts = %d total, %d sampled; want %d total, %d sampled",
			summary.Count, summary.Sampled, pollStatsSampleLimit+1, pollStatsSampleLimit)
	}
}

func TestPollStatsLog(t *testing.T) {
	stats := new(PollStats)
	stats.begin()
	start := clockReading{wall: time.Unix(1_700_000_000, 0), mono: time.Unix(1_700_000_000, 0)}
	end := clockReading{wall: start.wall.Add(2 * time.Millisecond), mono: start.mono.Add(2 * time.Millisecond)}
	stats.addPoll(poll{start: start, end: end}, nil)
	stats.addWindow(true, false)

	var output bytes.Buffer
	stats.Log(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo})))
	for _, want := range []string{
		`msg="serial PPS polling statistics" windows=1 edges=1 settledEdges=0`,
		`msg="serial PPS state read times" count=1 min=2ms median=2ms mean=2ms p90=2ms max=2ms`,
		`msg="serial PPS between-read times" count=0`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("log %q does not contain %q", output.String(), want)
		}
	}
}

func TestPollStatsLogSkipsUnusedStats(t *testing.T) {
	var output bytes.Buffer
	new(PollStats).Log(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if output.Len() != 0 {
		t.Errorf("unused polling statistics logged %q", output.String())
	}
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
					since := e.Wall.Sub(f.epoch)
					pulse := pulseIndex(e.Wall, f.epoch)
					if err := since - time.Duration(pulse)*period; err < -tc.expectTol || err > tc.expectTol {
						t.Errorf("edge %d at %v: error %v from pulse %d, want within %v", i, e.Wall, err, pulse, tc.expectTol)
					}
					if i == 0 && (pulse < tc.expectFirstPulse || pulse > tc.expectLastPulse) {
						t.Errorf("first published edge is pulse %d, want settling to end between pulses %d and %d",
							pulse, tc.expectFirstPulse, tc.expectLastPulse)
					}
					if i > 0 {
						d := e.Wall.Sub(got[i-1].Wall)
						if d < period-2*tc.expectTol || d > period+2*tc.expectTol {
							t.Errorf("edge %d follows edge %d by %v, want ~%v", i, i-1, d, period)
						}
					}
				}
			})
		})
	}
}

func TestPollMissedPulseKeepsLatch(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 17}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		seen := make(map[int]bool)
		for pulse := 0; pulse < 18; {
			pulse = pulseIndex(nextSettled(candidates).Wall, f.epoch)
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
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 31}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		var first int
		for first <= 15 {
			first = pulseIndex(nextSettled(candidates).Wall, f.epoch)
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
// settled window of about initialPolls gaps, so it needs several hundred
// simulated pulses to reach the floor.
func TestPollShrinksToFloor(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		for pulseIndex(nextSettled(candidates).Wall, f.epoch) < 900 {
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
			last = pulseIndex(nextSettled(candidates).Wall, f.epoch)
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
		if first := pulseIndex(got[0].Wall, f.epoch); first > 15 {
			t.Errorf("first edge published at pulse %d, want settling despite the jitter plateau", first)
		}
		// Settling in the jitter plateau leaves the window at 15.625ms or
		// wider; the query-paced floor is reached at 3.9ms.
		if capture.window == 0 || capture.window > 8*time.Millisecond {
			t.Errorf("settled at window %v, want the latch to hold out until the queries pace the loop", capture.window)
		}
		for i, e := range got {
			pulse := pulseIndex(e.Wall, f.epoch)
			if err := e.Wall.Sub(f.epoch) - time.Duration(pulse)*period; err < -500*time.Microsecond || err > 500*time.Microsecond {
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
		if first := pulseIndex(got[0].Wall, f.epoch); first > 40 {
			t.Errorf("first edge published at pulse %d, want acquisition well before pulse 40", first)
		}
		for i, e := range got {
			pulse := pulseIndex(e.Wall, f.epoch)
			if err := e.Wall.Sub(f.epoch) - time.Duration(pulse)*period; err < -3*time.Millisecond || err > 3*time.Millisecond {
				t.Errorf("edge %d at %v: error %v from pulse %d, want within 3ms", i, e.Wall, err, pulse)
			}
		}
	})
}

type errPin struct{ err error }

func (p errPin) ModemControlPinState() (gpsio.ModemControlPinState, error) { return 0, p.err }

func TestPollReaderError(t *testing.T) {
	e := errors.New("query failed")
	if err := Poll(context.Background(), testLog, errPin{err: e}, Wiring{Pin: gpsio.ModemCTS}, nil, nil); err != e {
		t.Fatalf("Poll error = %v, want %v", err, e)
	}
}
