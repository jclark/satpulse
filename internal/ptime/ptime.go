package ptime

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

type ClockTime struct {
	T     Time
	Epoch Epoch
}

// Time in TAI timescale represented as nanoseconds since 1970-01-01T00:00:00 TAI
type Time int64

// timeOfDay can be negative, but not longer than -24 hours
type UTCTime struct {
	date      time.Time     // time of the start of the day (at midnight)
	timeOfDay time.Duration // duration since start of day at midnight
}

type LeapSeconds struct {
	LastDate  time.Time // last scheduled leap second (must be last day of a month)
	OffBefore uint8     // TAI-UTC offset before leap second
	OffAfter  uint8     // TAI-UTC offset after leap second
}

// GPS epoch
var epochGPS = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

// Offset in seconds between TAI and UTC at GPS epoch
const epochGPSOffsetTAI = 19

// week is number of complete weeks since start of first Sunday in 1980
// iTOW is milliseconds since start of week (Sunday)
func GPS(week int16, iTOW uint32) Time {
	ms := epochGPS.AddDate(0, 0, int(week)*7).UnixMilli() + epochGPSOffsetTAI*1000 + int64(iTOW)
	return Time(ms * 1e6)
}

func GPSDate(week uint16, day time.Weekday) time.Time {
	return epochGPS.AddDate(0, 0, int(week)*7+int(day))
}

func UTC(year uint16, month, day, hour, min, sec uint8, nanos int32) UTCTime {
	date := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
	t := time.Date(int(year), time.Month(month), int(day), int(hour), int(min), int(sec), int(nanos), time.UTC)
	return UTCTime{date, t.Sub(date)}
}

func (leaps *LeapSeconds) UTCtoTime(ut *UTCTime) Time {
	t := ut.date.Add(ut.timeOfDay).UnixNano()
	var s uint8
	if ut.date.After(leaps.LastDate) {
		s = leaps.OffAfter
	} else {
		s = leaps.OffBefore
	}
	return Time(t + int64(s)*1e9)
}

func TimespecToTime(t unix.Timespec) Time {
	return Time(t.Nano())
}

func (t Time) Timespec() unix.Timespec {
	return unix.NsecToTimespec(int64(t))
}

func (t Time) String() string {
	n := int64(t)
	// FIXME deal with negative here
	return fmt.Sprintf("%d.%09d", n/1e9, n%1e9)
}

func (t Time) IsZero() bool {
	return int64(t) == 0
}

func (t Time) Add(d time.Duration) Time {
	return Time(int64(t) + int64(d))
}

func (t Time) Sub(t2 Time) time.Duration {
	return time.Duration(int64(t) - int64(t2))
}

type Epoch uint64

const InitialEpoch = Epoch(math.MaxUint64)

func (e Epoch) Ambig() bool {
	return (e & 1) != 0
}

type AtomicEpoch atomic.Uint64

func (c *AtomicEpoch) Init() {
	(*atomic.Uint64)(c).Store(uint64(InitialEpoch))
}

func (c *AtomicEpoch) Inc() Epoch {
	return Epoch((*atomic.Uint64)(c).Add(1))
}

func (c *AtomicEpoch) Load() Epoch {
	return Epoch((*atomic.Uint64)(c).Load())
}

func Picoseconds(ps int32) time.Duration {
	if ps < 0 {
		return -Picoseconds(-ps)
	}
	return time.Duration(((ps + 500) / 1000))
}
