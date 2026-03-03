package unc

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nov"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

// bestNavPosVel extracts geodetic position and velocity from a BESTNAV message.
// Position and velocity have independent solution status, so either can be nil.
func bestNavPosVel(ne *gpsprot.NavEpochMsg, m *uncmsg.BestNav) (*gpsprot.PosGeoMsg, *gpsprot.VelGeoMsg) {
	var posG *gpsprot.PosGeoMsg
	var velG *gpsprot.VelGeoMsg
	quality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs,
		m.GalBDS3Sig, m.GPSGLOBDS2Sig)
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
	quality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs,
		m.GalBDS3Sig, m.GPSGLOBDS2Sig)
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

// quality populates solution quality fields on the NavEpochMsg from BESTNAV/BESTNAVXYZ
// fields. Called regardless of PSolStatus: sets FixLevelNone when not SolComputed.
func quality(ne *gpsprot.NavEpochMsg, solStatus uncmsg.SolStatus, posType uncmsg.PosVelType,
	diffAge float32, stnID novmsg.StationID, numSVs, numSolnSVs uint8,
	galBds3, gpsGloBds2 novmsg.HexByte) {
	ne.NumSVUsed.Set(uint16(numSolnSVs))
	ne.NumSVTracked.Set(uint16(numSVs))
	ne.SignalsUsed = nov.SignalsUsed(gpsGloBds2, galBds3)
	if diffAge > 0 {
		ne.DiffAge.Set(gpsprot.Seconds(float64(diffAge)))
	}
	// Parse station ID for RTCMRefBaseID or correction enrichment.
	if v, ok := nov.StationIDValue(stnID); ok {
		ne.RTCMRefBaseID.Set(v)
	} else if v, ok := nov.StationIDUint(stnID); ok && v >= 4096 {
		ne.Correction |= stnIDCorrection(v)
	}
	if solStatus != uncmsg.SolComputed {
		ne.FixLevel = gpsprot.FixLevelNone
		return
	}
	// Unicore-specific pos type values first.
	switch posType {
	case uncmsg.PosVelFixedPos:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.FixDim = gpsprot.FixDimTimeOnly
		return
	case uncmsg.PosVelINS:
		ne.FixLevel = gpsprot.FixLevelNone
		ne.FixDim = gpsprot.FixDim3D
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case uncmsg.PosVelPPPAR:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.FixDim = gpsprot.FixDim3D
		ne.Correction = gpsprot.CorrPPPConverged.Expand()
		return
	case uncmsg.PosVelPPPRTK:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.FixDim = gpsprot.FixDim3D
		ne.Correction = gpsprot.CorrPPPRTK.Expand()
		return
	}
	// Shared pos type values.
	fl, fd, ck, aux, ok := nov.PosTypeQuality(uint32(posType))
	if ok {
		ne.FixLevel = fl
		ne.FixDim = fd
		ne.Correction = ck
		ne.AuxSrc |= aux
	}
}

// stnIDCorrection maps Unicore station IDs >= 4096 to correction kind.
// Unicore uses 9xxx values for satellite-based correction services.
func stnIDCorrection(v uint16) gpsprot.CorrKind {
	switch {
	case v >= 9901 && v <= 9905: // BeiDou B2b PPP
		return gpsprot.CorrWideArea.Expand()
	case v >= 9959 && v <= 9961: // BeiDou B2b PPP
		return gpsprot.CorrWideArea.Expand()
	case v == 9964: // Galileo E6 HAS
		return gpsprot.CorrWideArea.Expand()
	case v >= 9934 && v <= 9939: // QZSS L6 MDC
		return gpsprot.CorrCLAS.Expand()
	case v >= 9974 && v <= 9979: // QZSS L6 CLAS
		return gpsprot.CorrCLAS.Expand()
	case v >= 9990 && v <= 9999: // L-band
		return gpsprot.CorrWideArea.Expand()
	default:
		return 0
	}
}

// staDOP populates DOP fields on the NavEpochMsg from a STADOP message.
func staDOP(ne *gpsprot.NavEpochMsg, m *uncmsg.StaDOP) {
	ne.DOP.Geom.Set(float64(m.GDOP))
	ne.DOP.Pos.Set(float64(m.PDOP))
	ne.DOP.Hor.Set(float64(m.HDOP))
	ne.DOP.Vert.Set(float64(m.VDOP))
	ne.DOP.Time.Set(float64(m.TDOP))
}

// Unit conversion helpers for Unicore floating-point fields.

func mpsToSpeed(v float64) gpsprot.Speed {
	return gpsprot.MetersPerSecondFromFloat(v)
}

func degreesToAngle(v float64) gpsprot.Angle {
	return gpsprot.DegreesFromFloat(v)
}
