package sdbp

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

// posGeoDatLLA3 extracts geodetic position from DatLLA3.
func posGeoDatLLA3(ne *gpsprot.NavEpochMsg, m *sdbpbin.DatLLA3) *gpsprot.PosGeoMsg {
	if m.Valid == 0 {
		return nil
	}
	qualityFromValid(ne, m.Valid)
	ne.NumSVUsed.Set(uint16(m.FixSats))
	ne.Acc.Hor.Set(gpsprot.Length(m.HAcc) * gpsprot.Millimeter)
	ne.Acc.Vert.Set(gpsprot.Length(m.VAcc) * gpsprot.Millimeter)
	h := gpsprot.Meters(float64(m.AltMSL) + float64(m.GeoidSep))
	msg := &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(m.Lat), gpsprot.DegreesFromFloat(m.Lon)},
		Height:      opt.Make(h),
		HeightMSL:   opt.Make(gpsprot.Meters(float64(m.AltMSL))),
		NativeMsgID: "DAT-LLA3",
	}
	return msg
}

// velGeoDatLLA3 extracts geodetic velocity from DatLLA3.
func velGeoDatLLA3(ne *gpsprot.NavEpochMsg, m *sdbpbin.DatLLA3) *gpsprot.VelGeoMsg {
	if m.Valid == 0 {
		return nil
	}
	// qualityFromValid already called by posGeoDatLLA3 for this message.
	ne.Acc.GroundSpeed.Set(gpsprot.Speed(m.SpeedAcc) * gpsprot.CentimeterPerSecond)
	if m.HeadingAcc != 0xFFFFFFFF {
		ne.Acc.Course.Set(gpsprot.Angle(m.HeadingAcc) * gpsprot.Degrees / 100)
	}
	return &gpsprot.VelGeoMsg{
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(float64(m.GroundSpeed))),
		Course:      opt.Make(gpsprot.DegreesFromFloat(float64(m.Heading))),
		NativeMsgID: "DAT-LLA3",
	}
}

// posECEFDatECEF2 extracts ECEF position from DatECEF2.
func posECEFDatECEF2(ne *gpsprot.NavEpochMsg, m *sdbpbin.DatECEF2) *gpsprot.PosECEFMsg {
	if m.Valid == 0 {
		return nil
	}
	qualityFromValid(ne, m.Valid)
	ne.NumSVUsed.Set(uint16(m.FixSats))
	maxAcc := max(m.XAcc, m.YAcc, m.ZAcc)
	ne.Acc.Pos.Set(gpsprot.Length(maxAcc) * gpsprot.Millimeter)
	return &gpsprot.PosECEFMsg{
		Pos:         gpsprot.Point3D{gpsprot.Meters(m.X), gpsprot.Meters(m.Y), gpsprot.Meters(m.Z)},
		NativeMsgID: "DAT-ECEF2",
	}
}

// velECEFDatECEF2 extracts ECEF velocity from DatECEF2.
func velECEFDatECEF2(ne *gpsprot.NavEpochMsg, m *sdbpbin.DatECEF2) *gpsprot.VelECEFMsg {
	if m.Valid == 0 {
		return nil
	}
	// qualityFromValid already called by posECEFDatECEF2 for this message.
	maxAcc := max(m.VXAcc, m.VYAcc, m.VZAcc)
	ne.Acc.Speed.Set(gpsprot.Speed(maxAcc) * gpsprot.CentimeterPerSecond)
	return &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.VX)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VY)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VZ)),
		},
		NativeMsgID: "DAT-ECEF2",
	}
}

// velGeoDatNED3 extracts NED velocity from DatNED3.
func velGeoDatNED3(ne *gpsprot.NavEpochMsg, m *sdbpbin.DatNED3) *gpsprot.VelGeoMsg {
	if m.Valid == 0 {
		return nil
	}
	qualityFromValid(ne, m.Valid)
	maxAcc := max(m.VNAcc, m.VEAcc, m.VDAcc)
	ne.Acc.Speed.Set(gpsprot.Speed(maxAcc) * gpsprot.CentimeterPerSecond)
	speed2d := math.Sqrt(float64(m.VN)*float64(m.VN) + float64(m.VE)*float64(m.VE))
	speed3d := math.Sqrt(speed2d*speed2d + float64(m.VD)*float64(m.VD))
	return &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.VN)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VE)),
			gpsprot.MetersPerSecondFromFloat(float64(m.VD)),
		}),
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(speed2d)),
		Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(speed3d)),
		NativeMsgID: "DAT-NED3",
	}
}

// dopDatDOP accumulates DOP values from DatDOP into the NavEpochMsg.
func dopDatDOP(ne *gpsprot.NavEpochMsg, m *sdbpbin.DatDOP) {
	if ne == nil {
		return
	}
	qualityFromValid(ne, m.Valid)
	ne.NumSVUsed.Set(uint16(m.FixSats))
	ne.DOP.Geom = opt.Make(float64(m.GDOP) / sdbpbin.DatDOPMeter)
	ne.DOP.Pos = opt.Make(float64(m.PDOP) / sdbpbin.DatDOPMeter)
	ne.DOP.Hor = opt.Make(float64(m.HDOP) / sdbpbin.DatDOPMeter)
	ne.DOP.Vert = opt.Make(float64(m.VDOP) / sdbpbin.DatDOPMeter)
	ne.DOP.Time = opt.Make(float64(m.TDOP) / sdbpbin.DatDOPMeter)
}

// qualityFromValid sets FixLevel, SolutionDim, Correction and AuxSrc
// from the SDBP Validity Flag byte.
func qualityFromValid(ne *gpsprot.NavEpochMsg, valid sdbpbin.DatValid) {
	posType := valid & sdbpbin.DatValidPosType
	fixQual := valid & sdbpbin.DatValidFixQual
	// No-fix cases.
	switch posType {
	case sdbpbin.DatValidPosUnavailable:
		ne.FixLevel = gpsprot.FixLevelNone
		return
	case sdbpbin.DatValidPosDR:
		ne.FixLevel = gpsprot.FixLevelNone
		ne.AuxSrc = gpsprot.AuxSrcDR
		return
	}
	if fixQual == sdbpbin.DatValidFixDR {
		ne.FixLevel = gpsprot.FixLevelNone
		ne.AuxSrc = gpsprot.AuxSrcDR
		return
	}
	// Valid GNSS fix.
	ne.FixLevel = gpsprot.FixLevelCode
	switch posType {
	case sdbpbin.DatValidPos2D:
		ne.SolutionDim = gpsprot.SolutionDim2D
	case sdbpbin.DatValidPos3D:
		ne.SolutionDim = gpsprot.SolutionDim3D
	case sdbpbin.DatValidPosTiming:
		ne.SolutionDim = gpsprot.SolutionDimTimeOnly
	}
	switch fixQual {
	case sdbpbin.DatValidFixDifferential:
		ne.Correction |= gpsprot.CorrUsed
	case sdbpbin.DatValidFixRTKFixed:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.Correction |= gpsprot.CorrOSR | gpsprot.CorrUsed
	case sdbpbin.DatValidFixRTKFloat:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.Correction |= gpsprot.CorrOSR | gpsprot.CorrUsed
	}
}
