package unc

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nov"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
	"github.com/jclark/satpulse/gps/ptime"
)

func timeMsgFromRecTime(hdr *uncmsg.MsgHdr, recTime *uncmsg.RecTime, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
	if recTime.ClockStatus != novmsg.ClockStatusValid {
		return nil, nil
	}
	t := gpsprot.TimeMsg{
		Tag:         tag,
		NativeMsgID: "RECTIME",
	}
	if hdr.TimeStatus >= uncmsg.TimeStatusFine {
		t.GNSS, t.TAITime = msgHdrTime(hdr)
	}
	return nov.TimeMsgSetUTC(&t, &recTime.Time)
}

func msgHdrTime(hdr *uncmsg.MsgHdr) (gpsprot.GNSS, ptime.Time) {
	switch hdr.TimeRef {
	case uncmsg.TimeRefGPS:
		return gpsprot.GPS, ptime.GPS(int16(hdr.Week), time.Duration(hdr.MillisecondsOfWeek)*time.Millisecond)
	case uncmsg.TimeRefBDS:
		return gpsprot.BDS, ptime.BeiDou(int16(hdr.Week), time.Duration(hdr.MillisecondsOfWeek)*time.Millisecond)
	}
	return 0, 0
}

func msgHdrWeekStart(hdr *uncmsg.MsgHdr) ptime.Time {
	switch hdr.TimeRef {
	case uncmsg.TimeRefGPS:
		return ptime.GPS(int16(hdr.Week), 0)
	case uncmsg.TimeRefBDS:
		return ptime.BeiDou(int16(hdr.Week), 0)
	}
	return 0
}

type utcConversionParams struct {
	Correction ptime.CorrectionParams
	LeapSecond ptime.LeapSecond
}

func utcConversionParamsFromGPSUTC(utc *uncmsg.GPSUTC, now ptime.Time) (*utcConversionParams, error) {
	gnssLS, ok := gnssLeapSecondParams(utc.WnLsf, utc.Dn, utc.DeltatLs, utc.DeltatLsf)
	if !ok {
		return nil, nil
	}
	ls, err := ptime.GPSLeapSecond(gnssLS, now)
	if err != nil {
		return nil, err
	}
	return &utcConversionParams{
		Correction: ptime.CorrectionParams{
			Ref:   ptime.GPS(int16(utc.UTCWn), time.Duration(utc.Tot)*time.Second),
			Bias:  float64(utc.A0) * 1e9,
			Drift: float64(utc.A1),
		},
		LeapSecond: ls,
	}, nil
}

func utcConversionParamsFromGALUTC(utc *uncmsg.GALUTC, now ptime.Time) (*utcConversionParams, error) {
	gnssLS, ok := gnssLeapSecondParams(utc.WnLsf, utc.Dn, utc.DeltatLs, utc.DeltatLsf)
	if !ok {
		return nil, nil
	}
	ls, err := ptime.GalileoLeapSecond(gnssLS, now)
	if err != nil {
		return nil, err
	}
	return &utcConversionParams{
		Correction: ptime.CorrectionParams{
			Ref:   ptime.Galileo(int16(utc.UTCWn), time.Duration(utc.Tot)*time.Hour),
			Bias:  float64(utc.A0) * 1e9,
			Drift: float64(utc.A1),
		},
		LeapSecond: ls,
	}, nil
}

func utcConversionParamsFromBD3UTC(utc *uncmsg.BD3UTC, now ptime.Time) (*utcConversionParams, error) {
	gnssLS, ok := gnssLeapSecondParams(utc.WnLsf, utc.Dn, utc.DeltatLs, utc.DeltatLsf)
	if !ok {
		return nil, nil
	}
	ls, err := ptime.BeiDouLeapSecond(gnssLS, now)
	if err != nil {
		return nil, err
	}
	return &utcConversionParams{
		Correction: ptime.CorrectionParams{
			Ref:   ptime.BeiDou(int16(utc.UTCWn), time.Duration(utc.Tot)*time.Second),
			Bias:  float64(utc.A0) * 1e9,
			Drift: float64(utc.A1),
		},
		LeapSecond: ls,
	}, nil
}

func utcConversionParamsFromBDSUTC(utc *uncmsg.BDSUTC, weekStart ptime.Time, now ptime.Time) (*utcConversionParams, error) {
	gnssLS, ok := gnssLeapSecondParams(utc.WnLsf, utc.Dn, utc.DeltatLs, utc.DeltatLsf)
	if !ok {
		return nil, nil
	}
	ls, err := ptime.BeiDouLeapSecond(gnssLS, now)
	if err != nil {
		return nil, err
	}
	return &utcConversionParams{
		Correction: ptime.CorrectionParams{
			Ref:   weekStart,
			Bias:  float64(utc.A0) * 1e9,
			Drift: float64(utc.A1),
		},
		LeapSecond: ls,
	}, nil
}

// gnssLeapSecondParams converts UTC message leap second fields to a
// GNSSLeapSecond. Returns false if the fields indicate no data is available.
func gnssLeapSecondParams(wnLsf uint32, dn uint32, deltaLS int32, deltaLSF int32) (ptime.GNSSLeapSecond, bool) {
	if wnLsf == 0 && dn == 0 && deltaLS == 0 && deltaLSF == 0 {
		// no data available
		return ptime.GNSSLeapSecond{}, false
	}
	return ptime.GNSSLeapSecond{
		WNLSF:    uint8(wnLsf),
		DN:       uint8(dn),
		DeltaLS:  int8(deltaLS),
		DeltaLSF: int8(deltaLSF),
	}, true
}
