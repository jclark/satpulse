package mon

import (
	"math"
	"time"

	"github.com/jclark/gps2phc/internal/ptime"
)

const samplesToKeep = 3600

type SyncAnalyzer struct {
	offsets        *FloatQueue
	lastSampleTime time.Time
}

func NewSyncAnalyzer() *SyncAnalyzer {
	return &SyncAnalyzer{
		offsets: NewFloatQueue(samplesToKeep),
	}
}

const holdoverDuration = 10 * time.Second

// For sync, require that the maximum absolute offset is less than 50ns.
// Reasoning is we are claiming 100ns accuracy, and we need to budget for other sources of error,
// specifically errors in GPS signal
const syncMaxOffsetSecs = 50e-9
const syncNOffsets = 5

func (mon *SyncAnalyzer) InSync(now time.Time) bool {
	if now.Sub(mon.lastSampleTime) > holdoverDuration {
		return false
	}
	if mon.offsets.Len() < syncNOffsets {
		return false
	}
	return maxAbs(mon.offsets.LastN(syncNOffsets)) <= syncMaxOffsetSecs
}

func (mon *SyncAnalyzer) Sample(ref ptime.Time, local ptime.ClockTime, tRead time.Time, _ bool) {
	off := local.T.Sub(ref)
	mon.offsets.Append(float64(int64(off)) / 1e9)
	mon.lastSampleTime = tRead
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

type FloatQueue struct {
	buf        []float64
	start, end int
	maxLen     int
}

func NewFloatQueue(maxLen int) *FloatQueue {
	buf := make([]float64, maxLen*2)
	return &FloatQueue{
		buf:    buf,
		maxLen: maxLen,
	}
}

func (q *FloatQueue) Clear() {
	q.start = 0
	q.end = 0
}

func (q *FloatQueue) Len() int {
	return q.end - q.start
}

func (q *FloatQueue) Append(f float64) {
	if q.end == len(q.buf) {
		copy(q.buf, q.Slice())
		q.end -= q.start
		q.start = 0
	}
	q.buf[q.end] = f
	if q.end-q.start == q.maxLen {
		q.start++
	}
	q.end++
}

func (q *FloatQueue) Slice() []float64 {
	return q.buf[q.start:q.end]
}

func (q *FloatQueue) LastN(n int) []float64 {
	start := q.end - n
	if start < q.start {
		start = q.start
	}
	return q.buf[start:q.end]
}
