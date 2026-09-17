package septentrio

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// radToDeg converts radians to a gpsprot.Angle.
func radToDeg(rad float64) gpsprot.Angle {
	return gpsprot.DegreesFromFloat(rad * 180 / math.Pi)
}

// posGeoPVTGeodetic builds a geodetic position from PVTGeodetic. It returns nil
// when there is no GNSS PVT or any coordinate is at its do-not-use sentinel.
func posGeoPVTGeodetic(m *sbfbin.PVTGeodetic) *gpsprot.PosGeoMsg {
	if m.Mode.Fix() == sbfbin.ModeNoPVT ||
		m.Latitude == sbfbin.PVTCoordinateDNU ||
		m.Longitude == sbfbin.PVTCoordinateDNU ||
		m.Height == sbfbin.PVTCoordinateDNU {
		return nil
	}
	msg := &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{radToDeg(m.Latitude), radToDeg(m.Longitude)},
		Height:      opt.Make(gpsprot.Meters(m.Height)),
		NativeMsgID: "PVTGeodetic",
	}
	if m.Undulation != sbfbin.PVTCoordinateDNU {
		msg.HeightMSL = opt.Make(gpsprot.Meters(m.Height - float64(m.Undulation)))
	}
	return msg
}

// posECEFPVTCartesian builds an ECEF position from PVTCartesian.
func posECEFPVTCartesian(m *sbfbin.PVTCartesian) *gpsprot.PosECEFMsg {
	if m.Mode.Fix() == sbfbin.ModeNoPVT ||
		m.X == sbfbin.PVTCoordinateDNU ||
		m.Y == sbfbin.PVTCoordinateDNU ||
		m.Z == sbfbin.PVTCoordinateDNU {
		return nil
	}
	return &gpsprot.PosECEFMsg{
		Pos:         gpsprot.Point3D{gpsprot.Meters(m.X), gpsprot.Meters(m.Y), gpsprot.Meters(m.Z)},
		NativeMsgID: "PVTCartesian",
	}
}

// velGeoPVTGeodetic builds a geodetic velocity from PVTGeodetic. VelNED's Down
// component is -Vu. Course comes from COG, whose do-not-use is independent of
// the velocity fields (COG is also DNU below ~0.1 m/s).
func velGeoPVTGeodetic(m *sbfbin.PVTGeodetic) *gpsprot.VelGeoMsg {
	if m.Mode.Fix() == sbfbin.ModeNoPVT ||
		m.Vn == sbfbin.PVTVelocityDNU ||
		m.Ve == sbfbin.PVTVelocityDNU ||
		m.Vu == sbfbin.PVTVelocityDNU {
		return nil
	}
	msg := &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.Vn)),
			gpsprot.MetersPerSecondFromFloat(float64(m.Ve)),
			gpsprot.MetersPerSecondFromFloat(float64(-m.Vu)),
		}),
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(math.Hypot(float64(m.Vn), float64(m.Ve)))),
		NativeMsgID: "PVTGeodetic",
	}
	if m.COG != sbfbin.PVTCourseOverGroundDNU {
		msg.Course = opt.Make(gpsprot.DegreesFromFloat(float64(m.COG)))
	}
	return msg
}

// velECEFPVTCartesian builds an ECEF velocity from PVTCartesian.
func velECEFPVTCartesian(m *sbfbin.PVTCartesian) *gpsprot.VelECEFMsg {
	if m.Mode.Fix() == sbfbin.ModeNoPVT ||
		m.Vx == sbfbin.PVTVelocityDNU ||
		m.Vy == sbfbin.PVTVelocityDNU ||
		m.Vz == sbfbin.PVTVelocityDNU {
		return nil
	}
	return &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(float64(m.Vx)),
			gpsprot.MetersPerSecondFromFloat(float64(m.Vy)),
			gpsprot.MetersPerSecondFromFloat(float64(m.Vz)),
		},
		NativeMsgID: "PVTCartesian",
	}
}
