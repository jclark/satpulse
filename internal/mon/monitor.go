package mon

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/logfile"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/sse"
)

const samplesToKeep = 30

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
	lf             logfile.LogFile
}

type stats struct {
	accumPhase accumPhase
	accumFreq  accumFreq
	interval   int
}

type Servo interface {
	Sample(ref ptime.Time, local ptime.ClockTime, delayed bool)
	FreqOffset() float64
	Locked(era ptime.Era) bool // this says whether it is currently using the PI controller
}

type MonitorConfig struct {
	LeapSecond   ptime.LeapSecond
	LogInterval  int
	ClockLogPath string
	RefClock     *ProxyRefClock
	Grandmaster  *Grandmaster
	SSECh        chan<- sse.Event
}

const ClockLogExtension = ".log"

func NewMonitor(servo Servo, lg *slog.Logger, cfg MonitorConfig) (*Monitor, error) {
	mon := &Monitor{
		servo:      servo,
		lg:         lg,
		leapSecond: cfg.LeapSecond,
		samples:    newSampleWindow(samplesToKeep),
		gm:         cfg.Grandmaster,
		rc:         cfg.RefClock,
		sseCh:      cfg.SSECh,
		stats:      stats{interval: cfg.LogInterval},
	}

	err := mon.lf.Open(cfg.ClockLogPath)
	if err != nil {
		return nil, err
	}
	mon.writeLogHeader()
	return mon, nil
}

func (mon *Monitor) Close() {
	mon.lg.Debug("closing monitor")
	mon.updateInSync(false)
	if mon.gm != nil {
		mon.gm.Close()
	}
	if mon.stats.accumPhase.n > 0 {
		mon.stats.flush(mon.lg)
	}
	mon.lf.Close(mon.lg)
}

func (mon *Monitor) ReopenLog() {
	mon.lf.Reopen(mon.lg)
	mon.writeLogHeader()
}

func (mon *Monitor) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	mon.addMissingOffsets(ref)
	off := local.T.Sub(ref)
	kind := sampleOK
	if !delayed && mon.isOutlier(off, local.Era) {
		kind = sampleOutlier
	} else {
		mon.servo.Sample(ref, local, delayed)
	}
	freq := mon.servo.FreqOffset()
	mon.recordSample(kind, off, local.Era, freq)
	mon.writeLogEntry(kind, ref, off, local.Era, freq)
	mon.sendEvent(kind, off, local.Era, freq)
	if delayed {
		return
	}
	inSync := mon.isInSync()
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
	for i := 1; i < diff; i++ {
		mon.recordSample(sampleMissing, 0, mon.samples.Era, mon.servo.FreqOffset())
	}
}

func (mon *Monitor) recordSample(kind sampleKind, off time.Duration, era ptime.Era, freq float64) {
	offSecs := off.Seconds()
	mon.samples.append(kind, offSecs, era)
	interval := mon.stats.interval
	if interval <= 0 {
		return
	}
	locked := mon.servo.Locked(era)
	if locked {
		if interval > 1 {
			mon.stats.sample(mon.lg, kind, offSecs, freq)
			return
		}
		if kind == sampleOK {
			mon.lg.Info("adjusting clock frequency", "off", off, "freq", freq)
			return
		}
	}
	// in case where sampleOK and not locked, the servo will log
	if kind == sampleMissing {
		mon.lg.Info("missed 1PPS sample")
		return
	}
	if kind == sampleOutlier {
		mon.lg.Info("outlier sample", "off", off, "freq", freq)
		return
	}
}

const logHeader = "# mjd offset freq outlier era\n"

func (mon *Monitor) writeLogHeader() {
	if mon.lf.File == nil {
		return
	}
	_, err := mon.lf.File.WriteString(logHeader)
	mon.lf.HandleWriteError(err, mon.lg)
}

func (mon *Monitor) writeLogEntry(kind sampleKind, ref ptime.Time, off time.Duration, era ptime.Era, freq float64) {
	if mon.lf.File == nil || kind == sampleMissing {
		return
	}
	outlierFlag := 0
	if kind == sampleOutlier {
		outlierFlag = 1
	}
	// Stable32 treats 0 as meaning a gap, so we output 1e-99 for 0.
	// This means we need to output things in seconds (using exponenential format), not nanoseconds.
	// Almost all the time, the absolute value of off will be < 100,
	// so we format to a width of 3 so the values including the sign will align.
	offStr := " 1e-99"
	if off != 0 {
		offStr = fmt.Sprintf("%3de-9", off)
	}
	// We represent dates in MJD, because that is what Stable32 can handle.
	// It also seems to be a standard approach in the timekeeping world.
	mjd := mon.leapSecond.TimeToMJD(ref)
	// 5 decimal places for MJD is sufficient to distinguish seconds; but we use 6 so it's clear when there's a gap
	_, err := fmt.Fprintf(mon.lf.File, "%.6f %s %.0f %d %d\n", mjd, offStr, freq, outlierFlag, uint64(era))
	mon.lf.HandleWriteError(err, mon.lg)
}

type SampleEvent struct {
	Offset            float64 `json:"offset"` // in nanoseconds
	Freq              float64 `json:"freq"`   // in parts per billion
	StepCount         uint32  `json:"stepCount"`
	StepCountChanging bool    `json:"stepCountChanging,omitempty"`
	Outlier           bool    `json:"outlier,omitempty"`
}

func (mon *Monitor) sendEvent(kind sampleKind, off time.Duration, era ptime.Era, freq float64) {
	if mon.sseCh == nil {
		return
	}
	stepCount, changing := era.StepCount()
	event, err := sse.Make("phc", &SampleEvent{
		Offset:            float64(off),
		StepCount:         uint32(stepCount),
		StepCountChanging: changing,
		Freq:              freq,
		Outlier:           kind == sampleOutlier,
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

const holdoverSecs = 10
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
	mon.lastSampleTime = mon.lastSampleTime.Add(time.Second)
	mon.lastRefTime = mon.lastRefTime.Add(time.Second)
	mon.recordSample(sampleMissing, 0, mon.samples.Era, mon.servo.FreqOffset())
	if t > holdoverSecs*time.Second {
		mon.updateInSync(false)
	}
}

func (mon *Monitor) isInSync() bool {
	return mon.samples.isInSync(mon.inSync, &defaultSampleConfig)
}

func (mon *Monitor) isOutlier(off time.Duration, era ptime.Era) bool {
	offSecs := off.Seconds()

	// if this offset isn't bad enough to take use out of sync,
	// then there's no need to consider it as an outlier
	// this should be a quick check that succeeds most of the time
	if math.Abs(offSecs) <= defaultSampleConfig.maxOffset {
		return false
	}
	// don't do outlier detection unless we are using the PI controller
	if !mon.servo.Locked(era) {
		return false
	}
	return mon.samples.madIsOutlier(offSecs, &defaultSampleConfig)
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

func (s *stats) sample(lg *slog.Logger, kind sampleKind, off float64, freq float64) {
	s.accumPhase.add(kind, off)
	s.accumFreq.add(freq)
	if s.accumPhase.n == s.interval {
		s.flush(lg)
	}
}

func (s *stats) flush(lg *slog.Logger) {
	lg.Info("summary",
		"absOffMax", fmt.Sprintf("%.0f", s.accumPhase.maxAbs*1e9),
		"absOffMean", fmt.Sprintf("%.1f", s.accumPhase.meanAbs()*1e9),
		"offRMS", fmt.Sprintf("%.1f", s.accumPhase.rms()*1e9),
		"freqMean", fmt.Sprintf("%.0f", s.accumFreq.mean()),
		"freqStdDev", fmt.Sprintf("%.0f", s.accumFreq.stddev()),
		"nSecs", s.accumPhase.n,
		"nMissing", s.accumPhase.nMissing,
		"nOutliers", s.accumPhase.nOutliers)
	s.accumPhase = accumPhase{}
	s.accumFreq = accumFreq{}
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
