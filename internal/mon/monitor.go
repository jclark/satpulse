package mon

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/gps4ptp/internal/ptime"
)

const samplesToKeep = 3600

type Monitor struct {
	offsets        *Queue[float64]
	lastSampleTime time.Time
	leapSecond     ptime.LeapSecond
	lg             *slog.Logger
	gm             *Grandmaster // maybe nil
	inSync         bool
	lastRefTime    ptime.Time
	ppsStopped     bool
}

func NewMonitor(leapSecond ptime.LeapSecond, updateCh chan<- GrandmasterUpdateRequest, lg *slog.Logger) *Monitor {
	mon := &Monitor{
		leapSecond: leapSecond,
		lg:         lg,
		offsets:    NewQueue[float64](samplesToKeep),
	}
	if updateCh != nil {
		mon.gm = NewGrandmaster(updateCh)
	}
	return mon
}

func (mon *Monitor) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	off := local.T.Sub(ref)
	mon.offsets.Append(float64(int64(off)) / 1e9)
	ref = ref.Round(time.Second)
	if !mon.lastRefTime.IsZero() {
		diff := int(ref.Sub(mon.lastRefTime) / time.Second)
		if diff > 1 {
			mon.lg.Warn("missed 1PPS samples", "n", diff-1)
			for i := 1; i < diff; i++ {
				mon.offsets.Append(math.NaN())
			}
		}
	}
	mon.lastRefTime = ref
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
	if mon.offsets.Len() < syncNOffsets {
		return false
	}
	a := accumSlice(mon.offsets.LastN(syncNOffsets))
	if a.nNaN > 0 {
		// if we've gone out of sync, don't allow any missing values;
		// otherwise allow up to syncMaxMissing
		if !mon.inSync || a.nNaN > syncMaxMissing {
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
	if mon.gm != nil && !mon.lastRefTime.IsZero() {
		mon.gm.Update(inSync, mon.leapSecond.StateAt(mon.lastRefTime))
	}
}

func (mon *Monitor) SetLeapSecond(leapSecond ptime.LeapSecond) {
	if leapSecond == mon.leapSecond {
		return
	}
	mon.leapSecond = leapSecond
}

type accum struct {
	sum        float64
	sumSquares float64
	maxAbs     float64
	nNaN       int
}

func accumSlice(values []float64) accum {
	var a accum
	a.addSlice(values)
	return a
}

func (a *accum) addSlice(values []float64) {
	for _, v := range values {
		a.addValue(v)
	}
}

func (a *accum) addValue(v float64) {
	if math.IsNaN(v) {
		a.nNaN++
	} else {
		a.sum += v
		a.sumSquares += v * v
		a.maxAbs = math.Max(a.maxAbs, math.Abs(v))
	}
}
