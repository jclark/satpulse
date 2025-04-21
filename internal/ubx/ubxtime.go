package ubx

import (
	"math"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

func timeNavTimeGPS(m *bin.NavTimeGPS) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEGPS"}
	if (m.Valid&bin.NavTimeGPSTOWValid) != 0 && (m.Valid&bin.NavTimeGPSWeekValid) != 0 {
		// iTOW field is in milliseconds
		t.TAITime = ptime.GPS(m.Week, msTOW(m.ITOW)+nsTOW(m.FTOW))
	}
	if (m.Valid & bin.NavTimeGPSLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusGPS
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsprot.GPS
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeBDS(m *bin.NavTimeBDS) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEBDS"}
	if (m.Valid&bin.NavTimeBDSSOWValid) != 0 && (m.Valid&bin.NavTimeBDSWeekValid) != 0 {
		t.TAITime = ptime.BeiDou(m.Week, sTOW(m.SOW)+nsTOW(m.FSOW))
	}
	if (m.Valid & bin.NavTimeBDSLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusBeiDou
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsprot.BDS
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeGal(m *bin.NavTimeGal) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEGAL"}
	if (m.Valid&bin.NavTimeGalTOWValid) != 0 && (m.Valid&bin.NavTimeGalWnoValid) != 0 {
		// galTOW field is in seconds
		t.TAITime = ptime.Galileo(m.GalWno, sTOW(m.GalTOW)+nsTOW(m.FGalTOW))
	}
	if (m.Valid & bin.NavTimeGalLeapSValid) != 0 {
		t.UTCOffset = m.LeapS + ptime.TAIMinusGalileo
	}
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = gpsprot.GAL
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeGLO(m *bin.NavTimeGLO) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEGLO"}
	if (m.Valid&bin.NavTimeGLOTODValid) != 0 && (m.Valid&bin.NavTimeGLODateValid) != 0 {
		u := ptime.GLONASS(m.N4, m.Nt, sTOW(m.TOD)+nsTOW(m.FTOD))
		t.UTCTime = &u
	}
	t.GNSS = gpsprot.GLO
	t.NavEpoch = iTOWEpoch(m.ITOW)
	t.Accuracy = time.Duration(m.TAcc)
	return &t
}

func timeNavTimeUTC(m *bin.NavTimeUTC) *gpsprot.TimeMsg {
	if (m.Valid & bin.NavTimeUTCValidUTC) == 0 {
		return nil
	}
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-TIMEUTC"}
	u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, m.Nano)
	t.UTCTime = &u
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = utcStandardToGNSS(m.Valid.UTCStandard())
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavPVT(m *bin.NavPVT) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "UBX-NAV-PVT"}
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

func utcStandardToGNSS(u bin.UTCStandard) gpsprot.GNSS {
	switch u {
	case bin.UTCStandardUSNO:
		return gpsprot.GPS
	case bin.UTCStandardSU:
		return gpsprot.GLO
	case bin.UTCStandardNTSC:
		return gpsprot.BDS
	case bin.UTCStandardEU:
		return gpsprot.GAL
	default:
		return 0
	}
}

func timeTimTP(m *bin.TimTP) *gpsprot.TimeMsg {
	if (m.Flags & bin.TimTPTimeBase) == bin.TimTPTimeBaseUTC {
		// In this case the m.TOWMS will not be the GPS time (but will have GPS-UTC offset subtracted)
		// This will be problematic around a leap second, so ignore.
		// Can we do better?
		return nil
	}
	t := gpsprot.TimeMsg{Ref: gpsprot.PrePulse, NativeMsgID: "UBX-TIM-TP"}
	if (m.Flags & bin.TimTPQErrInvalid) == 0 {
		off := ptime.Picoseconds(m.QErr)
		t.PulseOffset = &off
	}
	conv := ptime.GPS
	switch m.RefInfo & bin.TimTPTimeRefGNSS {
	case bin.TimTPTimeRefGPS:
		t.GNSS = gpsprot.GPS
	case bin.TimTPTimeRefGLONASS:
		t.GNSS = gpsprot.GLO
	case bin.TimTPTimeRefBeiDou:
		t.GNSS = gpsprot.BDS
		conv = ptime.BeiDou
	case bin.TimTPTimeRefGalileo:
		t.GNSS = gpsprot.GAL
		conv = ptime.Galileo
	default:
		return nil
	}
	t.TAITime = conv(int16(m.Week), msTOW(m.TOWMS)+msScaledTOW(m.TOWSubMS))
	return &t
}

func timeTimTos(m *bin.TimTos) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{Ref: gpsprot.PostPulse, NativeMsgID: "UBX-TIM-TOS"}
	if (m.Flags & bin.TimTosUTCTimeValid) != 0 {
		u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Minute, m.Second, 0)
		t.UTCTime = &u
		t.GNSS = utcStandardToGNSS(m.UTCStandard)
		t.Accuracy = time.Duration(m.UTCUncertainty)
	}
	// XXX how to deal with UTCOffset?
	// If we have a GNSS time that we understand, then use that for accuracy/GNSS metadata.
	// GLONASS works in UTC not TAI, so I don't see how it can work with a week number.
	// XXX need to check what happens if we enable only GLONASS
	if (m.Flags & bin.TimTosGNSSTimeValid) != 0 {
		g, toTAI := toTAIFunc(m.GNSSID)
		if toTAI != nil && uint32(int16(m.Week)) == m.Week {
			t.TAITime = toTAI(int16(m.Week), sTOW(m.TOW))
			t.GNSS = g
			t.Accuracy = time.Duration(m.GNSSUncertainty)
			pulseOff := time.Duration(m.GNSSOffset)
			t.PulseOffset = &pulseOff
		}
	}
	return &t
}

func toTAIFunc(g bin.GNSSID) (gpsprot.GNSS, func(int16, time.Duration) ptime.Time) {
	switch g {
	case bin.GPS:
		return gpsprot.GPS, ptime.GPS
	case bin.BDS:
		return gpsprot.BDS, ptime.BeiDou
	case bin.GAL:
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
