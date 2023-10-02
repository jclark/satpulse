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
	nConsecSamples int
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
	if !mon.lastRefTime.IsZero() && ref.Sub(mon.lastRefTime) != time.Second {
		mon.lg.Info("non-consecutive samples", "prev", mon.lastRefTime, "cur", ref)
		mon.nConsecSamples = 0
	}
	mon.nConsecSamples++
	mon.lastRefTime = ref
	if delayed {
		return
	}
	inSync := mon.samplesInSync()
	now := time.Now()
	mon.lastSampleTime = now
	if mon.ppsStopped {
		mon.lg.Info("1PPS signal restored")
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
		mon.lg.Info("1PPS signal stopped")
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

// number of consecutive samples (no missing pulses) required for synchronization
const syncNConsec = 3

func (mon *Monitor) samplesInSync() bool {
	if mon.offsets.Len() < syncNOffsets || mon.nConsecSamples < syncNConsec {
		return false
	}
	return maxAbs(mon.offsets.LastN(syncNOffsets)) <= syncMaxOffsetSecs
}

func (mon *Monitor) updateInSync(inSync bool) {
	if inSync != mon.inSync {
		mon.lg.Info("synchronization status has changed", "inSync", inSync)
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

func maxAbs(values []float64) (max float64) {
	for _, v := range values {
		max = math.Max(max, math.Abs(v))
	}
	return
}

func rms(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(values)))
}
