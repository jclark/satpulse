package ubx

import (
	"fmt"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

func satellitesNavSat(u *bin.NavSat) *gpsprot.SatellitesMsg {
	// This is the minimum quality signal we require to include in the SatellitesMsg.
	// With CodeLocked, we have a CN0 of > 0 and it's reasonably stable.
	const minQuality = bin.NavSatQualityCodeLocked
	svs := make([]gpsprot.SVInfo, 0, u.NumSVs)
	for _, usv := range u.SVs {
		if usv.Flags&bin.NavSatQuality < minQuality {
			continue
		}
		svid := gnssSVID(usv.GNSSID, usv.SVID)
		svs = append(svs, gpsprot.SVInfo{
			ID: svid,
			Signals: []gpsprot.SignalInfo{
				{CN0: usv.CNO},
			},
			Azimuth:   usv.Azim,
			Elevation: usv.Elev,
			Used:      usv.Flags&bin.NavSatSVUsed != 0,
		})
	}
	return &gpsprot.SatellitesMsg{
		SVs:         svs,
		Tag:         Tag,
		NativeMsgID: "UBX-NAV-SAT",
		UsedValid:   true,
	}
}

func satellitesAddNavSig(sats *gpsprot.SatellitesMsg, u *bin.NavSig) {
	sigs := make(map[gpsprot.SVID][]gpsprot.SignalInfo)
	sigUsed := make(map[gpsprot.SVID]struct{})
	const minQuality = bin.NavSigQualityCodeLocked
	for _, usig := range u.Signals {
		if usig.QualityInd < minQuality {
			continue
		}
		svid := gnssSVID(usig.GNSSID, usig.SVID)
		sigs[svid] = append(sigs[svid], gpsprot.SignalInfo{
			ID:  signalID(usig.GNSSID, usig.SigID),
			CN0: usig.CNO,
		})
		if usig.SigFlags&bin.NavSigPrUsed != 0 {
			sigUsed[svid] = struct{}{}
		}
	}
	for i := range sats.SVs {
		sv := &sats.SVs[i]
		svSigs := sigs[sv.ID]
		if len(svSigs) == 0 {
			continue
		}
		// I think the used we get from NavSat is L1 only, but I think the right used semantic is any signal is used
		sv.Signals = svSigs
		if _, ok := sigUsed[sv.ID]; ok {
			sv.Used = true
		}
	}
}

// sigIDMap maps GNSSID and SigID to a SignalID.
var sigIDMap = map[bin.GNSSID]map[byte]gpsprot.SignalID{
	bin.GPS: {
		0: gpsprot.SigIDGPSL1CA, // L1 C/A
		3: gpsprot.SigIDGPSL2CL, // L2 CL
		4: gpsprot.SigIDGPSL2CM, // L2 CM
		6: gpsprot.SigIDGPSL5I,  // L5 I
		7: gpsprot.SigIDGPSL5Q,  // L5 Q
	},
	bin.SBAS: {
		0: gpsprot.SigIDGPSL1CA, // L1 C/A (uses GPS L1 C/A)
	},
	bin.GAL: {
		0:  gpsprot.SigIDGALE1C,  // E1 C
		1:  gpsprot.SigIDGALE1B,  // E1 B
		3:  gpsprot.SigIDGALE5aI, // E5a I
		4:  gpsprot.SigIDGALE5aQ, // E5a Q
		5:  gpsprot.SigIDGALE5bI, // E5b I
		6:  gpsprot.SigIDGALE5bQ, // E5b Q
		8:  gpsprot.SigIDGALE6B,  // E6 B
		9:  gpsprot.SigIDGALE6C,  // E6 C
		10: gpsprot.SigIDGALE6A,  // E6 A
	},
	bin.BDS: {
		0:  gpsprot.SigIDBDSB1ID1, // B1I D1
		1:  gpsprot.SigIDBDSB1ID2, // B1I D2
		2:  gpsprot.SigIDBDSB2ID1, // B2I D1
		3:  gpsprot.SigIDBDSB2ID2, // B2I D2
		4:  gpsprot.SigIDBDSB3ID1, // B3I D1
		5:  gpsprot.SigIDBDSB1CP,  // B1C pilot
		6:  gpsprot.SigIDBDSB1CD,  // B1C data
		7:  gpsprot.SigIDBDSB2aP,  // B2a pilot
		8:  gpsprot.SigIDBDSB2aD,  // B2a data
		10: gpsprot.SigIDBDSB3ID2, // B3I D2
	},
	bin.QZSS: {
		0:  gpsprot.SigIDQZSSL1CA, // L1 C/A
		1:  gpsprot.SigIDQZSSL1S,  // L1S
		4:  gpsprot.SigIDQZSSL2CM, // L2 CM
		5:  gpsprot.SigIDQZSSL2CL, // L2 CL
		8:  gpsprot.SigIDQZSSL5I,  // L5 I
		9:  gpsprot.SigIDQZSSL5Q,  // L5 Q
		12: gpsprot.SigIDQZSSL1CB, // L1C B
	},
	bin.GLO: {
		0: gpsprot.SigIDGLOL1, // L1 OF
		2: gpsprot.SigIDGLOL2, // L2 OF
	},
	bin.NavIC: {
		0: gpsprot.SigIDNAVICL5, // L5 A
	},
}

func signalID(gnssID bin.GNSSID, sigID byte) gpsprot.SignalID {
	if sigMap, ok := sigIDMap[gnssID]; ok {
		if signalID, ok := sigMap[sigID]; ok {
			return signalID
		}
	}
	// Return a fallback signal ID if not found
	return gpsprot.SignalID(fmt.Sprintf("UBX%d", sigID))
}

func satellitesNavSVInfo(u *bin.NavSVInfo) *gpsprot.SatellitesMsg {
	const minQuality = bin.NavSVInfoQualityCodeLockOnSignal
	svs := make([]gpsprot.SVInfo, 0, u.NumCh)
	for _, usv := range u.SVs {
		if usv.Quality&bin.NavSVInfoQualityInd < minQuality {
			continue
		}
		svs = append(svs, gpsprot.SVInfo{
			ID: svInfoSVID(usv.SVID),
			Signals: []gpsprot.SignalInfo{
				{CN0: usv.CNO},
			},
			Azimuth:   usv.Azim,
			Elevation: usv.Elev,
			Used:      usv.Flags&bin.NavSVInfoSVUsed != 0,
		})
	}
	return &gpsprot.SatellitesMsg{
		SVs:         svs,
		Tag:         Tag,
		NativeMsgID: "UBX-NAV-SVINFO",
		UsedValid:   true,
	}
}

func gnssSVID(gnss bin.GNSSID, uSVID byte) gpsprot.SVID {
	switch gnss {
	case bin.SBAS:
		// UBX SVIDs for SBAS are the PRN, which start at 120.
		if uSVID >= 120 {
			// RINEX (which we follow) uses PRN - 100 for SBAS (to keep numbers to two digits)
			uSVID -= 100
		}
	case bin.GLO:
		if uSVID == 255 {
			// UBX uses 255 for unknown GLONASS SVID (NMEA uses null)
			uSVID = gpsprot.GLOUnknown
		}
	}	
	return gpsprot.SVID{GNSS: idToGNSS(gnss), Num: uSVID}
}

func svInfoSVID(uSVID byte) gpsprot.SVID {
	g := gpsprot.GPS
	num := uSVID
	if uSVID >= 120 && uSVID <= 158 {
		g = gpsprot.SBAS
		num -= 100 // RINEX 3.04 uses PRN - 120 for SBAS (to keep numbers to two digits)
	} else if uSVID >= 193 && uSVID <= 197 {
		g = gpsprot.QZSS
		num -= 192
	} else if uSVID >= 65 && uSVID <= 96 {
		g = gpsprot.GLO
		num -= 64
	} else if uSVID == 255 {
		g = gpsprot.GLO
		num = gpsprot.GLOUnknown
	}
	return gpsprot.SVID{GNSS: g, Num: num}
}
