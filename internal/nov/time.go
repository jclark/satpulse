package nov

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/novmsg"
	"github.com/jclark/satpulse/internal/ptime"
)

func timeTime(header novmsg.MessageHeader, m *novmsg.Time, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
	if m.ClockStatus != novmsg.ClockStatusValid {
		return nil, nil
	}
	t := gpsprot.TimeMsg{
		Tag:         tag,
		NativeMsgID: "TIME",
	}
	// NovAtel always uses GPS week numbers
	t.GNSS = gpsprot.GPS
	if header.TimeStatus != novmsg.TimeStatusUnknown {
		tow := time.Duration(header.MillisecondsOfWeek) * time.Millisecond
		t.TAITime = ptime.GPS(int16(header.Week), tow)
	}
	return TimeMsgSetUTC(&t, m)
}

func TimeMsgSetUTC(t *gpsprot.TimeMsg, m *novmsg.Time) (*gpsprot.TimeMsg, error) {
	if m.UTCStatus == novmsg.UTCStatusValid {
		nanos := int32(m.UTCMs%1000) * 1e6
		utc := ptime.UTC(uint16(m.UTCYear), m.UTCMonth, m.UTCDay, m.UTCHour, m.UTCMin, uint8(m.UTCMs/1000), nanos)
		t.UTCTime = &utc
		t.UTCOffset = convertUTCOffset(m.UTCOffset)
		if t.UTCOffset == 0 {
			return nil, fmt.Errorf("invalid UTC offset %f", m.UTCOffset)
		}
	}
	t.Accuracy = convertAccuracy(m.OffsetStd)
	return t, nil
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