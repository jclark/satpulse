package ubx

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func posECEFNavPosECEF(m *ubxbin.NavPosECEF) *gpsprot.PosECEFMsg {
	return &gpsprot.PosECEFMsg{
		Pos:         point3DCm(m.ECEF),
		NativeMsgID: "NAV-POSECEF",
	}
}

func velECEFNavVelECEF(m *ubxbin.NavVelECEF) *gpsprot.VelECEFMsg {
	return &gpsprot.VelECEFMsg{
		Vel:         speed3CmS(m.ECEFV),
		NativeMsgID: "NAV-VELECEF",
	}
}

func posGeoNavPosLLH(m *ubxbin.NavPosLLH) *gpsprot.PosGeoMsg {
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      lengthMmOpt(m.Height),
		HeightMSL:   lengthMmOpt(m.HMSL),
		NativeMsgID: "NAV-POSLLH",
	}
}

func velGeoNavVelNED(m *ubxbin.NavVelNED) *gpsprot.VelGeoMsg {
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

func posGeoNavPVT(m *ubxbin.NavPVT) *gpsprot.PosGeoMsg {
	if !pvtFixValid(m) || (m.Flags3&ubxbin.NavPVTInvalidLlh) != 0 {
		return nil
	}
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      lengthMmOpt(m.Height),
		HeightMSL:   lengthMmOpt(m.HMSL),
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
		Course:      angle1e5Opt(m.HeadMot),
		NativeMsgID: "NAV-PVT",
	}
}

// navEpoch* functions populate NavEpochMsg accuracy fields from UBX messages.

func navEpochNavPosECEF(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPosECEF) {
	ne.Acc.Pos.Set(lengthCm(m.PAcc))
}

func navEpochNavVelECEF(ne *gpsprot.NavEpochMsg, m *ubxbin.NavVelECEF) {
	ne.Acc.Speed.Set(speedCmS(m.SAcc))
}

func navEpochNavPosLLH(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPosLLH) {
	ne.Acc.Hor.Set(lengthMm(m.HAcc))
	ne.Acc.Vert.Set(lengthMm(m.VAcc))
}

func navEpochNavVelNED(ne *gpsprot.NavEpochMsg, m *ubxbin.NavVelNED) {
	ne.Acc.Speed.Set(speedCmS(m.SAcc))
	ne.Acc.Course.Set(angle1e5(m.CAcc))
}

func navEpochNavPVT(ne *gpsprot.NavEpochMsg, m *ubxbin.NavPVT) {
	ne.Acc.Hor.Set(lengthMm(m.HAcc))
	ne.Acc.Vert.Set(lengthMm(m.VAcc))
	ne.Acc.Speed.Set(speedMmS(m.SAcc))
	ne.Acc.Course.Set(angle1e5(m.HeadAcc))
	navEpochTimeAcc(ne, m.TAcc)
}

func navEpochTimeAcc(ne *gpsprot.NavEpochMsg, tAcc uint32) {
	ne.Acc.Time.Set(time.Duration(tAcc))
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
