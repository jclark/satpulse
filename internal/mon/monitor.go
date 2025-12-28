package mon

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/ptpgm"
	"github.com/jclark/satpulse/internal/refclock"
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
	gm             *ptpgm.Grandmaster   // maybe nil
	rc             *refclock.ProxyRefClock // maybe nil
	syncState      SyncState
	paused         bool
	lastRefTime    ptime.Time
	ppsStopped     int // number of missing samples recorded since 1PPS stopped
	sampler        Sampler
	cfg            syncConfig
}

// SampleKind indicates the kind of sample
type SampleKind = phcsync.SampleKind

const (
	SampleMissing = phcsync.SampleMissing
	SampleOK      = phcsync.SampleOK
	SampleOutlier = phcsync.SampleOutlier
)

// SyncState indicates whether the clock is synchronized
type SyncState = ptpgm.SyncState

const (
	NoSync = ptpgm.NoSync
	InSync = ptpgm.InSync
)

// SampleData contains all the information about a sample
type SampleData struct {
	Kind      SampleKind
	Ref       ptime.Time
	Offset    time.Duration
	Freq      float64
	FreqDelta float64
	SyncState SyncState
	Era       ptime.Era
}

// Sampler is the interface for receiving samples
type Sampler interface {
	Sample(data SampleData)
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
	RefClock      *refclock.ProxyRefClock
	Grandmaster   *ptpgm.Grandmaster
	Sampler       Sampler // never nil, uses DefaultObserver when no observability needed
}

func NewMonitor(servo Servo, lg *slog.Logger, cfg MonitorConfig) (*Monitor, error) {
	mon := &Monitor{
		servo:      servo,
		lg:         lg,
		leapSecond: cfg.LeapSecond,
		samples:    newSampleState(sampleWindowSize),
		gm:         cfg.Grandmaster,
		rc:         cfg.RefClock,
		sampler:    cfg.Sampler,
		syncState:  NoSync,
		cfg:        defaultSyncConfig,
	}

	if mon.gm != nil {
		err := mon.gm.SetClockAccuracy(cfg.ClockAccuracy)
		if err != nil {
			return nil, err
		}
	}
	mon.cfg.maxOffset = cfg.ClockAccuracy.Seconds()
	return mon, nil
}

func (mon *Monitor) Close() {
	mon.lg.Debug("closing monitor")
	mon.updateSyncState(NoSync)
	if mon.gm != nil {
		mon.gm.Close()
	}
}

func (mon *Monitor) Pause() {
	mon.updateSyncState(NoSync)
	mon.servo.Reset()
	mon.paused = true
}

func (mon *Monitor) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	mon.addMissingOffsets(ref)
	mon.paused = false
	off := local.T.Sub(ref)
	kind := SampleOK
	freq := mon.servo.FreqOffset()
	freqDelta := 0.0
	if !delayed && mon.isOutlier(off, local.Era) {
		kind = SampleOutlier
	} else {
		mon.servo.Sample(ref, local, delayed)
		f := mon.servo.FreqOffset()
		freqDelta = f - freq
		freq = f
	}
	mon.recordSample(kind, off, local.Era)
	nextState := mon.nextSyncState()
	mon.sampler.Sample(SampleData{
		Kind:      kind,
		Ref:       ref,
		Offset:    off,
		Freq:      freq,
		FreqDelta: freqDelta,
		SyncState: nextState,
		Era:       local.Era,
	})
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
		mon.recordSample(SampleMissing, 0, mon.samples.era)
	}
}

func (mon *Monitor) recordSample(kind SampleKind, off time.Duration, era ptime.Era) {
	mon.samples.sample(kind, off.Seconds(), era, mon.syncState, &mon.cfg)
}

func (mon *Monitor) SysSample(ref ptime.Time, sys time.Time) {
	if mon.rc == nil || mon.syncState == NoSync {
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
	if mon.syncState == NoSync {
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
	freq := mon.servo.FreqOffset()
	mon.recordSample(SampleMissing, 0, mon.samples.era)
	nextState := mon.nextSyncState()
	mon.sampler.Sample(SampleData{
		Kind:      SampleMissing,
		Ref:       mon.lastRefTime,
		Offset:    0,
		Freq:      freq,
		SyncState: nextState,
		Era:       mon.samples.era,
	})
	mon.updateSyncState(nextState)
}

func (mon *Monitor) nextSyncState() SyncState {
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

func (mon *Monitor) updateSyncState(state SyncState) {
	if state != mon.syncState {
		mon.lg.Warn("synchronization status has changed", "newSyncState", state)
		mon.syncState = state
		mon.samples.resetForSyncChange(state)
	}
	mon.gmUpdate()
}

func (mon *Monitor) gmUpdate() {
	if mon.gm != nil && !mon.lastRefTime.IsZero() {
		mon.gm.Update(gmSyncState(mon.syncState), mon.leapSecond.StateAt(mon.lastRefTime))
	}
}

func gmSyncState(ss SyncState) ptpgm.SyncState {
	switch ss {
	case InSync:
		return ptpgm.InSync
	}
	return ptpgm.NoSync
}

func (mon *Monitor) SetLeapSecond(leapSecond ptime.LeapSecond) {
	if leapSecond == mon.leapSecond {
		return
	}
	mon.leapSecond = leapSecond
	mon.gmUpdate()
}
