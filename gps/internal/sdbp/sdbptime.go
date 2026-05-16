package sdbp

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
	"github.com/jclark/satpulse/gps/ptime"
)

// timeDatGPST converts DatGPST to TimeMsg.
// Returns nil when the time is invalid.
func timeDatGPST(m *sdbpbin.DatGPST) *gpsprot.TimeMsg {
	if !m.TimeValid() {
		return nil
	}
	t := gpsprot.TimeMsg{
		Ref:         gpsprot.NavSolution,
		NativeMsgID: "DAT-GPST",
		GNSS:        gpsprot.GPS,
		TAITime:     ptime.GPS(int16(m.Week), ptime.Seconds(m.TOW)),
	}
	if m.TimeAccuracy != sdbpbin.TimeAccuracyInvalid {
		t.Accuracy = time.Duration(m.TimeAccuracy) * time.Nanosecond
	}
	return &t
}

// timeDatBDST converts DatBDST to TimeMsg.
// Returns nil when the time is invalid.
func timeDatBDST(m *sdbpbin.DatBDST) *gpsprot.TimeMsg {
	if !m.TimeValid() {
		return nil
	}
	t := gpsprot.TimeMsg{
		Ref:         gpsprot.NavSolution,
		NativeMsgID: "DAT-BDST",
		GNSS:        gpsprot.BDS,
		TAITime:     ptime.BeiDou(int16(m.Week), ptime.Seconds(m.TOW)),
	}
	if m.TimeAccuracy != sdbpbin.TimeAccuracyInvalid {
		t.Accuracy = time.Duration(m.TimeAccuracy) * time.Nanosecond
	}
	return &t
}

// timeDatGALT converts DatGALT to TimeMsg.
// Returns nil when the time is invalid.
func timeDatGALT(m *sdbpbin.DatGALT) *gpsprot.TimeMsg {
	if !m.TimeValid() {
		return nil
	}
	t := gpsprot.TimeMsg{
		Ref:         gpsprot.NavSolution,
		NativeMsgID: "DAT-GALT",
		GNSS:        gpsprot.GAL,
		TAITime:     ptime.Galileo(int16(m.Week), ptime.Seconds(m.TOW)),
	}
	if m.TimeAccuracy != sdbpbin.TimeAccuracyInvalid {
		t.Accuracy = time.Duration(m.TimeAccuracy) * time.Nanosecond
	}
	return &t
}

// timeDatTPPS converts DatTPPS to TimeMsg.
// When TimeRef is UTC or GLONASS, fills UTCTime.
// When TimeRef is a GNSS system, fills TAITime.
func timeDatTPPS(m *sdbpbin.DatTPPS) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{
		Ref:         gpsprot.PrePulse,
		NativeMsgID: "DAT-TPPS",
	}
	tow := ptime.Seconds(m.TOW)
	switch m.TimeRef {
	case sdbpbin.TimeRefUTC:
		t.UTCTime.Set(ptime.GPSUTC(m.Week, tow))
		t.GNSS = gpsprot.GPS
	case sdbpbin.TimeRefBDS:
		t.TAITime = ptime.BeiDou(int16(m.Week), tow)
		t.GNSS = gpsprot.BDS
	case sdbpbin.TimeRefGPS:
		t.TAITime = ptime.GPS(int16(m.Week), tow)
		t.GNSS = gpsprot.GPS
	case sdbpbin.TimeRefGLO:
		// GLONASS time = UTC + 3h, using GPS week numbers.
		// Convert to date/timeOfDay, then subtract 3h.
		// UTCTime supports negative TimeOfDay (down to -24h).
		ut := ptime.GPSUTC(m.Week, tow)
		ut.TimeOfDay -= 3 * time.Hour
		t.UTCTime.Set(ut)
		t.GNSS = gpsprot.GLO
	case sdbpbin.TimeRefGAL:
		t.TAITime = ptime.Galileo(int16(m.Week), tow)
		t.GNSS = gpsprot.GAL
	default:
		t.TAITime = ptime.GPS(int16(m.Week), tow)
		t.GNSS = gpsprot.GPS
	}
	if m.PPSResidual != 0 {
		t.PulseOffset.Set(float64(m.PPSResidual) * 1e-3) // ps -> ns
	}
	return &t
}

// timeDatUTCT2 converts DatUTCT2 to TimeMsg.
// Returns nil when the time fields are not valid.
func timeDatUTCT2(m *sdbpbin.DatUTCT2) *gpsprot.TimeMsg {
	if m.Valid&(sdbpbin.DatUTCT2HMS|sdbpbin.DatUTCT2YMD) != sdbpbin.DatUTCT2HMS|sdbpbin.DatUTCT2YMD {
		return nil
	}
	date := time.Date(int(m.Year), time.Month(m.Month), int(m.Day), 0, 0, 0, 0, time.UTC)
	tod := time.Duration(m.Hour)*time.Hour +
		time.Duration(m.Min)*time.Minute +
		time.Duration(m.Sec)*time.Second +
		time.Duration(m.SecFrac)*time.Nanosecond
	t := gpsprot.TimeMsg{
		Ref:         gpsprot.NavSolution,
		NativeMsgID: "DAT-UTCT2",
		UTCTime:     opt.Make(ptime.UTCTime{Date: date, TimeOfDay: tod}),
		GNSS:        utct2GNSS(m.RefGrid),
	}
	if m.Accuracy != 0 {
		t.Accuracy = time.Duration(m.Accuracy) * time.Nanosecond
	}
	return &t
}

func utct2GNSS(ref sdbpbin.DatUTCT2RefGrid) gpsprot.GNSS {
	switch ref {
	case sdbpbin.DatUTCT2RefBDS:
		return gpsprot.BDS
	case sdbpbin.DatUTCT2RefGPS:
		return gpsprot.GPS
	case sdbpbin.DatUTCT2RefGLO:
		return gpsprot.GLO
	case sdbpbin.DatUTCT2RefGAL:
		return gpsprot.GAL
	default:
		return 0
	}
}

// leapSecondDatUTCT2 converts DatUTCT2 to LeapSecondMsg.
// Returns nil when leap forecast/correction bits are not set.
func leapSecondDatUTCT2(m *sdbpbin.DatUTCT2) *gpsprot.LeapSecondMsg {
	if m.Valid&(sdbpbin.DatUTCT2LeapForecast|sdbpbin.DatUTCT2LeapCorr) !=
		sdbpbin.DatUTCT2LeapForecast|sdbpbin.DatUTCT2LeapCorr {
		return nil
	}
	cur := int16(m.LeapSec) + ptime.TAIMinusGPS
	after := cur + int16(m.LeapChange)
	date := time.Date(int(m.LeapYear), time.Month(m.LeapMonth), int(m.LeapDay), 0, 0, 0, 0, time.UTC)
	return &gpsprot.LeapSecondMsg{
		LeapSecond: ptime.LeapSecondOnDate(date, cur, after),
		GNSS:       utct2GNSS(m.RefGrid),
	}
}

// leapSecondDatGNSSU converts a DatGNSSU to LeapSecondMsg using the appropriate GNSS-specific decoder.
// Returns nil on error.
func leapSecondDatGNSSU(m *sdbpbin.DatGNSSU, gnss gpsprot.GNSS) *gpsprot.LeapSecondMsg {
	g := ptime.GNSSLeapSecond{
		WNLSF:    uint8(m.WNLSF),
		DN:       m.DN,
		DeltaLS:  m.DeltaTLS,
		DeltaLSF: m.DeltaTLSF,
	}
	tow := ptime.Seconds(float64(m.TOT))
	var now ptime.Time
	var ls ptime.LeapSecond
	var err error
	switch gnss {
	case gpsprot.BDS:
		now = ptime.BeiDou(int16(m.WNOT), tow)
		ls, err = ptime.BeiDouLeapSecond(g, now)
	case gpsprot.GAL:
		now = ptime.Galileo(int16(m.WNOT), tow)
		ls, err = ptime.GalileoLeapSecond(g, now)
	default:
		now = ptime.GPS(int16(m.WNOT), tow)
		ls, err = ptime.GPSLeapSecond(g, now)
	}
	if err != nil {
		return nil
	}
	return &gpsprot.LeapSecondMsg{LeapSecond: ls, GNSS: gnss}
}
