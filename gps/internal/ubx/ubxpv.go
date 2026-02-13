package ubx

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func posECEFNavPosECEF(m *ubxbin.NavPosECEF) *gpsprot.PosECEFMsg {
	return &gpsprot.PosECEFMsg{
		Pos:         point3DCm(m.ECEF),
		PAcc:        opt.Make(lengthCm(m.PAcc)),
		NativeMsgID: "NAV-POSECEF",
	}
}

func velECEFNavVelECEF(m *ubxbin.NavVelECEF) *gpsprot.VelECEFMsg {
	return &gpsprot.VelECEFMsg{
		Vel:         speed3CmS(m.ECEFV),
		SAcc:        speedCmSOpt(m.SAcc),
		NativeMsgID: "NAV-VELECEF",
	}
}

func posGeoNavPosLLH(m *ubxbin.NavPosLLH) *gpsprot.PosGeoMsg {
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      lengthMmOpt(m.Height),
		HeightMSL:   lengthMmOpt(m.HMSL),
		HAcc:        lengthMmOpt(m.HAcc),
		VAcc:        lengthMmOpt(m.VAcc),
		NativeMsgID: "NAV-POSLLH",
	}
}

func velGeoNavVelNED(m *ubxbin.NavVelNED) *gpsprot.VelGeoMsg {
	return &gpsprot.VelGeoMsg{
		VelNED:      opt.Make(speed3CmS(m.VelNED)),
		Speed:       speedCmSOpt(m.Speed),
		GroundSpeed: speedCmSOpt(m.GSpeed),
		Heading:     angle1e5Opt(m.Heading),
		SAcc:        speedCmSOpt(m.SAcc),
		HeadAcc:     angle1e5Opt(m.CAcc),
		NativeMsgID: "NAV-VELNED",
	}
}

func pvtFixValid(m *ubxbin.NavPVT) bool {
	return m.FixType >= ubxbin.NavPVT2DFix && (m.Flags&ubxbin.NavPVTGNSSFixOK) != 0
}

func posGeoNavPVT(m *ubxbin.NavPVT) *gpsprot.PosGeoMsg {
	if !pvtFixValid(m) || (m.Flags3&ubxbin.NavPVTInvalidLlh) != 0 {
		return nil
	}
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      lengthMmOpt(m.Height),
		HeightMSL:   lengthMmOpt(m.HMSL),
		HAcc:        lengthMmOpt(m.HAcc),
		VAcc:        lengthMmOpt(m.VAcc),
		NativeMsgID: "NAV-PVT",
	}
}

func velGeoNavPVT(m *ubxbin.NavPVT) *gpsprot.VelGeoMsg {
	if !pvtFixValid(m) {
		return nil
	}
	return &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			speedMmS(m.VelN), speedMmS(m.VelE), speedMmS(m.VelD),
		}),
		GroundSpeed: opt.Make(speedMmS(m.GSpeed)),
		Heading:     angle1e5Opt(m.HeadMot),
		SAcc:        opt.Make(speedMmS(m.SAcc)),
		HeadAcc:     angle1e5Opt(m.HeadAcc),
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
