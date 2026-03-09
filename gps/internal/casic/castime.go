package casic

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
)

// timeNavTimeUTC converts NavTimeUTC to TimeMsg.
// Always returns a TimeMsg, but with nil UTCTime when the solution is invalid.
func timeNavTimeUTC(m *casbin.NavTimeUTC) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "NAV-TIMEUTC"}
	if m.DateValid < casbin.NavDateFromSatellite || (m.Valid&casbin.NavTimeUTCTOWValid) == 0 {
		return &t
	}
	// MsErr is residual error in ms after rounding to whole milliseconds
	nanos := int32(math.Round(float64(m.MsErr) * 1e6))
	u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, nanos)
	t.UTCTime = &u
	// TAcc is variance scaled by 1/c², so actual variance = TAcc * c²
	// Convert to standard deviation: sqrt(TAcc) * c
	const c = 299792458.0
	if m.TAcc > 0 {
		t.Accuracy = time.Duration(math.Sqrt(float64(m.TAcc)) * c * float64(time.Second))
	}
	t.GNSS = casicGNSSIDToGNSS(uint8(m.TimeSrc))
	return &t
}

// timeNavSol converts NavSol to TimeMsg.
// Always returns a TimeMsg, but with zero TAITime when the solution is invalid.
func timeNavSol(m *casbin.NavSol) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "NAV-SOL"}
	if m.PosValid < casbin.NavPos3D {
		return &t
	}
	t.GNSS, t.TAITime = gnssTime(m.TimeSrc, m.Week, m.TOW)
	return &t
}

// timeTimTP converts TimTP to TimeMsg.
// The TimTP message gives the time of the next pulse, emitted before the pulse.
func timeTimTP(m *casbin.TimTP) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{Ref: gpsprot.PrePulse, NativeMsgID: "TIM-TP"}
	t.GNSS, t.TAITime = gnssTime(m.RefTimeGNSS(), m.Wn, m.TOW)
	return &t
}

// timeNav2Sol converts Nav2Sol to TimeMsg.
// Returns nil when the fix is below 2D.
func timeNav2Sol(m *casbin.Nav2Sol) *gpsprot.TimeMsg {
	if m.FixFlags < casbin.Nav2Fix2D {
		return nil
	}
	t := gpsprot.TimeMsg{NativeMsgID: "NAV2-SOL"}
	t.GNSS = gpsprot.GPS
	t.TAITime = ptime.GPS(int16(m.Wn), time.Duration(m.TOW)*time.Millisecond)
	return &t
}

// gnssTime converts CASIC GNSS ID, week number, and TOW to gpsprot.GNSS and TAI time.
// Returns zero GNSS and Time for unsupported GNSS IDs.
func gnssTime(id casbin.GNSSID, week uint16, towSec float64) (gpsprot.GNSS, ptime.Time) {
	tow := ptime.Seconds(towSec)
	switch id {
	case casbin.GPS:
		return gpsprot.GPS, ptime.GPS(int16(week), tow)
	case casbin.BDS:
		return gpsprot.BDS, ptime.BeiDou(int16(week), tow)
	case casbin.GLN:
		return gpsprot.GLO, ptime.GLONASSWeek(int16(week), tow)
	}
	return 0, 0
}

// timeNav2TimeUTC converts Nav2TimeUTC to TimeMsg.
// Returns nil when the time solution is not usable.
func timeNav2TimeUTC(m *casbin.Nav2TimeUTC) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "NAV2-TIMEUTC"}
	if m.TFlags&(casbin.Nav2TimeTOWValid|casbin.Nav2TimeReliable) != casbin.Nav2TimeTOWValid|casbin.Nav2TimeReliable {
		return &t
	}
	// Fractional time: Cs (centiseconds) + Subcs (cs error) + Subms (sub-ms, scale 2^-30)
	fracMs := float64(m.Cs)*10 + float64(m.Subcs) + float64(m.Subms)*math.Exp2(-30)
	nanos := int32(math.Round(fracMs * 1e6))
	u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, nanos)
	t.UTCTime = &u
	// V6 TAcc is nanoseconds (direct std dev, not variance)
	if m.TAcc > 0 {
		t.Accuracy = time.Duration(m.TAcc)
	}
	t.GNSS = casicGNSSIDToGNSS(uint8(m.TimeSrc))
	return &t
}

// casicGNSSIDToGNSS maps CASIC GNSS ID to gpsprot.GNSS.
// The same numbering is used by V5 GNSSID, V6 Nav2TimeSrc, and V6 NAV2-SIG gnssid.
func casicGNSSIDToGNSS(id uint8) gpsprot.GNSS {
	switch id {
	case 0:
		return gpsprot.GPS
	case 1:
		return gpsprot.BDS
	case 2:
		return gpsprot.GLO
	case 3:
		return gpsprot.GAL
	case 4:
		return gpsprot.QZSS
	case 5:
		return gpsprot.SBAS
	case 6:
		return gpsprot.NAVIC
	}
	return 0
}
