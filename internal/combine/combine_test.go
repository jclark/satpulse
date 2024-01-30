package combine

import (
	"io"
	"log/slog"
	"math/bits"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

type testEvent interface {
	emit(c *Combiner)
	t() time.Time
}

type testMsgEvent struct {
	gpsprot.TimeMsg
	tRead time.Time
}

func (e testMsgEvent) emit(c *Combiner) {
	c.TimeMsg(e.TAITime, e.tRead, e.PulseOffset, e.Ref)
}

func (e testMsgEvent) t() time.Time { return e.tRead }

func (e pulseEdge) emit(c *Combiner) {
	c.PulseEdge(e.ClockTime, e.tRead)
}

func (e pulseEdge) t() time.Time { return e.tRead }

func TestCombinerNavSoln1(t *testing.T) {
	combinerTest(t, 1000, genNavSoln, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerNavSoln2(t *testing.T) {
	combinerTest(t, 1000, genNavSoln, PulseType{EdgesPerPulse: 2, PulseWidth: time.Second / 10}, 1)
}

func TestDetectPulseWidth(t *testing.T) {
	combinerTest(t, 1000, genNavSoln|genDetectPulseWidth, PulseType{EdgesPerPulse: 2, PulseWidth: time.Second / 10}, 1)
}

func TestCombinerPostPulse1(t *testing.T) {
	combinerTest(t, 10000, genPostPulse, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerPostPulse2(t *testing.T) {
	combinerTest(t, 1000, genPostPulse, PulseType{EdgesPerPulse: 2, PulseWidth: time.Second / 10}, 1)
}

func TestCombinerPrePulse1(t *testing.T) {
	combinerTest(t, 1000, genPrePulse|genNavSoln, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerPrePulseOff1(t *testing.T) {
	combinerTest(t, 1000, genPrePulse|genNavSoln|genPulseOff, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerPostPulseOff1(t *testing.T) {
	combinerTest(t, 1000, genPostPulse|genPulseOff, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerFreqLow(t *testing.T) {
	combinerTest(t, 1000, genNavSoln|genFreqLow, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerFreqHigh(t *testing.T) {
	combinerTest(t, 1000, genNavSoln|genFreqHigh, PulseType{EdgesPerPulse: 1}, 1)
}

const (
	genPrePulse uint = 1 << iota
	genPostPulse
	genNavSoln
	genPulseOff
	genDetectPulseWidth
	genFreqLow
	genFreqHigh
)

func combinerTest(t *testing.T, nSecs int, flags uint, pt PulseType, nDelayed int) {
	events, samples := genTestData(nSecs, flags, pt, nil)
	sampler := newTestSampler(t, samples, nDelayed)
	pulseWidth := pt.PulseWidth
	if flags&genDetectPulseWidth != 0 {
		pt.PulseWidth = 0
	}
	cfg := Config{}
	cfg.SetDefault(pt)
	if pt.EdgesPerPulse == 1 {
		// this is for CM4
		cfg.PulsePollInterval = 250 * time.Millisecond
	}
	c, err := NewCombiner(pt, sampler, testLogger(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		e.emit(c)
	}
	sampler.Done()
	if c.PulseDiscardCount > 0 {
		t.Errorf("discarded %d pulses, want 0", c.PulseDiscardCount)
	}
	if flags&genDetectPulseWidth != 0 {
		w := c.edgeFilter.(*detectPulseWidthFilter).avgPulseWidth
		if (w - pulseWidth).Abs() > cfg.PulseWidthAccuracy+cfg.PulseReadDelay {
			t.Errorf("avgPulseWidth = %v, want about %v", w, pt.PulseWidth)
		}
	}
}

func testLogger() *slog.Logger {
	handler := slog.NewTextHandler(io.Discard, nil)
	return slog.New(handler)
}

type testSampler struct {
	t        *testing.T
	expected []sampleData
	i        int
	nDelayed int // expect this many to be delayed initially
}

var _ Sampler = (*testSampler)(nil)

func (s *testSampler) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	for {
		if s.i >= len(s.expected) {
			s.t.Errorf("excess sample")
			return
		}
		expected := s.expected[s.i]
		expectDelayed := s.i < s.nDelayed
		s.i++
		refSec := ref.Round(time.Second)
		if refSec <= expected.sec {
			expectedRef := expected.refTime()
			if expectedRef != ref {
				s.t.Errorf("sample %d: ref = %v, want %v", s.i, ref, expectedRef)
			} else if expected.pulse.ClockTime != local {
				s.t.Errorf("sample %d: local = %v, want %v", s.i, local, expected.pulse.ClockTime)
			} else if expectDelayed != delayed {
				s.t.Errorf("sample %d: delayed = %v, want %v", s.i, delayed, expectDelayed)
			}
			break
		}
		// ref >= expected.sec
		s.t.Errorf("missing sample %d for second %v", s.i, expected.sec)
	}
}

func (s *testSampler) Done() {
	if s.i < len(s.expected) {
		s.t.Errorf("too few samples: got %d, want %d", s.i, len(s.expected))
	}
}

func newTestSampler(t *testing.T, expected []sampleData, nDelayed int) *testSampler {
	return &testSampler{t: t, expected: expected, nDelayed: nDelayed}
}

const randSeed = 42

func genTestData(nSecs int, flags uint, pt PulseType, cfgOpt *Config) ([]testEvent, []sampleData) {
	var cfg Config
	if cfgOpt != nil {
		cfg = *cfgOpt
	} else {
		cfg.SetDefault(pt)
	}
	r := rand.New(rand.NewSource(randSeed))
	readStart := time.Now()
	taiStart := ptime.Time(0)
	taiStart = taiStart.Add(time.Hour * 24 * 365 * 50)
	phcBase := ptime.Time(42)
	eventsPerPulse := bits.OnesCount(flags&(genPrePulse|genPostPulse|genNavSoln)) + pt.EdgesPerPulse
	events := make([]testEvent, 0, nSecs*eventsPerPulse)
	samples := make([]sampleData, nSecs)
	era := ptime.Era(0)        // uncertain
	eraSecs := min(nSecs/7, 5) // 7 is 1 + 18/3; works woth phcBase stepping below
	var pulseOff time.Duration
	// if we are using just prePulse, then we won't get a pulseOff for the first pulse
	if flags&(genPulseOff|genPostPulse) == (genPulseOff | genPostPulse) {
		pulseOff = 7 // first pulse offset
	}
	for i := 0; i < nSecs; i++ {
		elapsed := time.Duration(i) * time.Second
		tRead := readStart.Add(elapsed)
		tai := taiStart.Add(elapsed)
		delay := randD(r, cfg.SerialDelay)
		if flags&genPostPulse != 0 {
			events = append(events, testMsgEvent{
				tRead: tRead.Add(delay),
				TimeMsg: gpsprot.TimeMsg{
					TAITime:     tai,
					Ref:         gpsprot.PostPulse,
					PulseOffset: maybePulseOff(pulseOff, flags),
				},
			})
		}
		if flags&genNavSoln != 0 {
			delay += randD(r, cfg.NavSolnDelay)
			events = append(events, testMsgEvent{
				tRead:   tRead.Add(delay),
				TimeMsg: gpsprot.TimeMsg{TAITime: tai, Ref: gpsprot.NavSolution},
			})
		}
		switch i % eraSecs {
		case 0:
			era++ // go from uncertain to certain
		case eraSecs - 1:
			// Step the PHC, making it closer to the correct time
			// Start of with PHC is unset (so about 50 years in nanoseconds, which is 1.5e18)
			// Divide by 1000 six times will get us down to nanoseconds
			phcBase += (taiStart - phcBase) / 1000
			era++ // go from certain to uncertain
		}
		phc := phcBase.Add(phcScaleDuration(elapsed, flags))
		pulseDelay := randD(r, cfg.PulseReadDelay+cfg.PulsePollInterval)
		edge := pulseEdge{
			ClockTime: ptime.ClockTime{
				T:   phc.Add(randSignedD(r, time.Microsecond)),
				Era: era,
			},
			tRead: tRead.Add(pulseDelay),
		}
		events = append(events, edge)
		samples[i] = sampleData{
			sec:         tai,
			pulseOffset: pulseOff,
			pulse:       edge,
		}
		if pt.EdgesPerPulse == 2 {
			pulseDelay = randD(r, cfg.PulseReadDelay+cfg.PulsePollInterval)
			edge = pulseEdge{
				ClockTime: ptime.ClockTime{
					T:   phc.Add(phcScaleDuration(pt.PulseWidth, flags) + randSignedD(r, cfg.PulseWidthAccuracy)),
					Era: era,
				},
				tRead: tRead.Add(pt.PulseWidth + pulseDelay),
			}
			events = append(events, edge)
		}
		// PrePulse for the next second
		if flags&genPulseOff != 0 {
			pulseOff = randSignedD(r, time.Nanosecond*15)
			if pulseOff == 0 {
				pulseOff++
			}
		}
		if flags&genPrePulse != 0 {
			delay := randD(r, cfg.SerialDelay+time.Second/5)
			events = append(events, testMsgEvent{
				tRead: tRead.Add(delay),
				TimeMsg: gpsprot.TimeMsg{
					TAITime:     tai.Add(time.Second),
					Ref:         gpsprot.PrePulse,
					PulseOffset: maybePulseOff(pulseOff, flags),
				},
			})
		}
	}
	slices.SortFunc(events, func(a, b testEvent) int { return a.t().Compare(b.t()) })
	return events, samples
}

func phcScaleDuration(t time.Duration, flags uint) time.Duration {
	percent := 0
	switch flags & (genFreqLow | genFreqHigh) {
	case genFreqLow:
		percent = 90
	case genFreqHigh:
		percent = 110
	default:
		return t
	}
	return time.Duration((t * time.Duration(percent)) / 100)
}

func maybePulseOff(pulseOff time.Duration, flags uint) *time.Duration {
	if flags&genPulseOff != 0 {
		return &pulseOff
	}
	return nil
}

// randSignedD returns a random duration in the range [-d, d].
func randSignedD(r *rand.Rand, d time.Duration) time.Duration {
	// if d is 2, we want the interval [-2, 2]
	return time.Duration(r.Int63n(int64(d)*2+1) - int64(d))
}

func randD(r *rand.Rand, d time.Duration) time.Duration {
	return time.Duration(r.Int63n(int64(d)))
}

func TestSearch(t *testing.T) {
	secStates := secMsgList{
		{sec: 100},
		{sec: 200},
		{sec: 300},
		{sec: 400},
	}

	testCases := []struct {
		sec      ptime.Time
		expected int
	}{
		{100, 0},
		{50, 0},
		{500, 4},
		{400, 3},
	}

	for _, tc := range testCases {
		i := secStates.search(tc.sec)
		if i != tc.expected {
			t.Errorf("search(secStates, %v) = %v, want %v", tc.sec, i, tc.expected)
		}
	}
}
