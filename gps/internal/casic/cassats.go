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
	gnss := gnssIDToGNSS(fixed.System)
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
