package novmsg

import "fmt"

// prnRange is the range of RANGE PRNs for a satellite system, and the
// offset to subtract to get the RINEX satellite number.
type prnRange struct {
	first, last, offset uint16
}

// RangeRINEX maps the satellite system, PRN and signal type of a RANGE
// record to a RINEX satellite, such as "C01", a RINEX signal code, such as
// "2I", and the phase shift, in cycles, to add to the carrier phase (the
// negated ADR) to align it as RINEX requires. It uses the OEM7 numbering
// (Tables 12, 164 and 167 of the OEM7 Commands and Logs Reference Manual),
// which applies no phase shifts. It returns an empty satellite for a record
// it has no mapping for.
func RangeRINEX(sys SatSystem, prn uint16, sigType uint8) (sat, sig string, phaseShift float64) {
	return rangeRINEX(sys, prn, prnRanges[sys], obsSigMap[sys][sigType], 0)
}

// SinoRangeRINEX is RangeRINEX for SinoGNSS receivers: the OEM7 numbering
// with the SinoGNSS differences overriding it.
func SinoRangeRINEX(sys SatSystem, prn uint16, sigType uint8) (sat, sig string, phaseShift float64) {
	r, ok := sinoPRNRanges[sys]
	if !ok {
		r = prnRanges[sys]
	}
	sig = sinoObsSigMap[sys][sigType]
	if sig == "" {
		sig = obsSigMap[sys][sigType]
	}
	return rangeRINEX(sys, prn, r, sig, sinoPhaseShift[sys][sigType])
}

func rangeRINEX(sys SatSystem, prn uint16, r prnRange, sig string, phaseShift float64) (string, string, float64) {
	letter := rinexSys(sys)
	if letter == "" || sig == "" || r.first == 0 || prn < r.first || prn > r.last {
		return "", "", 0
	}
	return fmt.Sprintf("%s%02d", letter, prn-r.offset), sig, phaseShift
}

// rinexSys returns the RINEX satellite system letter for a RANGE satellite
// system. It returns "" for SatSystemOther and unknown values.
func rinexSys(sys SatSystem) string {
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

// prnRanges is OEM7 Table 12. QZSS L1S (SBAS PRN 183-191) is left out,
// since RINEX gives it the QZSS satellite numbers.
var prnRanges = map[SatSystem]prnRange{
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

// obsSigMap is OEM7 Tables 164 and 167.
var obsSigMap = map[SatSystem]map[uint8]string{
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

// sinoPhaseShift has the SinoGNSS phase shifts. On the same satellite, a
// K901's L1C phase is a quarter cycle behind its L1C/A phase, for GPS and
// QZSS, where RINEX requires L1L to be aligned to L1C; its other signals
// agree with its MSM7, whose phase RTCM requires to be aligned.
var sinoPhaseShift = map[SatSystem]map[uint8]float64{
	SatSystemGPS:  {16: 0.25},
	SatSystemQZSS: {16: 0.25},
}
