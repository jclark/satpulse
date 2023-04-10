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
	Date      time.Time     // time of the start of the day (at midnight)
	TimeOfDay time.Duration // duration since start of day at midnight
}

type LeapSecond struct {
	OffChangeTime Time  // Time at which TAI-UTC offset changes (i.e. when leap second is over)
	UTCOffBefore  int16 // TAI-UTC offset before leap second
	UTCOffAfter   int16 // TAI-UTC offset after leap second
}

// GPS epoch
var epochGPS = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)
var epochUnix = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)

// Number of seconds by which TAI time is ahead of GPS time
const TAIMinusGPS = 19

// week is number of complete weeks since start of first Sunday in 1980
// iTOW is milliseconds since start of week (Sunday)
// fTOW is nanoseconds
func GPS(week int16, iTOW uint32, fTOW int32) Time {
	ms := epochGPS.AddDate(0, 0, int(week)*7).UnixMilli() + TAIMinusGPS*1000 + int64(iTOW)
	return Time(ms*1e6 + int64(fTOW))
}

func GPSDate(week uint16, day time.Weekday) time.Time {
	return epochGPS.AddDate(0, 0, int(week)*7+int(day))
}

func UTC(year uint16, month, day, hour, min, sec uint8, nanos int32) UTCTime {
	date := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
	t := time.Date(int(year), time.Month(month), int(day), int(hour), int(min), int(sec), int(nanos), time.UTC)
	return UTCTime{date, t.Sub(date)}
}

// LeapSecondOnDate returns a LeapSecond that occurs at the end of the UTC day of the given date.
// date should be the last day of a month
func LeapSecondOnDate(date time.Time, utcOffBefore, utcOffAfter int16) LeapSecond {
	return LeapSecond{
		OffChangeTime: Time((date.AddDate(0, 0, 1).Unix() + int64(utcOffAfter)) * 1e9),
		UTCOffBefore:  utcOffBefore,
		UTCOffAfter:   utcOffAfter,
	}
}

func (ls LeapSecond) UTCtoTime(ut UTCTime) Time {
	var s int16
	// It is essential to do the comparison using the date of the leap second
	if ut.Date.After(ls.Date()) {
		s = ls.UTCOffAfter
	} else {
		s = ls.UTCOffBefore
	}
	return Time(ut.Date.Add(ut.TimeOfDay).UnixNano() + int64(s)*1e9)
}

// Date returns the date of the leap second
// The leap second occurs at the end of the UTC day of that date.
func (ls LeapSecond) Date() time.Time {
	return time.Unix(int64(ls.OffChangeTime)/1e9-int64(ls.UTCOffAfter), 0).AddDate(0, 0, -1)
}

func (ls LeapSecond) FormatTime(t Time) string {
	// XXX this is wrong
	return epochUnix.Add(time.Duration(t)).Format(time.RFC3339)
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

func (t Time) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
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

func (t Time) Round(d time.Duration) Time {
	// Let the time package do the tricky bit
	return Time(int64(time.Duration(int64(t)).Round(d)))
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
