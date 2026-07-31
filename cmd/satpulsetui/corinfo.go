package main

import (
	"strconv"
	"strings"
)

// rtcmInfo returns the MSM label and description for an RTCM message
// type, mirroring the workbench's rtcm.ts tables.
func rtcmInfo(msgType string) (msm, desc string) {
	n, err := strconv.Atoi(msgType)
	if err != nil {
		return "", ""
	}
	msmBases := []struct {
		base int
		gnss string
	}{
		{1070, "GPS"}, {1080, "GLONASS"}, {1090, "Galileo"}, {1100, "SBAS"},
		{1110, "QZSS"}, {1120, "BeiDou"}, {1130, "NavIC"},
	}
	msmDesc := []string{
		1: "Compact {} Pseudoranges",
		2: "Compact {} PhaseRanges",
		3: "Compact {} Pseudoranges and PhaseRanges",
		4: "Full {} Pseudoranges and PhaseRanges plus CNR",
		5: "Full {} Pseudoranges, PhaseRanges, PhaseRangeRate and CNR",
		6: "Full {} Pseudoranges and PhaseRanges plus CNR (high resolution)",
		7: "Full {} Pseudoranges, PhaseRanges, PhaseRangeRate and CNR (high resolution)",
	}
	for _, b := range msmBases {
		if off := n - b.base; off >= 1 && off <= 7 {
			return "MSM" + strconv.Itoa(off), strings.Replace(msmDesc[off], "{}", b.gnss, 1)
		}
	}
	return "", rtcmDesc[n]
}

var rtcmDesc = map[int]string{
	1001: "GPS Basic RTK, L1 Only",
	1002: "GPS Extended RTK, L1 Only",
	1003: "GPS Basic RTK, L1 & L2",
	1004: "GPS Extended RTK, L1 & L2",
	1005: "Stationary Antenna Reference Point, No Height Information",
	1006: "Stationary Antenna Reference Point, with Height Information",
	1007: "Antenna Descriptor",
	1008: "Antenna Descriptor & Serial Number",
	1009: "GLONASS Basic RTK, L1 Only",
	1010: "GLONASS Extended RTK, L1 Only",
	1011: "GLONASS Basic RTK, L1 & L2",
	1012: "GLONASS Extended RTK, L1 & L2",
	1013: "System Parameters",
	1019: "GPS Satellite Ephemeris Data",
	1020: "GLONASS Satellite Ephemeris Data",
	1033: "Receiver and Antenna Descriptors",
	1041: "NavIC Satellite Ephemeris Data",
	1042: "BeiDou Satellite Ephemeris Data",
	1044: "QZSS Satellite Ephemeris Data",
	1045: "Galileo F/NAV Satellite Ephemeris Data",
	1046: "Galileo I/NAV Satellite Ephemeris Data",
	1230: "GLONASS L1 and L2 Code-Phase Biases",
}

// spartnInfo returns the description for a SPARTN "type.subtype"
// message ID, mirroring the workbench's spartn.ts.
func spartnInfo(msgType string) string {
	t, st, ok := strings.Cut(msgType, ".")
	if !ok {
		return ""
	}
	typ, err1 := strconv.Atoi(t)
	sub, err2 := strconv.Atoi(st)
	if err1 != nil || err2 != nil {
		return ""
	}
	gnss := map[int]string{0: "GPS", 1: "GLONASS", 2: "Galileo", 3: "BeiDou", 4: "QZSS"}
	switch typ {
	case 0:
		if g, ok := gnss[sub]; ok {
			return g + " Orbit, Clock, Bias (OCB)"
		}
		return "Orbit, Clock, Bias (OCB)"
	case 1:
		if g, ok := gnss[sub]; ok {
			return g + " High-precision Atmosphere (HPAC)"
		}
		return "High-precision Atmosphere (HPAC)"
	case 2:
		if sub == 0 {
			return "Geographic Area Definition (GAD)"
		}
	case 3:
		if sub == 0 {
			return "Basic-precision Atmosphere (BPAC)"
		}
	case 4:
		switch sub {
		case 0:
			return "Encryption/Authentication Support (EAS): Dynamic Key"
		case 1:
			return "Encryption/Authentication Support (EAS): Group Authentication"
		}
		return "Encryption/Authentication Support (EAS)"
	case 120:
		switch sub {
		case 0:
			return "Proprietary: In-house Test"
		case 1:
			return "Proprietary: u-blox AG"
		case 2:
			return "Proprietary: Swift Navigation"
		}
		return "Proprietary"
	}
	return ""
}
