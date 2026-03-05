package nov

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/ptime"
)

func timeMsgFromTime(common *novmsg.CommonHdr, m *novmsg.Time, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
	if m.ClockStatus != novmsg.ClockStatusValid {
		return nil, nil
	}
	t := gpsprot.TimeMsg{
		Tag:         tag,
		NativeMsgID: "TIME",
	}
	// leave t.GNSS zero; we don't know what the reference GNSS is
	if common.TimeStatus != novmsg.TimeStatusUnknown {
		tow := time.Duration(common.MillisecondsOfWeek) * time.Millisecond
		t.TAITime = ptime.GPS(int16(common.Week), tow)
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

func leapSecondIonUTC(common *novmsg.CommonHdr, ionutc *novmsg.IonUTC, mh gpsprot.MsgHandler, tRead time.Time) (bool, error) {
	_, now := msgHdrTime(common)
	params, err := utcConversionParamsFromIonUTC(ionutc, now)
	if err != nil {
		return false, err
	}
	lsm := &gpsprot.LeapSecondMsg{
		LeapSecond: params.LeapSecond,
		GNSS:       gpsprot.GPS,
	}
	mh.LeapSecond(lsm, tRead)
	return true, nil
}

// msgHdrTime extracts GNSS and time from NovAtel message header
func msgHdrTime(common *novmsg.CommonHdr) (gpsprot.GNSS, ptime.Time) {
	// NovAtel messages are always GPS time reference
	return gpsprot.GPS, ptime.GPS(int16(common.Week), time.Duration(common.MillisecondsOfWeek)*time.Millisecond)
}

type utcConversionParams struct {
	Correction ptime.CorrectionParams
	LeapSecond ptime.LeapSecond
}

func utcConversionParamsFromIonUTC(ionutc *novmsg.IonUTC, now ptime.Time) (*utcConversionParams, error) {
	gnssLS := ptime.GNSSLeapSecond{
		WNLSF:    uint8(ionutc.WnLsf),
		DN:       uint8(ionutc.Dn),
		DeltaLS:  int8(ionutc.DeltatLs),
		DeltaLSF: int8(ionutc.DeltatLsf),
	}
	ls, err := ptime.GPSLeapSecond(gnssLS, now)
	if err != nil {
		return nil, err
	}
	return &utcConversionParams{
		Correction: ptime.CorrectionParams{
			Ref:   ptime.GPS(int16(ionutc.UTCWn), time.Duration(ionutc.Tot)*time.Second),
			Bias:  ionutc.A0 * 1e9,
			Drift: ionutc.A1,
		},
		LeapSecond: ls,
	}, nil
}
