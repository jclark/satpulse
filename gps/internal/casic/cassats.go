package casic

import (
	"time"

	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// casicSignalID maps CASIC GNSSID to the SignalID for the L1 legacy signal.
// CASIC reports only one CN0 per satellite (L1 legacy signal).
var casicSignalID = map[casbin.GNSSID]gpsprot.SignalID{
	casbin.GPS: gpsprot.SigIDGPSL1CA,
	casbin.BDS: gpsprot.SigIDBDSB1I,
	casbin.GLN: gpsprot.SigIDGLOL1,
}

// casicSVID converts CASIC GNSS ID and SVID to gpsprot.SVID.
// CASIC binary reports SBAS and QZSS in NAV-GPSINFO (system=GPS) using raw PRN numbers.
// We detect these by PRN range and convert to RINEX numbering (PRN-100 for SBAS, PRN-192 for QZSS).
func casicSVID(gnss gpsprot.GNSS, svid uint8) gpsprot.SVID {
	if gnss == gpsprot.GPS {
		if svid >= 120 && svid <= 158 {
			return gpsprot.SVID{GNSS: gpsprot.SBAS, Num: svid - 100}
		}
		if svid >= 193 && svid <= 199 {
			return gpsprot.SVID{GNSS: gpsprot.QZSS, Num: svid - 192}
		}
	}
	return gpsprot.SVID{GNSS: gnss, Num: svid}
}

// convertNavSatInfo converts CASIC satellite info to gpsprot format.
// Filters out satellites with CNO == 0 (no signal lock).
func convertNavSatInfo(fixed *casbin.NavSatInfoFixed, svs []casbin.NavSVInfo) []gpsprot.SVInfo {
	gnss := casicGNSSIDToGNSS(uint8(fixed.System))
	sigID := casicSignalID[fixed.System]
	result := make([]gpsprot.SVInfo, 0, len(svs))
	for i := range svs {
		sv := &svs[i]
		if sv.CNO == 0 {
			continue
		}
		used := sv.Flags&casbin.NavSVUsed != 0
		result = append(result, gpsprot.SVInfo{
			ID: casicSVID(gnss, sv.SVID),
			LookAngles: &gpsprot.LookAngles{
				Azimuth:   sv.Azim,
				Elevation: sv.Elev,
			},
			Signals: []gpsprot.SignalInfo{{
				ID:   sigID,
				CN0:  sv.CNO,
				Used: used,
			}},
			Used: used,
		})
	}
	return result
}

// casicSigIDMap maps CASIC V6 SigID to gpsprot.SignalID for per-signal display.
var casicSigIDMap = map[casbin.SigID]gpsprot.SignalID{
	casbin.SigGPSL1CA:   gpsprot.SigIDGPSL1CA,
	casbin.SigGPSL5:     gpsprot.SigIDGPSL5I,
	casbin.SigSBASL1:    gpsprot.SigIDGPSL1CA, // SBAS uses GPS L1 C/A
	casbin.SigSBASL5:    gpsprot.SigIDGPSL5I,
	casbin.SigGLOL1:     gpsprot.SigIDGLOL1,
	casbin.SigGALE1:     gpsprot.SigIDGALE1,
	casbin.SigGALE5a:    gpsprot.SigIDGALE5a,
	casbin.SigBDSB1IGEO: gpsprot.SigIDBDSB1I,
	casbin.SigBDSB1IMEO: gpsprot.SigIDBDSB1I,
	casbin.SigBDSB1C:    gpsprot.SigIDBDSB1C,
	casbin.SigBDSB2a:    gpsprot.SigIDBDSB2a,
	casbin.SigQZSSL1CA:  gpsprot.SigIDQZSSL1CA,
	casbin.SigQZSSL5:    gpsprot.SigIDQZSSL5I,
	casbin.SigNAVICL5:   gpsprot.SigIDNAVICL5,
}

// casicSigToSignal maps CASIC V6 SigID to gpsprot.Signal for SignalsUsed accumulation.
var casicSigToSignal = map[casbin.SigID]gpsprot.Signal{
	casbin.SigGPSL1CA:   gpsprot.SigGPSL1CA,
	casbin.SigGPSL5:     gpsprot.SigGPSL5,
	casbin.SigSBASL1:    gpsprot.SigSBASL1CA,
	casbin.SigSBASL5:    gpsprot.SigSBASL5,
	casbin.SigGLOL1:     gpsprot.SigGLOL1,
	casbin.SigGALE1:     gpsprot.SigGALE1,
	casbin.SigGALE5a:    gpsprot.SigGALE5a,
	casbin.SigBDSB1IGEO: gpsprot.SigBDSB1I,
	casbin.SigBDSB1IMEO: gpsprot.SigBDSB1I,
	casbin.SigBDSB1C:    gpsprot.SigBDSB1C,
	casbin.SigBDSB2a:    gpsprot.SigBDSB2a,
	casbin.SigQZSSL1CA:  gpsprot.SigQZSSL1CA,
	casbin.SigQZSSL5:    gpsprot.SigQZSSL5,
	casbin.SigNAVICL5:   gpsprot.SigNAVICL5,
}

// nav2SigSVID converts NAV2-SIG GNSSID and SVID to gpsprot.SVID.
// Unlike V5 where SBAS/QZSS are embedded in GPS messages by PRN range,
// V6 NAV2-SIG has explicit GNSS IDs. SBAS PRNs need offset subtraction (PRN-100).
func nav2SigSVID(gnssID uint8, svid uint8) gpsprot.SVID {
	gnss := casicGNSSIDToGNSS(gnssID)
	if gnss == gpsprot.SBAS {
		return gpsprot.SVID{GNSS: gpsprot.SBAS, Num: svid - 100}
	}
	return gpsprot.SVID{GNSS: gnss, Num: svid}
}

// satsNav2Sig converts NAV2-SIG per-signal entries to a SatellitesMsg.
// Groups entries by SVID, building per-satellite records with multiple SignalInfo entries.
func satsNav2Sig(m *casbin.Nav2Sig) *gpsprot.SatellitesMsg {
	sigIndex := make(map[gpsprot.SVID]int) // SVID -> index in svs
	svs := make([]gpsprot.SVInfo, 0)
	for i := range m.Sigs {
		sig := &m.Sigs[i]
		if sig.CNO == 0 {
			continue
		}
		svid := nav2SigSVID(sig.GNSSID, sig.SVID)
		idx, ok := sigIndex[svid]
		if !ok {
			idx = len(svs)
			svs = append(svs, gpsprot.SVInfo{
				ID: svid,
				LookAngles: &gpsprot.LookAngles{
					Azimuth:   int16(sig.Azim),
					Elevation: int8(sig.Elev),
				},
			})
			sigIndex[svid] = idx
		}
		used := sig.SolFlags&0x01 != 0
		sigs := &svs[idx].Signals
		*sigs = append(*sigs, gpsprot.SignalInfo{
			ID:   casicSigIDMap[sig.SigID],
			CN0:  sig.CNO,
			Used: used,
		})
	}
	// Propagate Used flag from signals to SVInfo
	for i := range svs {
		sv := &svs[i]
		for _, sig := range sv.Signals {
			if sig.Used {
				sv.Used = true
				break
			}
		}
	}
	if len(svs) == 0 {
		return nil
	}
	return &gpsprot.SatellitesMsg{
		SVs:          svs,
		Tag:          Tag,
		NativeMsgID:  "NAV2-SIG",
		UsedValidity: gpsprot.SatelliteUsedSignal,
	}
}

// corrFromNav2Sig accumulates correction info from NAV2-SIG per-signal CorFlags
// and SignalsUsed from per-signal SigID on the NavEpochMsg.
func corrFromNav2Sig(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sig) {
	if ne == nil {
		return
	}
	for i := range m.Sigs {
		sig := &m.Sigs[i]
		if sig.SolFlags&0x01 == 0 { // not used in solution
			continue
		}
		// Accumulate SignalsUsed
		if s, ok := casicSigToSignal[sig.SigID]; ok {
			ne.SignalsUsed |= gpsprot.SignalSetOf(s)
		}
		// Accumulate correction source
		switch sig.CorFlags & 0x07 {
		case 1: // SBAS
			ne.Correction |= gpsprot.CorrSBAS | gpsprot.CorrUsed
		case 2: // BDS B2b PPP
			ne.Correction |= gpsprot.CorrWideArea | gpsprot.CorrUsed
		case 3: // RTCM2
			ne.Correction |= gpsprot.CorrRTCM | gpsprot.CorrUsed
		case 4: // OSR
			ne.Correction |= gpsprot.CorrBaseStation | gpsprot.CorrRTCM | gpsprot.CorrUsed
		case 5: // SSR
			ne.Correction |= gpsprot.CorrWideArea | gpsprot.CorrRTCM | gpsprot.CorrUsed
		}
	}
}

// satAccum accumulates satellite info within an epoch.
// It does not track the epoch itself—that's PacketProcessor's job.
type satAccum struct {
	nEpochs   int              // number of complete epochs seen (for early-flush gating)
	received  uint8            // bit vector of GNSSID received this epoch (1<<GPS | 1<<BDS | 1<<GLN)
	predicted uint8            // bit vector of GNSS expected (from previous epoch)
	svs       []gpsprot.SVInfo // accumulated satellites
}

// accum adds satellite info from a NAV-xxxINFO message.
// May trigger early flush if all expected GNSS types received.
func (a *satAccum) accum(fixed *casbin.NavSatInfoFixed, svs []casbin.NavSVInfo, mh gpsprot.MsgHandler, tRead time.Time) {
	converted := convertNavSatInfo(fixed, svs)
	a.svs = append(a.svs, converted...)
	a.received |= 1 << fixed.System
	// Check early-flush conditions (only after seeing 2 complete epochs)
	if a.nEpochs >= 2 &&
		(a.received == 0x07 || (a.predicted != 0 && a.received == a.predicted)) {
		a.flush(mh, tRead)
	}
}

// epochChange is called by PacketProcessor.flushNavEpoch() on epoch change.
// Increments nEpochs then flushes.
func (a *satAccum) epochChange(mh gpsprot.MsgHandler, tRead time.Time) {
	a.nEpochs++
	a.flush(mh, tRead)
}

// flush emits the accumulated SatellitesMsg and resets state.
func (a *satAccum) flush(mh gpsprot.MsgHandler, tRead time.Time) {
	if len(a.svs) == 0 {
		a.predicted = a.received
		a.received = 0
		return
	}
	msg := &gpsprot.SatellitesMsg{
		SVs:          a.svs,
		Tag:          Tag,
		NativeMsgID:  "NAV-SATINFO",
		UsedValidity: gpsprot.SatelliteUsedSignal,
	}
	if mh != nil {
		mh.Satellites(msg, tRead)
	}
	a.predicted = a.received
	a.received = 0
	a.svs = nil
}
