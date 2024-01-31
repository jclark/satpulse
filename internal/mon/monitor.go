package mon

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
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
	logFile        *os.File
	logPath        string
}

type stats struct {
	accumPhase accumPhase
	accumFreq  accumFreq
	interval   int
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

type MonitorConfig struct {
	LeapSecond   ptime.LeapSecond
	LogInterval  int
	ClockLogPath string
	RefClock     *ProxyRefClock
	Grandmaster  *Grandmaster
	SSECh        chan<- sse.Event
}

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
		logPath:    cfg.ClockLogPath,
	}

	err := mon.openLog()
	if err != nil {
		return nil, err
	}
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
	mon.closeLog()
}

func (mon *Monitor) openLog() error {
	if mon.logPath == "" {
		return nil
	}
	dir := filepath.Dir(mon.logPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}
	return mon.doOpenLog()
}

func (mon *Monitor) ReopenLog() {
	if mon.logPath == "" {
		return
	}
	mon.closeLog()
	err := mon.doOpenLog()
	if err != nil {
		mon.lg.Error("error reopening log file", "path", mon.logPath, "err", err)
	}
}

func (mon *Monitor) closeLog() {
	if mon.logFile == nil {
		return
	}
	err := mon.logFile.Close()
	if err != nil {
		mon.lg.Error("error closing log file", "err", err, "path", mon.logPath)
	}
	mon.logFile = nil
}

func (mon *Monitor) doOpenLog() error {
	f, err := os.OpenFile(mon.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	mon.logFile = f
	mon.writeLogHeader()
	return nil
}

func (mon *Monitor) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	mon.addMissingOffsets(ref)
	off := local.T.Sub(ref)
	kind := sampleOK
	if !delayed && mon.isOutlier(off) {
		kind = sampleOutlier
	} else {
		mon.servo.Sample(ref, local, delayed)
	}
	freq := mon.servo.FreqOffset()
	mon.recordSample(kind, off, freq)
	mon.writeLogEntry(kind, ref, off, local.Era, freq)
	mon.sendEvent(kind, off, local.Era, freq)
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
	for i := 1; i < diff; i++ {
		mon.recordSample(sampleMissing, 0, mon.servo.FreqOffset())
	}
}

func (mon *Monitor) recordSample(kind sampleKind, off time.Duration, freq float64) {
	offSecs := off.Seconds()
	mon.samples.append(kind, offSecs)
	interval := mon.stats.interval
	if interval <= 0 {
		return
	}
	locked := mon.servo.Locked()
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
	if mon.logFile == nil {
		return
	}
	_, err := mon.logFile.WriteString(logHeader)
	mon.handleLogWriteError(err)
}

func (mon *Monitor) writeLogEntry(kind sampleKind, ref ptime.Time, off time.Duration, era ptime.Era, freq float64) {
	if mon.logFile == nil || kind == sampleMissing {
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
	_, err := fmt.Fprintf(mon.logFile, "%.5f %s %.0f %d %d\n", mjd, offStr, freq, outlierFlag, uint64(era))
	mon.handleLogWriteError(err)
}

func (mon *Monitor) handleLogWriteError(err error) {
	if err == nil {
		return
	}
	mon.lg.Error("error writing to log file", "err", err, "path", mon.logPath)
	mon.logFile.Close()
	mon.logFile = nil
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
	mon.lastSampleTime = mon.lastSampleTime.Add(time.Second)
	mon.lastRefTime = mon.lastRefTime.Add(time.Second)
	mon.recordSample(sampleMissing, 0, mon.servo.FreqOffset())
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
	return a.maxAbs <= syncMaxOffsetSecs
}

func (mon *Monitor) isOutlier(off time.Duration) bool {
	// don't do outlier detection unles we are in sync
	if !mon.inSync {
		return false
	}
	// if the window isn't full yet (i.e. just after start up),
	// then don't do outlier detection
	if mon.samples.length() < mon.samples.capacity() {
		return false
	}
	// if this offset isn't bad enough to take use out of sync,
	// then there's no need to consider it as an outlier
	absOff := math.Abs(off.Seconds())
	if absOff <= syncMaxOffsetSecs {
		return false
	}
	// if the previous sample was something strange (missing or outlier),
	// then don't make this one an outlier
	// XXX we could be more sophisticated here, but the outliers I've seen
	// are single stray pulses, so this should be good enough
	// We want to avoid any possibility of ignoring a lot of pulses from the GPS.
	if mon.samples.last(0).kind != sampleOK {
		return false
	}
	return true
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

func (w *sampleWindow) append(kind sampleKind, off float64) {
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
