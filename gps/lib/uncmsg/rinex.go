package uncmsg

// RINEXSys returns the RINEX satellite system letter for an OBSVM tracking
// status satellite system value.
// It returns "" for unrecognized systems.
func RINEXSys(system SysID) string {
	switch system {
	case SysGPS:
		return "G"
	case SysGLO:
		return "R"
	case SysSBAS:
		return "S"
	case SysGAL:
		return "E"
	case SysBDS:
		return "C"
	case SysQZSS:
		return "J"
	case SysNAVIC:
		return "I"
	}
	return ""
}

// RINEXSatNum converts a Unicore OBSVM PRN/slot number to a RINEX satellite
// number. It returns 0 for reserved or unmapped satellite numbers.
func RINEXSatNum(system SysID, prnSlot uint16) uint8 {
	switch system {
	case SysGPS:
		if prnSlot >= 1 && prnSlot <= 32 {
			return uint8(prnSlot)
		}
	case SysGLO:
		if prnSlot >= 38 && prnSlot <= 61 {
			return uint8(prnSlot - 37)
		}
	case SysSBAS:
		if prnSlot >= 120 && prnSlot <= 158 {
			return uint8(prnSlot - 100)
		}
	case SysGAL:
		if prnSlot >= 1 && prnSlot <= 36 {
			return uint8(prnSlot)
		}
	case SysBDS:
		if prnSlot >= 1 && prnSlot <= 63 {
			return uint8(prnSlot)
		}
	case SysQZSS:
		if prnSlot >= 193 && prnSlot <= 202 {
			return uint8(prnSlot - 192)
		}
	case SysNAVIC:
		if prnSlot >= 1 && prnSlot <= 14 {
			return uint8(prnSlot)
		}
	}
	return 0
}

// RINEXObsSig returns the RINEX two-character signal identifier for an OBSVM
// channel tracking status satellite system, signal type, and L2C flag.
// It returns "" for reserved or unmapped signal types.
// The L2C flag (bit 26 of the channel tracking status word) overrides the
// nominal L2P(Y) reading of sigType 9 to L2C(M) on GPS and QZSS. QZSS does
// not actually broadcast L2P(Y), so only the L2C-set case is emitted.
func RINEXObsSig(system SysID, sigType FreqID, l2c bool) string {
	if l2c {
		switch system {
		case SysGPS:
			if sigType == FreqGPSL2P {
				return "2S"
			}
		case SysQZSS:
			if sigType == FreqQZSSL2CM {
				return "2S"
			}
		}
	}
	return rinexObsSigMap[system][sigType]
}

// rinexObsSigMap maps OBSVM channel tracking status signal types to RINEX
// signal identifiers.
var rinexObsSigMap = map[SysID]map[FreqID]string{
	SysGPS: {
		FreqGPSL1CA: "1C",
		FreqGPSL1CP: "1L",
		FreqGPSL1CD: "1S",
		FreqGPSL5D:  "5I",
		FreqGPSL5P:  "5Q",
		FreqGPSL2P:  "2W",
		FreqGPSL2CL: "2L",
	},
	SysGLO: {
		FreqGLOL1CA: "1C",
		FreqGLOL2CA: "2C",
		FreqGLOG3I:  "3I",
		FreqGLOG3Q:  "3Q",
	},
	SysGAL: {
		FreqGALE1B:  "1B",
		FreqGALE1C:  "1C",
		FreqGALE5AP: "5Q",
		FreqGALE5BP: "7Q",
		FreqGALE6B:  "6B",
		FreqGALE6C:  "6C",
	},
	SysBDS: {
		FreqBDSB1I:  "2I",
		FreqBDSB1Q:  "2Q",
		FreqBDSB1CP: "1P",
		FreqBDSB1CD: "1D",
		FreqBDSB2Q:  "7Q",
		FreqBDSB2I:  "7I",
		FreqBDSB2aP: "5P",
		FreqBDSB2aD: "5D",
		FreqBDSB3Q:  "6Q",
		FreqBDSB3I:  "6I",
		FreqBDSB2bI: "7D",
	},
	SysQZSS: {
		FreqQZSSL1CA: "1C",
		FreqQZSSL1CB: "1E",
		FreqQZSSL1CP: "1L",
		FreqQZSSL1S:  "1Z",
		FreqQZSSL5D:  "5I",
		FreqQZSSL1CD: "1S",
		FreqQZSSL5P:  "5Q",
		FreqQZSSL2CL: "2L",
		FreqQZSSL6D:  "6S",
		FreqQZSSL6E:  "6E",
	},
	SysSBAS: {
		FreqSBASL1CA: "1C",
		FreqSBASL5I:  "5I",
	},
	SysNAVIC: {
		FreqNAVICL5D: "5A",
	},
}
