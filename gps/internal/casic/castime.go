package casic

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
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
	t.UTCTime.Set(ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, nanos))
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

// timeTim2Tpx converts Tim2Tpx to TimeMsg.
// Always returns a TimeMsg, but with zero TAITime when flags are insufficient.
// GLONASS produces UTCTime instead.
func timeTim2Tpx(m *casbin.Tim2Tpx) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{Ref: gpsprot.PostPulse, NativeMsgID: "TIM2-TPX"}
	if m.PPSFlag&(casbin.Tim2PPSTOWValid|casbin.Tim2PPSWnValid) != casbin.Tim2PPSTOWValid|casbin.Tim2PPSWnValid {
		return &t
	}
	tow := tim2TOW(m.TOW, m.TOWSubms)
	var taiMinusGNSS int16
	switch m.TSrc {
	case casbin.Tim2TSrcGPS:
		t.GNSS, t.TAITime = gpsprot.GPS, ptime.GPS(int16(m.Wn), tow)
		taiMinusGNSS = ptime.TAIMinusGPS
	case casbin.Tim2TSrcBDS:
		t.GNSS, t.TAITime = gpsprot.BDS, ptime.BeiDou(int16(m.Wn), tow)
		taiMinusGNSS = ptime.TAIMinusBeiDou
	case casbin.Tim2TSrcGAL:
		t.GNSS, t.TAITime = gpsprot.GAL, ptime.Galileo(int16(m.Wn), tow)
		taiMinusGNSS = ptime.TAIMinusGalileo
	case casbin.Tim2TSrcGLN:
		t.GNSS = gpsprot.GLO
		t.UTCTime.Set(ptime.GLONASSWeekUTC(m.Wn, tow))
	}
	if m.QuanErr != 0 {
		t.PulseOffset.Set(float64(m.QuanErr) * 0.1) // 0.1 ns -> ns
	}
	if m.TAcc > 0 {
		t.Accuracy = time.Duration(math.Round(float64(m.TAcc) * 0.1))
	}
	if taiMinusGNSS > 0 && m.PPSFlag&casbin.Tim2PPSLsValid != 0 {
		t.UTCOffset = uint8(m.LeapSec) + uint8(taiMinusGNSS)
	}
	return &t
}

// timeNav2Sol converts Nav2Sol to TimeMsg.
// Returns nil when the fix is below 2D.
func timeNav2Sol(m *casbin.Nav2Sol, gnss gpsprot.GNSS) *gpsprot.TimeMsg {
	if m.FixFlags < casbin.PVT2D {
		return nil
	}
	t := gpsprot.TimeMsg{NativeMsgID: "NAV2-SOL"}
	t.GNSS = gnss
	t.TAITime = ptime.GPS(int16(m.Wn), time.Duration(m.TOW)*time.Millisecond)
	return &t
}

// tim2TOW combines a TIM2 message's integer-millisecond TOW and its
// 2^-30-scaled fractional-millisecond field into a duration.
func tim2TOW(tow uint32, subms int32) time.Duration {
	return time.Duration(tow)*time.Millisecond +
		time.Duration(math.Round(float64(subms)*math.Exp2(-30)*1e6))
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
	t.UTCTime.Set(ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, nanos))
	// V6 TAcc is nanoseconds (direct std dev, not variance)
	if m.TAcc > 0 {
		t.Accuracy = time.Duration(math.Round(float64(m.TAcc)))
	}
	if m.LeapSec > 0 {
		t.UTCOffset = uint8(m.LeapSec) + ptime.TAIMinusGPS
	}
	t.GNSS = nav2TimeSrcToGNSS(m.TimeSrc)
	return &t
}

// timeTim2TimeGNSS converts Tim2TimeGNSS to TimeMsg.
// Always returns a TimeMsg, but with zero TAITime when the time flags are insufficient.
func timeTim2TimeGNSS(m *casbin.Tim2TimeGNSS, gnss gpsprot.GNSS, toTAI func(int16, time.Duration) ptime.Time, taiMinusGNSS int16, msgID string) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: msgID, GNSS: gnss}
	if m.TFlag&(casbin.Tim2TimeFlagTOWValid|casbin.Tim2TimeFlagWnValid) != casbin.Tim2TimeFlagTOWValid|casbin.Tim2TimeFlagWnValid {
		return &t
	}
	t.TAITime = toTAI(int16(m.Wn), tim2TOW(uint32(m.TOW), m.TOWSubms))
	if m.TAcc > 0 {
		t.Accuracy = time.Duration(math.Round(float64(m.TAcc)))
	}
	if m.TFlag&casbin.Tim2TimeFlagLsValid != 0 {
		t.UTCOffset = uint8(m.Ls) + uint8(taiMinusGNSS)
	}
	return &t
}

// timeTim2TimeGLN converts Tim2TimeGLN to TimeMsg with UTCTime.
// GLONASS time tracks UTC, so we produce UTCTime rather than TAITime.
func timeTim2TimeGLN(m *casbin.Tim2TimeGNSS) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "TIM2-TIMEGLN", GNSS: gpsprot.GLO}
	if m.TFlag&(casbin.Tim2TimeFlagTOWValid|casbin.Tim2TimeFlagWnValid) != casbin.Tim2TimeFlagTOWValid|casbin.Tim2TimeFlagWnValid {
		return &t
	}
	t.UTCTime.Set(ptime.GLONASSWeekUTC(m.Wn, tim2TOW(uint32(m.TOW), m.TOWSubms)))
	if m.TAcc > 0 {
		t.Accuracy = time.Duration(math.Round(float64(m.TAcc)))
	}
	return &t
}

// leapTim2TimeGNSS extracts a LeapSecondMsg from Tim2TimeGNSS.
// Returns nil when no leap second event is reported.
func leapTim2TimeGNSS(m *casbin.Tim2TimeGNSS, gnss gpsprot.GNSS, taiMinusGNSS int16) *gpsprot.LeapSecondMsg {
	if m.LsFlag != casbin.Tim2LsEventNormal || m.LsYear == 0 {
		return nil
	}
	year := int(m.LsYear >> 1)
	month := time.December
	day := 31
	if m.LsYear&1 == 0 {
		month = time.June
		day = 30
	}
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	utcOffBefore := int16(m.Ls) + taiMinusGNSS
	utcOffAfter := int16(m.Lsf) + taiMinusGNSS
	return &gpsprot.LeapSecondMsg{
		LeapSecond: ptime.LeapSecondOnDate(date, utcOffBefore, utcOffAfter),
		GNSS:       gnss,
	}
}

// gnssIDToGNSS maps CASIC GNSSID to gpsprot.GNSS.
func gnssIDToGNSS(id casbin.GNSSID) gpsprot.GNSS {
	switch id {
	case casbin.GPS:
		return gpsprot.GPS
	case casbin.BDS:
		return gpsprot.BDS
	case casbin.GLN:
		return gpsprot.GLO
	case casbin.GAL:
		return gpsprot.GAL
	case casbin.QZSS:
		return gpsprot.QZSS
	case casbin.SBAS:
		return gpsprot.SBAS
	case casbin.NAVIC:
		return gpsprot.NAVIC
	}
	return 0
}

// nav2TimeSrcToGNSS maps V6 Nav2TimeSrc to gpsprot.GNSS.
func nav2TimeSrcToGNSS(id casbin.Nav2TimeSrc) gpsprot.GNSS {
	switch id {
	case casbin.Nav2TimeSrcGPS:
		return gpsprot.GPS
	case casbin.Nav2TimeSrcBDS:
		return gpsprot.BDS
	case casbin.Nav2TimeSrcGLN:
		return gpsprot.GLO
	case casbin.Nav2TimeSrcGAL:
		return gpsprot.GAL
	case casbin.Nav2TimeSrcIRN:
		return gpsprot.NAVIC
	}
	return 0
}

// leapTim2Ls extracts a LeapSecondMsg from TIM2-LS. The week of the
// event carries only the bottom 8 bits, resolved against the read time
// like the UBX configurator resolves UBX-NAV-TIMELS: take the
// candidate at or before now and walk back in 256-week steps until the
// date lands on the last day of a quarter, where leap seconds occur.
func leapTim2Ls(m *casbin.Tim2Ls, tRead time.Time) *gpsprot.LeapSecondMsg {
	if m.RaimType != casbin.Tim2RaimLsNormal {
		return nil
	}
	utcOffBefore := int16(m.Dtls) + ptime.TAIMinusGPS
	utcOffAfter := int16(m.Dtlsf) + ptime.TAIMinusGPS
	date, ok := leapDate8BitWeek(m.Wnlsf, m.Dn, tRead)
	if !ok {
		return nil
	}
	return &gpsprot.LeapSecondMsg{
		LeapSecond: ptime.LeapSecondOnDate(date, utcOffBefore, utcOffAfter),
		GNSS:       gnssIDToGNSS(m.GNSSID),
	}
}

// leapMsgUTC extracts a LeapSecondMsg from the V5 MSG-GPSUTC and
// MSG-BDSUTC navigation-data messages, which share the leap second
// fields. dnBase is 1 for GPS (day numbers 1-7) and 0 for BeiDou.
func leapMsgUTC(dtls, dtlsf int8, wnlsf, dn uint8, valid casbin.MsgUTCValid,
	gnss gpsprot.GNSS, taiMinusGNSS int16, dnBase uint8, tRead time.Time) *gpsprot.LeapSecondMsg {
	if valid != casbin.MsgUTCValidOK {
		return nil
	}
	date, ok := leapDate8BitWeek(wnlsf, dn-dnBase+1, tRead)
	if !ok {
		return nil
	}
	utcOffBefore := int16(dtls) + taiMinusGNSS
	utcOffAfter := int16(dtlsf) + taiMinusGNSS
	return &gpsprot.LeapSecondMsg{
		LeapSecond: ptime.LeapSecondOnDate(date, utcOffBefore, utcOffAfter),
		GNSS:       gnss,
	}
}

// leapDate8BitWeek resolves a leap second event date from an 8-bit
// week number and a 1-based day number. The candidate week at or
// before the week of tRead is tried first, then earlier 256-week
// epochs, accepting the first date on the last day of a quarter.
func leapDate8BitWeek(wnlsf, dn uint8, tRead time.Time) (time.Time, bool) {
	if dn < 1 || dn > 7 {
		return time.Time{}, false
	}
	nowWeek := int(tRead.Sub(ptime.GPSDate(0, 0)) / (7 * 24 * time.Hour))
	week := nowWeek - int(uint8(nowWeek)-wnlsf)
	if week > nowWeek {
		week -= 0x100
	}
	for i := 0; i <= 2; i++ {
		t := ptime.GPSDate(uint16(week), time.Weekday(dn-1))
		if isLastDayOfQuarter(t) {
			return t, true
		}
		week -= 0x100
	}
	return time.Time{}, false
}

func isLastDayOfQuarter(t time.Time) bool {
	return t.AddDate(0, 0, 1).Day() == 1 && t.Month()%3 == 0
}

// surveyTim2TimePos converts TIM2-TIMEPOS to a SurveyMsg. The protocol
// reports the survey running time and current accuracy but no
// observation count; position validity flag 15 means the timing
// position is fixed (survey complete or position set).
func surveyTim2TimePos(m *casbin.Tim2TimePos) *gpsprot.SurveyMsg {
	sv := &gpsprot.SurveyMsg{
		Position: gpsprot.Point3D{
			gpsprot.Meters(m.XTim),
			gpsprot.Meters(m.YTim),
			gpsprot.Meters(m.ZTim),
		},
		Accuracy:   gpsprot.Meters(float64(m.SurPacc)),
		ObsTime:    gpsprot.Duration(time.Duration(m.SurTimer) * time.Second),
		Valid:      m.FixFlag == casbin.PVTTimingFixed,
		InProgress: m.FixFlag != casbin.PVTTimingFixed && m.SurTimer > 0,
	}
	return sv
}
