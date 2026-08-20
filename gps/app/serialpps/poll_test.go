package serialpps

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

// acquireCapture records the window attribute of the "serial PPS acquired"
// debug line, so tests can check where in the descent the latch fired. Read
// it only after Poll has returned.
type acquireCapture struct {
	slog.Handler
	window time.Duration
}

func (h *acquireCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *acquireCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "serial PPS acquired" {
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
		expectFirstPulse int // acquisition length bounds, in pulses
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
				sawSettling := false
				for len(got) < 3 {
					candidate := <-candidates
					if !candidate.Settled {
						sawSettling = true
						continue
					}
					got = append(got, candidate)
				}
				cancel()
				if err := <-errCh; err != context.Canceled {
					t.Fatalf("Poll error = %v, want context.Canceled", err)
				}
				if !sawSettling {
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
						t.Errorf("first published edge is pulse %d, want acquisition to end between pulses %d and %d",
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

// TestTrackSimulation drives the production tracking control with a
// query-paced reader, matching pollWindow's contract: edges have absolute
// timing jitter, the query pace varies per attempt, a transition found on the
// read crossing the deadline still counts as a catch, and a miss forwards the
// previous catch's bracket. The late-opens scenario reproduces the hardware's
// sporadic oversleep of the window open: an edge arriving before the late
// open cannot be observed, so the window size that misses is discovered only
// by missing, and the window must stay above it. The last two scenarios
// verify that misses at a transiently large window cost at most one
// shrinkAfter hold there: nothing is remembered, so nothing can be
// remembered wrongly.
func TestTrackSimulation(t *testing.T) {
	const openLate = 900 * time.Microsecond
	tests := []struct {
		name             string
		lateEvery        int           // every lateEvery-th open is openLate late; 0 means none
		offFrom, offTo   int           // edges suppressed for attempts in [offFrom, offTo)
		dropAt           int           // one further suppressed attempt (0 means none)
		flapFrom, flapTo int           // in [flapFrom, flapTo), suppress all but every third edge
		maxMisses        int           // per minute
		maxReads         int           // per attempt after the first minute
		maxMinuteReads   int           // per minute after the first
		convergeWindow   time.Duration // the window must shrink to this
		convergeBy       int           // by this attempt
		recoverWindow    time.Duration // after an outage the window must return to this
		recoverBy        int           // by this attempt
	}{
		{name: "prompt opens", maxMisses: 1, maxReads: 11, maxMinuteReads: 390,
			convergeWindow: 1500 * time.Microsecond, convergeBy: 20},
		{name: "late opens", lateEvery: 20, maxMisses: 4, maxReads: 17, maxMinuteReads: 600,
			convergeWindow: 2400 * time.Microsecond, convergeBy: 15},
		{name: "outage", offFrom: 150, offTo: 158, maxMisses: 9, maxReads: 70, maxMinuteReads: 2500,
			convergeWindow: 1500 * time.Microsecond, convergeBy: 20,
			recoverWindow: 2 * time.Millisecond, recoverBy: 280},
		// A single drop during the post-outage descent freezes the window at
		// the transiently large size it happened to hit, for shrinkAfter
		// catches; the cost is bounded, and the descent then completes.
		{name: "outage with drop", offFrom: 150, offTo: 158, dropAt: 170,
			maxMisses: 9, maxReads: 70, maxMinuteReads: 2100,
			convergeWindow: 1500 * time.Microsecond, convergeBy: 20,
			recoverWindow: 2 * time.Millisecond, recoverBy: 560},
		// Flapping ratchets the window up while it lasts (each cycle is a
		// two-miss run, so the window is frozen, growing but never
		// shrinking); one shrinkAfter hold later it has fully recovered.
		{name: "flapping", flapFrom: 150, flapTo: 180,
			maxMisses: 20, maxReads: 70, maxMinuteReads: 2100,
			convergeWindow: 1500 * time.Microsecond, convergeBy: 20,
			recoverWindow: 2 * time.Millisecond, recoverBy: 600},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			type sample struct {
				window, predictionError time.Duration
				caught                  bool
				stateReads              int
			}
			type minute struct {
				stateReads, catches, misses, minReads, maxReads int
				endWindow                                       time.Duration
			}
			const minutes = 12
			jitters := [...]time.Duration{-250 * time.Microsecond, -150 * time.Microsecond,
				-50 * time.Microsecond, 50 * time.Microsecond, 150 * time.Microsecond, 250 * time.Microsecond}
			brackets := [...]time.Duration{100 * time.Microsecond, 250 * time.Microsecond,
				150 * time.Microsecond, 90 * time.Microsecond, 200 * time.Microsecond}
			done := errors.New("simulation complete")
			samples := make([]sample, 0, minutes*60)
			var events []trackEvent
			nextEdge := time.Duration(0)
			lastBracket := brackets[0]
			err := track(initialPolls*minSpacing, func(window time.Duration, _ bool) (trackObservation, error) {
				i := len(samples)
				if i == cap(samples) {
					return trackObservation{}, done
				}
				jitter := time.Duration(0)
				if i == 3 {
					jitter = 300 * time.Microsecond
				} else if i > 3 {
					jitter = jitters[(i-4)%len(jitters)]
				}
				late := time.Duration(0)
				if tc.lateEvery > 0 && (i+1)%tc.lateEvery == 0 {
					late = openLate
				}
				bracket := brackets[i%len(brackets)]
				pace := max(window/initialPolls, bracket)
				edge := time.Duration(i)*period + jitter
				predictionError := edge - nextEdge
				suppressed := tc.offTo > 0 && i >= tc.offFrom && i < tc.offTo ||
					tc.dropAt > 0 && i == tc.dropAt ||
					tc.flapTo > 0 && i >= tc.flapFrom && i < tc.flapTo && (i-tc.flapFrom)%3 != 2
				caught := predictionError > late-window/2 && predictionError < window/2+pace &&
					!suppressed
				sweep := max(window-late, 0)
				if caught {
					sweep = predictionError + window/2 - late
					lastBracket = bracket
				}
				stateReads := int((sweep+pace-1)/pace) + 1
				samples = append(samples, sample{window: window, predictionError: predictionError,
					caught: caught, stateReads: stateReads})
				return trackObservation{caught: caught, predictionError: predictionError,
					lastBracket: lastBracket, stateReads: stateReads}, nil
			}, func(d time.Duration) {
				nextEdge += d
			}, func(e trackEvent) {
				events = append(events, e)
			})
			if !errors.Is(err, done) {
				t.Fatalf("track error = %v, want simulation completion", err)
			}
			got := make([]minute, minutes)
			for i, s := range samples {
				m := &got[i/60]
				m.stateReads += s.stateReads
				if s.caught {
					m.catches++
				} else {
					m.misses++
				}
				if m.minReads == 0 || s.stateReads < m.minReads {
					m.minReads = s.stateReads
				}
				m.maxReads = max(m.maxReads, s.stateReads)
				m.endWindow = s.window
			}
			for i, m := range got {
				t.Logf("minute %d: stateReads=%d catches=%d misses=%d min=%d max=%d endWindow=%v",
					i+1, m.stateReads, m.catches, m.misses, m.minReads, m.maxReads, m.endWindow)
			}
			for i, m := range got {
				if m.misses > tc.maxMisses {
					t.Errorf("minute %d misses = %d, want at most %d", i+1, m.misses, tc.maxMisses)
				}
				if i > 0 && m.maxReads > tc.maxReads {
					t.Errorf("minute %d max state reads = %d, want at most %d", i+1, m.maxReads, tc.maxReads)
				}
				if i > 0 && m.stateReads > tc.maxMinuteReads {
					t.Errorf("minute %d state reads = %d, want at most %d", i+1, m.stateReads, tc.maxMinuteReads)
				}
			}
			convergedAt := -1
			for i, s := range samples {
				if s.window <= tc.convergeWindow {
					convergedAt = i
					break
				}
			}
			if convergedAt < 0 || convergedAt > tc.convergeBy {
				t.Errorf("window first at or below %v at attempt %d, want by attempt %d",
					tc.convergeWindow, convergedAt, tc.convergeBy)
			}
			if tc.offTo > 0 {
				if s := samples[tc.offTo]; !s.caught {
					t.Errorf("attempt %d after the outage missed at window %v, want the grown window to recapture the pulse immediately",
						tc.offTo, s.window)
				}
			}
			if resume := max(tc.offTo, tc.flapTo); resume > 0 && tc.recoverWindow > 0 {
				recoveredAt := -1
				for i, s := range samples[resume:] {
					if s.window <= tc.recoverWindow {
						recoveredAt = resume + i
						break
					}
				}
				if recoveredAt < 0 || recoveredAt > tc.recoverBy {
					t.Errorf("window back at or below %v at attempt %d, want by attempt %d",
						tc.recoverWindow, recoveredAt, tc.recoverBy)
				}
			}
			for _, e := range events {
				if e.kind == trackLost {
					t.Errorf("tracking event kind = %v, want no loss events", e.kind)
				}
			}
			if samples[3].window <= 2*samples[3].predictionError.Abs() {
				t.Errorf("disturbance window = %v for prediction error %v, want the edge retained inside the margin",
					samples[3].window, samples[3].predictionError)
			}
		})
	}
}

// TestTrackFeedback pins the feedback law step by step: shrinking reported
// only once per halving and growth once per doubling, half of each prediction
// error advancing the prediction, a first miss growing the window by two
// brackets, a second miss doubling, the recovery being reported, and the
// window then holding, frozen, after the short run of misses.
func TestTrackFeedback(t *testing.T) {
	done := errors.New("simulation complete")
	var windows, advances []time.Duration
	var atFloors []bool
	var events []trackEvent
	observations := []trackObservation{
		{caught: true, lastBracket: 100 * time.Microsecond},
		{caught: true, predictionError: 750 * time.Microsecond, lastBracket: 100 * time.Microsecond},
		{caught: false, lastBracket: 100 * time.Microsecond},
		{caught: false, lastBracket: 100 * time.Microsecond},
		{caught: true, lastBracket: 100 * time.Microsecond},
	}
	err := track(800*time.Microsecond, func(window time.Duration, atFloor bool) (trackObservation, error) {
		windows = append(windows, window)
		atFloors = append(atFloors, atFloor)
		if len(windows) > len(observations) {
			return trackObservation{}, done
		}
		return observations[len(windows)-1], nil
	}, func(d time.Duration) {
		advances = append(advances, d)
	}, func(e trackEvent) {
		events = append(events, e)
	})
	if !errors.Is(err, done) {
		t.Fatalf("track error = %v, want simulation completion", err)
	}
	if want := []time.Duration{800 * time.Microsecond, 750 * time.Microsecond,
		1700 * time.Microsecond, 1900 * time.Microsecond, 3800 * time.Microsecond,
		3800 * time.Microsecond}; !reflect.DeepEqual(windows, want) {
		t.Errorf("tracking windows = %v, want %v", windows, want)
	}
	if want := []time.Duration{period, period + 375*time.Microsecond, period, period,
		period}; !reflect.DeepEqual(advances, want) {
		t.Errorf("prediction advances = %v, want %v", advances, want)
	}
	if want := []bool{false, false, true, false, false, false}; !reflect.DeepEqual(atFloors, want) {
		t.Errorf("atFloor per attempt = %v, want %v", atFloors, want)
	}
	wantEvents := []trackEventKind{trackStarted, trackChanged, trackMissed,
		trackMissed, trackRecovered}
	if len(events) != len(wantEvents) {
		t.Fatalf("tracking events = %v, want kinds %v", events, wantEvents)
	}
	for i, want := range wantEvents {
		if events[i].kind != want {
			t.Errorf("tracking event %d kind = %v, want %v", i, events[i].kind, want)
		}
	}
}

// TestTrackLoss pins the give-up rule: doubling reaches the full-period
// window on the 11th consecutive miss, and only missLimit further misses at
// that window declare the pulse gone.
func TestTrackLoss(t *testing.T) {
	var windows []time.Duration
	var events []trackEvent
	err := track(time.Millisecond, func(window time.Duration, _ bool) (trackObservation, error) {
		windows = append(windows, window)
		return trackObservation{caught: false, lastBracket: 100 * time.Microsecond}, nil
	}, func(time.Duration) {}, func(e trackEvent) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("track error = %v, want nil after loss", err)
	}
	if len(windows) != 11+missLimit {
		t.Errorf("pulse declared gone after %d attempts, want %d", len(windows), 11+missLimit)
	}
	last := events[len(events)-1]
	if last.kind != trackLost || last.window != maxWindow {
		t.Errorf("last event = kind %v window %v, want loss at the full-period window", last.kind, last.window)
	}
}

// TestTrackAbsenceShrinksAtOnce pins the freeze exception: a run of absentRun
// consecutive misses means the pulse was absent, so shrinking resumes on the
// recovery catch instead of holding for shrinkAfter catches.
func TestTrackAbsenceShrinksAtOnce(t *testing.T) {
	done := errors.New("simulation complete")
	var windows []time.Duration
	observations := []trackObservation{
		{caught: false, lastBracket: 100 * time.Microsecond},
		{caught: false, lastBracket: 100 * time.Microsecond},
		{caught: false, lastBracket: 100 * time.Microsecond},
		{caught: true, lastBracket: 100 * time.Microsecond},
		{caught: true, lastBracket: 100 * time.Microsecond},
	}
	err := track(800*time.Microsecond, func(window time.Duration, _ bool) (trackObservation, error) {
		windows = append(windows, window)
		if len(windows) > len(observations) {
			return trackObservation{}, done
		}
		return observations[len(windows)-1], nil
	}, func(time.Duration) {}, func(trackEvent) {})
	if !errors.Is(err, done) {
		t.Fatalf("track error = %v, want simulation completion", err)
	}
	if want := []time.Duration{800 * time.Microsecond, 1000 * time.Microsecond,
		2000 * time.Microsecond, 4000 * time.Microsecond, 3750 * time.Microsecond,
		3515625 * time.Nanosecond}; !reflect.DeepEqual(windows, want) {
		t.Errorf("tracking windows = %v, want %v", windows, want)
	}
}

// TestPollShortOutageKeepsTracking checks that an outage shorter than the
// give-up horizon does not discard the phase: the grown window recaptures the
// pulse on its first reappearance, publishing it unsettled while the window
// is still shrinking back, and candidates settle again once it has.
func TestPollShortOutageKeepsTracking(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 31}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		var first CandidateEdge
		for pulseIndex(first.Timestamp, f.epoch) <= 15 || first.Timestamp.IsZero() {
			first = <-candidates
		}
		resettled := pulseIndex(nextSettled(candidates).Timestamp, f.epoch)
		cancel()
		<-errCh
		if p := pulseIndex(first.Timestamp, f.epoch); p != f.offTo {
			t.Errorf("first edge after outage is pulse %d, want recapture at pulse %d", p, f.offTo)
		}
		if first.Settled {
			t.Error("recapture candidate at the outage-grown window is settled, want unsettled while the window shrinks back")
		}
		if resettled > f.offTo+110 {
			t.Errorf("candidates settled again at pulse %d, want within ~100 pulses of the recapture", resettled)
		}
	})
}

// TestPollAcquiresWithCoarseStateRefresh exercises the former fixed point: the
// ordinary cached query takes only 5 us, but a state refresh stretches each
// catching bracket to about 2 ms. The old window-driven acquisition stalled
// above minSpacing while every caught window remained sleep-paced.
func TestPollAcquiresWithCoarseStateRefresh(t *testing.T) {
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
			t.Fatal("Poll did not acquire with coarse modem-state refreshes")
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
		for _, field := range []string{"window=", "nextWindow=", "stateReads=", "bracket=", "misses=1"} {
			if !strings.Contains(status, field) {
				t.Errorf("track status %q does not contain %q", status, field)
			}
		}
	})
}

func TestPollOutageReacquires(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 51}
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
			t.Errorf("first edge after outage is pulse %d, want a fresh acquisition between pulses %d and %d",
				first, f.offTo+9, f.offTo+18)
		}
	})
}

// TestPollTrackingConverges checks that tracking shrinks the acquired window
// to a handful of state queries per pulse within a small fraction of the time
// the former additive controller needed.
func TestPollTrackingConverges(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, Wiring{Pin: gpsio.ModemCTS}, candidates, nil) }()
		for pulseIndex(nextSettled(candidates).Timestamp, f.epoch) < 100 {
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
// which the acquired window is initially shrunk too far to cover, is learned
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

// TestPollAcquiresDespiteSleepJitter reproduces the daemon's sleep-overshoot
// regime: wakeups after an idle gap run up to ~0.9 ms late, and one poll
// mid-acquisition stalls outright, stretching its bracket -- the noise the
// former bracket-comparison latch latched on, publishing millisecond-class
// samples from a still-wide window. Acquisition must ignore bracket noise and
// wait until the queries pace the loop, where the jitter vanishes and
// edges are located to the query time. The stall is timed to hit the
// bracket of the pulse-4 catch, mid-halving.
func TestPollAcquiresDespiteSleepJitter(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 100 * time.Microsecond, wakeJitter: 900 * time.Microsecond,
			stallAfter: 3999 * time.Millisecond, stall: 3 * time.Millisecond}
		capture := &acquireCapture{Handler: slog.DiscardHandler}
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
			t.Errorf("first edge published at pulse %d, want acquisition despite the jitter plateau", first)
		}
		// Acquiring in the jitter plateau leaves the window at 15.625ms or
		// wider; the query-paced floor is reached at 3.9ms.
		if capture.window == 0 || capture.window > 8*time.Millisecond {
			t.Errorf("acquired at window %v, want the latch to hold out until the queries pace the loop", capture.window)
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
// queries resume at the next pulse, so acquisition must continue until the
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
		capture := &acquireCapture{Handler: slog.DiscardHandler}
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
			t.Errorf("acquired at window %v, want the one-window query slowdown suppressed", capture.window)
		}
	})
}

// TestPollNarrowPulse checks that a pulse narrower than the cold-start
// spacing (Septentrio's 5 ms default) is acquired by the phase sweep at the
// cap and then tracked normally, since the acquired spacing is below the
// width.
// TestPollNarrowPulse sweeps the pulse phase across the 7.8125 ms spacing of
// the second acquisition stage. The 2 ms pulse fits between the polls of the
// second and third stages at most phases, and a miss repeats the
// pulse-relative poll positions, so acquisition depends on the per-miss grid
// sweep finding the pulse.
func TestPollNarrowPulse(t *testing.T) {
	for k := range 6 {
		t.Run(strconv.Itoa(k), func(t *testing.T) {
			testPollNarrowPulse(t, 350*time.Millisecond+time.Duration(k)*1300*time.Microsecond)
		})
	}
}

func testPollNarrowPulse(t *testing.T, epochOffset time.Duration) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(epochOffset), width: 2 * time.Millisecond,
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
		first := pulseIndex(got[0].Timestamp, f.epoch)
		t.Logf("first edge at pulse %d", first)
		if first > 40 {
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
