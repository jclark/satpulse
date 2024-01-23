package mon

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/sse"
)

const samplesToKeep = 60

type Monitor struct {
	samples        *sampleWindow
	lastSampleTime time.Time
	leapSecond     ptime.LeapSecond
	servo          Servo // never nil
	lg             *slog.Logger
	gm             *Grandmaster   // maybe nil
	rc             *ProxyRefClock // maybe nil
	inSync         bool
	lastRefTime    ptime.Time
	ppsStopped     bool
	sseCh          chan<- sse.Event
	stats          stats
}

type stats struct {
	interval   int
	accumPhase accumPhase
	accumFreq  accumFreq
}

type sampleKind int

const (
	sampleMissing sampleKind = iota
	sampleOK
	sampleOutlier
)

type sampleData struct {
	off  float64
	kind sampleKind
}

type sampleWindow struct {
	window[sampleData]
}

type Servo interface {
	Sample(ref ptime.Time, local ptime.ClockTime, delayed bool)
	FreqOffset() float64
	Locked() bool // this says whether it is currently using the PI controller
}

func NewMonitor(leapSecond ptime.LeapSecond, servo Servo, gm *Grandmaster, rc *ProxyRefClock, lg *slog.Logger, sseCh chan<- sse.Event) *Monitor {
	mon := &Monitor{
		leapSecond: leapSecond,
		servo:      servo,
		lg:         lg,
		samples:    newSampleWindow(samplesToKeep),
		gm:         gm,
		rc:         rc,
		sseCh:      sseCh,
		stats:      stats{interval: 30},
	}
	return mon
}

func (mon *Monitor) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	mon.addMissingOffsets(ref)
	off := local.T.Sub(ref)
	offSecs := off.Seconds()
	mon.servo.Sample(ref, local, delayed)

	freq := mon.servo.FreqOffset()
	if mon.servo.Locked() {
		if mon.stats.interval == 0 {
			mon.lg.Info("adjusting clock frequency", "off", off, "freq", freq)
		} else if mon.stats.accum(sampleOK, offSecs, freq) {
			mon.stats.log(mon.lg)
			mon.stats.clear()
		}
	}
	mon.sendEvent(off, local.Era, freq)
	mon.samples.append(offSecs, sampleOK)
	if delayed {
		return
	}
	inSync := mon.samplesInSync()
	now := time.Now()
	mon.lastSampleTime = now
	if mon.ppsStopped {
		mon.lg.Warn("1PPS signal restored")
		mon.ppsStopped = false
	}
	mon.updateInSync(inSync)
}

func (mon *Monitor) addMissingOffsets(ref ptime.Time) {
	ref = ref.Round(time.Second)
	lastRef := mon.lastRefTime
	mon.lastRefTime = ref
	if lastRef.IsZero() {
		return
	}
	diff := int(ref.Sub(lastRef) / time.Second)
	if diff <= 1 {
		return
	}
	mon.lg.Warn("missed 1PPS samples", "n", diff-1)
	freq := mon.servo.FreqOffset()
	locked := mon.servo.Locked()
	// XXX we should add a missing sample in Tick, then we wouldn't need to protect against an avalanche of missing samples here
	logged := false
	for i := 1; i < diff; i++ {
		mon.samples.append(0, sampleMissing)
		if locked && mon.stats.interval > 0 && mon.stats.accum(sampleMissing, 0, freq) {
			if !logged {
				mon.stats.log(mon.lg)
				logged = true
			}
			mon.stats.clear()
		}
	}
}

type SampleEvent struct {
	Offset            float64 `json:"offset"` // in nanoseconds
	Freq              float64 `json:"freq"`   // in parts per billion
	StepCount         uint32  `json:"stepCount"`
	StepCountChanging bool    `json:"stepCountChanging,omitempty"`
}

func (mon *Monitor) sendEvent(off time.Duration, era ptime.Era, freq float64) {
	if mon.sseCh == nil {
		return
	}
	stepCount, changing := era.StepCount()
	event, err := sse.Make("phc", &SampleEvent{
		Offset:            float64(off),
		StepCount:         uint32(stepCount),
		StepCountChanging: changing,
		Freq:              freq,
	})
	if err != nil {
		mon.lg.Error("error creating sample event", "err", err)
		return
	}
	mon.sseCh <- event
}

func (mon *Monitor) SysSample(ref ptime.Time, sys time.Time) {
	if mon.rc == nil || !mon.inSync {
		return
	}
	err := mon.rc.Sample(sys, ref, mon.leapSecond)
	if err != nil {
		mon.lg.Warn(err.Error())
	}
}

const holdoverDuration = 10 * time.Second
const sampleIntervalMax = time.Second + time.Second/2

func (mon *Monitor) Tick(now time.Time) {
	if !mon.inSync {
		return
	}
	t := now.Sub(mon.lastSampleTime)

	if t <= sampleIntervalMax {
		return
	}

	if !mon.ppsStopped {
		mon.lg.Warn("1PPS signal stopped")
		mon.ppsStopped = true
	}

	if t > holdoverDuration {
		mon.updateInSync(false)
	}
}

// For sync, require that the maximum absolute offset is less than 50ns.
// Reasoning is we are claiming 100ns accuracy, and we need to budget for other sources of error,
// specifically errors in GPS signal
const syncMaxOffsetSecs = 50e-9
const syncNOffsets = 5
const syncMaxMissing = 2

func (mon *Monitor) samplesInSync() bool {
	w := mon.samples
	if w.length() < syncNOffsets {
		return false
	}
	a := w.accum(syncNOffsets)
	if a.nMissing > 0 {
		// if we've gone out of sync, don't allow any missing values;
		// otherwise allow up to syncMaxMissing
		if !mon.inSync || a.nMissing > syncMaxMissing {
			return false
		}
	}
	return a.maxAbs < syncMaxOffsetSecs
}

func (mon *Monitor) updateInSync(inSync bool) {
	if inSync != mon.inSync {
		mon.lg.Warn("synchronization status has changed", "inSync", inSync)
		mon.inSync = inSync
	}
	mon.gmUpdate()
}

func (mon *Monitor) gmUpdate() {
	if mon.gm != nil && !mon.lastRefTime.IsZero() {
		mon.gm.Update(mon.inSync, mon.leapSecond.StateAt(mon.lastRefTime))
	}
}

func (mon *Monitor) SetLeapSecond(leapSecond ptime.LeapSecond) {
	if leapSecond == mon.leapSecond {
		return
	}
	mon.leapSecond = leapSecond
	mon.gmUpdate()
}

func newSampleWindow(n int) *sampleWindow {
	return &sampleWindow{
		window: *newWindow[sampleData](n),
	}
}

func (w *sampleWindow) append(off float64, kind sampleKind) {
	w.window.append(sampleData{off: off, kind: kind})
}

func (w *sampleWindow) accum(n int) accumPhase {
	var a accumPhase
	w.iterate(func(i int, s sampleData) bool {
		if i >= n {
			return false
		}
		a.add(s.kind, s.off)
		return true
	})
	return a
}

func (s *stats) accum(kind sampleKind, off float64, freq float64) bool {
	s.accumPhase.add(kind, off)
	s.accumFreq.add(freq)
	return s.accumPhase.n == s.interval
}

func (s *stats) clear() {
	s.accumPhase = accumPhase{}
	s.accumFreq = accumFreq{}
}

func (s *stats) log(lg *slog.Logger) {
	lg.Info("stats", "absOffMean", s.accumPhase.meanAbs(), "offRMS", s.accumPhase.rms(), "maxAbsOff", s.accumPhase.maxAbs, "nMissing", s.accumPhase.nMissing, "nOutliers", s.accumPhase.nOutliers, "freqMean", s.accumFreq.mean(), "freqStddev", s.accumFreq.stddev())
}

type accumPhase struct {
	sumAbs     float64
	sumSquares float64
	maxAbs     float64
	n          int
	nMissing   int
	nOutliers  int
}

type accumFreq struct {
	sum                float64
	sumMeanDiffSquares float64
	n                  int
}

func (a *accumPhase) add(kind sampleKind, v float64) {
	a.n++
	switch kind {
	case sampleOK:
		av := math.Abs(v)
		a.sumAbs += av
		a.sumSquares += v * v
		a.maxAbs = math.Max(a.maxAbs, av)
	case sampleMissing:
		a.nMissing++
	case sampleOutlier:
		a.nOutliers++
	}
}

func (a *accumFreq) add(v float64) {
	a.n++
	a.sum += v
	md := v - a.sum/float64(a.n)
	a.sumMeanDiffSquares += md * md
}

func (a *accumPhase) meanAbs() float64 {
	return a.sumAbs / float64(a.n)
}

func (a *accumPhase) rms() float64 {
	return math.Sqrt(a.sumSquares / float64(a.n))
}

func (a *accumFreq) mean() float64 {
	return a.sum / float64(a.n)
}

func (a *accumFreq) stddev() float64 {
	return math.Sqrt(a.sumMeanDiffSquares / float64(a.n))
}
