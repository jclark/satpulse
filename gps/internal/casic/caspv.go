package casic

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// qualityFromPosValid populates NavEpochMsg quality fields from CASIC NavPosValid.
// Called by both NAV-SOL and NAV-PV extraction.
func qualityFromPosValid(ne *gpsprot.NavEpochMsg, pv casbin.NavPosValid, numSV uint8, pdop float32) {
	switch pv {
	case casbin.NavPosExternal:
		ne.FixLevel = gpsprot.FixLevelNotMeasured
	case casbin.NavPosDeadReckoning:
		ne.FixLevel = gpsprot.FixLevelNone
		ne.AuxSrc = gpsprot.AuxSrcDR
	case casbin.NavPosQuickMode, casbin.NavPos3D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim3D
	case casbin.NavPos2D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim2D
	case casbin.NavPosGNSSDR:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.AuxSrc = gpsprot.AuxSrcDR
	default: // NavPosInvalid, NavPosRoughEstimate, NavPosMaintaining
		ne.FixLevel = gpsprot.FixLevelNone
	}
	ne.NumSVUsed.Set(uint16(numSV))
	ne.DOP.Pos = opt.Make(float64(pdop))
}

// posECEFNavSol extracts ECEF position from NAV-SOL.
// Returns nil when PosValid < NavPos2D. Always populates quality fields on ne.
func posECEFNavSol(ne *gpsprot.NavEpochMsg, m *casbin.NavSol) *gpsprot.PosECEFMsg {
	qualityFromPosValid(ne, m.PosValid, m.NumSV, m.PDOP)
	if m.PosValid < casbin.NavPos2D {
		return nil
	}
	ne.Acc.Pos.Set(gpsprot.Meters(math.Sqrt(float64(m.PAcc))))
	return &gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			gpsprot.Meters(m.ECEFX), gpsprot.Meters(m.ECEFY), gpsprot.Meters(m.ECEFZ),
		},
		NativeMsgID: "NAV-SOL",
	}
}

// velECEFNavSol extracts ECEF velocity from NAV-SOL.
// Returns nil when VelValid < NavVel2D.
func velECEFNavSol(ne *gpsprot.NavEpochMsg, m *casbin.NavSol) *gpsprot.VelECEFMsg {
	if m.VelValid < casbin.NavVel2D {
		return nil
	}
	ne.Acc.Speed.Set(gpsprot.MetersPerSecondFromFloat(math.Sqrt(float64(m.SAcc))))
	return &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.ECEFVX)),
			gpsprot.MetersPerSecondFromFloat(float64(m.ECEFVY)),
			gpsprot.MetersPerSecondFromFloat(float64(m.ECEFVZ)),
		},
		NativeMsgID: "NAV-SOL",
	}
}

// posGeoNavPv extracts geodetic position from NAV-PV.
// Returns nil when PosValid < NavPos2D. Always populates quality fields on ne.
func posGeoNavPv(ne *gpsprot.NavEpochMsg, m *casbin.NavPv) *gpsprot.PosGeoMsg {
	qualityFromPosValid(ne, m.PosValid, m.NumSV, m.PDOP)
	if m.PosValid < casbin.NavPos2D {
		return nil
	}
	ne.Acc.Hor.Set(gpsprot.Meters(math.Sqrt(float64(m.HAcc))))
	ne.Acc.Vert.Set(gpsprot.Meters(math.Sqrt(float64(m.VAcc))))
	h := gpsprot.Meters(float64(m.Height))
	msg := &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(m.Lat), gpsprot.DegreesFromFloat(m.Lon)},
		Height:      opt.Make(h),
		NativeMsgID: "NAV-PV",
	}
	// CASIC V5 reports SepGeoid as 0; omit HeightMSL rather than duplicate Height
	if m.SepGeoid != 0 {
		msg.HeightMSL = opt.Make(h - gpsprot.Meters(float64(m.SepGeoid)))
	}
	return msg
}

// velGeoNavPv extracts geodetic velocity from NAV-PV.
// Returns nil when VelValid < NavVel2D.
func velGeoNavPv(ne *gpsprot.NavEpochMsg, m *casbin.NavPv) *gpsprot.VelGeoMsg {
	if m.VelValid < casbin.NavVel2D {
		return nil
	}
	ne.Acc.Speed.Set(gpsprot.MetersPerSecondFromFloat(math.Sqrt(float64(m.SAcc))))
	ne.Acc.Course.Set(gpsprot.DegreesFromFloat(math.Sqrt(float64(m.CAcc))))
	return &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.VelN)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VelE)),
			gpsprot.MetersPerSecondFromFloat(float64(-m.VelU)),
		}),
		Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(float64(m.Speed3D))),
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(float64(m.Speed2D))),
		Course:      opt.Make(gpsprot.DegreesFromFloat(float64(m.Heading))),
		NativeMsgID: "NAV-PV",
	}
}

// dopNavDop extracts DOP values from NAV-DOP into the NavEpochMsg.
func dopNavDop(ne *gpsprot.NavEpochMsg, m *casbin.NavDop) {
	ne.DOP.Pos = opt.Make(float64(m.PDOP))
	ne.DOP.Hor = opt.Make(float64(m.HDOP))
	ne.DOP.Vert = opt.Make(float64(m.VDOP))
	ne.DOP.Time = opt.Make(float64(m.TDOP))
}
