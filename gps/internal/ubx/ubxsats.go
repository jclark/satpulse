package ubx

import (
	"fmt"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func satellitesNavSat(u *ubxbin.NavSat) *gpsprot.SatellitesMsg {
	// This is the minimum quality signal we require to include in the SatellitesMsg.
	// With CodeLocked, we have a CN0 of > 0 and it's reasonably stable.
	const minQuality = ubxbin.NavSatQualityCodeLocked
	svs := make([]gpsprot.SVInfo, 0, u.NumSVs)
	for _, usv := range u.SVs {
		if usv.Flags&ubxbin.NavSatQuality < minQuality {
			continue
		}
		svid := gnssSVID(usv.GNSSID, usv.SVID)
		used := usv.Flags&ubxbin.NavSatSVUsed != 0
		svs = append(svs, gpsprot.SVInfo{
			ID: svid,
			Signals: []gpsprot.SignalInfo{
				{
					ID:   navSatSignalId(usv.GNSSID),
					CN0:  usv.CNO,
					Used: used,
				},
			},
			LookAngles: &gpsprot.LookAngles{
				Azimuth:   usv.Azim,
				Elevation: usv.Elev,
			},
			Used: used,
		})
	}
	return &gpsprot.SatellitesMsg{
		SVs:         svs,
		Tag:         Tag,
		NativeMsgID: "NAV-SAT",
		// Although we are told only that the satellite is used, we only have one signal,
		// so that signal is used iff the satellite is used.
		// This is different from the NMEA case, where we have multiple signals,
		// but know only where the satellite is used.
		UsedValidity: gpsprot.SatelliteUsedSignal,
	}
}

// navSatSigIDMap maps GNSSID to a SignalID for the signal level reported in a UBX-NAV-SAT message.
// It turns out UBX-NAV-SAT reports the signal level for the L1 signal.
// Not entirely clear which L1 signal we should report, but we opt for the legacy one.
// We try to be not too specific here, so we use E1 rather than E1-C (the pilot signal, which is what UBX-NAV-SIG reports),
// and B1I rather than B1I D1 (since there's a chance that it is D2).
var navSatSigIDMap = map[ubxbin.GNSSID]gpsprot.SignalID{
	ubxbin.GPS:  gpsprot.SigIDGPSL1CA,
	ubxbin.SBAS: gpsprot.SigIDGPSL1CA,
	ubxbin.GAL:  gpsprot.SigIDGALE1,
	ubxbin.BDS:  gpsprot.SigIDBDSB1I,
	ubxbin.QZSS: gpsprot.SigIDQZSSL1CA,
	ubxbin.GLO:  gpsprot.SigIDGLOL1,
}

// navSatSignalId returns the SignalID for the signal level reported in a UBX-NAV-SAT message.
func navSatSignalId(gnssID ubxbin.GNSSID) gpsprot.SignalID {
	if sigID, ok := navSatSigIDMap[gnssID]; ok {
		return sigID
	}
	return gpsprot.SigIDInvalid
}

// svInfoSignalID returns the L1 SignalID for NAV-SVINFO, which reports one signal per SV.
func svInfoSignalID(gnss gpsprot.GNSS) gpsprot.SignalID {
	switch gnss {
	case gpsprot.GPS, gpsprot.SBAS:
		return gpsprot.SigIDGPSL1CA
	case gpsprot.GAL:
		return gpsprot.SigIDGALE1
	case gpsprot.BDS:
		return gpsprot.SigIDBDSB1I
	case gpsprot.QZSS:
		return gpsprot.SigIDQZSSL1CA
	case gpsprot.GLO:
		return gpsprot.SigIDGLOL1
	}
	return gpsprot.SigIDInvalid
}

func satellitesNavSig(u *ubxbin.NavSig) *gpsprot.SatellitesMsg {
	sigIndex := make(map[gpsprot.SVID]int)
	const minQuality = ubxbin.NavSigQualityCodeLocked
	svs := make([]gpsprot.SVInfo, 0)
	for _, usig := range u.Signals {
		if usig.QualityInd < minQuality {
			continue
		}
		svid := gnssSVID(usig.GNSSID, usig.SVID)
		i, ok := sigIndex[svid]
		if !ok {
			i = len(svs)
			svs = append(svs, gpsprot.SVInfo{ID: svid})
			sigIndex[svid] = i
		}
		sigs := &svs[i].Signals
		*sigs = append(*sigs, gpsprot.SignalInfo{
			ID:   signalID(usig.GNSSID, usig.SigID),
			CN0:  usig.CNO,
			Used: usig.SigFlags&ubxbin.NavSigPrUsed != 0,
		})
	}
	for i := range svs {
		sv := &svs[i]
		for _, sig := range sv.Signals {
			if sig.Used {
				sv.Used = true
				break
			}
		}
	}
	return &gpsprot.SatellitesMsg{
		SVs:          svs,
		Tag:          Tag,
		NativeMsgID:  "NAV-SIG",
		UsedValidity: gpsprot.SatelliteUsedSignal,
	}
}

// satellitesCombine combines two SatellitesMsg one from UBX-NAV-SAT and one from UBX-NAV-SIG.
// It replaces the signals in UBX-NAV-SAT with the signals from UBX-NAV-SIG.
func satellitesCombine(sats, sigs *gpsprot.SatellitesMsg) *gpsprot.SatellitesMsg {
	if sats == nil {
		return sigs
	}
	if sigs == nil {
		return sats
	}
	sats = satellitesCopy(sats)
	svIndex := make(map[gpsprot.SVID]int)
	for i, sv := range sats.SVs {
		svIndex[sv.ID] = i
	}
	for _, sv := range sigs.SVs {
		if i, ok := svIndex[sv.ID]; ok {
			// replace the signals and Used flag from UBX-NAV-SAT with those from UBX-NAV-SIG
			sats.SVs[i].Signals = sv.Signals
			sats.SVs[i].Used = sv.Used
		} else {
			svIndex[sv.ID] = len(sats.SVs) - 1
			sats.SVs = append(sats.SVs, sv)
		}
	}
	return sats
}

// satellitesCopy creates a deep copy of the SatellitesMsg.
// The slices are both copied, but the LookAngles is not.
func satellitesCopy(sats *gpsprot.SatellitesMsg) *gpsprot.SatellitesMsg {
	if sats == nil {
		return nil
	}
	copied := *sats
	copied.SVs = make([]gpsprot.SVInfo, len(sats.SVs))
	copy(copied.SVs, sats.SVs)
	for i := range sats.SVs {
		copiedSV := &copied.SVs[i]
		copiedSV.Signals = make([]gpsprot.SignalInfo, len(copiedSV.Signals))
		copy(copiedSV.Signals, sats.SVs[i].Signals)
	}
	return &copied
}

// sigIDMap maps GNSSID and SigID to a SignalID.
var sigIDMap = map[ubxbin.GNSSID]map[byte]gpsprot.SignalID{
	ubxbin.GPS: {
		0: gpsprot.SigIDGPSL1CA, // L1 C/A
		3: gpsprot.SigIDGPSL2CL, // L2 CL
		4: gpsprot.SigIDGPSL2CM, // L2 CM
		6: gpsprot.SigIDGPSL5I,  // L5 I
		7: gpsprot.SigIDGPSL5Q,  // L5 Q
	},
	ubxbin.SBAS: {
		0: gpsprot.SigIDGPSL1CA, // L1 C/A (uses GPS L1 C/A)
	},
	ubxbin.GAL: {
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
	ubxbin.BDS: {
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
	ubxbin.QZSS: {
		0:  gpsprot.SigIDQZSSL1CA, // L1 C/A
		1:  gpsprot.SigIDQZSSL1S,  // L1S
		4:  gpsprot.SigIDQZSSL2CM, // L2 CM
		5:  gpsprot.SigIDQZSSL2CL, // L2 CL
		8:  gpsprot.SigIDQZSSL5I,  // L5 I
		9:  gpsprot.SigIDQZSSL5Q,  // L5 Q
		12: gpsprot.SigIDQZSSL1CB, // L1C B
	},
	ubxbin.GLO: {
		0: gpsprot.SigIDGLOL1, // L1 OF
		2: gpsprot.SigIDGLOL2, // L2 OF
	},
	ubxbin.NavIC: {
		0: gpsprot.SigIDNAVICL5, // L5 A
	},
}

func signalID(gnssID ubxbin.GNSSID, sigID byte) gpsprot.SignalID {
	if sigMap, ok := sigIDMap[gnssID]; ok {
		if signalID, ok := sigMap[sigID]; ok {
			return signalID
		}
	}
	// Return a fallback signal ID if not found
	return gpsprot.SignalID(fmt.Sprintf("UBX%d", sigID))
}

// corrKindNavSig accumulates CorrKind bits from used signals in a NAV-SIG message.
func corrKindNavSig(u *ubxbin.NavSig) gpsprot.CorrKind {
	var corr gpsprot.CorrKind
	for _, s := range u.Signals {
		if s.SigFlags&ubxbin.NavSigPrUsed == 0 {
			continue
		}
		corr |= corrSourceKind(s.CorrSource)
	}
	return corrKindResolve(corr)
}

// corrKindNavSat accumulates CorrKind bits from used SVs in a NAV-SAT message.
func corrKindNavSat(u *ubxbin.NavSat) gpsprot.CorrKind {
	var corr gpsprot.CorrKind
	for _, sv := range u.SVs {
		if sv.Flags&ubxbin.NavSatSVUsed == 0 {
			continue
		}
		if sv.Flags&ubxbin.NavSatSbasCorrUsed != 0 {
			corr |= gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSBAS
		}
		if sv.Flags&ubxbin.NavSatRtcmCorrUsed != 0 {
			corr |= gpsprot.CorrUsed | gpsprot.CorrRTCM
		}
		if sv.Flags&ubxbin.NavSatSlasCorrUsed != 0 {
			corr |= gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS
		}
		if sv.Flags&ubxbin.NavSatSpartnCorrUsed != 0 {
			corr |= gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSPARTN
		}
		if sv.Flags&ubxbin.NavSatClasCorrUsed != 0 {
			corr |= gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS
		}
		if sv.Flags&ubxbin.NavSatLppCorrUsed != 0 {
			corr |= gpsprot.CorrUsed
		}
		if sv.Flags&ubxbin.NavSatHasCorrUsed != 0 {
			corr |= gpsprot.CorrUsed | gpsprot.CorrSSR
		}
	}
	return corrKindResolve(corr)
}

func corrSourceKind(src ubxbin.NavSigCorrSource) gpsprot.CorrKind {
	switch src {
	case ubxbin.NavSigCorrSourceSBAS:
		return gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSBAS
	case ubxbin.NavSigCorrSourceBeiDou:
		return gpsprot.CorrUsed | gpsprot.CorrSSR
	case ubxbin.NavSigCorrSourceRTCM2:
		return gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrOSR
	case ubxbin.NavSigCorrSourceRTCM3OSR:
		return gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrOSR
	case ubxbin.NavSigCorrSourceRTCM3SSR:
		return gpsprot.CorrUsed | gpsprot.CorrRTCM | gpsprot.CorrSSR
	case ubxbin.NavSigCorrSourceQZSSSLAS:
		return gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS
	case ubxbin.NavSigCorrSourceSPARTN:
		return gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrSPARTN
	case ubxbin.NavSigCorrSourceCLAS:
		return gpsprot.CorrUsed | gpsprot.CorrSSR | gpsprot.CorrCLAS
	case ubxbin.NavSigCorrSourceLPPOSR:
		return gpsprot.CorrUsed | gpsprot.CorrOSR
	case ubxbin.NavSigCorrSourceLPPSSR:
		return gpsprot.CorrUsed | gpsprot.CorrSSR
	case ubxbin.NavSigCorrSourceGALHAS:
		return gpsprot.CorrUsed | gpsprot.CorrSSR
	default:
		return 0
	}
}

// corrKindResolve applies the conflict rule: if both base-station and
// wide-area correction sources appear, keep only CorrUsed and CorrRTCM.
func corrKindResolve(corr gpsprot.CorrKind) gpsprot.CorrKind {
	if corr&gpsprot.CorrOSR != 0 && corr&gpsprot.CorrSSR != 0 {
		corr = (corr & gpsprot.CorrUsed) | (corr & gpsprot.CorrRTCM)
	}
	return corr
}

func satellitesNavSVInfo(u *ubxbin.NavSVInfo) *gpsprot.SatellitesMsg {
	const minQuality = ubxbin.NavSVInfoQualityCodeLockOnSignal
	svs := make([]gpsprot.SVInfo, 0, u.NumCh)
	for _, usv := range u.SVs {
		if usv.Quality&ubxbin.NavSVInfoQualityInd < minQuality {
			continue
		}
		svid := svInfoSVID(usv.SVID)
		used := usv.Flags&ubxbin.NavSVInfoSVUsed != 0
		svs = append(svs, gpsprot.SVInfo{
			ID: svid,
			Signals: []gpsprot.SignalInfo{{
				ID:   svInfoSignalID(svid.GNSS),
				CN0:  usv.CNO,
				Used: used,
			}},
			LookAngles: &gpsprot.LookAngles{
				Azimuth:   usv.Azim,
				Elevation: usv.Elev,
			},
			Used: used,
		})
	}
	return &gpsprot.SatellitesMsg{
		SVs:          svs,
		Tag:          Tag,
		NativeMsgID:  "NAV-SVINFO",
		UsedValidity: gpsprot.SatelliteUsedSignal,
	}
}

func gnssSVID(gnss ubxbin.GNSSID, uSVID byte) gpsprot.SVID {
	switch gnss {
	case ubxbin.SBAS:
		// UBX SVIDs for SBAS are the PRN, which start at 120.
		if uSVID >= 120 {
			// RINEX (which we follow) uses PRN - 100 for SBAS (to keep numbers to two digits)
			uSVID -= 100
		}
	case ubxbin.GLO:
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
