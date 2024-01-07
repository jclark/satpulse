package ptime

import (
	"fmt"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

type ClockTime struct {
	T   Time
	Era Era
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

type LeapSecondState struct {
	UTCOffset   int16
	LeapTonight LeapSecondKind
}

type LeapSecondKind int16

const (
	LeapSecondNone     LeapSecondKind = 0
	LeapSecondPositive LeapSecondKind = 1
	LeapSecondNegative LeapSecondKind = -1
)

var epochUnix = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)

// GPS epoch for week numbers
var epochGPS = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

var epochGlonass = time.Date(1996, time.January, 1, 0, 0, 0, 0, time.UTC)

// Galileo epoch
// Leap seconds are handled by TAIMinusGalileo constant
var epochGalileo = time.Date(1999, time.August, 22, 0, 0, 0, 0, time.UTC)

// BeiDou epoch
var epochBeiDou = time.Date(2006, time.January, 1, 0, 0, 0, 0, time.UTC)

// Number of seconds by which TAI time is ahead of GPS time
const TAIMinusGPS = 19

// Number of seconds by which TAI time is ahead of Galileo time
// Time of week in Galileo and GPS are the same
// (so Galileo epoch is not aligned with TAI or UTC midnight)
const TAIMinusGalileo = TAIMinusGPS

// Number of seconds by which TAI time is ahead of BeiDou time
const TAIMinusBeiDou = 33

// GPS creates a Time from a GPS week number and time of week
// week is number of complete weeks since start of first Sunday in 1980
// tow is duration since start of week
func GPS(week int16, tow time.Duration) Time {
	return gnss(week, tow, epochGPS, TAIMinusGPS)
}

func GPSDate(week uint16, day time.Weekday) time.Time {
	return epochGPS.AddDate(0, 0, int(week)*7+int(day))
}

// Galileo creates a Time from a Galileo week number and time of week
// week is number of complete weeks since Galileo epoch
// tow is duration since start of week
func Galileo(week int16, tow time.Duration) Time {
	return gnss(week, tow, epochGalileo, TAIMinusGalileo)
}

// BeiDou creates a Time from a BeiDou week number and time of week
// week is number of complete weeks since BeiDou epoch
// tow is duration since start of week
func BeiDou(week int16, tow time.Duration) Time {
	return gnss(week, tow, epochBeiDou, TAIMinusBeiDou)
}

func gnss(week int16, tow time.Duration, epoch time.Time, epochTAIOffset int64) Time {
	// The Unix method in Go doesn't consider leap seconds
	// GNSS time also doesn't consider leap seconds since that GNSS's epoch
	// We therefore need to correct just for the leap seconds applicable at the epoch
	s := epoch.AddDate(0, 0, int(week)*7).Unix() + epochTAIOffset
	return Time(s*1e9 + int64(tow))
}

// GLONASS creates a UTCTime from a GLONASS interval number, day number and time of day
// The interval number denotes a 4-year cycle, with interval 1 starting on 1996-01-01.
// The day number is the day number within the interval, with day 1 being January 1st.
func GLONASS(intervalNumber byte, dayNumber uint16, tod time.Duration) UTCTime {
	// GLONASS time appears to be Moscow time (UTC+3)
	// Our UTCTime representations allow negative time-of-days, so we can use that here
	// I don't understand how GLONASS works around leap seconds: I suspect we are not right in the vicinity of a leap second
	return UTCTime{epochGlonass.AddDate(int(intervalNumber-1)*4, 0, int(dayNumber)-1), tod - 3*time.Hour}
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
	off := ls.UTCOffAfter
	if t < ls.OffChangeTime {
		off = ls.UTCOffBefore
	}
	// XXX this won't display leap second correctly
	return epochUnix.Add(time.Duration(t) - time.Duration(off)*time.Second).Format(time.RFC3339)
}

func (ls LeapSecond) StateAt(t Time) LeapSecondState {
	var state LeapSecondState
	if t >= ls.OffChangeTime {
		state.UTCOffset = ls.UTCOffAfter
	} else {
		state.UTCOffset = ls.UTCOffBefore
		if t >= ls.OffChangeTime.Add(-12*time.Hour) {
			state.LeapTonight = LeapSecondKind(ls.UTCOffAfter - ls.UTCOffBefore)
		}
	}
	return state
}

func Unix(sec int64, nsec int64) Time {
	return Time(sec*1e9 + nsec)
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

// An Era represents a period of time within which a clock has not been stepped.
// When the time of a clock is read, an era is associated with it.
// When a clock is stepped, it may be uncertain whether a particular read of the
// clock happened before or after the step. We handle this by incrementing the
// twice during the step: once before the step is started, and once after we know
// it has taken effect. Even-numbered eras are used to represent the uncertain period
// while the clock is being stepped.
type Era uint64

// Uncertain returns true if the era is uncertain.
// Uncertain eras are even numbered.
func (e Era) Uncertain() bool {
	return (e & 1) == 0
}

func (e Era) StepCount() (count uint64, changing bool) {
	count = uint64(e) >> 1
	changing = e.Uncertain()
	if !changing {
		count++
	}
	return
}

type AtomicEra atomic.Uint64

func (c *AtomicEra) Inc() Era {
	return Era((*atomic.Uint64)(c).Add(1))
}

func (c *AtomicEra) Load() Era {
	return Era((*atomic.Uint64)(c).Load())
}

func Picoseconds(ps int32) time.Duration {
	if ps < 0 {
		return -Picoseconds(-ps)
	}
	return time.Duration(((ps + 500) / 1000))
}
