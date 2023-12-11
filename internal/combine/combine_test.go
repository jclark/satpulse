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
	c.TimeMsg(e.TAITime, e.tRead, nil, e.Ref)
}

func (e testMsgEvent) t() time.Time { return e.tRead }

func (e pulseEdge) emit(c *Combiner) {
	c.PulseEdge(e.ClockTime, e.tRead)
}

func (e pulseEdge) t() time.Time { return e.tRead }

func TestCombinerNavSoln1(t *testing.T) {
	combinerTest(t, 10000, genNavSoln, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerNavSoln2(t *testing.T) {
	combinerTest(t, 1000, genNavSoln, PulseType{EdgesPerPulse: 2, PulseWidth: time.Second / 10}, 1)
}

func TestCombinerPostPulse1(t *testing.T) {
	combinerTest(t, 10000, genPostPulse, PulseType{EdgesPerPulse: 1}, 1)
}

func TestCombinerPostPulse2(t *testing.T) {
	combinerTest(t, 1000, genPostPulse, PulseType{EdgesPerPulse: 2, PulseWidth: time.Second / 10}, 1)
}

const (
	genPrePulse uint = 1 << iota
	genPostPulse
	genNavSoln
	genPulseOff
)

func combinerTest(t *testing.T, nSecs int, flags uint, pt PulseType, nDelayed int) {
	events, samples := genEvents(nSecs, flags, pt, nil)
	sampler := newTestSampler(t, samples, nDelayed)
	c := NewCombiner(pt, sampler, testLogger(), nil)
	for _, e := range events {
		e.emit(c)
	}
	sampler.Done()
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
		if ref <= expected.sec {
			if expected.sec != ref {
				s.t.Errorf("sample %d: ref = %v, want %v", s.i, ref, expected.sec)
			} else if expected.pulse.ClockTime != local {
				s.t.Errorf("sample %d: local = %v, want %v", s.i, local, expected.pulse.ClockTime)
			} else if expectDelayed != delayed {
				s.t.Errorf("sample %d: delayed = %v, want %v", s.i, delayed, expectDelayed)
			}
			break
		}
		// ref >= expected.sec
		s.t.Errorf("missing sample for second %v", expected.sec)
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

func genEvents(nSecs int, flags uint, pt PulseType, cfgOpt *Config) ([]testEvent, []sampleData) {
	var cfg Config
	if cfgOpt != nil {
		cfg = *cfgOpt
	} else {
		cfg = DefaultConfig(pt)
	}
	r := rand.New(rand.NewSource(randSeed))
	readStart := time.Now()
	taiStart := ptime.Time(0)
	taiStart = taiStart.Add(time.Hour * 24 * 365 * 50)
	eventsPerPulse := bits.OnesCount(flags&(genPrePulse|genPostPulse|genNavSoln)) + pt.EdgesPerPulse
	events := make([]testEvent, 0, nSecs*eventsPerPulse)
	samples := make([]sampleData, nSecs)
	era := ptime.Era(1)
	for i := 0; i < nSecs; i++ {
		elapsed := time.Duration(i) * time.Second
		tRead := readStart.Add(elapsed)
		tai := taiStart.Add(elapsed)
		delay := randD(r, cfg.SerialDelay)
		if flags&genPostPulse != 0 {
			events = append(events, testMsgEvent{
				tRead:   tRead.Add(delay),
				TimeMsg: gpsprot.TimeMsg{TAITime: tai, Ref: gpsprot.PostPulse},
			})
		}
		if flags&genNavSoln != 0 {
			delay += randD(r, cfg.NavSolnDelay)
			events = append(events, testMsgEvent{
				tRead:   tRead.Add(delay),
				TimeMsg: gpsprot.TimeMsg{TAITime: tai, Ref: gpsprot.NavSolution},
			})
		}
		pulseDelay := randD(r, cfg.PulseReadDelay+cfg.PulsePollInterval)
		edge := pulseEdge{
			ClockTime: ptime.ClockTime{
				T:   tai.Add(randSignedD(r, time.Microsecond)),
				Era: era,
			},
			tRead: tRead.Add(pulseDelay),
		}
		events = append(events, edge)
		samples[i] = sampleData{
			sec:         tai,
			pulseOffset: 0,
			pulse:       edge,
		}
		if pt.EdgesPerPulse == 2 {
			pulseDelay = randD(r, cfg.PulseReadDelay+cfg.PulsePollInterval)
			edge = pulseEdge{
				ClockTime: ptime.ClockTime{
					T:   tai.Add(pt.PulseWidth + randSignedD(r, cfg.PulseWidthAccuracy)),
					Era: era,
				},
				tRead: tRead.Add(pt.PulseWidth + pulseDelay),
			}
			events = append(events, edge)
		}
	}
	slices.SortFunc(events, func(a, b testEvent) int { return a.t().Compare(b.t()) })
	return events, samples
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
