package unc

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/uncmsg"
)

func timeRecTime(header uncmsg.MessageHeader, m *uncmsg.RecTime, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
	if m.ClockStatus != uncmsg.ClockStatusValid {
		return nil, nil
	}
	t := gpsprot.TimeMsg{
		Tag:         tag,
		NativeMsgID: "RECTIME",
	}
	gnss, toTAI := timeRefToTAI(header.TimeRef)
	t.GNSS = gnss
	if toTAI != nil && header.TimeStatus == uncmsg.TimeStatusFine {
		tow := time.Duration(header.MillisecondsOfWeek) * time.Millisecond
		t.TAITime = toTAI(int16(header.Week), tow)
	}

	if m.UTCStatus == uncmsg.UTCStatusValid {
		nanos := int32(m.UTCMs%1000) * 1e6
		utc := ptime.UTC(uint16(m.UTCYear), m.UTCMonth, m.UTCDay, m.UTCHour, m.UTCMin, uint8(m.UTCMs/1000), nanos)
		t.UTCTime = &utc
		t.UTCOffset = convertUTCOffset(m.UTCOffset)
		if t.UTCOffset == 0 {
			return nil, fmt.Errorf("invalid UTC offset %f", m.UTCOffset)
		}
	}
	t.Accuracy = convertAccuracy(m.OffsetStd)
	return &t, nil
}

func timeRefToTAI(ref uncmsg.TimeRef) (gpsprot.GNSS, func(int16, time.Duration) ptime.Time) {
	switch ref {
	case uncmsg.TimeRefGPS:
		return gpsprot.GPS, ptime.GPS
	case uncmsg.TimeRefBDS:
		return gpsprot.BDS, ptime.BeiDou
	}
	return 0, nil
}

// convertUTCOffset converts GPS-UTC offset to TAI-UTC offset.
// Returns 0 if the conversion fails (fractional value or out of uint8 range).
func convertUTCOffset(f float64) uint8 {
	// This looks simple, but it is surprisingly tricky to make it robust in all cases
	floatOff := ptime.TAIMinusGPS - f
	intOff := uint8(floatOff)
	if float64(intOff) != floatOff {
		return 0
	}
	return intOff
}

// convertAccuracy converts float64 seconds to time.Duration for accuracy values.
// Returns 0 for invalid inputs (negative, NaN, Inf, out of range).
// Rounds fractional nanoseconds up to avoid underestimating accuracy.
func convertAccuracy(seconds float64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	// Convert to nanoseconds with ceiling
	nanos := math.Ceil(seconds * 1e9)
	// Check if conversion is valid using round-trip test
	dur := time.Duration(nanos)
	if float64(dur) != nanos {
		return 0 // Overflow, NaN, or Inf
	}
	return dur
}