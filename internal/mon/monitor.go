package mon

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/logfile"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/sse"
)

const sampleWindowSize = 60

// Warn if the 1PPS signal stops for more than this number of seconds.
// Occasional missed samples are common, so we don't want to warn about them.
const ppsStoppedWarn = 3

type Monitor struct {
	samples        *sampleState
	lastSampleTime time.Time
	leapSecond     ptime.LeapSecond
	servo          Servo // never nil
	lg             *slog.Logger
	gm             *Grandmaster   // maybe nil
	rc             *ProxyRefClock // maybe nil
	syncState      syncState
	paused         bool
	lastRefTime    ptime.Time
	ppsStopped     int // number of missing samples recorded since 1PPS stopped
	sseCh          chan<- sse.Event
	stats          stats
	lf             logfile.LogFile
	cfg            syncConfig
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
	Reset()
}

type MonitorConfig struct {
	LeapSecond    ptime.LeapSecond
	LogInterval   int
	ClockLogPath  string
	ClockAccuracy time.Duration
	RefClock      *ProxyRefClock
	Grandmaster   *Grandmaster
	SSECh         chan<- sse.Event
}

const ClockLogExtension = ".log"

func NewMonitor(servo Servo, lg *slog.Logger, cfg MonitorConfig) (*Monitor, error) {
	mon := &Monitor{
		servo:      servo,
		lg:         lg,
		leapSecond: cfg.LeapSecond,
		samples:    newSampleState(sampleWindowSize),
		gm:         cfg.Grandmaster,
		rc:         cfg.RefClock,
		sseCh:      cfg.SSECh,
		stats:      stats{interval: cfg.LogInterval},
		syncState:  noSync,
		cfg:        defaultSyncConfig,
	}

	err := mon.lf.Open(cfg.ClockLogPath)
	if err != nil {
		return nil, err
	}
	err = mon.gm.SetClockAccuracy(cfg.ClockAccuracy)
	if err != nil {
		return nil, err
	}
	mon.cfg.maxOffset = cfg.ClockAccuracy.Seconds()
	mon.writeLogHeader()
	return mon, nil
}

func (mon *Monitor) Close() {
	mon.lg.Debug("closing monitor")
	mon.updateSyncState(noSync)
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

func (mon *Monitor) Pause() {
	mon.updateSyncState(noSync)
	mon.servo.Reset()
	mon.paused = true
}

func (mon *Monitor) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	mon.addMissingOffsets(ref)
	mon.paused = false
	off := local.T.Sub(ref)
	kind := sampleOK
	if !delayed && mon.isOutlier(off, local.Era) {
		kind = sampleOutlier
	} else {
		mon.servo.Sample(ref, local, delayed)
	}
	freq := mon.servo.FreqOffset()
	mon.recordSample(kind, off, local.Era, freq)
	nextState := mon.nextSyncState()
	mon.writeLogEntry(kind, ref, off, local.Era, freq, nextState)
	mon.sendEvent(kind, off, local.Era, freq, nextState)
	if delayed {
		return
	}
	now := time.Now()
	mon.lastSampleTime = now
	if mon.ppsStopped >= ppsStoppedWarn {
		mon.lg.Warn("1PPS signal restored")
	}
	mon.ppsStopped = 0
	mon.updateSyncState(nextState)
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
		mon.recordSample(sampleMissing, 0, mon.samples.era, mon.servo.FreqOffset())
	}
}

func (mon *Monitor) recordSample(kind sampleKind, off time.Duration, era ptime.Era, freq float64) {
	offSecs := off.Seconds()
	mon.samples.sample(kind, offSecs, era, mon.syncState, &mon.cfg)
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
		if !mon.paused {
			mon.lg.Info("missed 1PPS sample")
		}
		return
	}
	if kind == sampleOutlier {
		mon.lg.Info("outlier sample", "off", off, "freq", freq)
		return
	}
}

const logHeader = "# date time offset freq outlier era sync\n"

func (mon *Monitor) writeLogHeader() {
	if mon.lf.File == nil {
		return
	}
	_, err := mon.lf.File.WriteString(logHeader)
	mon.lf.HandleWriteError(err, mon.lg)
}

func (mon *Monitor) writeLogEntry(kind sampleKind, ref ptime.Time, off time.Duration, era ptime.Era, freq float64, syncState syncState) {
	if mon.lf.File == nil || kind == sampleMissing {
		return
	}
	outlierFlag := 0
	if kind == sampleOutlier {
		outlierFlag = 1
	}
	// Almost all the time, the absolute value of off will be < 100,
	// so we format to a width of 3 so the values including the sign will align.
	_, err := fmt.Fprintf(mon.lf.File, "%s %3d %.0f %d %d %d\n", logDateTime(ref, mon.leapSecond), off, freq, outlierFlag, uint64(era), int(syncState))
	mon.lf.HandleWriteError(err, mon.lg)
}

func logDateTime(ref ptime.Time, ls ptime.LeapSecond) string {
	// We need to round because the reference time (which will be a whole number of seconds)
	// may have had a pulse offset (sawtooth correction) of a few nanoseconds applied.
	s := ls.FormatTime(ref.Round(time.Second))
	s = strings.Replace(s, "T", " ", 1)
	return strings.Replace(s, "Z", "", 1)
}

type SampleEvent struct {
	Offset            float64 `json:"offset"` // in nanoseconds
	Freq              float64 `json:"freq"`   // in parts per billion
	StepCount         uint32  `json:"stepCount"`
	StepCountChanging bool    `json:"stepCountChanging,omitempty"`
	Outlier           bool    `json:"outlier,omitempty"`
	SyncState         string  `json:"syncState"`
}

func (mon *Monitor) sendEvent(kind sampleKind, off time.Duration, era ptime.Era, freq float64, syncState syncState) {
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
		SyncState:         syncState.String(),
	})
	if err != nil {
		mon.lg.Error("error creating sample event", "err", err)
		return
	}
	mon.sseCh <- event
}

func (mon *Monitor) SysSample(ref ptime.Time, sys time.Time) {
	if mon.rc == nil || mon.syncState == noSync {
		return
	}
	err := mon.rc.Sample(sys, ref, mon.leapSecond)
	if err != nil {
		mon.lg.Warn(err.Error())
	}
}

const sampleIntervalMax = time.Second + time.Second/2

// This is called 4 times per second.
func (mon *Monitor) Tick(now time.Time) {
	if mon.syncState == noSync {
		return
	}
	t := now.Sub(mon.lastSampleTime)

	if t <= sampleIntervalMax {
		return
	}

	mon.ppsStopped++
	if mon.ppsStopped == ppsStoppedWarn {
		mon.lg.Warn("1PPS signal stopped")
	}
	// At this point we are overdue for a new sample.
	// Note that we are updating lastSampleTime to one second after the previous sample time rather than now.
	mon.lastSampleTime = mon.lastSampleTime.Add(time.Second)
	mon.lastRefTime = mon.lastRefTime.Add(time.Second)
	mon.recordSample(sampleMissing, 0, mon.samples.era, mon.servo.FreqOffset())
	mon.updateSyncState(mon.nextSyncState())
}

func (mon *Monitor) nextSyncState() syncState {
	state := mon.samples.nextSyncState(mon.syncState, &mon.cfg)
	mon.lg.Debug("computed next sync state", "syncState", state, "emaOffset", mon.samples.emaOffset, "invalidWeight", mon.samples.invalidWeight, "goodSampleCount", mon.samples.goodSampleCount)
	return state
}

func (mon *Monitor) isOutlier(off time.Duration, era ptime.Era) bool {
	offSecs := off.Seconds()
	if !mon.servo.Locked(era) {
		return false
	}
	return mon.samples.madIsOutlier(offSecs, &mon.cfg)
}

func (mon *Monitor) updateSyncState(state syncState) {
	if state != mon.syncState {
		mon.lg.Warn("synchronization status has changed", "newSyncState", state)
		mon.syncState = state
		mon.samples.resetForSyncChange(state)
	}
	mon.gmUpdate()
}

func (mon *Monitor) gmUpdate() {
	if mon.gm != nil && !mon.lastRefTime.IsZero() {
		mon.gm.Update(mon.syncState, mon.leapSecond.StateAt(mon.lastRefTime))
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
