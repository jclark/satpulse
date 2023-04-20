package ubx

import (
	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

func (m *Message) TimeMode() *gpsmsg.TimeMode {
	switch u := m.um.(type) {
	case *bin.CfgTmode2:
		return timeMode2(u)
	case *bin.CfgTmode3:
		return timeMode3(u)
	}
	return nil
}

func timeMode2(u *bin.CfgTmode2) *gpsmsg.TimeMode {
	tm := &gpsmsg.TimeMode{}
	switch u.TimeMode {
	case bin.CfgTmode2Disabled:
		break
	case bin.CfgTmode2FixedMode:
		tm.FixedPos = &gpsmsg.FixedPosMode{}
		if u.Flags&bin.CfgTmode2LLA == 0 {
			tm.FixedPos.ECEF = &gpsmsg.ECEF{
				X: float64(u.EcefXOrLat) * 1e-2,
				Y: float64(u.EcefYOrLon) * 1e-2,
				Z: float64(u.EcefZOrAlt) * 1e-2,
			}
			tm.FixedPos.Acc = float64(u.FixedPosAcc) * 1e-3
		} else {
			tm.FixedPos.LLA = &gpsmsg.LLA{
				Lat: float64(u.EcefXOrLat) * 1e-7,
				Lon: float64(u.EcefYOrLon) * 1e-7,
			}
			if u.Flags&bin.CfgTMode2AltInv == 0 {
				alt := float64(u.EcefZOrAlt) * 1e-2
				tm.FixedPos.LLA.Alt = &alt
			}
		}
	case bin.CfgTmode2SurveyIn:
		tm.SurveyIn = &gpsmsg.SurveyInMode{
			MinDur:   float64(u.SvinMinDur),
			AccLimit: float64(u.SvinAccLimit) * 1e-3,
		}
	}
	return tm
}

func timeMode3(u *bin.CfgTmode3) *gpsmsg.TimeMode {
	tm := &gpsmsg.TimeMode{}
	switch u.Flags & bin.CfgTmode3Mode {
	case bin.CfgTmode3Disabled:
		break
	case bin.CfgTmode3FixedMode:
		tm.FixedPos = &gpsmsg.FixedPosMode{}
		zOrAlt := hpToLength(u.EcefZOrAlt, u.EcefZOrAltHP)
		if u.Flags&bin.CfgTmode3LLA == 0 {
			tm.FixedPos.ECEF = &gpsmsg.ECEF{
				X: hpToLength(u.EcefXOrLat, u.EcefXOrLatHP),
				Y: hpToLength(u.EcefYOrLon, u.EcefYOrLonHP),
				Z: zOrAlt,
			}
			tm.FixedPos.Acc = float64(u.FixedPosAcc) * 1e-4
		} else {
			tm.FixedPos.LLA = &gpsmsg.LLA{
				Lat: hpToAngle(u.EcefXOrLat, u.EcefXOrLatHP),
				Lon: hpToAngle(u.EcefYOrLon, u.EcefYOrLonHP),
				Alt: &zOrAlt,
			}
		}
	case bin.CfgTmode3SurveyIn:
		tm.SurveyIn = &gpsmsg.SurveyInMode{
			MinDur:   float64(u.SvinMinDur),
			AccLimit: float64(u.SvinAccLimit) * 1e-4,
		}
	}
	return tm
}

func hpToLength(l int32, h int8) float64 {
	return float64(l)*1e-2 + float64(h)*1e-4
}

func hpToAngle(l int32, h int8) float64 {
	return float64(l)*1e-7 + float64(h)*1e-9
}
