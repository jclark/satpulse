package casic

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// qualityFromNav2FixFlags populates NavEpochMsg quality fields from V6 Nav2FixFlags.
// Unlike V5 qualityFromPosValid, does not set DOP (comes from NAV2-DOP or NAV2-SOL separately).
func qualityFromNav2FixFlags(ne *gpsprot.NavEpochMsg, ff casbin.Nav2FixFlags, numSV uint8) {
	switch ff {
	default: // 0=Invalid, 2=RoughEstimate, 3=Hold
		ne.FixLevel = gpsprot.FixLevelNone
	case casbin.Nav2FixExternal:
		ne.FixLevel = gpsprot.FixLevelNotMeasured
	case casbin.Nav2FixDeadReckoning:
		ne.FixLevel = gpsprot.FixLevelNone
		ne.AuxSrc = gpsprot.AuxSrcDR
	case casbin.Nav2FixQuickMode, casbin.Nav2Fix3D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim3D
	case casbin.Nav2Fix2D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim2D
	case casbin.Nav2FixDGPS:
		ne.FixLevel = gpsprot.FixLevelCodeCorrected
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction |= gpsprot.CorrUsed
	case casbin.Nav2FixRTKFloat:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction |= gpsprot.CorrBaseStation | gpsprot.CorrUsed
	case casbin.Nav2FixRTKFixed:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction |= gpsprot.CorrBaseStation | gpsprot.CorrUsed
	case casbin.Nav2FixTimingFixed:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDimTimeOnly
	}
	ne.NumSVUsed.Set(uint16(numSV))
}

// posECEFNav2Sol extracts ECEF position from NAV2-SOL.
// Returns nil when FixFlags < Nav2Fix2D. Always populates quality fields on ne.
func posECEFNav2Sol(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sol) *gpsprot.PosECEFMsg {
	qualityFromNav2FixFlags(ne, m.FixFlags, m.NumFixTot)
	ne.DOP.Pos = opt.Make(float64(m.PDOP))
	if m.FixFlags < casbin.Nav2Fix2D {
		return nil
	}
	// V6 PAcc is std dev (not variance), use directly
	ne.Acc.Pos.Set(gpsprot.Meters(float64(m.PAcc)))
	return &gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			gpsprot.Meters(m.X), gpsprot.Meters(m.Y), gpsprot.Meters(m.Z),
		},
		NativeMsgID: "NAV2-SOL",
	}
}

// velECEFNav2Sol extracts ECEF velocity from NAV2-SOL.
// Returns nil when VelFlags < Nav2Vel2D.
func velECEFNav2Sol(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sol) *gpsprot.VelECEFMsg {
	if m.VelFlags < casbin.Nav2Vel2D {
		return nil
	}
	// V6 SAcc is std dev (not variance), use directly
	ne.Acc.Speed.Set(gpsprot.MetersPerSecondFromFloat(float64(m.SAcc)))
	return &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.VX)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VY)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VZ)),
		},
		NativeMsgID: "NAV2-SOL",
	}
}

// posGeoNav2Pvh extracts geodetic position from NAV2-PVH.
// Returns nil when FixFlags < Nav2Fix2D. Always populates quality fields on ne.
func posGeoNav2Pvh(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Pvh) *gpsprot.PosGeoMsg {
	qualityFromNav2FixFlags(ne, m.FixFlags, m.NumFixTot)
	if m.FixFlags < casbin.Nav2Fix2D {
		return nil
	}
	// V6 accuracy is std dev (not variance), use directly
	ne.Acc.Hor.Set(gpsprot.Meters(float64(m.HAcc)))
	ne.Acc.Vert.Set(gpsprot.Meters(float64(m.VAcc)))
	h := gpsprot.Meters(float64(m.Height))
	msg := &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(m.Lat), gpsprot.DegreesFromFloat(m.Lon)},
		Height:      opt.Make(h),
		HeightMSL:   opt.Make(h - gpsprot.Meters(float64(m.SepGeoid))),
		NativeMsgID: "NAV2-PVH",
	}
	return msg
}

// velGeoNav2Pvh extracts geodetic velocity from NAV2-PVH.
// Returns nil when VelFlags < Nav2Vel2D.
func velGeoNav2Pvh(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Pvh) *gpsprot.VelGeoMsg {
	if m.VelFlags < casbin.Nav2Vel2D {
		return nil
	}
	// V6 accuracy is std dev (not variance), use directly
	ne.Acc.Speed.Set(gpsprot.MetersPerSecondFromFloat(float64(m.SAcc)))
	ne.Acc.Course.Set(gpsprot.DegreesFromFloat(float64(m.CAcc)))
	return &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.VelN)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VelE)),
			gpsprot.MetersPerSecondFromFloat(float64(-m.VelU)),
		}),
		Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(float64(m.Speed3D))),
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(float64(m.Speed2D))),
		Course:      opt.Make(gpsprot.DegreesFromFloat(float64(m.Heading))),
		NativeMsgID: "NAV2-PVH",
	}
}

// dopNav2Dop extracts DOP values from NAV2-DOP into the NavEpochMsg.
func dopNav2Dop(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Dop) {
	ne.DOP.Pos = opt.Make(float64(m.PDOP))
	ne.DOP.Hor = opt.Make(float64(m.HDOP))
	ne.DOP.Vert = opt.Make(float64(m.VDOP))
	ne.DOP.Time = opt.Make(float64(m.TDOP))
}
