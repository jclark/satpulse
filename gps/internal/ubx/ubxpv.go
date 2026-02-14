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
