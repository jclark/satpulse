package ubx

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func posECEFNavPosECEF(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPosECEF) *gpsprot.PosECEFMsg {
	ne.Acc.Pos.Set(lengthCm(m.PAcc))
	return &gpsprot.PosECEFMsg{
		Pos:         point3DCm(m.ECEF),
		NativeMsgID: "NAV-POSECEF",
	}
}

func velECEFNavVelECEF(ne *gpsprot.NavEpochMsg, m *ubxbin.NavVelECEF) *gpsprot.VelECEFMsg {
	ne.Acc.Speed.Set(speedCmS(m.SAcc))
	return &gpsprot.VelECEFMsg{
		Vel:         speed3CmS(m.ECEFV),
		NativeMsgID: "NAV-VELECEF",
	}
}

func posGeoNavPosLLH(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPosLLH) *gpsprot.PosGeoMsg {
	ne.Acc.Hor.Set(lengthMm(m.HAcc))
	ne.Acc.Vert.Set(lengthMm(m.VAcc))
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      lengthMmOpt(m.Height),
		HeightMSL:   lengthMmOpt(m.HMSL),
		NativeMsgID: "NAV-POSLLH",
	}
}

func velGeoNavVelNED(ne *gpsprot.NavEpochMsg, m *ubxbin.NavVelNED) *gpsprot.VelGeoMsg {
	ne.Acc.Speed.Set(speedCmS(m.SAcc))
	ne.Acc.Course.Set(angle1e5(m.CAcc))
	return &gpsprot.VelGeoMsg{
		VelNED:      opt.Make(speed3CmS(m.VelNED)),
		Speed3D:     speedCmSOpt(m.Speed),
		GroundSpeed: speedCmSOpt(m.GSpeed),
		Course:      angle1e5Opt(m.Heading),
		NativeMsgID: "NAV-VELNED",
	}
}

func pvtFixValid(m *ubxbin.NavPVT) bool {
	return m.FixType >= ubxbin.NavPVT2DFix && (m.Flags&ubxbin.NavPVTGNSSFixOK) != 0
}

func posGeoNavPVT(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPVT) *gpsprot.PosGeoMsg {
	if !pvtFixValid(m) || (m.Flags3&ubxbin.NavPVTInvalidLlh) != 0 {
		return nil
	}
	ne.Acc.Hor.Set(lengthMm(m.HAcc))
	ne.Acc.Vert.Set(lengthMm(m.VAcc))
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      lengthMmOpt(m.Height),
		HeightMSL:   lengthMmOpt(m.HMSL),
		NativeMsgID: "NAV-PVT",
	}
}

func velGeoNavPVT(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPVT) *gpsprot.VelGeoMsg {
	if !pvtFixValid(m) {
		return nil
	}
	ne.Acc.Speed.Set(speedMmS(m.SAcc))
	ne.Acc.Course.Set(angle1e5(m.HeadAcc))
	return &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			speedMmS(m.VelN), speedMmS(m.VelE), speedMmS(m.VelD),
		}),
		GroundSpeed: opt.Make(speedMmS(m.GSpeed)),
		Course:      angle1e5Opt(m.HeadMot),
		NativeMsgID: "NAV-PVT",
	}
}

// qualityNavPVT extracts solution quality metadata from a NAV-PVT message.
// Always called, even when the fix is invalid, to report "no fix" states.
func qualityNavPVT(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPVT) {
	switch m.FixType {
	case ubxbin.NavPVTDeadReckoningOnly:
		ne.AuxSrc |= gpsprot.AuxSrcDR
	case ubxbin.NavPVT2DFix:
		ne.FixDim = gpsprot.FixDim2D
	case ubxbin.NavPVT3DFix:
		ne.FixDim = gpsprot.FixDim3D
	case ubxbin.NavPVTGNSSDeadReckoning:
		ne.FixDim = gpsprot.FixDim3D
		ne.AuxSrc |= gpsprot.AuxSrcDR
	case ubxbin.NavPVTTimeOnlyFix:
		ne.FixDim = gpsprot.FixDimTimeOnly
	}
	if m.Flags&ubxbin.NavPVTHeadVehValid != 0 {
		ne.AuxSrc |= gpsprot.AuxSrcINS
	}
	if m.Flags&ubxbin.NavPVTGNSSFixOK == 0 || m.FixType == ubxbin.NavPVTDeadReckoningOnly {
		ne.FixLevel = gpsprot.FixLevelNone
	} else {
		switch m.Flags & ubxbin.NavPctCarrSoln {
		case ubxbin.NavPVTCarrSolnFixed:
			ne.FixLevel = gpsprot.FixLevelCarrierFixed
			ne.Correction |= gpsprot.CorrUsed
		case ubxbin.NavPVTCarrSolnFloat:
			ne.FixLevel = gpsprot.FixLevelCarrierFloat
			ne.Correction |= gpsprot.CorrUsed
		default:
			if m.Flags&ubxbin.NavPVTDiffSoln != 0 {
				ne.FixLevel = gpsprot.FixLevelCodeCorrected
				ne.Correction |= gpsprot.CorrUsed
			} else {
				ne.FixLevel = gpsprot.FixLevelCode
			}
		}
	}
	ne.NumSVUsed = opt.Make(uint16(m.NumSV))
	ne.DOP.Pos = dop01(m.PDOP)
	if d, ok := corrAgeDuration(m.Flags3 & ubxbin.NavPVTLastCorrectionAgeMask); ok {
		ne.DiffAge = opt.Make(d)
	}
}

func corrAgeDuration(age ubxbin.NavPVTFlags3) (gpsprot.Duration, bool) {
	switch age {
	case ubxbin.NavPVTLastCorrectionAgeNotAvailable:
		return 0, false
	case ubxbin.NavPVTLastCorrectionAge0to1:
		return 0, true
	case ubxbin.NavPVTLastCorrectionAge1to2:
		return 1 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge2to5:
		return 2 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge5to10:
		return 5 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge10to15:
		return 10 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge15to20:
		return 15 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge20to30:
		return 20 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge30to45:
		return 30 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge45to60:
		return 45 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge60to90:
		return 60 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge90to120:
		return 90 * gpsprot.Second, true
	case ubxbin.NavPVTLastCorrectionAge120Plus:
		return 120 * gpsprot.Second, true
	default:
		return 0, false
	}
}

func dopNavDOP(ne *gpsprot.NavEpochMsg, m *ubxbin.NavDOP) {
	ne.DOP.Geom = dop01(m.GDOP)
	ne.DOP.Pos = dop01(m.PDOP)
	ne.DOP.Hor = dop01(m.HDOP)
	ne.DOP.Vert = dop01(m.VDOP)
	ne.DOP.Time = dop01(m.TDOP)
}

// Unit conversion helpers for UBX binary fields.

type integer interface{ ~int32 | ~uint32 }

func lengthCm[T integer](v T) gpsprot.Length {
	return gpsprot.Length(v) * gpsprot.Centimeter
}

func lengthMm[T integer](v T) gpsprot.Length {
	return gpsprot.Length(v) * gpsprot.Millimeter
}

func point3DCm(v [3]int32) gpsprot.Point3D {
	return gpsprot.Point3D{lengthCm(v[0]), lengthCm(v[1]), lengthCm(v[2])}
}

func speedCmS[T integer](v T) gpsprot.Speed {
	return gpsprot.Speed(v) * gpsprot.CentimeterPerSecond
}

func speedMmS[T integer](v T) gpsprot.Speed {
	return gpsprot.Speed(v) * gpsprot.MillimeterPerSecond
}

func speed3CmS(v [3]int32) [3]gpsprot.Speed {
	return [3]gpsprot.Speed{speedCmS(v[0]), speedCmS(v[1]), speedCmS(v[2])}
}

func angle1e7(v int32) gpsprot.Angle {
	return gpsprot.Angle(v) * 100
}

func lengthMmOpt[T integer](v T) opt.Val[gpsprot.Length] {
	return opt.Make(lengthMm(v))
}

func speedCmSOpt[T integer](v T) opt.Val[gpsprot.Speed] {
	return opt.Make(speedCmS(v))
}

func angle1e5[T integer](v T) gpsprot.Angle {
	return gpsprot.Angle(v) * 10000
}

func angle1e5Opt[T integer](v T) opt.Val[gpsprot.Angle] {
	return opt.Make(angle1e5(v))
}

func dop01(v uint16) opt.Val[float64] {
	return opt.Make(float64(v) * 0.01)
}
