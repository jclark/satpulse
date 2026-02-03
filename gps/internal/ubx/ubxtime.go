package ubx

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func timeNavTimeGPS(m *ubxbin.NavTimeGPS) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEGPS"}
	if (m.Valid&ubxbin.NavTimeGPSTOWValid) != 0 && (m.Valid&ubxbin.NavTimeGPSWeekValid) != 0 {
		// iTOW field is in milliseconds
		t.TAITime = ptime.GPS(m.Week, msTOW(m.ITOW)+nsTOW(m.FTOW))
	}
	if (m.Valid & ubxbin.NavTimeGPSLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusGPS
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsprot.GPS
	return &t
}

func timeNavTimeBDS(m *ubxbin.NavTimeBDS) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEBDS"}
	if (m.Valid&ubxbin.NavTimeBDSSOWValid) != 0 && (m.Valid&ubxbin.NavTimeBDSWeekValid) != 0 {
		t.TAITime = ptime.BeiDou(m.Week, sTOW(m.SOW)+nsTOW(m.FSOW))
	}
	if (m.Valid & ubxbin.NavTimeBDSLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusBeiDou
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsprot.BDS
	return &t
}

func timeNavTimeGal(m *ubxbin.NavTimeGal) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEGAL"}
	if (m.Valid&ubxbin.NavTimeGalTOWValid) != 0 && (m.Valid&ubxbin.NavTimeGalWnoValid) != 0 {
		// galTOW field is in seconds
		t.TAITime = ptime.Galileo(m.GalWno, sTOW(m.GalTOW)+nsTOW(m.FGalTOW))
	}
	if (m.Valid & ubxbin.NavTimeGalLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusGalileo
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsprot.GAL
	return &t
}

func timeNavTimeGLO(m *ubxbin.NavTimeGLO) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEGLO"}
	if (m.Valid&ubxbin.NavTimeGLOTODValid) != 0 && (m.Valid&ubxbin.NavTimeGLODateValid) != 0 {
		u := ptime.GLONASS(m.N4, m.Nt, sTOW(m.TOD)+nsTOW(m.FTOD))
		t.UTCTime = &u
	}
	t.GNSS = gpsprot.GLO
	t.Accuracy = time.Duration(m.TAcc)
	return &t
}

func timeNavTimeUTC(m *ubxbin.NavTimeUTC) *gpsprot.TimeMsg {
	if (m.Valid & ubxbin.NavTimeUTCValidUTC) == 0 {
		return nil
	}
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEUTC"}
	u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, m.Nano)
	t.UTCTime = &u
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = utcStandardToGNSS(m.Valid.UTCStandard())
	return &t
}

func timeNavPVT(m *ubxbin.NavPVT) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-PVT"}
	if (m.Valid & (ubxbin.NavPVTValidTime | ubxbin.NavPVTValidDate)) == (ubxbin.NavPVTValidTime | ubxbin.NavPVTValidDate) {
		u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, m.Nano)
		t.UTCTime = &u
	}
	t.Accuracy = time.Duration(m.TAcc)
	// XXX there are some interesting validity flags that we should try to represent
	return &t
}

func utcStandardToGNSS(u ubxbin.UTCStandard) gpsprot.GNSS {
	switch u {
	case ubxbin.UTCStandardUSNO:
		return gpsprot.GPS
	case ubxbin.UTCStandardSU:
		return gpsprot.GLO
	case ubxbin.UTCStandardNTSC:
		return gpsprot.BDS
	case ubxbin.UTCStandardEU:
		return gpsprot.GAL
	default:
		return 0
	}
}

func timeTimTP(m *ubxbin.TimTP) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{Ref: gpsprot.PrePulse, NativeMsgID: "UBX-TIM-TP"}
	if (m.Flags & ubxbin.TimTPQErrInvalid) == 0 {
		off := float64(m.QErr) / 1000.0
		t.PulseOffset = &off
	}
	tow := msTOW(m.TOWMS) + msScaledTOW(m.TOWSubMS)
	if (m.Flags & ubxbin.TimTPTimeBase) == ubxbin.TimTPTimeBaseUTC {
		// XXX This will be problematic around a leap second.
		// we should return nil in the case that we might be on a leap second
		utc := ptime.GPSUTC(m.Week, tow)
		t.UTCTime = &utc
		return &t
	}
	conv := ptime.GPS
	switch m.RefInfo & ubxbin.TimTPTimeRefGNSS {
	case ubxbin.TimTPTimeRefGPS:
		t.GNSS = gpsprot.GPS
	case ubxbin.TimTPTimeRefGLONASS:
		t.GNSS = gpsprot.GLO
	case ubxbin.TimTPTimeRefBeiDou:
		t.GNSS = gpsprot.BDS
		conv = ptime.BeiDou
	case ubxbin.TimTPTimeRefGalileo:
		t.GNSS = gpsprot.GAL
		conv = ptime.Galileo
	default:
		return nil
	}
	t.TAITime = conv(int16(m.Week), tow)
	return &t
}

func timeTimTos(m *ubxbin.TimTos) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{Ref: gpsprot.PostPulse, NativeMsgID: "UBX-TIM-TOS"}
	if (m.Flags & ubxbin.TimTosUTCTimeValid) != 0 {
		u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Minute, m.Second, 0)
		t.UTCTime = &u
		t.GNSS = utcStandardToGNSS(m.UTCStandard)
		t.Accuracy = time.Duration(m.UTCUncertainty)
	}
	// XXX how to deal with UTCOffset?
	// If we have a GNSS time that we understand, then use that for accuracy/GNSS metadata.
	// GLONASS works in UTC not TAI, so I don't see how it can work with a week number.
	// XXX need to check what happens if we enable only GLONASS
	if (m.Flags & ubxbin.TimTosGNSSTimeValid) != 0 {
		g, toTAI := toTAIFunc(m.GNSSID)
		if toTAI != nil && uint32(int16(m.Week)) == m.Week {
			t.TAITime = toTAI(int16(m.Week), sTOW(m.TOW))
			t.GNSS = g
			t.Accuracy = time.Duration(m.GNSSUncertainty)
			pulseOff := float64(m.GNSSOffset)
			t.PulseOffset = &pulseOff
		}
	}
	return &t
}

func toTAIFunc(g ubxbin.GNSSID) (gpsprot.GNSS, func(int16, time.Duration) ptime.Time) {
	switch g {
	case ubxbin.GPS:
		return gpsprot.GPS, ptime.GPS
	case ubxbin.BDS:
		return gpsprot.BDS, ptime.BeiDou
	case ubxbin.GAL:
		return gpsprot.GAL, ptime.Galileo
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
