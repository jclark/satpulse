package sdbp

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
	"github.com/jclark/satpulse/gps/ptime"
)

// timeDatGPST converts DatGPST to TimeMsg.
// DAT-GPST reports GPS time (week/TOW in GPS time scale).
// Returns nil when the time is invalid.
func timeDatGPST(m *sdbpbin.DatGPST) *gpsprot.TimeMsg {
	if !m.TimeValid() {
		return nil
	}
	t := gpsprot.TimeMsg{
		Ref:         gpsprot.NavSolution,
		NativeMsgID: "DAT-GPST",
		GNSS:        gpsprot.GPS,
		TAITime:     ptime.GPS(int16(m.GPSWeek), ptime.Seconds(m.TOW)),
	}
	if m.TimeAccuracy != 0xFFFFFFFF {
		t.Accuracy = time.Duration(m.TimeAccuracy) * time.Nanosecond
	}
	return &t
}

// timeDatTPPS converts DatTPPS to TimeMsg.
// When TimeRef is UTC, the TOW is in UTC and we fill UTCTime.
// When TimeRef is a GNSS system, the TOW is in that GNSS time scale and we fill TAITime.
func timeDatTPPS(m *sdbpbin.DatTPPS) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{
		Ref:         gpsprot.PrePulse,
		NativeMsgID: "DAT-TPPS",
	}
	tow := ptime.Seconds(m.TOW)
	switch m.TimeRef {
	case sdbpbin.TPPSTimeRefUTC:
		ut := ptime.GPSUTC(m.Week, tow)
		t.UTCTime = &ut
		t.GNSS = gpsprot.GPS
	case sdbpbin.TPPSTimeRefBDS:
		t.TAITime = ptime.BeiDou(int16(m.Week), tow)
		t.GNSS = gpsprot.BDS
	case sdbpbin.TPPSTimeRefGPS:
		t.TAITime = ptime.GPS(int16(m.Week), tow)
		t.GNSS = gpsprot.GPS
	case sdbpbin.TPPSTimeRefGLO:
		t.TAITime = ptime.GPS(int16(m.Week), tow) // assumes GPS week numbering
		t.GNSS = gpsprot.GLO
	case sdbpbin.TPPSTimeRefGAL:
		t.TAITime = ptime.GPS(int16(m.Week), tow) // Galileo uses same epoch as GPS
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
