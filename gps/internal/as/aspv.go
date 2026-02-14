package as

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// posEcefNavPosEcef converts asbin.NavPosEcef to gpsprot.PosECEFMsg.
func posEcefNavPosEcef(ne *gpsprot.NavEpochMsg, m *asbin.NavPosEcef) *gpsprot.PosECEFMsg {
	ne.Acc.Pos.Set(lengthCm(m.PAcc))
	return &gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			lengthCm(m.EcefX), lengthCm(m.EcefY), lengthCm(m.EcefZ),
		},
		NativeMsgID: "NAV-POSECEF",
	}
}

// posGeoNavPosLlh converts asbin.NavPosLlh to gpsprot.PosGeoMsg.
func posGeoNavPosLlh(ne *gpsprot.NavEpochMsg, m *asbin.NavPosLlh) *gpsprot.PosGeoMsg {
	ne.Acc.Hor.Set(lengthMm(m.HAcc))
	ne.Acc.Vert.Set(lengthMm(m.VAcc))
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      opt.Make(lengthMm(m.Height)),
		HeightMSL:   opt.Make(lengthMm(m.HMSL)),
		NativeMsgID: "NAV-POSLLH",
	}
}

// velEcefNavVelEcef converts asbin.NavVelEcef to gpsprot.VelECEFMsg.
func velEcefNavVelEcef(ne *gpsprot.NavEpochMsg, m *asbin.NavVelEcef) *gpsprot.VelECEFMsg {
	ne.Acc.Speed.Set(speedCmS(m.SAcc))
	return &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			speedCmS(m.EcefVX), speedCmS(m.EcefVY), speedCmS(m.EcefVZ),
		},
		NativeMsgID: "NAV-VELECEF",
	}
}

// velGeoNavVelNed converts asbin.NavVelNed to gpsprot.VelGeoMsg.
func velGeoNavVelNed(ne *gpsprot.NavEpochMsg, m *asbin.NavVelNed) *gpsprot.VelGeoMsg {
	ne.Acc.Speed.Set(speedCmS(m.SAcc))
	ne.Acc.Course.Set(angle1e5(m.CAcc))
	return &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			speedCmS(m.VelN), speedCmS(m.VelE), speedCmS(m.VelD),
		}),
		Speed3D:     opt.Make(speedCmS(m.Speed)),
		GroundSpeed: opt.Make(speedCmS(m.GSpeed)),
		Course:      opt.Make(angle1e5(m.Heading)),
		NativeMsgID: "NAV-VELNED",
	}
}

// Unit conversion helpers for Allystar binary fields.

type integer interface{ ~int32 | ~uint32 }

func lengthCm[T integer](v T) gpsprot.Length {
	return gpsprot.Length(v) * gpsprot.Centimeter
}

func lengthMm[T integer](v T) gpsprot.Length {
	return gpsprot.Length(v) * gpsprot.Millimeter
}

func speedCmS[T integer](v T) gpsprot.Speed {
	return gpsprot.Speed(v) * gpsprot.CentimeterPerSecond
}

func angle1e7(v int32) gpsprot.Angle {
	return gpsprot.Angle(v) * 100
}

func angle1e5[T integer](v T) gpsprot.Angle {
	return gpsprot.Angle(v) * 10000
}
