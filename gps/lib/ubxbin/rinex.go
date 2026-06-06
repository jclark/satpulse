package ubxbin

// RINEXSys returns the RINEX satellite system letter for gnssId.
// It returns "" for IMES and any unrecognised value.
func RINEXSys(gnssId GNSSID) string {
	switch gnssId {
	case GPS:
		return "G"
	case SBAS:
		return "S"
	case GAL:
		return "E"
	case BDS:
		return "C"
	case QZSS:
		return "J"
	case GLO:
		return "R"
	case NavIC:
		return "I"
	}
	return ""
}

// RINEXSatNum converts a UBX svId to a RINEX satellite number.
// For SBAS, the UBX PRN (120..158) is mapped to the RINEX PRN-100 form (20..58).
// For GLONASS, the UBX "unknown slot" value 255 maps to 0.
// Other systems pass through unchanged.
func RINEXSatNum(gnssId GNSSID, svId byte) uint8 {
	switch gnssId {
	case SBAS:
		if svId >= 120 {
			return svId - 100
		}
	case GLO:
		if svId == 255 {
			return 0
		}
	}
	return svId
}

// RINEXSig returns the RINEX two-character signal identifier for the UBX
// (gnssId, sigId) pair, or "" if the pair is not recognised.
// The mapping follows u-blox X20-HPG-2.00 Table 4 and RINEX 4.02 Tables 10-16.
func RINEXSig(gnssId GNSSID, sigId byte) string {
	// Reading from a nil inner map returns "", so an unknown gnssId yields "".
	return rinexSigMap[gnssId][sigId]
}

// rinexSigMap maps (GNSSID, sigId) to the RINEX signal identifier.
// BeiDou D1 and D2 navigation message variants share the same physical
// signal and so map to the same RINEX code (2I for B1I, 6I for B3I).
var rinexSigMap = map[GNSSID]map[byte]string{
	GPS: {
		0: "1C",
		3: "2L",
		4: "2S",
		6: "5I",
		7: "5Q",
	},
	SBAS: {
		0: "1C",
	},
	GAL: {
		0:  "1C",
		1:  "1B",
		3:  "5I",
		4:  "5Q",
		5:  "7I",
		6:  "7Q",
		8:  "6B",
		9:  "6C",
		10: "6A",
	},
	BDS: {
		0:  "2I",
		1:  "2I",
		2:  "7I",
		3:  "7I",
		4:  "6I",
		5:  "1P",
		6:  "1D",
		7:  "5P",
		8:  "5D",
		10: "6I",
	},
	QZSS: {
		0:  "1C",
		1:  "1Z",
		4:  "2S",
		5:  "2L",
		8:  "5I",
		9:  "5Q",
		12: "1E",
	},
	GLO: {
		0: "1C",
		2: "2C",
	},
	NavIC: {
		0: "5A",
	},
}
