package phcsample

import (
	"errors"
	"testing"
	"time"
)

// monoBaseTime and utcBaseTime are deliberately far apart so the tests
// exercise wallClock across divergent mono/UTC domains. The fit and
// gates must not rely on tRead and utc sharing an epoch.
var (
	monoBaseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	utcBaseTime  = time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
)

type wallClockTestInput struct {
	cfg         Config
	n           int
	step        time.Duration // time between consecutive points; default 1s
	delay       time.Duration // per-point tRead lag versus its reported utc
	residuals   []time.Duration
	queryOffset time.Duration
}

func (in wallClockTestInput) run() (utc time.Time, ok bool, wc *wallClock, err error) {
	cfg := in.cfg
	wc = newWallClock(&cfg)
	step := in.step
	if step == 0 {
		step = time.Second
	}
	for i := 0; i < in.n; i++ {
		utc := utcBaseTime.Add(time.Duration(i) * step)
		tRead := monoBaseTime.Add(time.Duration(i) * step).Add(in.delay)
		if i < len(in.residuals) {
			tRead = tRead.Add(in.residuals[i])
		}
		wc.Add(tRead, utc)
	}
	queryMono := monoBaseTime.Add(in.queryOffset)
	utc, err = wc.SecondAt(queryMono)
	ok = err == nil
	return utc, ok, wc, err
}

func TestWallClockWarmUp(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name      string
		n         int
		step      time.Duration
		expectErr error
	}{
		{name: "no data", n: 0, expectErr: ErrNotReady},
		{name: "single point", n: 1, expectErr: ErrNotReady},
		{name: "two points below min span", n: 2, step: 500 * time.Millisecond, expectErr: ErrNotReady},
		{name: "enough points but span below min", n: 3, step: 500 * time.Millisecond, expectErr: ErrNotReady},
		{name: "span reaches MinMsgSpan", n: 4, step: time.Second, expectErr: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := wallClockTestInput{
				cfg:         cfg,
				n:           tc.n,
				step:        tc.step,
				delay:       100 * time.Millisecond,
				queryOffset: 0,
			}.run()
			if tc.expectErr == nil {
				if err != nil {
					t.Errorf("expected success, got err: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.expectErr) {
				t.Errorf("err = %v, want %v", err, tc.expectErr)
			}
		})
	}
}

func TestWallClockDelaySweep(t *testing.T) {
	cfg := DefaultConfig()
	// Enough points for MinMsgSpan (3s default) at 1 Hz.
	n := 4
	tests := []struct {
		name        string
		delay       time.Duration
		queryOffset time.Duration
		expectUTC   time.Time
	}{
		{name: "50ms delay, t=0", delay: 50 * time.Millisecond, queryOffset: 0, expectUTC: utcBaseTime},
		{name: "100ms delay, t=0", delay: 100 * time.Millisecond, queryOffset: 0, expectUTC: utcBaseTime},
		{name: "250ms delay, t=0", delay: 250 * time.Millisecond, queryOffset: 0, expectUTC: utcBaseTime},
		{name: "100ms delay, t=2s", delay: 100 * time.Millisecond, queryOffset: 2 * time.Second, expectUTC: utcBaseTime.Add(2 * time.Second)},
		{name: "100ms delay, t=2.4s rounds down", delay: 100 * time.Millisecond, queryOffset: 2*time.Second + 400*time.Millisecond, expectUTC: utcBaseTime.Add(2 * time.Second)},
		{name: "100ms delay, t=2.6s rounds up", delay: 100 * time.Millisecond, queryOffset: 2*time.Second + 600*time.Millisecond, expectUTC: utcBaseTime.Add(3 * time.Second)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := cfg
			cfg.ExpectedDelay = tc.delay.Seconds()
			utc, _, _, err := wallClockTestInput{
				cfg:         cfg,
				n:           n,
				delay:       tc.delay,
				queryOffset: tc.queryOffset,
			}.run()
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !utc.Equal(tc.expectUTC) {
				t.Errorf("utc = %v, want %v", utc, tc.expectUTC)
			}
		})
	}
}

// TestWallClockToleratesDivergentMonoUtcFrames confirms wallClock does
// not assume tRead and utc share an epoch. The scaffolding already uses
// monoBaseTime (year 2000) distinct from utcBaseTime (year 2026); this
// test makes the intent explicit.
func TestWallClockToleratesDivergentMonoUtcFrames(t *testing.T) {
	cfg := DefaultConfig()
	utc, _, _, err := wallClockTestInput{
		cfg:         cfg,
		n:           4,
		delay:       100 * time.Millisecond,
		queryOffset: 0,
	}.run()
	if err != nil {
		t.Fatalf("unexpected err despite legal mono/utc divergence: %v", err)
	}
	if !utc.Equal(utcBaseTime) {
		t.Errorf("utc = %v, want %v", utc, utcBaseTime)
	}
}

func TestWallClockStaleQuery(t *testing.T) {
	cfg := DefaultConfig()
	_, _, _, err := wallClockTestInput{
		cfg:         cfg,
		n:           4,
		delay:       100 * time.Millisecond,
		queryOffset: 3*time.Second + ptimeSeconds(cfg.MaxMsgGap) + time.Second,
	}.run()
	if err == nil {
		t.Fatalf("expected stale-query error, got nil")
	}
	if errors.Is(err, ErrNotReady) {
		t.Errorf("stale query should not surface as ErrNotReady; got %v", err)
	}
}

func TestWallClockClockRateMismatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ClockRateLimit = 0.01 // 1%, tight on purpose
	// 20% rate mismatch: step utc by 1s but mono by 1.2s
	wc := newWallClock(&cfg)
	for i := range 8 {
		utc := utcBaseTime.Add(time.Duration(i) * time.Second)
		tRead := monoBaseTime.Add(time.Duration(i)*1200*time.Millisecond).Add(100 * time.Millisecond)
		wc.Add(tRead, utc)
	}
	_, err := wc.SecondAt(monoBaseTime)
	if err == nil {
		t.Fatalf("expected clock rate error, got nil")
	}
	if errors.Is(err, ErrNotReady) {
		t.Errorf("rate mismatch should not surface as ErrNotReady; got %v", err)
	}
}

func TestWallClockMsgTimingScatterTolerantOfMinority(t *testing.T) {
	cfg := DefaultConfig()
	// One point in ten is offset by 250 ms — the sort of extra-delay spike you
	// get from a slower-cadence message type on a shared serial link. Median
	// absolute residual stays well below MsgTimingVariation.
	residuals := []time.Duration{0, 0, 0, 0, 0, 250 * time.Millisecond, 0, 0, 0, 0}
	_, _, _, err := wallClockTestInput{
		cfg:         cfg,
		n:           10,
		delay:       100 * time.Millisecond,
		residuals:   residuals,
		queryOffset: 0,
	}.run()
	if err != nil {
		t.Errorf("unexpected error with 1/10 outlier: %v", err)
	}
}

func TestWallClockMsgTimingScatterRejectsMajority(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MsgTimingVariation = 0.02 // 20ms — tight to force failure
	// Alternating residuals of ±80 ms. Median absolute residual ≈ 80 ms, above threshold.
	residuals := []time.Duration{
		80 * time.Millisecond, -80 * time.Millisecond,
		80 * time.Millisecond, -80 * time.Millisecond,
		80 * time.Millisecond, -80 * time.Millisecond,
	}
	_, _, _, err := wallClockTestInput{
		cfg:         cfg,
		n:           len(residuals),
		delay:       100 * time.Millisecond,
		residuals:   residuals,
		queryOffset: 0,
	}.run()
	if err == nil {
		t.Fatalf("expected msg timing scatter error, got nil")
	}
	if errors.Is(err, ErrNotReady) {
		t.Errorf("scatter should not surface as ErrNotReady; got %v", err)
	}
}

func TestWallClockReset(t *testing.T) {
	cfg := DefaultConfig()
	wc := newWallClock(&cfg)
	for i := range 5 {
		utc := utcBaseTime.Add(time.Duration(i) * time.Second)
		tRead := monoBaseTime.Add(time.Duration(i) * time.Second).Add(100 * time.Millisecond)
		wc.Add(tRead, utc)
	}
	if _, err := wc.SecondAt(monoBaseTime); err != nil {
		t.Fatalf("pre-reset: unexpected err: %v", err)
	}
	wc.Reset()
	if _, err := wc.SecondAt(monoBaseTime); !errors.Is(err, ErrNotReady) {
		t.Errorf("post-reset: err = %v, want ErrNotReady", err)
	}
}

func TestWallClockMsgWindowPruning(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MsgWindow = 5 // seconds
	wc := newWallClock(&cfg)
	for i := range 20 {
		utc := utcBaseTime.Add(time.Duration(i) * time.Second)
		tRead := monoBaseTime.Add(time.Duration(i) * time.Second).Add(100 * time.Millisecond)
		wc.Add(tRead, utc)
	}
	// After 20 1-second additions with 5-second retention, at most 6 points
	// remain (the window accepts observations whose tRead is within 5s of the
	// newest).
	if got := len(wc.points); got > 6 {
		t.Errorf("len(points) = %d, want <= 6 after pruning", got)
	}
}

// ptimeSeconds is a small helper so tests don't pull in ptime just for
// one constant conversion.
func ptimeSeconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
