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
	// Fill semantics: only set from signal mask bytes if not already set by BESTSAT.
	if ne.GNSSUsed == 0 {
		gs, b := signalsUsed(gpsGloBds2, galBds3)
		ne.GNSSUsed = gs
		ne.BandsUsed = b
	}
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

// stnIDCorrection maps Unicore station IDs >= 4096 to correction kind.
// Unicore uses 9xxx values for satellite-based correction services.
func stnIDCorrection(v uint16) gpsprot.CorrKind {
	switch {
	case v >= 9901 && v <= 9905: // BeiDou B2b PPP
		return gpsprot.CorrPPPB2b.Expand()
	case v >= 9959 && v <= 9961: // BeiDou B2b PPP
		return gpsprot.CorrPPPB2b.Expand()
	case v == 9964: // Galileo E6 HAS
		return gpsprot.CorrPPPHAS.Expand()
	case v >= 9934 && v <= 9939: // QZSS L6 MDC
		return gpsprot.CorrPPPMDC.Expand()
	case v >= 9974 && v <= 9979: // QZSS L6 CLAS
		return gpsprot.CorrCLAS.Expand()
	case v >= 9990 && v <= 9999: // L-band
		return gpsprot.CorrSSR.Expand()
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

// signalsUsed converts Unicore UM980 signal mask bytes (Table 7-172, Table 7-173)
// to GNSS and band sets.
func signalsUsed(gpsGloBds2, galBds3 novmsg.HexByte) (gpsprot.GNSSSet, gpsprot.Band) {
	var gs gpsprot.GNSSSet
	var b gpsprot.Band
	// gpsGloBds2 (Table 7-172):
	// bit 0: GPS L1, bit 1: GPS L2, bit 2: GPS L5, bit 3: BDS B3
	// bit 4: GLO L1, bit 5: GLO L2, bit 6: BDS B1, bit 7: BDS B2
	if gpsGloBds2&0x01 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GPS)
		b |= gpsprot.BandL1
	}
	if gpsGloBds2&0x02 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GPS)
		b |= gpsprot.BandL2
	}
	if gpsGloBds2&0x04 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GPS)
		b |= gpsprot.BandL5
	}
	if gpsGloBds2&0x08 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandE6
	}
	if gpsGloBds2&0x10 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GLO)
		b |= gpsprot.BandL1
	}
	if gpsGloBds2&0x20 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GLO)
		b |= gpsprot.BandL2
	}
	if gpsGloBds2&0x40 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandL1
	}
	if gpsGloBds2&0x80 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandL5 | gpsprot.BandE5b
	}
	// galBds3 (Table 7-173):
	// bit 0: GAL E1, bit 1: GAL E5b, bit 2: GAL E5a, bit 3: Reserved
	// bit 4: BDS3 B1I, bit 5: BDS3 B3I, bit 6: BDS3 B2a, bit 7: BDS3 B1C
	if galBds3&0x01 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandL1
	}
	if galBds3&0x02 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandE5b
	}
	if galBds3&0x04 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandL5
	}
	if galBds3&0x10 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandL1
	}
	if galBds3&0x20 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandE6
	}
	if galBds3&0x40 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandL5
	}
	if galBds3&0x80 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandL1
	}
	return gs, b
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
