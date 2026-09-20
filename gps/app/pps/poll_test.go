package pps

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

	"github.com/jclark/satpulse/time/lib/median"
)

// acquireCapture records the window attribute of the "serial PPS acquired"
// debug line, so tests can check where in the descent the latch fired. Read
// it only after Poll has returned.
type acquireCapture struct {
	slog.Handler
	window time.Duration
}

var truncatesSubMillisecondSleeps = sleepDuration(time.Microsecond) == 0

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
// none), modelling a delivery tail. A nonzero wakeJitter makes the fake's
// wait, installed as PollParams.Wait, overshoot every timer sleep by that
// much or an eighth of it alternately, modelling the sleep overshoot
// observed inside the daemon: queries after a sleep run late by a varying
// amount, back-to-back queries do not. A nonzero stall delays the single
// first query at or after
// stallAfter (relative to epoch) by that much, stretching one bracket --
// the noise event that made a latch comparing consecutive brackets misfire
// in the daemon. A nonzero slowCallDur replaces callDur from slowFrom until
// slowTo, modelling a transient run of slow queries. A nonzero stateRefresh
// exposes pulse-state changes only on that time grid. calls counts the state
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
	stateRefresh   time.Duration
	stalled        bool
	seq            uint32
	calls          atomic.Int64
}

func (f *fakePulse) wait(ctx context.Context, t time.Time) (bool, error) {
	slept, err := waitUntil(ctx, t)
	if slept && f.wakeJitter > 0 {
		if f.seq++; f.seq%2 == 0 {
			time.Sleep(f.wakeJitter)
		} else {
			time.Sleep(f.wakeJitter / 8)
		}
	}
	return slept, err
}

func (f *fakePulse) InPulse() (bool, error) {
	f.calls.Add(1)
	if f.stall > 0 && !f.stalled && time.Since(f.epoch) >= f.stallAfter {
		f.stalled = true
		time.Sleep(f.stall)
	}
	callDur := f.callDur
	since := time.Since(f.epoch)
	if f.slowCallDur > 0 && since >= f.slowFrom && since < f.slowTo {
		callDur = f.slowCallDur
	}
	time.Sleep(callDur)
	return f.state(time.Since(f.epoch)), nil
}

func (f *fakePulse) state(since time.Duration) bool {
	if f.stateRefresh > 0 && since >= 0 {
		since = since.Truncate(f.stateRefresh)
	}
	n := int(since / period)
	off := since % period
	if f.lateEvery > 0 && n%f.lateEvery == 0 {
		off -= f.late
	}
	return since >= 0 && off >= 0 && off < f.width && !(f.offTo > 0 && n >= f.offFrom && n < f.offTo)
}

func TestPoll(t *testing.T) {
	tests := []struct {
		name                string
		epochOffset         time.Duration // pulse 0's leading edge relative to start
		callDur             time.Duration
		expectFirstPulse    int // acquisition length bounds, in pulses
		expectLastPulse     int
		truncatedFirstPulse int // bounds when sub-millisecond sleeps are truncated
		truncatedLastPulse  int
		usable              time.Duration // uncertainty limit for an edge at query resolution
		expectTol           time.Duration // per-edge timestamp error bound
	}{
		{name: "slow query (FT232R class)", epochOffset: 350 * time.Millisecond, callDur: 2 * time.Millisecond,
			expectFirstPulse: 2, expectLastPulse: 12, usable: 2 * time.Millisecond, expectTol: 3 * time.Millisecond},
		{name: "fast query", epochOffset: 350 * time.Millisecond, callDur: 20 * time.Microsecond,
			expectFirstPulse: 3, expectLastPulse: 18,
			truncatedFirstPulse: 3, truncatedLastPulse: 15, usable: 100 * time.Microsecond, expectTol: 100 * time.Microsecond},
		{name: "cold start inside pulse", epochOffset: -20 * time.Millisecond, callDur: 20 * time.Microsecond,
			expectFirstPulse: 3, expectLastPulse: 18,
			truncatedFirstPulse: 3, truncatedLastPulse: 15, usable: 100 * time.Microsecond, expectTol: 100 * time.Microsecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runBubble(t, func(t *testing.T) {
				expectFirstPulse, expectLastPulse := tc.expectFirstPulse, tc.expectLastPulse
				if truncatesSubMillisecondSleeps && tc.truncatedFirstPulse != 0 {
					expectFirstPulse, expectLastPulse = tc.truncatedFirstPulse, tc.truncatedLastPulse
				}
				f := &fakePulse{epoch: time.Now().Add(tc.epochOffset), width: 100 * time.Millisecond, callDur: tc.callDur}
				ctx, cancel := context.WithCancel(context.Background())
				candidates := make(chan CandidateEdge)
				errCh := make(chan error, 1)
				go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
				var got []CandidateEdge
				sawCoarse := false
				for len(got) < 3 {
					candidate := <-candidates
					if candidate.Anomalous || max(candidate.Uncertainty[0], candidate.Uncertainty[1]) > tc.usable {
						sawCoarse = true
						continue
					}
					got = append(got, candidate)
				}
				cancel()
				if err := <-errCh; err != context.Canceled {
					t.Fatalf("Poll error = %v, want context.Canceled", err)
				}
				if !sawCoarse {
					t.Error("Poll did not report any coarse candidates during acquisition")
				}
				for i, e := range got {
					if e.Uncertainty[0] <= 0 || e.Uncertainty[1] <= 0 || e.PollWidths[0] <= 0 || e.PollWidths[1] <= 0 {
						t.Errorf("candidate %d uncertainty = %v poll widths = %v, want all positive", i, e.Uncertainty, e.PollWidths)
					}
					if !e.TRead.After(e.Timestamp) {
						t.Errorf("candidate %d read time %v is not after timestamp %v", i, e.TRead, e.Timestamp)
					}
					since := e.Timestamp.Sub(f.epoch)
					pulse := pulseIndex(e.Timestamp, f.epoch)
					if err := since - time.Duration(pulse)*period; err < -tc.expectTol || err > tc.expectTol {
						t.Errorf("edge %d at %v: error %v from pulse %d, want within %v", i, e.Timestamp, err, pulse, tc.expectTol)
					}
					if i == 0 && (pulse < expectFirstPulse || pulse > expectLastPulse) {
						t.Errorf("first usable edge is pulse %d, want between pulses %d and %d",
							pulse, expectFirstPulse, expectLastPulse)
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
// open cannot be observed, so the extent that misses is discovered only by
// missing, and the miss-double-shrink cycle pays an isolated miss for it
// every so often. Even with prompt opens the jitter sits at the edge of the
// extent that shrinkStop brackets of the narrowest bracket give, so the
// cycle shows there too, at a lower rate. The outage scenario is a run of
// misses the doubling survives without maxExtent handing back to
// acquisition; the wide-catch scenario stretches every 30th bracket,
// which must not widen the extent.
func TestTrackSimulation(t *testing.T) {
	const openLate = 900 * time.Microsecond
	tests := []struct {
		name           string
		lateEvery      int           // every lateEvery-th open is openLate late; 0 means none
		offFrom, offTo int           // edges suppressed for attempts in [offFrom, offTo)
		wideEvery      int           // every wideEvery-th catch has a wide bracket; 0 means none
		maxMisses      int           // per minute
		maxReads       int           // per attempt after the first minute
		maxMinuteReads int           // per minute after the first
		convergeExtent time.Duration // the extent must shrink to this
		convergeBy     int           // by this attempt
		recoverExtent  time.Duration // after an outage the extent must return to this
		recoverBy      int           // by this attempt
	}{
		{name: "prompt opens", maxMisses: 2, maxReads: 12, maxMinuteReads: 700,
			convergeExtent: 1500 * time.Microsecond, convergeBy: 30},
		{name: "late opens", lateEvery: 20, maxMisses: 4, maxReads: 24, maxMinuteReads: 1000,
			convergeExtent: 1500 * time.Microsecond, convergeBy: 30},
		{name: "outage", offFrom: 150, offTo: 156, maxMisses: 8, maxReads: 1200, maxMinuteReads: 6000,
			convergeExtent: 1500 * time.Microsecond, convergeBy: 30,
			recoverExtent: 2 * time.Millisecond, recoverBy: 300},
		{name: "wide catches", wideEvery: 30, maxMisses: 2, maxReads: 12, maxMinuteReads: 700,
			convergeExtent: 1500 * time.Microsecond, convergeBy: 30},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			type sample struct {
				extent, predictionError time.Duration
				outcome                 outcome
				stateReads              int
			}
			type minute struct {
				stateReads, catches, misses, minReads, maxReads int
				endExtent                                       time.Duration
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
			err := track(initialPolls*minSpacing, func(extent time.Duration) (trackObservation, error) {
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
				pace := bracket
				edge := time.Duration(i)*period + jitter
				predictionError := edge - nextEdge
				suppressed := tc.offTo > 0 && i >= tc.offFrom && i < tc.offTo
				o := miss
				if predictionError > late-extent/2 && predictionError < extent/2+pace && !suppressed {
					o = caught
					if tc.wideEvery > 0 && (i+1)%tc.wideEvery == 0 {
						bracket *= 4
					}
				}
				sweep := max(extent-late, 0)
				if o != miss {
					sweep = predictionError + extent/2 - late
					lastBracket = bracket
				}
				stateReads := int((sweep+pace-1)/pace) + 1
				samples = append(samples, sample{extent: extent, predictionError: predictionError,
					outcome: o, stateReads: stateReads})
				return trackObservation{outcome: o, predictionError: predictionError,
					bracket: lastBracket, stateReads: stateReads}, nil
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
				switch s.outcome {
				case caught:
					m.catches++
				default:
					m.misses++
				}
				if m.minReads == 0 || s.stateReads < m.minReads {
					m.minReads = s.stateReads
				}
				m.maxReads = max(m.maxReads, s.stateReads)
				m.endExtent = s.extent
			}
			for i, m := range got {
				t.Logf("minute %d: stateReads=%d catches=%d misses=%d min=%d max=%d endExtent=%v",
					i+1, m.stateReads, m.catches, m.misses, m.minReads, m.maxReads, m.endExtent)
			}
			for i := 1; tc.offTo == 0 && i < len(samples); i++ {
				if samples[i-1].outcome == miss && samples[i].outcome == miss {
					t.Errorf("attempts %d and %d both missed, want misses isolated", i-1, i)
				}
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
				if s.extent <= tc.convergeExtent {
					convergedAt = i
					break
				}
			}
			if convergedAt < 0 || convergedAt > tc.convergeBy {
				t.Errorf("extent first at or below %v at attempt %d, want by attempt %d",
					tc.convergeExtent, convergedAt, tc.convergeBy)
			}
			if tc.offTo > 0 {
				if s := samples[tc.offTo]; s.outcome != caught {
					t.Errorf("attempt %d after the outage missed at extent %v, want the grown extent to recapture the pulse immediately",
						tc.offTo, s.extent)
				}
				recoveredAt := -1
				for i, s := range samples[tc.offTo:] {
					if s.extent <= tc.recoverExtent {
						recoveredAt = tc.offTo + i
						break
					}
				}
				if recoveredAt < 0 || recoveredAt > tc.recoverBy {
					t.Errorf("extent back at or below %v at attempt %d, want by attempt %d",
						tc.recoverExtent, recoveredAt, tc.recoverBy)
				}
			}
			for i := 1; i < len(samples); i++ {
				if prev := samples[i-1]; prev.outcome == caught && samples[i].extent > prev.extent {
					t.Errorf("attempt %d extent = %v after a catch at %v, want no increase", i, samples[i].extent, prev.extent)
				}
			}
			for _, e := range events {
				if e.kind == trackLost {
					t.Errorf("tracking event kind = %v, want no loss events", e.kind)
				}
			}
			for i, s := range samples {
				if s.extent > maxExtent {
					t.Errorf("attempt %d extent = %v, want at most maxExtent %v", i, s.extent, maxExtent)
					break
				}
			}
			if samples[3].extent <= 2*samples[3].predictionError.Abs() {
				t.Errorf("disturbance extent = %v for prediction error %v, want the edge retained inside the margin",
					samples[3].extent, samples[3].predictionError)
			}
		})
	}
}

// TestTrackFeedback pins the feedback law step by step: a catch shrinks
// the extent by 1/shrinkDivisor but not below shrinkStop brackets, half of
// each prediction error advances the prediction, every miss doubles,
// and a catch with a wide bracket never widens the extent.
func TestTrackFeedback(t *testing.T) {
	done := errors.New("simulation complete")
	var extents, advances []time.Duration
	var events []trackEvent
	observations := []trackObservation{
		{outcome: caught, bracket: 100 * time.Microsecond},
		{outcome: caught, predictionError: 750 * time.Microsecond, bracket: 50 * time.Microsecond},
		{outcome: caught, predictionError: 2 * time.Millisecond, bracket: 400 * time.Microsecond},
		{outcome: miss, bracket: 400 * time.Microsecond},
		{outcome: miss, bracket: 400 * time.Microsecond},
		{outcome: caught, bracket: 100 * time.Microsecond},
		{outcome: caught, bracket: time.Millisecond},
	}
	err := track(800*time.Microsecond, func(extent time.Duration) (trackObservation, error) {
		extents = append(extents, extent)
		if len(extents) > len(observations) {
			return trackObservation{}, done
		}
		return observations[len(extents)-1], nil
	}, func(d time.Duration) {
		advances = append(advances, d)
	}, func(e trackEvent) {
		events = append(events, e)
	})
	if !errors.Is(err, done) {
		t.Fatalf("track error = %v, want simulation completion", err)
	}
	if want := []time.Duration{800 * time.Microsecond, 800 * time.Microsecond, 775 * time.Microsecond,
		775 * time.Microsecond, 1550 * time.Microsecond, 3100 * time.Microsecond,
		3003125 * time.Nanosecond, 3003125 * time.Nanosecond}; !reflect.DeepEqual(extents, want) {
		t.Errorf("tracking extents = %v, want %v", extents, want)
	}
	if want := []time.Duration{period, period + 375*time.Microsecond, period + time.Millisecond, period, period,
		period, period}; !reflect.DeepEqual(advances, want) {
		t.Errorf("prediction advances = %v, want %v", advances, want)
	}
	wantEvents := []trackEventKind{trackStarted, trackMissed, trackMissed}
	if len(events) != len(wantEvents) {
		t.Fatalf("tracking events = %v, want kinds %v", events, wantEvents)
	}
	for i, want := range wantEvents {
		if events[i].kind != want {
			t.Errorf("tracking event %d kind = %v, want %v", i, events[i].kind, want)
		}
	}
	if events[1].failures != 1 || events[2].failures != 2 {
		t.Errorf("failure counts = %d %d, want 1 and 2", events[1].failures, events[2].failures)
	}
}

// TestTrackShrinkReported checks that shrinking is reported once per
// halving: from 3.2 ms, good catches with a 100 us bracket shrink the extent
// by 1/32 each, and the first report comes when it passes 1.6 ms.
func TestTrackShrinkReported(t *testing.T) {
	done := errors.New("simulation complete")
	var events []trackEvent
	attempts := 0
	err := track(3200*time.Microsecond, func(extent time.Duration) (trackObservation, error) {
		if attempts++; attempts > 40 {
			return trackObservation{}, done
		}
		return trackObservation{outcome: caught, bracket: 100 * time.Microsecond}, nil
	}, func(time.Duration) {}, func(e trackEvent) {
		events = append(events, e)
	})
	if !errors.Is(err, done) {
		t.Fatalf("track error = %v, want simulation completion", err)
	}
	if len(events) != 2 || events[1].kind != trackChanged {
		t.Fatalf("events = %v, want start and one shrink report", events)
	}
	if e := events[1]; 2*e.nextExtent > 3200*time.Microsecond || 2*e.extent <= 3200*time.Microsecond {
		t.Errorf("shrink reported at %v -> %v, want the catch that crosses 1.6 ms", e.extent, e.nextExtent)
	}
}

// TestTrackLoss pins the give-up rules: a miss whose doubling would exceed
// maxExtent, or failureLimit consecutive misses, hand back to
// acquisition.
func TestTrackLoss(t *testing.T) {
	tests := []struct {
		name         string
		extent       time.Duration
		outcome      outcome
		wantAttempts int
		wantExtent   time.Duration
	}{
		{name: "misses double to the bound", extent: time.Millisecond, outcome: miss, wantAttempts: 7, wantExtent: 64 * time.Millisecond},
		{name: "misses from a wide extent", extent: 20 * time.Millisecond, outcome: miss, wantAttempts: 3, wantExtent: 80 * time.Millisecond},
		{name: "consecutive misses", extent: time.Microsecond, outcome: miss, wantAttempts: failureLimit, wantExtent: 512 * time.Microsecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var extents []time.Duration
			var events []trackEvent
			err := track(tc.extent, func(extent time.Duration) (trackObservation, error) {
				extents = append(extents, extent)
				return trackObservation{outcome: tc.outcome, bracket: 100 * time.Microsecond}, nil
			}, func(time.Duration) {}, func(e trackEvent) {
				events = append(events, e)
			})
			if err != nil {
				t.Fatalf("track error = %v, want nil after loss", err)
			}
			if len(extents) != tc.wantAttempts {
				t.Errorf("pulse declared gone after %d attempts, want %d", len(extents), tc.wantAttempts)
			}
			last := events[len(events)-1]
			if last.kind != trackLost || last.extent != tc.wantExtent || last.failures != tc.wantAttempts {
				t.Errorf("last event = kind %v extent %v failures %d, want loss at extent %v after %d failures",
					last.kind, last.extent, last.failures, tc.wantExtent, tc.wantAttempts)
			}
		})
	}
}

// TestAnomalous checks classification against previous tracking widths,
// including startup, the exact threshold and coarse acquisition catches.
func TestAnomalous(t *testing.T) {
	for _, tc := range []struct {
		name     string
		history  []time.Duration
		width    time.Duration
		tracking bool
		want     bool
	}{
		{name: "first acquisition catch", width: time.Millisecond},
		{name: "first tracking catch", width: time.Millisecond, tracking: true},
		{name: "at threshold", history: []time.Duration{8 * time.Microsecond}, width: 32 * time.Microsecond, tracking: true},
		{name: "above threshold", history: []time.Duration{8 * time.Microsecond}, width: 32*time.Microsecond + 1, tracking: true, want: true},
		{name: "UART upper mode", history: []time.Duration{8 * time.Microsecond}, width: 25 * time.Microsecond, tracking: true},
		{name: "median tolerates an extreme", history: []time.Duration{8 * time.Microsecond, 9 * time.Microsecond, time.Millisecond}, width: 40 * time.Microsecond, tracking: true, want: true},
		{name: "acquisition uses tracking history", history: []time.Duration{8 * time.Microsecond}, width: time.Millisecond, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := poller{widths: median.New[time.Duration](widthHistory)}
			for _, w := range tc.history {
				p.widths.Add(w)
			}
			if got := p.anomalous(tc.width, tc.tracking); got != tc.want {
				t.Errorf("anomalous = %v, want %v", got, tc.want)
			}
			n := len(tc.history)
			if tc.tracking {
				n++
				if p.widths.Last() != tc.width {
					t.Errorf("last width = %v, want %v even when anomalous", p.widths.Last(), tc.width)
				}
			}
			if p.widths.Len() != n {
				t.Errorf("history length = %d, want %d", p.widths.Len(), n)
			}
		})
	}
}

// TestAnomalousAdapts checks that withheld widths can establish a new
// baseline, while intervening acquisition catches leave the history intact.
func TestAnomalousAdapts(t *testing.T) {
	p := poller{widths: median.New[time.Duration](widthHistory)}
	for range widthHistory {
		p.widths.Add(8 * time.Microsecond)
	}
	for range widthHistory {
		p.anomalous(100*time.Millisecond, false)
	}
	if p.widths.Median() != 8*time.Microsecond {
		t.Fatal("acquisition changed the tracking baseline")
	}
	for i := range widthHistory + 1 {
		want := i < 16
		if got := p.anomalous(time.Millisecond, true); got != want {
			t.Errorf("slow catch %d anomalous = %v, want %v", i+1, got, want)
		}
	}
	if p.widths.Len() != widthHistory || p.widths.Median() != time.Millisecond {
		t.Errorf("history = %d widths, median %v, want %d and 1ms", p.widths.Len(), p.widths.Median(), widthHistory)
	}
	if p.anomalous(8*time.Microsecond, true) {
		t.Error("return to narrow widths flagged as anomalous")
	}
}

// TestPollAnomaliesDoNotAffectControl gives identical measurements to two
// pollers whose histories make every catch anomalous in one and ordinary in
// the other. Acquisition, prediction updates and extent changes must match.
func TestPollAnomaliesDoNotAffectControl(t *testing.T) {
	type result struct {
		candidates []CandidateEdge
		events     []trackEvent
		calls      int64
		nextEdge   time.Time
	}
	var results [2]result
	for i, baseline := range []time.Duration{time.Nanosecond, period / 2} {
		runBubble(t, func(t *testing.T) {
			f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
				callDur: 100 * time.Microsecond}
			ctx, cancel := context.WithTimeout(context.Background(), 40*period)
			defer cancel()
			candidates := make(chan CandidateEdge, 40)
			p := poller{ctx: ctx, lg: testLog, r: f, ceCh: candidates,
				params: PollParams{InitialPolls: initialPolls, MinSpacing: minSpacing},
				widths: median.New[time.Duration](widthHistory)}
			for range widthHistory {
				p.widths.Add(baseline)
			}
			if err := p.init(); err != nil {
				t.Fatal(err)
			}
			extent, acquired, err := p.acquire()
			if err != nil || !acquired {
				t.Fatalf("acquisition = %v, %v, want success regardless of anomaly flags", acquired, err)
			}
			if p.widths.Median() != baseline {
				t.Fatal("acquisition changed tracking history")
			}
			done := errors.New("tracking complete")
			attempts := 0
			err = track(extent, func(extent time.Duration) (trackObservation, error) {
				if attempts == 12 {
					return trackObservation{}, done
				}
				attempts++
				o, e, err := p.pollWindow(extent, minSpacing, true)
				return trackObservation{outcome: o, predictionError: e, bracket: p.lastBracket, stateReads: p.stateReads}, err
			}, func(d time.Duration) {
				p.nextEdge = p.nextEdge.Add(d)
			}, func(e trackEvent) {
				results[i].events = append(results[i].events, e)
			})
			if err != done {
				t.Fatalf("tracking ended with %v, want completion after more than failureLimit catches", err)
			}
			close(candidates)
			for ce := range candidates {
				if ce.Anomalous != (i == 0) {
					t.Errorf("Anomalous = %v, want %v", ce.Anomalous, i == 0)
				}
				ce.Anomalous = false
				results[i].candidates = append(results[i].candidates, ce)
			}
			results[i].calls, results[i].nextEdge = f.calls.Load(), p.nextEdge
		})
	}
	if !reflect.DeepEqual(results[0], results[1]) {
		t.Error("anomaly flags changed candidates, controller events, read count or prediction")
	}
}

// TestPollShortOutageKeepsTracking checks that an outage shorter than the
// give-up horizon does not discard the phase: the doubled extent recaptures
// the pulse on its first reappearance, and since tracking polls at query
// resolution whatever the extent, the recapture is usable at once.
func TestPollShortOutageKeepsTracking(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 20 * time.Microsecond, offFrom: 16, offTo: 19}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
		var first CandidateEdge
		for pulseIndex(first.Timestamp, f.epoch) <= 15 || first.Timestamp.IsZero() {
			first = <-candidates
		}
		cancel()
		<-errCh
		if p := pulseIndex(first.Timestamp, f.epoch); p != f.offTo {
			t.Errorf("first edge after outage is pulse %d, want recapture at pulse %d", p, f.offTo)
		}
		if first.Anomalous || max(first.Uncertainty[0], first.Uncertainty[1]) > usableUncertainty {
			t.Errorf("recapture candidate anomalous=%v uncertainty=%v, want a usable catch at query resolution", first.Anomalous, first.Uncertainty)
		}
	})
}

// TestPollAcquiresWithCoarseStateRefresh models a driver that refreshes the
// pin state on a 2 ms grid behind 5 us cached queries: each edge becomes
// visible up to 2 ms late, but the bracket around it is two cached queries,
// so acquisition must complete and the edges must be usable.
func TestPollAcquiresWithCoarseStateRefresh(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{
			epoch:        time.Now().Add(350 * time.Millisecond),
			width:        100 * time.Millisecond,
			callDur:      5 * time.Microsecond,
			stateRefresh: 2 * time.Millisecond,
		}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
		deadline := time.After(20 * period)
		usable := 0
		timedOut := false
		for usable < 3 && !timedOut {
			select {
			case candidate := <-candidates:
				if !candidate.Anomalous && max(candidate.Uncertainty[0], candidate.Uncertainty[1]) <= usableUncertainty {
					usable++
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
		go func() { errCh <- Poll(ctx, lg, f, PollParams{}, candidates, nil) }()
		seen := make(map[int]bool)
		for pulse := 0; pulse < 18; {
			pulse = pulseIndex(nextUsable(candidates, usableUncertainty).Timestamp, f.epoch)
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
		for _, field := range []string{"extent=", "nextExtent=", "stateReads=", "bracket=", "failures=1"} {
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
		go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
		var first int
		for first <= 15 {
			first = pulseIndex(nextUsable(candidates, usableUncertainty).Timestamp, f.epoch)
		}
		cancel()
		<-errCh
		firstOffset, lastOffset := 3, 18
		if truncatesSubMillisecondSleeps {
			firstOffset, lastOffset = 3, 15
		}
		if first < f.offTo+firstOffset || first > f.offTo+lastOffset {
			t.Errorf("first edge after outage is pulse %d, want a fresh acquisition between pulses %d and %d",
				first, f.offTo+firstOffset, f.offTo+lastOffset)
		}
	})
}

// TestPollTrackingConverges checks that tracking shrinks the acquired extent
// to shrinkStop brackets plus a few state queries per pulse.
func TestPollTrackingConverges(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
		for pulseIndex(nextUsable(candidates, 2*time.Millisecond).Timestamp, f.epoch) < 100 {
		}
		start := f.calls.Load()
		for range 50 {
			nextUsable(candidates, 2*time.Millisecond)
		}
		perPulse := (f.calls.Load() - start) / 50
		cancel()
		<-errCh
		if perPulse > shrinkStop+4 {
			t.Errorf("steady state costs %d queries per pulse, want at most %d", perPulse, shrinkStop+4)
		}
	})
}

// TestPollDeliveryTailCostsIsolatedMisses checks that a recurring 1 ms
// delivery delay, beyond the extent that shrinkStop brackets give, is paid
// for with isolated misses only: each miss doubles the extent, which then
// shrinks back until the boundary is found again, so no two consecutive
// pulses are lost.
func TestPollDeliveryTailCostsIsolatedMisses(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 100 * time.Microsecond, lateEvery: 5, late: time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
		seen := make(map[int]bool)
		for last := 0; last < 500; {
			last = pulseIndex(nextUsable(candidates, usableUncertainty).Timestamp, f.epoch)
			seen[last] = true
		}
		cancel()
		<-errCh
		missed := 0
		for p := 400; p < 500; p++ {
			if !seen[p] {
				missed++
				if !seen[p-1] {
					t.Errorf("pulses %d and %d both missed, want misses isolated", p-1, p)
				}
			}
		}
		if missed > 25 {
			t.Errorf("%d of pulses 400-499 missed, want at most one per delivery-tail cycle", missed)
		}
	})
}

// TestPollAcquiresDespiteSleepJitter reproduces the daemon's sleep-overshoot
// regime: wakeups after a timer sleep run up to ~0.9 ms late, and one poll
// mid-acquisition stalls outright, stretching its bracket -- the noise the
// former bracket-comparison latch latched on, publishing millisecond-class
// samples from a still-wide window. Acquisition must ignore bracket noise and
// wait until the queries pace the loop, where the jitter vanishes and
// edges are located to the query time. The stall is timed to hit the
// bracket of the pulse-4 catch, mid-halving; the catch still advances
// acquisition normally.
func TestPollAcquiresDespiteSleepJitter(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 100 * time.Microsecond, wakeJitter: 900 * time.Microsecond,
			stallAfter: 3999 * time.Millisecond, stall: 3 * time.Millisecond}
		capture := &acquireCapture{Handler: slog.DiscardHandler}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, slog.New(capture), f, PollParams{Wait: f.wait}, candidates, nil) }()
		var got []CandidateEdge
		for len(got) < 20 {
			got = append(got, nextUsable(candidates, usableUncertainty))
		}
		cancel()
		<-errCh
		if first := pulseIndex(got[0].Timestamp, f.epoch); first > 15 {
			t.Errorf("first edge published at pulse %d, want acquisition despite the jitter plateau", first)
		}
		if truncatesSubMillisecondSleeps {
			// Once the spacing falls below Linux's sleep resolution, the reads
			// pace the loop. Two catches confirm that at the 31.25 ms window.
			if capture.window <= 16*time.Millisecond || capture.window > 32*time.Millisecond {
				t.Errorf("acquired at window %v, want two caught windows after sub-millisecond sleeps are truncated", capture.window)
			}
		} else {
			// Acquiring in the jitter plateau leaves the window at 15.625ms or
			// wider; the query-paced floor is reached at 3.9ms.
			if capture.window == 0 || capture.window > 8*time.Millisecond {
				t.Errorf("acquired at window %v, want the latch to hold out until the queries pace the loop", capture.window)
			}
		}
		for i, e := range got {
			pulse := pulseIndex(e.Timestamp, f.epoch)
			if i > 0 {
				prev := pulseIndex(got[i-1].Timestamp, f.epoch)
				if pulse > prev+2 {
					t.Errorf("edge %d is pulse %d after pulse %d, want convergence misses to be isolated", i, pulse, prev)
				}
			}
			// Usable candidates from the timer-paced stages carry the
			// overshoot inside their bracket; once the queries pace the loop
			// the edge is located to the query time.
			tol := usableUncertainty
			if i >= 10 {
				tol = 500 * time.Microsecond
			}
			if err := e.Timestamp.Sub(f.epoch) - time.Duration(pulse)*period; err < -tol || err > tol {
				t.Errorf("edge %d at pulse %d: error %v, want within %v", i, pulse, err, tol)
			}
		}
	})
}

// TestPollConfirmsQueryPacing checks that a single query slowdown does not
// open the publishing gate. On platforms with sub-millisecond sleeps, the
// slowdown covers the catch at the 15.625 ms window, where its 400 us queries
// outlast the 244 us target. On platforms that truncate those sleeps, it
// instead covers the 250 ms window, where a 5 ms query outlasts the 3.9 ms
// target but the following 1.95 ms spacing is still sleep-paced. Normal
// queries resume at the next pulse, so that catch resets the confirmation.
func TestPollConfirmsQueryPacing(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		slowAt := 6 * time.Second
		slowCallDur := 400 * time.Microsecond
		if truncatesSubMillisecondSleeps {
			slowAt = 2 * time.Second
			slowCallDur = 5 * time.Millisecond
		}
		f := &fakePulse{
			epoch:       time.Now().Add(350 * time.Millisecond),
			width:       100 * time.Millisecond,
			callDur:     20 * time.Microsecond,
			slowFrom:    slowAt - 10*time.Millisecond,
			slowTo:      slowAt + 10*time.Millisecond,
			slowCallDur: slowCallDur,
		}
		capture := &acquireCapture{Handler: slog.DiscardHandler}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, slog.New(capture), f, PollParams{}, candidates, nil) }()
		for capture.window == 0 {
			<-candidates
		}
		cancel()
		<-errCh
		if truncatesSubMillisecondSleeps {
			if capture.window <= 16*time.Millisecond || capture.window > 32*time.Millisecond {
				t.Errorf("acquired at window %v, want the one-window query slowdown suppressed before truncated sleeps pace the loop", capture.window)
			}
		} else if capture.window == 0 || capture.window >= 15*time.Millisecond {
			t.Errorf("acquired at window %v, want the one-window query slowdown suppressed", capture.window)
		}
	})
}

// TestPollNarrowPulse sweeps the pulse phase across the 7.8125 ms spacing of
// the second acquisition stage. The 2 ms pulse fits between the polls of the
// second and third stages at most phases, and a miss repeats the
// pulse-relative poll positions, so acquisition depends on the per-miss grid
// sweep finding the pulse. Tracking then holds lock normally, since the
// acquired spacing is below the pulse width; recovery from loss can widen
// the spacing beyond the width again and falls back to swept acquisition.
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
		go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
		var got []CandidateEdge
		for len(got) < 3 {
			got = append(got, nextUsable(candidates, 2*time.Millisecond))
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
	tests := []struct {
		name       string
		curInPulse bool
		curAt      time.Duration
		prevWidth  time.Duration
		curWidth   time.Duration
		deadline   time.Duration
		wantEdgeAt time.Duration
		wantMissed bool
	}{
		{
			name:       "transition before deadline",
			curInPulse: true,
			curAt:      4 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantEdgeAt: 2 * time.Millisecond,
		},
		{
			name:       "transition crossing deadline",
			curInPulse: true,
			curAt:      12 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantEdgeAt: 6 * time.Millisecond,
		},
		{
			name:     "no transition before deadline",
			curAt:    4 * time.Millisecond,
			deadline: 5 * time.Millisecond,
		},
		{
			name:       "no transition reaching deadline",
			curAt:      5 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "no transition crossing deadline",
			curAt:      6 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "bracket spanning a period",
			curInPulse: true,
			curAt:      1100 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "outer interval reaching a period",
			curInPulse: true,
			prevWidth:  200 * time.Millisecond,
			curAt:      800 * time.Millisecond,
			curWidth:   200 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "outer interval shorter than a period",
			curInPulse: true,
			prevWidth:  200 * time.Millisecond,
			curAt:      800 * time.Millisecond,
			curWidth:   199 * time.Millisecond,
			wantEdgeAt: 499750 * time.Microsecond,
		},
		{
			name:       "clock stepped backward inside off query",
			curInPulse: true,
			prevWidth:  -time.Millisecond,
			curAt:      4 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "clock stepped backward between queries",
			curInPulse: true,
			prevWidth:  2 * time.Millisecond,
			curAt:      time.Millisecond,
			curWidth:   2 * time.Millisecond,
			wantMissed: true,
		},
		{
			name:       "clock stepped backward inside on query",
			curInPulse: true,
			curAt:      4 * time.Millisecond,
			curWidth:   -time.Millisecond,
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
			prevEnd := clockReading{stamp: base.Add(tc.prevWidth), mono: base.Add(tc.prevWidth + monoSkew)}
			curEnd := clockReading{stamp: base.Add(tc.curAt + tc.curWidth), mono: base.Add(tc.curAt + tc.curWidth + monoSkew)}
			prev := reading{poll: poll{start: prevAt, end: prevEnd}}
			cur := reading{inPulse: tc.curInPulse, poll: poll{start: curAt, end: curEnd}}
			edge, missed := classify(prev, cur, base.Add(tc.deadline+monoSkew))
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

type pulseReaderFunc func() (bool, error)

func (f pulseReaderFunc) InPulse() (bool, error) { return f() }

// TestPollWindowUncertainty checks the published interval against the actual
// query endpoints, including unequal query widths and nanosecond rounding.
func TestPollWindowUncertainty(t *testing.T) {
	base := time.Unix(1_000, 0)
	for _, tc := range []struct {
		name        string
		offEnd      time.Duration
		onStart     time.Duration
		onEnd       time.Duration
		timestamp   time.Duration
		uncertainty [2]time.Duration
	}{
		{"equal queries", 20 * time.Microsecond, 30 * time.Microsecond, 50 * time.Microsecond,
			25 * time.Microsecond, [2]time.Duration{25 * time.Microsecond, 25 * time.Microsecond}},
		{"long off query", 80 * time.Microsecond, 100 * time.Microsecond, 120 * time.Microsecond,
			75 * time.Microsecond, [2]time.Duration{75 * time.Microsecond, 45 * time.Microsecond}},
		{"long on query", 20 * time.Microsecond, 40 * time.Microsecond, 120 * time.Microsecond,
			45 * time.Microsecond, [2]time.Duration{45 * time.Microsecond, 75 * time.Microsecond}},
		{"long gap", 20 * time.Microsecond, 100 * time.Microsecond, 120 * time.Microsecond,
			60 * time.Microsecond, [2]time.Duration{60 * time.Microsecond, 60 * time.Microsecond}},
		{"odd nanoseconds", 5, 8, 15, 6, [2]time.Duration{6, 9}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := []time.Time{base, base.Add(tc.offEnd), base.Add(tc.onStart), base.Add(tc.onEnd)}
			calls := 0
			candidates := make(chan CandidateEdge, 1)
			p := poller{
				widths: median.New[time.Duration](widthHistory),
				ctx:    context.Background(), lg: testLog, ceCh: candidates, nextEdge: base,
				r: pulseReaderFunc(func() (bool, error) {
					calls++
					return calls == 2, nil
				}),
				params: PollParams{
					MinSpacing: minSpacing,
					Now: func() time.Time {
						now := reads[0]
						reads = reads[1:]
						return now
					},
					Wait: func(context.Context, time.Time) (bool, error) { return false, nil },
				},
			}
			if o, _, err := p.pollWindow(time.Millisecond, minSpacing, true); err != nil || o == miss {
				t.Fatalf("pollWindow = %v, %v, want a catch", o, err)
			}
			ce := <-candidates
			if want := base.Add(tc.timestamp); !ce.Timestamp.Equal(want) {
				t.Errorf("timestamp = %v, want midpoint of poll midpoints %v", ce.Timestamp, want)
			}
			if ce.Uncertainty != tc.uncertainty {
				t.Errorf("uncertainty = %v, want %v", ce.Uncertainty, tc.uncertainty)
			}
			if want := [2]time.Duration{tc.offEnd, tc.onEnd - tc.onStart}; ce.PollWidths != want {
				t.Errorf("poll widths = %v, want %v", ce.PollWidths, want)
			}
			if !ce.Timestamp.Add(-ce.Uncertainty[0]).Equal(base) || !ce.Timestamp.Add(ce.Uncertainty[1]).Equal(ce.TRead) {
				t.Errorf("uncertainty does not reach the outer query endpoints: %+v", ce)
			}
		})
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

func (p errPin) InPulse() (bool, error) { return false, p.err }

func TestPollReaderError(t *testing.T) {
	e := errors.New("query failed")
	if err := Poll(context.Background(), testLog, errPin{err: e}, PollParams{}, nil, nil); err != e {
		t.Fatalf("Poll error = %v, want %v", err, e)
	}
}

// pulseIndex is the index of the pulse nearest t, counting from epoch.
func pulseIndex(t, epoch time.Time) int {
	return int((t.Sub(epoch) + period/2) / period)
}

// usableUncertainty mirrors the dispatcher's limit: a candidate that is not
// anomalous and is at most this uncertain is forwarded for timing.
const usableUncertainty = time.Millisecond

func nextUsable(candidates <-chan CandidateEdge, limit time.Duration) CandidateEdge {
	for {
		candidate := <-candidates
		if !candidate.Anomalous && max(candidate.Uncertainty[0], candidate.Uncertainty[1]) <= limit {
			return candidate
		}
	}
}

// TestPollFlagsStalledCatch checks that a tracking catch whose catching
// query stalled is flagged as anomalous, and nothing else is. The fake stalls the query
// that catches pulse 60 by 2 ms; the read that starts in the last 100 us
// before the edge is the catching one, so the stall is timed there.
func TestPollFlagsStalledCatch(t *testing.T) {
	runBubble(t, func(t *testing.T) {
		const stallPulse = 60
		f := &fakePulse{epoch: time.Now().Add(350 * time.Millisecond), width: 100 * time.Millisecond,
			callDur: 100 * time.Microsecond, stallAfter: stallPulse*period - 100*time.Microsecond, stall: 2 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		candidates := make(chan CandidateEdge)
		errCh := make(chan error, 1)
		go func() { errCh <- Poll(ctx, testLog, f, PollParams{}, candidates, nil) }()
		var got []CandidateEdge
		for len(got) == 0 || pulseIndex(got[len(got)-1].Timestamp, f.epoch) < stallPulse+2 {
			got = append(got, <-candidates)
		}
		cancel()
		if err := <-errCh; err != context.Canceled {
			t.Fatalf("Poll error = %v, want context.Canceled", err)
		}
		anomalous := 0
		for _, e := range got {
			pulse := pulseIndex(e.Timestamp, f.epoch)
			if pulse < 20 {
				continue
			}
			if e.Anomalous {
				anomalous++
				if pulse != stallPulse || max(e.Uncertainty[0], e.Uncertainty[1]) < 500*time.Microsecond {
					t.Errorf("anomalous catch at pulse %d with uncertainty %v, want only the stalled catch at pulse %d", pulse, e.Uncertainty, stallPulse)
				}
			} else if max(e.Uncertainty[0], e.Uncertainty[1]) > 500*time.Microsecond {
				t.Errorf("pulse %d with uncertainty %v not anomalous", pulse, e.Uncertainty)
			}
		}
		if anomalous != 1 {
			t.Errorf("%d anomalous catches, want 1", anomalous)
		}
	})
}
