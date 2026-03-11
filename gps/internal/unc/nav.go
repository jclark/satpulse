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
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs)
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

// pppNavPos extracts geodetic position from a PPPNAV message.
// PPPNAV is position-only (no velocity). It uses PriVendorHigh so it
// takes precedence over BESTNAV when both are available in the same epoch.
func pppNavPos(ne *gpsprot.NavEpochMsg, m *uncmsg.PPPNav) *gpsprot.PosGeoMsg {
	quality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs)
	if m.PSolStatus != uncmsg.SolComputed {
		return nil
	}
	posG := nov.PosGeo(ne, &m.Pos, "PPPNAV")
	posG.Priority = gpsprot.PriVendorHigh
	return posG
}

// bestNavXYZPosVel extracts ECEF position and velocity from a BESTNAVXYZ message.
// Position and velocity have independent solution status, so either can be nil.
func bestNavXYZPosVel(ne *gpsprot.NavEpochMsg, m *uncmsg.BestNavXYZ) (*gpsprot.PosECEFMsg, *gpsprot.VelECEFMsg) {
	var posE *gpsprot.PosECEFMsg
	var velE *gpsprot.VelECEFMsg
	quality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs)
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
// GNSSUsed and BandsUsed are not set here: the signal mask bytes in BESTNAV/BESTNAVXYZ
// are unreliable on Unicore firmware (e.g. reporting only L1 when multi-frequency
// signals are in use). BESTSAT provides accurate data for these fields instead.
func quality(ne *gpsprot.NavEpochMsg, solStatus uncmsg.SolStatus, posType uncmsg.PosVelType,
	diffAge float32, stnID novmsg.StationID, numSVs, numSolnSVs uint8) {
	ne.NumSVUsed.Set(uint16(numSolnSVs))
	ne.NumSVTracked.Set(uint16(numSVs))
	if diffAge > 0 {
		ne.DiffAge.Set(gpsprot.Seconds(float64(diffAge)))
	}
	// Parse station ID for RTCMRefBaseID or correction enrichment.
	if v, ok := nov.StationIDValue(stnID); ok {
		ne.RTCMRefBaseID.Set(v)
	}
	ne.Correction |= pppServiceCorrection(uncmsg.StnIDPPPService(stnID)).Expand()
	if solStatus != uncmsg.SolComputed {
		ne.FixLevel = gpsprot.FixLevelNone
		return
	}
	// Unicore-specific pos type values first.
	switch posType {
	case uncmsg.PosVelFixedPos:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDimTimeOnly
		return
	case uncmsg.PosVelINS:
		ne.FixLevel = gpsprot.FixLevelNone
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case uncmsg.PosVelPPPAR:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction |= gpsprot.CorrPPPConverged.Expand()
		return
	case uncmsg.PosVelPPPRTK:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction |= gpsprot.CorrPPPRTK.Expand()
		return
	}
	// Shared pos type values.
	fl, fd, ck, aux, ok := nov.PosTypeQuality(uint32(posType))
	if ok {
		ne.FixLevel = fl
		ne.SolutionDim = fd
		ne.Correction |= ck
		ne.AuxSrc |= aux
	}
}

// pppServiceCorrection maps a Unicore PPP service to correction kind.
func pppServiceCorrection(svc uncmsg.PPPService) gpsprot.CorrKind {
	switch svc {
	case uncmsg.PPPB2b:
		return gpsprot.CorrPPPB2b
	case uncmsg.PPPHAS:
		return gpsprot.CorrPPPHAS
	case uncmsg.PPPMDC:
		return gpsprot.CorrPPPMDC
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
	ne.DOP.North.Set(float64(m.NDOP))
	ne.DOP.East.Set(float64(m.EDOP))
}

// bestSatGNSSBands extracts GNSSUsed and BandsUsed from BESTSAT per-satellite data.
// This is more accurate than the signal mask bytes in BESTPOS/BESTXYZ.
func bestSatGNSSBands(sats []uncmsg.BestSatEntry) (gpsprot.GNSSSet, gpsprot.Band) {
	var gs gpsprot.GNSSSet
	var b gpsprot.Band
	for _, s := range sats {
		gnss := satSysToGNSS(s.System)
		if gnss == 0 {
			continue
		}
		gs |= gpsprot.GNSSSetOf(gnss)
		b |= sigMaskToBand(s.System, s.SigMask)
	}
	return gs, b
}

// sigMaskToBand converts a BESTSAT SigMask to Band for a given constellation.
// QZSS and NAVIC signal mask tables are not documented; they contribute to
// GNSSUsed but not BandsUsed.
func sigMaskToBand(sys uncmsg.SatSys, mask uncmsg.SigUsed) gpsprot.Band {
	var b gpsprot.Band
	switch sys {
	case uncmsg.SatSysGPS:
		if mask&uncmsg.SigUsedGPSL1 != 0 {
			b |= gpsprot.BandL1
		}
		if mask&uncmsg.SigUsedGPSL2 != 0 {
			b |= gpsprot.BandL2
		}
		if mask&uncmsg.SigUsedGPSL5 != 0 {
			b |= gpsprot.BandL5
		}
	case uncmsg.SatSysGLONASS:
		if mask&uncmsg.SigUsedGLOL1 != 0 {
			b |= gpsprot.BandL1
		}
		if mask&uncmsg.SigUsedGLOL2 != 0 {
			b |= gpsprot.BandL2
		}
		if mask&uncmsg.SigUsedGLOL3 != 0 {
			b |= gpsprot.BandE5b
		}
	case uncmsg.SatSysBEIDOU:
		if mask&uncmsg.SigUsedBDSB1 != 0 {
			b |= gpsprot.BandL1
		}
		if mask&uncmsg.SigUsedBDSB2 != 0 {
			b |= gpsprot.BandL5 | gpsprot.BandE5b
		}
		if mask&uncmsg.SigUsedBDSB3 != 0 {
			b |= gpsprot.BandE6
		}
	case uncmsg.SatSysGALILEO:
		if mask&uncmsg.SigUsedGALE1 != 0 {
			b |= gpsprot.BandL1
		}
		if mask&uncmsg.SigUsedGALE5A != 0 {
			b |= gpsprot.BandL5
		}
		if mask&uncmsg.SigUsedGALE5B != 0 {
			b |= gpsprot.BandE5b
		}
		if mask&uncmsg.SigUsedGALALTBOC != 0 {
			b |= gpsprot.BandL5 | gpsprot.BandE5b
		}
	}
	return b
}

// Unit conversion helpers for Unicore floating-point fields.

func mpsToSpeed(v float64) gpsprot.Speed {
	return gpsprot.MetersPerSecondFromFloat(v)
}

func degreesToAngle(v float64) gpsprot.Angle {
	return gpsprot.DegreesFromFloat(v)
}
