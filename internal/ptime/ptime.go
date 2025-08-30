package ptime

import (
	"fmt"
	"math"
	"strings"
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
	OffChangeTime Time  // Time at which TAI-UTC offset changes (i.e. when leap second is over 00:00:00 UTC)
	UTCOffBefore  int16 // TAI-UTC offset before leap second
	UTCOffAfter   int16 // TAI-UTC offset after leap second
}

type LeapSecondState struct {
	UTCOffset   int16          // TAI-UTC offset before the LeapTonight leap second
	LeapTonight LeapSecondKind // future leap second, if any
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

// GPSUTC creates a UTCTime from a GPS week number and time of week
// GPS week number means number of weeks since GPS epoch
// tow is time ignoring leap seconds since start of week
// This is what UBX-TIM-TP uses when time base is UTC.
func GPSUTC(week uint16, tow time.Duration) UTCTime {
	startOfWeek := epochGPS.AddDate(0, 0, int(week)*7)
	t := startOfWeek.Add(tow)
	y, m, d := t.Date()
	date := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
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

const LeapSecond2016Month = time.December
const LeapSecond2016Day = 31
const LeapSecond2016UTCOffBefore = 36
const LeapSecond2016UTCOffAfter = 37

var leapSecond2016Date = time.Date(2016, LeapSecond2016Month, LeapSecond2016Day, 0, 0, 0, 0, time.UTC)

// LeapSecond2016 returns the leap second that occurred at the end of 2016.
// This has been the current leap second for a long time (up at least till mid 2025).
func LeapSecond2016() LeapSecond {
	return LeapSecondOnDate(leapSecond2016Date, LeapSecond2016UTCOffBefore, LeapSecond2016UTCOffAfter)
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

// SysToTime converts a system time to a TAI time.
// The boolean return value is false if the system time is ambiguous,
// which happens if the system time is during a positive leap second;
// In this SysToTime returns the first TAI time that has this system time.
// This will give incorrect results if ls is not the applicable leap second for sys.
func (ls LeapSecond) SysToTime(sys time.Time) (Time, bool) {
	t := Time(sys.UnixNano())
	// This is the 00:00 following the leap second
	leapOverTime := time.Unix(int64(ls.OffChangeTime)/1e9-int64(ls.UTCOffAfter), 0)
	timeTillLeapOver := leapOverTime.Sub(sys)
	if timeTillLeapOver <= 0 {
		// leap second is already over
		return t.Add(time.Second * time.Duration(ls.UTCOffAfter)), true
	}
	// the system time is in the range [23:59:59, 00:00:00) on the day of the leap second
	// we don't know what TAI time this is, since this range will happen twice
	ambig := ls.UTCOffAfter > ls.UTCOffBefore && timeTillLeapOver <= time.Second
	return t.Add(time.Second * time.Duration(ls.UTCOffBefore)), !ambig
}

// TimeToSys converts a TAI time to a system time.
func (ls LeapSecond) TimeToSys(t Time) time.Time {
	off := ls.UTCOffAfter
	// Linux steps the clock back before a positive leap second.
	// So during the leap second, we want to apply the After offset.
	if ls.Compare(t) < 0 {
		off = ls.UTCOffBefore
	}
	return time.Unix(-int64(off), int64(t)).In(time.UTC)
}

const epochUnixMJD = 40587

// TimeToMJD converts a TAI time to a Modified Julian Date.
// This is leap-second aware. The fractional part of the MJD is the fraction of the actual length of the day,
// which will be 86401 seconds on a day with a positive leap second.
func (ls LeapSecond) TimeToMJD(t Time) float64 {
	dayDuration := 24 * time.Hour
	leapDayDuration := dayDuration + time.Duration(ls.UTCOffAfter-ls.UTCOffBefore)*time.Second
	timeTillLeap := ls.OffChangeTime.Sub(t)
	var startOfDay Time
	var dayNumber int64
	if timeTillLeap > 0 && timeTillLeap <= leapDayDuration {
		startOfDay = ls.OffChangeTime.Add(-leapDayDuration)
		dayNumber = int64(startOfDay.Add(-time.Second*time.Duration(ls.UTCOffBefore))) / int64(dayDuration)
		dayDuration = leapDayDuration
		// in this case startOfDay and t are in TAI
	} else {
		off := ls.UTCOffBefore
		if t >= ls.OffChangeTime {
			off = ls.UTCOffAfter
		}
		t = t.Add(-time.Second * time.Duration(off))
		dayNumber = int64(t) / int64(dayDuration)
		startOfDay = Time(dayNumber * int64(dayDuration))
		// in this case startOfDay and t are in POSIX time (ignoring leap seconds)
	}
	timeOfDay := t.Sub(startOfDay)
	dayNumber += epochUnixMJD
	return float64(dayNumber) + float64(timeOfDay)/float64(dayDuration)
}

// Compare returns -1, 0, 1 according as t is before, during or after the leap second
func (ls LeapSecond) Compare(t Time) int {
	timeTillLeapOver := ls.OffChangeTime.Sub(t)
	if timeTillLeapOver <= 0 {
		return 1
	}
	if ls.UTCOffAfter > ls.UTCOffBefore && timeTillLeapOver <= time.Second {
		return 0
	}
	return -1
}

// Date returns the date of the leap second
// The leap second occurs at the end of the UTC day of that date.
func (ls LeapSecond) Date() time.Time {
	return time.Unix(int64(ls.OffChangeTime)/1e9-int64(ls.UTCOffAfter), 0).AddDate(0, 0, -1)
}

func (ls LeapSecond) FormatTime(t Time) string {
	cmp := ls.Compare(t)
	off := ls.UTCOffAfter
	if cmp <= 0 {
		off = ls.UTCOffBefore
		if cmp == 0 {
			// Subtract a second if we're in a leap second; we will add it back later
			t = t.Add(-time.Second)
		}
	}
	s := epochUnix.Add(time.Duration(t) - time.Duration(off)*time.Second).Format(time.RFC3339)
	// If we're in a leap second (cmp == 0), fix the output
	if cmp == 0 {
		// Find the last colon, which will be just before the seconds field
		colonPos := strings.LastIndex(s, ":")
		if colonPos >= 0 && len(s) >= colonPos+3 {
			// Replace "59" with "60" in the seconds portion
			s = s[:colonPos+1] + "60" + s[colonPos+3:]
		}
	}
	return s
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

func (t *Time) UnmarshalText(data []byte) error {
	s := string(data)
	secs, nanos, ok := parsePtime(s)
	if !ok {
		return fmt.Errorf("could not parse as ptime.Time: %s", s)
	}
	if nanos < 0 || nanos >= 1e9 {
		return fmt.Errorf("invalid nanoseconds unmarshaling ptime.Time: %d", nanos)
	}
	*t = Time(secs*1e9 + nanos)
	return nil
}

func parsePtime(s string) (secs, nanos int64, ok bool) {
	before, after, found := strings.Cut(s, ".")
	if !found {
		return
	}
	if n, err := fmt.Sscanf(before, "%d", &secs); err != nil || n != 1 {
		return
	}
	if n, err := fmt.Sscanf(after, "%09d", &nanos); err != nil || n != 1 {
		return
	}
	ok = true
	return
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

func Picoseconds(ps int32) time.Duration {
	if ps < 0 {
		return -Picoseconds(-ps)
	}
	return time.Duration(((ps + 500) / 1000))
}

// GNSSLeapSecond is information about a future leap second in a form similar to what is broadcast by GNSS systems.
// GPS, Galileo, and BeiDou all use similar formats.
// Day numbering uses each GNSS's native format: GPS/Galileo use 1-based (Sunday=1), BeiDou uses 0-based (Sunday=0).
type GNSSLeapSecond struct {
	WNLSF    uint8 // low 8 bits of GNSS week number of future leap second
	DN       uint8 // day number in native GNSS format: GPS/Galileo 1-based (Sun=1), BeiDou 0-based (Sun=0)
	DeltaLS  int8  // leap seconds since GNSS epoch BEFORE the occurrence of the leap second
	DeltaLSF int8  // leap seconds since GNSS epoch AFTER the occurrence of the leap second
}

// GPSLeapSecond converts GPS leap second data to a LeapSecond.
// The day number (DN) in g should be 1-based as broadcast by GPS (Sunday=1).
func GPSLeapSecond(g GNSSLeapSecond, now Time) (LeapSecond, error) {
	return gnssLeapSecond(g, now, epochGPS, TAIMinusGPS, 1)
}

// GalileoLeapSecond converts Galileo leap second data to a LeapSecond.
// The day number (DN) in g should be 1-based as broadcast by Galileo (Sunday=1).
func GalileoLeapSecond(g GNSSLeapSecond, now Time) (LeapSecond, error) {
	return gnssLeapSecond(g, now, epochGalileo, TAIMinusGalileo, 1)
}

// BeiDouLeapSecond converts BeiDou leap second data to a LeapSecond.
// The day number (DN) in g should be 0-based as broadcast by BeiDou (Sunday=0).
func BeiDouLeapSecond(g GNSSLeapSecond, now Time) (LeapSecond, error) {
	return gnssLeapSecond(g, now, epochBeiDou, TAIMinusBeiDou, 0)
}

// gnssLeapSecond converts a GNSS-encoded leap second to a LeapSecond.
//
//	g: GNSS-encoded leap second
//	now: current time in TAI (as determined from GNSS)
//	epoch: time of the GNSS epoch (from which GNSS week and day numbers count)
//	epochLeapSeconds: (TAI − GNSS) at that epoch in whole seconds (GPS=19, GAL=19, BDS=33)
//	dnBase: day numbering base (0 for BeiDou, 1 for GPS/Galileo)
func gnssLeapSecond(g GNSSLeapSecond, now Time, epoch time.Time, epochLeapSeconds int16, dnBase uint8) (LeapSecond, error) {
	dayOfWeek := int(g.DN) - int(dnBase)
	isPast := g.DeltaLS == g.DeltaLSF

	// Find candidate leap seconds using forward search from epoch
	week := int(g.WNLSF)
	var candidates []LeapSecond
	maxTime := now.Add(6 * 30 * 24 * time.Hour) // now + 6 months (IERS horizon)

	for {
		date := epoch.AddDate(0, 0, week*7+dayOfWeek)

		// Stop if we've gone too far in the future (check date first to avoid unnecessary work)
		if Unix(date.Unix(), 0) > maxTime {
			break
		}

		// Skip dates before 2016
		if date.Before(leapSecond2016Date) {
			week += 256
			continue
		}

		// Must be a valid quarter-end date
		if !isLastDayOfQuarter(date) {
			week += 256
			continue
		}

		// Create candidate leap second
		candidate := LeapSecondOnDate(date, epochLeapSeconds+int16(g.DeltaLS), epochLeapSeconds+int16(g.DeltaLSF))

		// Check if candidate is appropriate for past/future with slack for edge cases
		const slack = 24 * time.Hour
		if isPast {
			if candidate.OffChangeTime <= now.Add(slack) {
				candidates = append(candidates, candidate)
			}
		} else {
			if candidate.OffChangeTime >= now.Add(-slack) {
				candidates = append(candidates, candidate)
			}
		}

		week += 256
	}

	// Check for ambiguity or no candidates
	if len(candidates) == 0 {
		return LeapSecond{}, fmt.Errorf("no valid leap second date found for WNLSF=%d DN=%d", g.WNLSF, g.DN)
	}
	if len(candidates) > 1 {
		return LeapSecond{}, fmt.Errorf("ambiguous leap second dates: %v", candidates)
	}

	ls := candidates[0]

	if isPast {
		ls.UTCOffBefore = guessPastLeapSecondOffset(ls.OffChangeTime, ls.UTCOffAfter)
	}

	return ls, nil
}

// guessPastLeapSecondOffset guesses the before UTC offset for a past leap second
// when only the after offset is known (DeltaLS == DeltaLSF).
// leapTime is the Time when the leap second offset change occurs (OffChangeTime).
func guessPastLeapSecondOffset(leapTime Time, after int16) int16 {
	leapSecond2016 := LeapSecond2016()
	if leapTime == leapSecond2016.OffChangeTime {
		// Known reference point
		return LeapSecond2016UTCOffBefore
	}

	// For other past leap seconds, derive based on after offset
	if after > leapSecond2016.UTCOffAfter {
		// Assume positive leap second (all historical ones have been)
		return after - 1
	} else if after < leapSecond2016.UTCOffAfter {
		// Would indicate negative leap second (theoretically possible but never happened)
		return after + 1
	} else { // after == 37
		// Could be 2016 or could have returned to 37 via negative leap
		// Assume 2016 behavior (positive leap second)
		return leapSecond2016.UTCOffBefore
	}
}

func isLastDayOfQuarter(t time.Time) bool {
	return t.AddDate(0, 0, 1).Day() == 1 && (t.Month()%3) == 0
}

// CorrectionParams holds parameters for computing a correction to a time.
// The correction is computed using the polynomial mode
//   delta = Bias + Drift * (t - Ref)
// The correction is expected to be subtracted from the time to be corrected.
// Typically, the CorrectionParams hold parameters derived from messages broadcast
// by a GNSS system. The correction is applied to GNSS system time as part
// of the process of deriving UTC time. These corrections are usually a few nanoseconds.
// These corrections are independent of offsets applied to handle leap seconds,
// and to handle differences in the epochs of different GNSS systems.
type CorrectionParams struct {
	Ref   Time // reference time
	Bias  float64    // bias in nanoseconds
	Drift float64    // drift seconds/seconds (i.e. dimensionless)
}

const MaxCorrection = 50 * time.Nanosecond
const MaxValidityPeriod = 7 * 24 * time.Hour

// ApplyCorrection applies the correction to a time.
// If the correction is not valid, then the time is returned unchanged.
func (c *CorrectionParams) ApplyCorrection(t Time) Time {
	delta, valid := c.Correction(t)
	if valid {
		return t.Add(-delta)
	}
	return t
}

// Correction computes the correction at time t.
// and returns the correction and whether it's valid.
// The correction is considered invalid
// - absolute value of the time difference from reference exceeds MaxValidityPeriod (1 week), or
// - absolute value of the correction exceeds MaxCorrection (50ns)
func (c *CorrectionParams) Correction(t Time) (time.Duration, bool) {
	timeDiff := t.Sub(c.Ref)
	delta := time.Duration(math.Round(c.Bias + float64(timeDiff)*c.Drift))

	if timeDiff < -MaxValidityPeriod || timeDiff > MaxValidityPeriod {
		return delta, false
	}

	absDelta := time.Duration(math.Abs(float64(delta)))
	if absDelta >= MaxCorrection {
		return delta, false
	}

	return delta, true
}
