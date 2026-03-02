package nov

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// PosTypeQuality maps a NovAtel/Unicore PosType numeric value to quality fields.
// It handles values shared across all vendors. Returns false for vendor-specific
// values that the caller must handle.
func PosTypeQuality(pt uint32) (gpsprot.FixLevel, gpsprot.FixDim, gpsprot.CorrKind, gpsprot.AuxSrc, bool) {
	switch pt {
	case 0: // NONE
		return gpsprot.FixLevelNone, 0, 0, 0, true
	case 2: // FIXEDHEIGHT
		return gpsprot.FixLevelCode, gpsprot.FixDim2D, 0, 0, true
	case 8: // DOPPLER_VELOCITY
		return 0, gpsprot.FixDimVelocityOnly, 0, 0, true
	case 16: // SINGLE
		return gpsprot.FixLevelCode, gpsprot.FixDim3D, 0, 0, true
	case 17: // PSRDIFF
		return gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), 0, true
	case 18: // SBAS / WAAS
		return gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrSBAS.Expand(), 0, true
	case 32: // L1_FLOAT
		return gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), 0, true
	case 33: // IONOFREE_FLOAT
		return gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true
	case 34: // NARROW_FLOAT
		return gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true
	case 48: // L1_INT
		return gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), 0, true
	case 49: // WIDE_INT
		return gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrPartialDualFreq.Expand(), 0, true
	case 50: // NARROW_INT
		return gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true
	case 53: // INS_PSRSP
		return gpsprot.FixLevelCode, gpsprot.FixDim3D, 0, gpsprot.AuxSrcINS, true
	case 54: // INS_PSRDIFF
		return gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), gpsprot.AuxSrcINS, true
	case 55: // INS_RTKFLOAT
		return gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), gpsprot.AuxSrcINS, true
	case 56: // INS_RTKFIXED
		return gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), gpsprot.AuxSrcINS, true
	case 68: // PPP_CONVERGING
		return gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrPPPConverging.Expand(), 0, true
	case 69: // PPP
		return gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrPPPConverged.Expand(), 0, true
	default:
		return 0, 0, 0, 0, false
	}
}

// SignalsUsed converts NovAtel/Unicore signal mask bytes to a SignalSet.
// The bit definitions are shared between Unicore UM980 and NovAtel OEM7.
func SignalsUsed(gpsGloBds2, galBds3 novmsg.HexByte) gpsprot.SignalSet {
	var ss gpsprot.SignalSet
	// GPS/GLO/BDS2 signal mask (gpsGloBds2):
	// bit 0: GPS L1CA, bit 1: GPS L2P, bit 2: GPS L2C, bit 3: GPS L5
	// bit 4: GLO L1, bit 5: GLO L2, bit 6: BDS B1I, bit 7: BDS B2I
	if gpsGloBds2&0x01 != 0 {
		ss |= 1 << gpsprot.SigGPSL1CA
	}
	if gpsGloBds2&0x02 != 0 {
		ss |= 1 << gpsprot.SigGPSL2P
	}
	if gpsGloBds2&0x04 != 0 {
		ss |= 1 << gpsprot.SigGPSL2C
	}
	if gpsGloBds2&0x08 != 0 {
		ss |= 1 << gpsprot.SigGPSL5
	}
	if gpsGloBds2&0x10 != 0 {
		ss |= 1 << gpsprot.SigGLOL1
	}
	if gpsGloBds2&0x20 != 0 {
		ss |= 1 << gpsprot.SigGLOL2
	}
	if gpsGloBds2&0x40 != 0 {
		ss |= 1 << gpsprot.SigBDSB1I
	}
	if gpsGloBds2&0x80 != 0 {
		ss |= 1 << gpsprot.SigBDSB2I
	}
	// GAL/BDS3 signal mask (galBds3):
	// bit 0: GAL E1, bit 1: GAL E5a, bit 2: GAL E5b
	// bit 4: BDS B1C, bit 5: BDS B2a, bit 6: BDS B3I, bit 7: BDS B2b
	if galBds3&0x01 != 0 {
		ss |= 1 << gpsprot.SigGALE1
	}
	if galBds3&0x02 != 0 {
		ss |= 1 << gpsprot.SigGALE5a
	}
	if galBds3&0x04 != 0 {
		ss |= 1 << gpsprot.SigGALE5b
	}
	if galBds3&0x10 != 0 {
		ss |= 1 << gpsprot.SigBDSB1C
	}
	if galBds3&0x20 != 0 {
		ss |= 1 << gpsprot.SigBDSB2a
	}
	if galBds3&0x40 != 0 {
		ss |= 1 << gpsprot.SigBDSB3I
	}
	if galBds3&0x80 != 0 {
		ss |= 1 << gpsprot.SigBDSB2b
	}
	return ss
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
