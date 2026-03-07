package unc

import (
	"fmt"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

// satellitesMsgFromSatsInfo converts a Unicore SATSINFO message to a gpsprot.SatellitesMsg
func satellitesMsgFromSatsInfo(_ *uncmsg.MsgHdr, satsInfo *uncmsg.SatsInfo, tag gpsprot.Tag) (*gpsprot.SatellitesMsg, error) {
	svs := make([]gpsprot.SVInfo, 0, len(satsInfo.Sats))
	for _, sat := range satsInfo.Sats {
		// SATSINFO guarantees that every satellite has at least one frequency.
		// Get the satellite GNSS system from the the first frequency (they will all be the same)
		sysID := sat.Freqs[0].SysStatus
		if sysID == uncmsg.SysNAVIC {
			// firmware at least up to build 17548 returns nonsense PRNs for NavIC, so skip 
			continue
		}
		svid, err := sysPRNToSVID(sysID, sat.PRN)
		if err != nil {
			return nil, err
		}

		signals := make([]gpsprot.SignalInfo, 0, len(sat.Freqs))
		for _, freq := range sat.Freqs {
			sigID := sysFreqToSignalID(freq.SysStatus, freq.FreqStatus)
			if sigID != gpsprot.SigIDInvalid {
				signals = append(signals, gpsprot.SignalInfo{
					ID:  sigID,
					CN0: freq.SNR,
				})
			}
		}
		svs = append(svs, gpsprot.SVInfo{
			ID:      svid,
			Signals: signals,
			LookAngles: &gpsprot.LookAngles{
				Azimuth:   int16(sat.Azimuth),
				Elevation: int8(sat.Elevation),
			},
		})
	}

	return &gpsprot.SatellitesMsg{
		SVs:          svs,
		Tag:          tag,
		NativeMsgID:  "SATSINFO",
		UsedValidity: gpsprot.SatelliteUsedInvalid, // SATSINFO doesn't provide usage information
	}, nil
}

// sysPRNToSVID converts Unicore SysID and PRN to gpsprot.SVID
func sysPRNToSVID(sysID uncmsg.SysID, prn byte) (gpsprot.SVID, error) {
	gnss := sysIDToGNSS(sysID)
	if gnss == 0 {
		return gpsprot.SVID{}, fmt.Errorf("unknown GNSS system: %d", sysID)
	}

	// Convert Unicore PRN to RINEX 3.04 format used by gpsprot.SVID
	num, ok := prnToNum(gnss, prn)
	if !ok {
		return gpsprot.SVID{}, fmt.Errorf("invalid PRN %d for GNSS %s", prn, gnss)
	}

	return gpsprot.SVID{
		GNSS: gnss,
		Num:  num,
	}, nil
}

// prnToNum converts Unicore PRN numbers to RINEX 3.04 numbers
// Based on Table 7-54 in protocol.md and SVID comment in gpsprot/msg.go
func prnToNum(gnss gpsprot.GNSS, prn byte) (byte, bool) {
	num := int(prn)
	switch gnss {
	case gpsprot.GLO:
		// GLONASS: PRN 38-61 → SVID = slot number (need mapping)
		// Unicore uses PRN 38-61, but RINEX uses slot number 1-24
		num -= 37
	case gpsprot.SBAS:
		// SBAS: PRN 120-158 → SVID = PRN - 100
		num -= 100
	case gpsprot.QZSS:
		// QZSS: PRN 193-202 → SVID = PRN - 192
		num -= 192
	}
	if gnss.IsValidSVNum(num) {
		return byte(num), true
	}
	return 0, false
}

// sysIDToGNSS maps Unicore SysID value to gpsprot.GNSS
// Based on Table 7-112 in protocol.md
func sysIDToGNSS(sysID uncmsg.SysID) gpsprot.GNSS {
	switch sysID {
	case uncmsg.SysGPS:
		return gpsprot.GPS
	case uncmsg.SysGLO:
		return gpsprot.GLO
	case uncmsg.SysSBAS:
		return gpsprot.SBAS
	case uncmsg.SysGAL:
		return gpsprot.GAL
	case uncmsg.SysBDS:
		return gpsprot.BDS
	case uncmsg.SysQZSS:
		return gpsprot.QZSS
	case uncmsg.SysNAVIC:
		return gpsprot.NAVIC
	default:
		return 0 // Invalid
	}
}

// freqKey represents a GNSS system and frequency identifier combination
type freqKey struct {
	gnss   gpsprot.GNSS
	freqID uncmsg.FreqID
}

// freqMap maps freqKey to gpsprot.SignalID
// Based on Table 7-113 in protocol.md
var freqMap = map[freqKey]gpsprot.SignalID{
	// GPS signals
	{gpsprot.GPS, uncmsg.FreqGPSL1CA}: gpsprot.SigIDGPSL1CA,
	{gpsprot.GPS, uncmsg.FreqGPSL2P}:  gpsprot.SigIDGPSL2P,
	{gpsprot.GPS, uncmsg.FreqGPSL1CP}: gpsprot.SigIDGPSL1CP,
	{gpsprot.GPS, uncmsg.FreqGPSL1CD}: gpsprot.SigIDGPSL1CD,
	{gpsprot.GPS, uncmsg.FreqGPSL5D}:  gpsprot.SigIDGPSL5I, // L5 data uses L5I
	{gpsprot.GPS, uncmsg.FreqGPSL5P}:  gpsprot.SigIDGPSL5Q, // L5 pilot uses L5Q
	{gpsprot.GPS, uncmsg.FreqGPSL2CL}: gpsprot.SigIDGPSL2CL,

	// GLONASS signals
	{gpsprot.GLO, uncmsg.FreqGLOL1CA}: gpsprot.SigIDGLOL1,  // L1 C/A maps to L1
	{gpsprot.GLO, uncmsg.FreqGLOL2CA}: gpsprot.SigIDGLOL2,  // L2 C/A maps to L2
	{gpsprot.GLO, uncmsg.FreqGLOG3I}:  gpsprot.SigIDGLOL3I, // G3 more properly known as L1
	{gpsprot.GLO, uncmsg.FreqGLOG3Q}:  gpsprot.SigIDGLOL3Q,

	// Galileo signals
	{gpsprot.GAL, uncmsg.FreqGALE1B}:  gpsprot.SigIDGALE1B,
	{gpsprot.GAL, uncmsg.FreqGALE1C}:  gpsprot.SigIDGALE1C,
	{gpsprot.GAL, uncmsg.FreqGALE5AP}: gpsprot.SigIDGALE5aQ, // E5a pilot is E5a-Q
	{gpsprot.GAL, uncmsg.FreqGALE5BP}: gpsprot.SigIDGALE5bQ, // E5b pilot is E5b-Q
	{gpsprot.GAL, uncmsg.FreqGALE6B}:  gpsprot.SigIDGALE6B,
	{gpsprot.GAL, uncmsg.FreqGALE6C}:  gpsprot.SigIDGALE6C,

	// BeiDou signals
	{gpsprot.BDS, uncmsg.FreqBDSB1I}:  gpsprot.SigIDBDSB1I,
	{gpsprot.BDS, uncmsg.FreqBDSB1Q}:  gpsprot.SigIDBDSB1Q,
	{gpsprot.BDS, uncmsg.FreqBDSB1CP}: gpsprot.SigIDBDSB1CP,
	{gpsprot.BDS, uncmsg.FreqBDSB1CD}: gpsprot.SigIDBDSB1CD,
	{gpsprot.BDS, uncmsg.FreqBDSB2Q}:  gpsprot.SigIDBDSB2Q,
	{gpsprot.BDS, uncmsg.FreqBDSB2I}:  gpsprot.SigIDBDSB2I,
	{gpsprot.BDS, uncmsg.FreqBDSB2aP}: gpsprot.SigIDBDSB2aP,
	{gpsprot.BDS, uncmsg.FreqBDSB2aD}: gpsprot.SigIDBDSB2aD,
	{gpsprot.BDS, uncmsg.FreqBDSB3Q}:  gpsprot.SigIDBDSB3Q,
	{gpsprot.BDS, uncmsg.FreqBDSB3I}:  gpsprot.SigIDBDSB3I,
	{gpsprot.BDS, uncmsg.FreqBDSB2bI}: gpsprot.SigIDBDSB2bI,

	// QZSS signals
	{gpsprot.QZSS, uncmsg.FreqQZSSL1CA}: gpsprot.SigIDQZSSL1CA,
	{gpsprot.QZSS, uncmsg.FreqQZSSL1CB}: gpsprot.SigIDQZSSL1CB,
	{gpsprot.QZSS, uncmsg.FreqQZSSL1CP}: gpsprot.SigIDQZSSL1CP,
	{gpsprot.QZSS, uncmsg.FreqQZSSL1S}:  gpsprot.SigIDQZSSL1S,
	{gpsprot.QZSS, uncmsg.FreqQZSSL5D}:  gpsprot.SigIDQZSSL5I, // L5 data uses L5-I
	{gpsprot.QZSS, uncmsg.FreqQZSSL1CD}: gpsprot.SigIDQZSSL1CD,
	{gpsprot.QZSS, uncmsg.FreqQZSSL5P}:  gpsprot.SigIDQZSSL5Q, // L5 pilot uses L5-Q
	{gpsprot.QZSS, uncmsg.FreqQZSSL2CL}: gpsprot.SigIDQZSSL2CL,
	{gpsprot.QZSS, uncmsg.FreqQZSSL6D}:  gpsprot.SigIDQZSSL6, // gpsprot uses L6 to mean L6D
	{gpsprot.QZSS, uncmsg.FreqQZSSL6E}:  gpsprot.SigIDQZSSL6E,

	// SBAS signals
	{gpsprot.SBAS, uncmsg.FreqSBASL1CA}: gpsprot.SigIDGPSL1CA, // SBAS uses GPS signal IDs
	{gpsprot.SBAS, uncmsg.FreqSBASL5I}:  gpsprot.SigIDGPSL5I,  // SBAS uses GPS signal IDs

	// NavIC signals
	{gpsprot.NAVIC, uncmsg.FreqNAVICL5D}: gpsprot.SigIDNAVICL5I, // L5 data uses L5-I
	{gpsprot.NAVIC, uncmsg.FreqNAVICL5P}: gpsprot.SigIDNAVICL5Q, // L5 pilot uses L5-Q
}

// sysFreqToSignalID maps Unicore system and frequency identifiers to gpsprot.SignalID
// Based on Table 7-113 in protocol.md
func sysFreqToSignalID(sysStatus uncmsg.SysID, freqStatus uncmsg.FreqID) gpsprot.SignalID {
	gnss := sysIDToGNSS(sysStatus)
	if gnss == 0 {
		return gpsprot.SigIDInvalid
	}

	key := freqKey{gnss: gnss, freqID: freqStatus}
	if signalID, ok := freqMap[key]; ok {
		return signalID
	}
	return gpsprot.SigIDInvalid
}
