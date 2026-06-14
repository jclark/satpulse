// Package ntime provides a domain-neutral nanosecond timestamp.
// A Time is just integer nanoseconds from an unspecified epoch; its
// domain (UTC, TAI, PHC-raw, ...) is determined by the producer and
// consumer, not by the type. It carries none of the wall-clock
// semantics of time.Time and none of the TAI interpretation of
// ptime.Time; its API is the small subset of arithmetic that the
// refclock sample path needs.
package ntime

import "time"

// Time is a timestamp in nanoseconds from an unspecified epoch.
type Time int64

// Sys converts a time.Time to a Time with the same nanosecond value.
func Sys(sys time.Time) Time {
	return Time(sys.UnixNano())
}

// SysTime converts t to a time.Time with the same nanosecond value,
// in the UTC zone.
func (t Time) SysTime() time.Time {
	return time.Unix(0, int64(t)).UTC()
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

func (t Time) Before(t2 Time) bool {
	return t < t2
}

func (t Time) Round(d time.Duration) Time {
	// Let the time package do the tricky bit
	return Time(int64(time.Duration(int64(t)).Round(d)))
}
