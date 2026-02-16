package unc

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

// bestNavPosVel extracts geodetic position and velocity from a BESTNAV message.
// Position and velocity have independent solution status, so either can be nil.
func bestNavPosVel(ne *gpsprot.NavEpochMsg, m *uncmsg.BestNav) (*gpsprot.PosGeoMsg, *gpsprot.VelGeoMsg) {
	var posG *gpsprot.PosGeoMsg
	var velG *gpsprot.VelGeoMsg
	if m.PSolStatus == uncmsg.SolComputed {
		ne.Acc.Hor.Set(metersToLength(math.Sqrt(
			float64(m.LatSigma)*float64(m.LatSigma) +
				float64(m.LonSigma)*float64(m.LonSigma))))
		ne.Acc.Vert.Set(metersToLength(float64(m.HgtSigma)))
		posG = &gpsprot.PosGeoMsg{
			LatLon:      [2]gpsprot.Angle{degreesToAngle(m.Lat), degreesToAngle(m.Lon)},
			Height:      opt.Make(metersToLength(float64(m.Hgt) + float64(m.Undulation))),
			HeightMSL:   opt.Make(metersToLength(m.Hgt)),
			NativeMsgID: "BESTNAV",
		}
	}
	if m.VSolStatus == uncmsg.SolComputed {
		ne.Acc.GroundSpeed.Set(mpsToSpeed(float64(m.HorSpdSigma)))
		ne.Acc.Speed.Set(mpsToSpeed(math.Sqrt(
			float64(m.HorSpdSigma)*float64(m.HorSpdSigma) +
				float64(m.VertSpdSigma)*float64(m.VertSpdSigma))))
		velG = &gpsprot.VelGeoMsg{
			GroundSpeed: opt.Make(mpsToSpeed(m.HorSpd)),
			Course:      opt.Make(degreesToAngle(m.TrkGnd)),
			NativeMsgID: "BESTNAV",
		}
	}
	return posG, velG
}

// bestNavXYZPosVel extracts ECEF position and velocity from a BESTNAVXYZ message.
// Position and velocity have independent solution status, so either can be nil.
func bestNavXYZPosVel(ne *gpsprot.NavEpochMsg, m *uncmsg.BestNavXYZ) (*gpsprot.PosECEFMsg, *gpsprot.VelECEFMsg) {
	var posE *gpsprot.PosECEFMsg
	var velE *gpsprot.VelECEFMsg
	if m.PSolStatus == uncmsg.SolComputed {
		ne.Acc.Pos.Set(metersToLength(math.Sqrt(
			float64(m.PXSigma)*float64(m.PXSigma) +
				float64(m.PYSigma)*float64(m.PYSigma) +
				float64(m.PZSigma)*float64(m.PZSigma))))
		posE = &gpsprot.PosECEFMsg{
			Pos:         gpsprot.Point3D{metersToLength(m.PX), metersToLength(m.PY), metersToLength(m.PZ)},
			NativeMsgID: "BESTNAVXYZ",
		}
	}
	if m.VSolStatus == uncmsg.SolComputed {
		ne.Acc.Speed.Set(mpsToSpeed(math.Sqrt(
			float64(m.VXSigma)*float64(m.VXSigma) +
				float64(m.VYSigma)*float64(m.VYSigma) +
				float64(m.VZSigma)*float64(m.VZSigma))))
		velE = &gpsprot.VelECEFMsg{
			Vel:         [3]gpsprot.Speed{mpsToSpeed(m.VX), mpsToSpeed(m.VY), mpsToSpeed(m.VZ)},
			NativeMsgID: "BESTNAVXYZ",
		}
	}
	return posE, velE
}

// Unit conversion helpers for Unicore floating-point fields.

func metersToLength(v float64) gpsprot.Length {
	return gpsprot.Meters(v)
}

func mpsToSpeed(v float64) gpsprot.Speed {
	return gpsprot.MetersPerSecondFromFloat(v)
}

func degreesToAngle(v float64) gpsprot.Angle {
	return gpsprot.DegreesFromFloat(v)
}
