package serialpps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/pps"
)

var testLog = slog.New(slog.DiscardHandler)

type testChangeWaiter struct {
	next chan testWaitResult
	// entered, when non-nil, reports that a wait is under way, so that a
	// test can cancel while the wait is blocked rather than before it.
	entered chan struct{}
}

type testWaitResult struct {
	change gpsio.SerialPinChange
	missed int
	err    error
}

func (w *testChangeWaiter) SerialPinState() (gpsio.SerialPinState, error) {
	return 0, nil
}

func (w *testChangeWaiter) WaitSerialPinChange(ctx context.Context, _ gpsio.SerialPin, _ gpsio.PPSMethod) (gpsio.SerialPinChange, int, error) {
	if w.entered != nil {
		w.entered <- struct{}{}
	}
	select {
	case r := <-w.next:
		return r.change, r.missed, r.err
	case <-ctx.Done():
		return gpsio.SerialPinChange{}, 0, ctx.Err()
	}
}

func TestWait(t *testing.T) {
	tRead := time.Now()
	timestamp := tRead.Add(time.Millisecond)
	w := &testChangeWaiter{next: make(chan testWaitResult, 3)}
	// An asserted transition is not a leading pulse edge and must not be
	// published. The following deasserted transition is published even when
	// the backend reports missed transitions.
	w.next <- testWaitResult{change: gpsio.SerialPinChange{Timestamp: timestamp.Add(-time.Second), TRead: tRead.Add(-time.Second), Asserted: true}}
	w.next <- testWaitResult{change: gpsio.SerialPinChange{Timestamp: timestamp, TRead: tRead}, missed: 2}
	candidates := make(chan pps.CandidateEdge, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	var logs bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	go func() { errCh <- Wait(ctx, lg, w, Wiring{Pin: gpsio.SerialPinCTS}, gpsio.PPSMethodWait, candidates) }()
	select {
	case candidate := <-candidates:
		if candidate.Timestamp != timestamp || candidate.TRead != tRead {
			t.Fatalf("Wait edge = %+v, want supplied timestamp and read time", candidate.Edge)
		}
		if candidate.Uncertainty != 0 {
			t.Errorf("Wait uncertainty = %v, want no polling-bracket uncertainty", candidate.Uncertainty)
		}
		if !candidate.Settled {
			t.Error("Wait candidate is not settled")
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

func TestWaitInvertedPolarity(t *testing.T) {
	tRead := time.Now()
	timestamp := tRead.Add(time.Millisecond)
	w := &testChangeWaiter{next: make(chan testWaitResult, 2)}
	// With PolarityAssert the pulse asserts the flag, so the deasserted
	// transition is a trailing edge and the asserted one is published.
	w.next <- testWaitResult{change: gpsio.SerialPinChange{Timestamp: timestamp.Add(-time.Second), TRead: tRead.Add(-time.Second)}}
	w.next <- testWaitResult{change: gpsio.SerialPinChange{Timestamp: timestamp, TRead: tRead, Asserted: true}}
	candidates := make(chan pps.CandidateEdge, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Wait(ctx, testLog, w, Wiring{Pin: gpsio.SerialPinCTS, Polarity: PolarityAssert}, gpsio.PPSMethodWait, candidates)
	}()
	select {
	case candidate := <-candidates:
		if candidate.Timestamp != timestamp || candidate.TRead != tRead {
			t.Fatalf("Wait edge = %+v, want the asserting transition", candidate.Edge)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not emit the asserting edge")
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
}

func TestWaitContextCancellation(t *testing.T) {
	w := &testChangeWaiter{next: make(chan testWaitResult), entered: make(chan struct{}, 1)}
	candidates := make(chan pps.CandidateEdge, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Wait(ctx, testLog, w, Wiring{Pin: gpsio.SerialPinCTS}, gpsio.PPSMethodWait, candidates) }()
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
	polled          bool
	cancel          context.CancelFunc
}

func (w *testFallbackWaiter) SerialPinState() (gpsio.SerialPinState, error) {
	w.polled = true
	w.cancel()
	return 0, nil
}

func (w *testFallbackWaiter) WaitSerialPinChange(_ context.Context, _ gpsio.SerialPin, method gpsio.PPSMethod) (gpsio.SerialPinChange, int, error) {
	w.methods = append(w.methods, method)
	if w.successfulWaits > 0 {
		w.successfulWaits--
		return gpsio.SerialPinChange{Asserted: true}, 0, nil
	}
	return gpsio.SerialPinChange{}, 0, w.err
}

// testPoller is a StateReader without the wait capability.
type testPoller struct {
	polled bool
	cancel context.CancelFunc
}

func (p *testPoller) SerialPinState() (gpsio.SerialPinState, error) {
	p.polled = true
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
			var logs bytes.Buffer
			lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			err := Detect(ctx, lg, w, Wiring{Pin: gpsio.SerialPinCTS}, Config{Method: tc.method}, make(chan pps.CandidateEdge, 1), nil)
			if !errors.Is(err, tc.expectErr) {
				t.Errorf("Detect error = %v, want %v", err, tc.expectErr)
			}
			if !reflect.DeepEqual(w.methods, tc.expectMethods) {
				t.Errorf("methods tried = %v, want %v", w.methods, tc.expectMethods)
			}
			if w.polled != tc.expectPolled {
				t.Errorf("polled = %v, want %v", w.polled, tc.expectPolled)
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
	p := &testPoller{cancel: cancel}
	var logs bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	err := Detect(ctx, lg, p, Wiring{Pin: gpsio.SerialPinCTS}, Config{}, make(chan pps.CandidateEdge, 1), nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Detect error = %v, want context.Canceled", err)
	}
	if !p.polled {
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
			err := Detect(ctx, testLog, &testPoller{cancel: cancel}, Wiring{Pin: gpsio.SerialPinCTS}, Config{Method: method}, make(chan pps.CandidateEdge, 1), nil)
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("Detect error = %v, want errors.ErrUnsupported", err)
			}
		})
	}
}
