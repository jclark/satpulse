package ubx

import (
	"math"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ptime"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

func timeNavTimeGPS(m *bin.NavTimeGPS) *gpsmsg.Time {
	t := gpsmsg.Time{}
	t.TAITime = ptime.GPS(m.Week, m.ITOW, m.FTOW)
	if (m.Valid & bin.NavTimeGPSLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusGPS
	}
	t.Accuracy = time.Duration(m.TAcc)
	g := gpsmsg.GPS
	t.GNSS = &g
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeUTC(m *bin.NavTimeUTC) *gpsmsg.Time {
	if m.Valid&bin.NavTimeUTCValidUTC == 0 {
		return nil
	}
	t := gpsmsg.Time{}
	u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, m.Nano)
	t.UTCTime = &u
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = utcStandardToGNSS(m.Valid.UTCStandard())
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func iTOWEpoch(iTOW uint32) uint32 {
	return iTOW + 1
}

func utcStandardToGNSS(u bin.UTCStandard) *gpsmsg.MajorGNSS {
	g := gpsmsg.GPS
	switch u {
	case bin.UTCStandardUSNO:
		g = gpsmsg.GPS
	case bin.UTCStandardSU:
		g = gpsmsg.GLONASS
	case bin.UTCStandardNTSC:
		g = gpsmsg.BeiDou
	case bin.UTCStandardEU:
		g = gpsmsg.Galileo
	default:
		return nil
	}
	return &g
}

func timeTimTP(m *bin.TimTP) *gpsmsg.Time {
	if m.Flags&bin.TimTPTimeBase == bin.TimTPTimeBaseUTC {
		// In this case the m.TOWMS will not be the GPS time (but will have UTC offset added)
		// This will be problematic around a leap second, so ignore.
		// Can we do better?
		return nil
	}
	t := gpsmsg.Time{PrecedesPulse: true}
	t.TAITime = ptime.GPS(int16(m.Week), m.TOWMS, scaledMSToNS(m.TOWSubMS))
	t.PulseOffset = ptime.Picoseconds(m.QErr)
	var g gpsmsg.MajorGNSS
	t.GNSS = &g
	switch m.RefInfo & bin.TimTPTimeRefGNSS {
	case bin.TimTPTimeRefGPS:
		g = gpsmsg.GPS
	case bin.TimTPTimeRefGLONASS:
		g = gpsmsg.GLONASS
	case bin.TimTPTimeRefBeiDou:
		g = gpsmsg.BeiDou
	case bin.TimTPTimeRefGalileo:
		g = gpsmsg.Galileo
	default:
		t.GNSS = nil
	}
	return &t
}

// XXX need unit test for this
func scaledMSToNS(ms uint32) int32 {
	return int32(math.Round(math.Ldexp(float64(ms), -32) * 1e6))
}
