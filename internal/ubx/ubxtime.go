package ubx

import (
	"math"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ptime"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

func timeNavTimeGPS(m *bin.NavTimeGPS) *gpsmsg.Time {
	t := gpsmsg.Time{SrcType: "UBX-NAV-TIMEGPS"}
	if (m.Valid&bin.NavTimeGPSTOWValid) != 0 && (m.Valid&bin.NavTimeGPSWeekValid) != 0 {
		// iTOW field is in milliseconds
		t.TAITime = ptime.GPS(m.Week, msTOW(m.ITOW)+nsTOW(m.FTOW))
	}
	if (m.Valid & bin.NavTimeGPSLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusGPS
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsmsg.GPS
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeBDS(m *bin.NavTimeBDS) *gpsmsg.Time {
	t := gpsmsg.Time{SrcType: "UBX-NAV-TIMEBDS"}
	if (m.Valid&bin.NavTimeBDSSOWValid) != 0 && (m.Valid&bin.NavTimeBDSWeekValid) != 0 {
		t.TAITime = ptime.BeiDou(m.Week, sTOW(m.SOW)+nsTOW(m.FSOW))
	}
	if (m.Valid & bin.NavTimeBDSLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusBeiDou
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsmsg.BeiDou
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeGal(m *bin.NavTimeGal) *gpsmsg.Time {
	t := gpsmsg.Time{SrcType: "UBX-NAV-TIMEGAL"}
	if (m.Valid&bin.NavTimeGalTOWValid) != 0 && (m.Valid&bin.NavTimeGalWnoValid) != 0 {
		// galTOW field is in seconds
		t.TAITime = ptime.Galileo(m.GalWno, sTOW(m.GalTOW)+nsTOW(m.FGalTOW))
	}
	if (m.Valid & bin.NavTimeGalLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusGalileo
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsmsg.Galileo
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeGLO(m *bin.NavTimeGLO) *gpsmsg.Time {
	t := gpsmsg.Time{SrcType: "UBX-NAV-TIMEGLO"}
	if (m.Valid&bin.NavTimeGLOTODValid) != 0 && (m.Valid&bin.NavTimeGLODateValid) != 0 {
		u := ptime.GLONASS(m.N4, m.Nt, sTOW(m.TOD)+nsTOW(m.FTOD))
		t.UTCTime = &u
	}
	t.GNSS = gpsmsg.GLONASS
	t.NavEpoch = iTOWEpoch(m.ITOW)
	t.Accuracy = time.Duration(m.TAcc)
	return &t
}

func timeNavTimeUTC(m *bin.NavTimeUTC) *gpsmsg.Time {
	if (m.Valid & bin.NavTimeUTCValidUTC) == 0 {
		return nil
	}
	t := gpsmsg.Time{SrcType: "UBX-NAV-TIMEUTC"}
	u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, m.Nano)
	t.UTCTime = &u
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = utcStandardToGNSS(m.Valid.UTCStandard())
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavPVT(m *bin.NavPVT) *gpsmsg.Time {
	t := gpsmsg.Time{SrcType: "UBX-NAV-PVT"}
	if (m.Valid & (bin.NavPVTValidTime | bin.NavPVTValidDate)) == (bin.NavPVTValidTime | bin.NavPVTValidDate) {
		u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, m.Nano)
		t.UTCTime = &u
	}
	t.Accuracy = time.Duration(m.TAcc)
	// XXX there are some interesting validity flags that we should try to represent
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func iTOWEpoch(iTOW uint32) uint32 {
	return iTOW + 1
}

func utcStandardToGNSS(u bin.UTCStandard) gpsmsg.MajorGNSS {
	switch u {
	case bin.UTCStandardUSNO:
		return gpsmsg.GPS
	case bin.UTCStandardSU:
		return gpsmsg.GLONASS
	case bin.UTCStandardNTSC:
		return gpsmsg.BeiDou
	case bin.UTCStandardEU:
		return gpsmsg.Galileo
	default:
		return 0
	}
}

func timeTimTP(m *bin.TimTP) *gpsmsg.Time {
	if (m.Flags & bin.TimTPTimeBase) == bin.TimTPTimeBaseUTC {
		// In this case the m.TOWMS will not be the GPS time (but will have GPS-UTC offset subtracted)
		// This will be problematic around a leap second, so ignore.
		// Can we do better?
		return nil
	}
	t := gpsmsg.Time{Ref: gpsmsg.NextPulse, SrcType: "UBX-TIM-TP"}
	t.PulseOffset = ptime.Picoseconds(m.QErr)
	conv := ptime.GPS
	switch m.RefInfo & bin.TimTPTimeRefGNSS {
	case bin.TimTPTimeRefGPS:
		t.GNSS = gpsmsg.GPS
	case bin.TimTPTimeRefGLONASS:
		t.GNSS = gpsmsg.GLONASS
	case bin.TimTPTimeRefBeiDou:
		t.GNSS = gpsmsg.BeiDou
		conv = ptime.BeiDou
	case bin.TimTPTimeRefGalileo:
		t.GNSS = gpsmsg.Galileo
		conv = ptime.Galileo
	default:
		return nil
	}
	t.TAITime = conv(int16(m.Week), msTOW(m.TOWMS)+msScaledTOW(m.TOWSubMS))
	return &t
}

func timeTimTos(m *bin.TimTos) *gpsmsg.Time {
	t := gpsmsg.Time{Ref: gpsmsg.LastPulse, SrcType: "UBX-TIM-TOS"}
	if (m.Flags & bin.TimTosUTCTimeValid) != 0 {
		u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Minute, m.Second, 0)
		t.UTCTime = &u
		t.GNSS = utcStandardToGNSS(m.UTCStandard)
		t.Accuracy = time.Duration(m.UTCUncertainty)
	}
	t.PulseOffset = time.Duration(m.UTCOffset)
	// If we have a GNSS time that we understand, then use that for accuracy/GNSS metadata.
	// GLONASS works in UTC not TAI, so I don't see how it can work with a week number.
	// XXX need to check what happens if we enable only GLONASS
	if (m.Flags & bin.TimTosGNSSTimeValid) != 0 {
		g, toTAI := toTAIFunc(m.GNSSID)
		if toTAI != nil && uint32(int16(m.Week)) == m.Week {
			t.TAITime = toTAI(int16(m.Week), sTOW(m.TOW))
			t.GNSS = g
			t.Accuracy = time.Duration(m.GNSSUncertainty)
			t.PulseOffset = time.Duration(m.GNSSOffset)
		}
	}
	return &t
}

func toTAIFunc(g bin.GNSSID) (gpsmsg.MajorGNSS, func(int16, time.Duration) ptime.Time) {
	switch g {
	case bin.GPS:
		return gpsmsg.GPS, ptime.GPS
	case bin.BeiDou:
		return gpsmsg.BeiDou, ptime.BeiDou
	case bin.Galileo:
		return gpsmsg.Galileo, ptime.Galileo
	}
	return 0, nil
}

func msScaledTOW(ms uint32) time.Duration {
	return time.Duration(math.Round(math.Ldexp(float64(ms), -32) * float64(time.Millisecond)))
}

func sTOW(s uint32) time.Duration {
	return time.Duration(s) * time.Second
}

func msTOW(ms uint32) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func nsTOW(ns int32) time.Duration {
	return time.Duration(ns) * time.Nanosecond
}
