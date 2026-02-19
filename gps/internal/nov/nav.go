package nov

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// PosGeo converts a Pos into a PosGeoMsg and accumulates
// accuracy into the NavEpochMsg.
func PosGeo[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.Pos[S, P], nativeMsgID string) *gpsprot.PosGeoMsg {
	ne.Acc.Hor.Set(gpsprot.Meters(math.Sqrt(
		float64(b.LatSigma)*float64(b.LatSigma) +
			float64(b.LonSigma)*float64(b.LonSigma))))
	ne.Acc.Vert.Set(gpsprot.Meters(float64(b.HgtSigma)))
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(b.Lat), gpsprot.DegreesFromFloat(b.Lon)},
		Height:      opt.Make(gpsprot.Meters(float64(b.Hgt) + float64(b.Undulation))),
		HeightMSL:   opt.Make(gpsprot.Meters(b.Hgt)),
		NativeMsgID: nativeMsgID,
		Priority:    gpsprot.PriVendorLow,
	}
}

// PosECEFXYZ converts XYZ position fields into a PosECEFMsg and accumulates
// accuracy into the NavEpochMsg.
func PosECEFXYZ[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.XYZ[S, P], nativeMsgID string) *gpsprot.PosECEFMsg {
	ne.Acc.Pos.Set(gpsprot.Meters(math.Sqrt(
		float64(b.PXSigma)*float64(b.PXSigma) +
			float64(b.PYSigma)*float64(b.PYSigma) +
			float64(b.PZSigma)*float64(b.PZSigma))))
	return &gpsprot.PosECEFMsg{
		Pos:         gpsprot.Point3D{gpsprot.Meters(b.PX), gpsprot.Meters(b.PY), gpsprot.Meters(b.PZ)},
		NativeMsgID: nativeMsgID,
		Priority:    gpsprot.PriVendorLow,
	}
}

// VelECEFXYZ converts XYZ velocity fields into a VelECEFMsg and accumulates
// accuracy into the NavEpochMsg.
func VelECEFXYZ[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.XYZ[S, P], nativeMsgID string) *gpsprot.VelECEFMsg {
	ne.Acc.Speed.Set(gpsprot.MetersPerSecondFromFloat(math.Sqrt(
		float64(b.VXSigma)*float64(b.VXSigma) +
			float64(b.VYSigma)*float64(b.VYSigma) +
			float64(b.VZSigma)*float64(b.VZSigma))))
	return &gpsprot.VelECEFMsg{
		Vel:         [3]gpsprot.Speed{gpsprot.MetersPerSecondFromFloat(b.VX), gpsprot.MetersPerSecondFromFloat(b.VY), gpsprot.MetersPerSecondFromFloat(b.VZ)},
		NativeMsgID: nativeMsgID,
		Priority:    gpsprot.PriVendorLow,
	}
}
