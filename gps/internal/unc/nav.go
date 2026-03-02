package unc

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nov"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

// bestNavPosVel extracts geodetic position and velocity from a BESTNAV message.
// Position and velocity have independent solution status, so either can be nil.
func bestNavPosVel(ne *gpsprot.NavEpochMsg, m *uncmsg.BestNav) (*gpsprot.PosGeoMsg, *gpsprot.VelGeoMsg) {
	var posG *gpsprot.PosGeoMsg
	var velG *gpsprot.VelGeoMsg
	if m.PSolStatus == uncmsg.SolComputed {
		posG = nov.PosGeo(ne, &m.Pos, "BESTNAV")
		posG.Priority = gpsprot.PriVendorLow
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
			Priority:    gpsprot.PriVendorLow,
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
		posE = nov.PosECEFXYZ(ne, &m.XYZ, "BESTNAVXYZ")
		posE.Priority = gpsprot.PriVendorLow
	}
	if m.VSolStatus == uncmsg.SolComputed {
		velE = nov.VelECEFXYZ(ne, &m.XYZ, "BESTNAVXYZ")
		velE.Priority = gpsprot.PriVendorLow
	}
	return posE, velE
}

// Unit conversion helpers for Unicore floating-point fields.

func mpsToSpeed(v float64) gpsprot.Speed {
	return gpsprot.MetersPerSecondFromFloat(v)
}

func degreesToAngle(v float64) gpsprot.Angle {
	return gpsprot.DegreesFromFloat(v)
}
