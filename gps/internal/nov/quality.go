package nov

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// PosTypeQuality maps a NovAtel/Unicore PosType numeric value to quality fields.
// It handles values shared across all vendors. Returns false for vendor-specific
// values that the caller must handle.
func PosTypeQuality(pt uint32) (gpsprot.FixLevel, gpsprot.SolutionDim, gpsprot.CorrKind, gpsprot.AuxSrc, bool) {
	switch pt {
	case novmsg.PosNone:
		return gpsprot.FixLevelNone, 0, 0, 0, true
	case novmsg.PosFixedHeight:
		return gpsprot.FixLevelCode, gpsprot.SolutionDim2D, 0, 0, true
	case novmsg.PosDopplerVelocity:
		return gpsprot.FixLevelDoppler, 0, 0, 0, true
	case novmsg.PosSingle:
		return gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, 0, true
	case novmsg.PosPSRDiff:
		return gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), 0, true
	case novmsg.PosSBAS:
		return gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrSBAS.Expand(), 0, true
	case novmsg.PosL1Float:
		return gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), 0, true
	case novmsg.PosIonoFreeFloat:
		return gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true
	case novmsg.PosNarrowFloat:
		return gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true
	case novmsg.PosL1Int:
		return gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), 0, true
	case novmsg.PosWideInt:
		return gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrPartialDualFreq.Expand(), 0, true
	case novmsg.PosNarrowInt:
		return gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true
	case novmsg.PosINSPSRSP:
		return gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, gpsprot.AuxSrcINS, true
	case novmsg.PosINSPSRDiff:
		return gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), gpsprot.AuxSrcINS, true
	case novmsg.PosINSRTKFloat:
		return gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), gpsprot.AuxSrcINS, true
	case novmsg.PosINSRTKFixed:
		return gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), gpsprot.AuxSrcINS, true
	case novmsg.PosPPPConverging:
		return gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverging.Expand(), 0, true
	case novmsg.PosPPP:
		return gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), 0, true
	default:
		return 0, 0, 0, 0, false
	}
}

// SignalsUsed converts NovAtel OEM7 signal mask bytes (Table 93, Table 94)
// to GNSS and band sets.
func SignalsUsed(gpsGloBds2, galBds3 novmsg.HexByte) (gpsprot.GNSSSet, gpsprot.Band) {
	var gs gpsprot.GNSSSet
	var b gpsprot.Band
	// gpsGloBds2 (Table 93):
	// bit 0: GPS L1, bit 1: GPS L2, bit 2: GPS L5, bit 3: Reserved
	// bit 4: GLO L1, bit 5: GLO L2, bit 6: GLO L3, bit 7: Reserved
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
	if gpsGloBds2&0x10 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GLO)
		b |= gpsprot.BandL1
	}
	if gpsGloBds2&0x20 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GLO)
		b |= gpsprot.BandL2
	}
	if gpsGloBds2&0x40 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GLO)
		b |= gpsprot.BandE5b
	}
	// galBds3 (Table 94):
	// bit 0: GAL E1, bit 1: GAL E5a, bit 2: GAL E5b, bit 3: GAL ALTBOC
	// bit 4: BDS B1, bit 5: BDS B2, bit 6: BDS B3, bit 7: GAL E6
	if galBds3&0x01 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandL1
	}
	if galBds3&0x02 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandL5
	}
	if galBds3&0x04 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandE5b
	}
	if galBds3&0x08 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandL5 | gpsprot.BandE5b
	}
	if galBds3&0x10 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandL1
	}
	if galBds3&0x20 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandL5 | gpsprot.BandE5b
	}
	if galBds3&0x40 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.BDS)
		b |= gpsprot.BandE6
	}
	if galBds3&0x80 != 0 {
		gs |= gpsprot.GNSSSetOf(gpsprot.GAL)
		b |= gpsprot.BandE6
	}
	return gs, b
}

// StationIDValue parses a NovAtel/Unicore StationID as a decimal number.
// Returns (value, true) only if the result is a valid RTCM station ID (<= 4095).
// Returns (0, false) for non-numeric or out-of-range values.
func StationIDValue(s novmsg.StationID) (uint16, bool) {
	n := len(s)
	for n > 0 && s[n-1] == 0 {
		n--
	}
	if n == 0 {
		return 0, false
	}
	var v uint16
	for i := 0; i < n; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + uint16(c-'0')
		if v > 4095 {
			return 0, false
		}
	}
	return v, true
}

// StationIDUint parses a NovAtel/Unicore StationID as a decimal number.
// Returns (value, true) for any numeric value. Returns (0, false) for non-numeric.
func StationIDUint(s novmsg.StationID) (uint16, bool) {
	n := len(s)
	for n > 0 && s[n-1] == 0 {
		n--
	}
	if n == 0 {
		return 0, false
	}
	var v uint32
	for i := 0; i < n; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + uint32(c-'0')
		if v > 0xFFFF {
			return 0, false
		}
	}
	return uint16(v), true
}
