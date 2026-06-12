package rtcmbin

// RINEXSys returns the RINEX satellite system letter for gnss.
// It returns "" for unrecognized constellations.
func RINEXSys(gnss GNSS) string {
	switch gnss {
	case GPS:
		return "G"
	case GLONASS:
		return "R"
	case GALILEO:
		return "E"
	case SBAS:
		return "S"
	case QZSS:
		return "J"
	case BEIDOU:
		return "C"
	case IRNSS:
		return "I"
	}
	return ""
}

// RINEXSatNum converts an RTCM MSM satellite ID to a RINEX satellite number.
// It returns 0 for reserved or unmapped satellite IDs.
func RINEXSatNum(gnss GNSS, satID uint8) uint8 {
	switch gnss {
	case GPS:
		if satID >= 1 && satID <= 63 {
			return satID
		}
	case GLONASS:
		if satID >= 1 && satID <= 24 {
			return satID
		}
	case GALILEO:
		if satID >= 1 && satID <= 50 {
			return satID
		}
	case SBAS:
		if satID >= 1 && satID <= 39 {
			return satID + 19
		}
	case QZSS:
		if satID >= 1 && satID <= 10 {
			return satID
		}
	case BEIDOU:
		if satID >= 1 && satID <= 63 {
			return satID
		}
	case IRNSS:
		if satID >= 1 && satID <= 14 {
			return satID
		}
	}
	return 0
}

// RINEXSig returns the RINEX two-character signal identifier for the RTCM
// MSM (gnss, sigID) pair, or "" if the pair is not recognized.
func RINEXSig(gnss GNSS, sigID uint8) string {
	return rinexSigMap[gnss][sigID]
}

// rinexSigMap maps RTCM MSM signal IDs to RINEX signal identifiers.
var rinexSigMap = map[GNSS]map[uint8]string{
	GPS: {
		2:  "1C",
		3:  "1P",
		4:  "1W",
		8:  "2C",
		9:  "2P",
		10: "2W",
		15: "2S",
		16: "2L",
		17: "2X",
		22: "5I",
		23: "5Q",
		24: "5X",
		30: "1S",
		31: "1L",
		32: "1X",
	},
	GLONASS: {
		2: "1C",
		3: "1P",
		8: "2C",
		9: "2P",
	},
	GALILEO: {
		2:  "1C",
		3:  "1A",
		4:  "1B",
		5:  "1X",
		6:  "1Z",
		8:  "6C",
		9:  "6A",
		10: "6B",
		11: "6X",
		12: "6Z",
		14: "7I",
		15: "7Q",
		16: "7X",
		18: "8I",
		19: "8Q",
		20: "8X",
		22: "5I",
		23: "5Q",
		24: "5X",
	},
	SBAS: {
		2:  "1C",
		22: "5I",
		23: "5Q",
		24: "5X",
	},
	QZSS: {
		2:  "1C",
		9:  "6S",
		10: "6L",
		11: "6X",
		15: "2S",
		16: "2L",
		17: "2X",
		22: "5I",
		23: "5Q",
		24: "5X",
		30: "1S",
		31: "1L",
		32: "1X",
	},
	BEIDOU: {
		2:  "2I",
		3:  "2Q",
		4:  "2X",
		8:  "6I",
		9:  "6Q",
		10: "6X",
		14: "7I",
		15: "7Q",
		16: "7X",
		22: "5D",
		23: "5P",
		24: "5X",
		25: "7D",
		30: "1D",
		31: "1P",
		32: "1X",
	},
	IRNSS: {
		8:  "9A",
		22: "5A",
	},
}
