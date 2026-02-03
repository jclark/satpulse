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
	t.GNSS = gnssIDToGNSS(m.TimeSrc)
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

func gnssIDToGNSS(id casbin.GNSSID) gpsprot.GNSS {
	switch id {
	case casbin.GPS:
		return gpsprot.GPS
	case casbin.BDS:
		return gpsprot.BDS
	case casbin.GLN:
		return gpsprot.GLO
	}
	return 0
}
