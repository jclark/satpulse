package sdbp

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
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
	if m.TimeAccuracy != 0xFFFFFFFF {
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
	if m.TimeAccuracy != 0xFFFFFFFF {
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
	if m.TimeAccuracy != 0xFFFFFFFF {
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
		ut := ptime.GPSUTC(m.Week, tow)
		t.UTCTime = &ut
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
		t.UTCTime = &ut
		t.GNSS = gpsprot.GLO
	case sdbpbin.TimeRefGAL:
		t.TAITime = ptime.Galileo(int16(m.Week), tow)
		t.GNSS = gpsprot.GAL
	default:
		t.TAITime = ptime.GPS(int16(m.Week), tow)
		t.GNSS = gpsprot.GPS
	}
	if m.PPSResidual != 0 {
		offset := float64(m.PPSResidual) * 1e-12
		t.PulseOffset = &offset
	}
	return &t
}
