package novmsg

// RangeMapping selects how RANGE satellite numbers and signal types map to
// RINEX. RANGE has the same layout for OEM7 and SinoGNSS, but SinoGNSS
// numbers QZSS, BDS and NavIC satellites differently and has signal types
// that OEM7 does not.
type RangeMapping uint8

// RANGE mappings.
const (
	// RangeMappingOEM7 is the mapping documented for OEM7 (Tables 12, 164
	// and 167 of the OEM7 Commands and Logs Reference Manual).
	RangeMappingOEM7 RangeMapping = iota
	// RangeMappingSinoGNSS is the SinoGNSS mapping: the OEM7 mapping with
	// the SinoGNSS differences overriding it.
	RangeMappingSinoGNSS
)

// RINEXSys returns the RINEX satellite system letter for a RANGE satellite
// system. It returns "" for SatSystemOther and unknown values.
func RINEXSys(sys SatSystem) string {
	switch sys {
	case SatSystemGPS:
		return "G"
	case SatSystemGLONASS:
		return "R"
	case SatSystemSBAS:
		return "S"
	case SatSystemGalileo:
		return "E"
	case SatSystemBeiDou:
		return "C"
	case SatSystemQZSS:
		return "J"
	case SatSystemNavIC:
		return "I"
	}
	return ""
}

// RINEXSatNum converts a RANGE PRN/slot to a RINEX satellite number. It
// returns 0 for a PRN outside the mapping's range for the system.
func (m RangeMapping) RINEXSatNum(sys SatSystem, prn uint16) uint8 {
	r, ok := sinoPRNRanges[sys]
	if !ok || m != RangeMappingSinoGNSS {
		r = oem7PRNRanges[sys]
	}
	if r.first == 0 || prn < r.first || prn > r.last {
		return 0
	}
	return uint8(prn - r.offset)
}

// prnRange is the range of RANGE PRNs for a satellite system, and the
// offset to subtract to get the RINEX satellite number.
type prnRange struct {
	first, last, offset uint16
}

// oem7PRNRanges is OEM7 Table 12. QZSS L1S (SBAS PRN 183-191) is left out,
// since RINEX gives it the QZSS satellite numbers.
var oem7PRNRanges = map[SatSystem]prnRange{
	SatSystemGPS:     {1, 32, 0},
	SatSystemSBAS:    {120, 158, 100},
	SatSystemGLONASS: {38, 61, 37},
	SatSystemGalileo: {1, 36, 0},
	SatSystemQZSS:    {193, 202, 192},
	SatSystemBeiDou:  {1, 63, 0},
	SatSystemNavIC:   {1, 14, 0},
}

// sinoPRNRanges are the SinoGNSS RANGE PRNs that differ from OEM7, from
// Table 2-2 of the SinoGNSS K8/K9 manual. The other SinoGNSS logs number
// Galileo 71-106 and SBAS 220-238, but RANGE uses the OEM7 numbers.
var sinoPRNRanges = map[SatSystem]prnRange{
	SatSystemQZSS:   {131, 140, 130},
	SatSystemBeiDou: {141, 203, 140},
	SatSystemNavIC:  {62, 70, 61},
}

// RINEXObsSig returns the RINEX two-character signal identifier for a
// RANGE satellite system and signal type. It returns "" for a combination
// the mapping does not have.
func (m RangeMapping) RINEXObsSig(sys SatSystem, sigType uint8) string {
	if m == RangeMappingSinoGNSS {
		if s := sinoObsSigMap[sys][sigType]; s != "" {
			return s
		}
	}
	return oem7ObsSigMap[sys][sigType]
}

// oem7ObsSigMap is OEM7 Tables 164 and 167.
var oem7ObsSigMap = map[SatSystem]map[uint8]string{
	SatSystemGPS: {
		0:  "1C", // L1C/A
		5:  "2P", // L2P
		9:  "2W", // L2P(Y), semi-codeless
		14: "5Q", // L5(Q)
		16: "1L", // L1C(P)
		17: "2S", // L2C(M)
	},
	SatSystemGLONASS: {
		0: "1C", // L1C/A
		1: "2C", // L2C/A
		5: "2P", // L2P
		6: "3Q", // L3(Q)
	},
	SatSystemGalileo: {
		2:  "1C", // E1(C)
		6:  "6B", // E6B
		7:  "6C", // E6C
		12: "5Q", // E5a(Q)
		17: "7Q", // E5b(Q)
		20: "8Q", // E5AltBOC(Q)
	},
	SatSystemBeiDou: {
		0:  "2I", // B1(I) with D1 data
		1:  "7I", // B2(I) with D1 data
		2:  "6I", // B3(I) with D1 data
		4:  "2I", // B1(I) with D2 data
		5:  "7I", // B2(I) with D2 data
		6:  "6I", // B3(I) with D2 data
		7:  "1P", // B1C(P)
		9:  "5P", // B2a(P)
		11: "7D", // B2b(I)
	},
	SatSystemQZSS: {
		0:  "1C", // L1C/A
		14: "5Q", // L5(Q)
		16: "1L", // L1C(P)
		17: "2S", // L2C(M)
		27: "6L", // L6P
	},
	SatSystemSBAS: {
		0: "1C", // L1C/A
		6: "5I", // L5(I)
	},
	SatSystemNavIC: {
		0: "5A", // L5 SPS
	},
}

// sinoObsSigMap has the SinoGNSS signal types that differ from OEM7. The
// SinoGNSS manual lacks the signal type table, so these were identified
// from K901 and K803 captures, by the carrier frequency given by psr/|adr|
// and by comparison with NMEA 4.11 GSV signal IDs. SinoGNSS tracks L2C as
// M+L, where OEM7 tracks M: its MSM7 labels L2C 2X, and its L2C C/N0 is
// 3 dB higher relative to L1 C/A than that of a receiver tracking L alone.
// The other RINEX attributes are those of the OEM7 signal of the same
// name. SinoGNSS reports BDS GEO satellites with the D1 signal types.
var sinoObsSigMap = map[SatSystem]map[uint8]string{
	SatSystemGPS: {
		2:  "5Q", // L5
		17: "2X", // L2C(M+L)
	},
	SatSystemQZSS: {
		17: "2X", // L2C(M+L)
	},
	SatSystemBeiDou: {
		8:  "1P", // B1C
		12: "5P", // B2a
		17: "7I", // B2I (BDS-2)
		19: "7D", // B2b (BDS-3)
	},
}

// RINEXPhaseShift returns the phase shift, in cycles, to add to the carrier
// phase (the negated ADR) of a RANGE signal to align it with the reference
// signal of its frequency band, as RINEX requires (RINEX 4.02 Table A45).
func (m RangeMapping) RINEXPhaseShift(sys SatSystem, sigType uint8) float64 {
	if m == RangeMappingSinoGNSS {
		return sinoPhaseShift[sys][sigType]
	}
	return 0
}

// sinoPhaseShift has the SinoGNSS phase shifts. On the same satellite, a
// K901's L1C phase is a quarter cycle behind its L1C/A phase, for GPS and
// QZSS, where RINEX requires L1L to be aligned to L1C; its other signals
// agree with its MSM7, whose phase RTCM requires to be aligned.
var sinoPhaseShift = map[SatSystem]map[uint8]float64{
	SatSystemGPS:  {16: 0.25},
	SatSystemQZSS: {16: 0.25},
}
