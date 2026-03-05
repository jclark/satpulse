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

// dopNavDop populates DOP fields on NavEpochMsg from asbin.NavDop.
func dopNavDop(ne *gpsprot.NavEpochMsg, m *asbin.NavDop) {
	ne.DOP.Geom.Set(dop100(m.GDOP))
	ne.DOP.Pos.Set(dop100(m.PDOP))
	ne.DOP.Hor.Set(dop100(m.HDOP))
	ne.DOP.Vert.Set(dop100(m.VDOP))
	ne.DOP.Time.Set(dop100(m.TDOP))
}

// posGeoNavAuto converts asbin.NavAuto to gpsprot.PosGeoMsg.
// Returns nil when FixState < 3 (no position fix).
func posGeoNavAuto(m *asbin.NavAuto) *gpsprot.PosGeoMsg {
	if m.FixState < asbin.NavAutoFix2D {
		return nil
	}
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{angle1e7(m.Lat), angle1e7(m.Lon)},
		Height:      opt.Make(lengthMm(m.Alt)),
		NativeMsgID: "NAV-AUTO",
	}
}

// velGeoNavAuto converts asbin.NavAuto to gpsprot.VelGeoMsg.
// Returns nil when FixState < 3 (no position fix).
func velGeoNavAuto(m *asbin.NavAuto) *gpsprot.VelGeoMsg {
	if m.FixState < asbin.NavAutoFix2D {
		return nil
	}
	return &gpsprot.VelGeoMsg{
		GroundSpeed: opt.Make(gpsprot.Speed(m.Speed) * gpsprot.CentimeterPerSecond),
		Course:      opt.Make(gpsprot.Angle(m.Heading) * 10_000_000),
		NativeMsgID: "NAV-AUTO",
	}
}

// qualityNavAuto populates solution quality fields on NavEpochMsg from NavAuto.
func qualityNavAuto(ne *gpsprot.NavEpochMsg, m *asbin.NavAuto) {
	switch m.FixState {
	case asbin.NavAutoFixNone, asbin.NavAutoFixAided:
		ne.FixLevel = gpsprot.FixLevelNone
	case asbin.NavAutoFixClkBias:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDimTimeOnly
	case asbin.NavAutoFix2D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim2D
	case asbin.NavAutoFix3D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim3D
	case asbin.NavAutoFixDGNSS:
		ne.FixLevel = gpsprot.FixLevelCodeCorrected
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrUsed
	case asbin.NavAutoFixRTKFloat:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrUsed
	case asbin.NavAutoFixRTKFixed:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrUsed
	}
	ne.NumSVUsed.Set(uint16(m.SatInUse))
	ne.NumSVTracked.Set(uint16(m.SatInView))
	if !ne.DOP.Pos.IsSet() {
		ne.DOP.Pos.Set(dop100(m.PDOP))
	}
	if !ne.DOP.Hor.IsSet() {
		ne.DOP.Hor.Set(dop100(m.HDOP))
	}
	if !ne.DOP.Vert.IsSet() {
		ne.DOP.Vert.Set(dop100(m.VDOP))
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

func dop100(v uint16) float64 {
	return float64(v) * 0.01
}
