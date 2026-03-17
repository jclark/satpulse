package sdbp

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

// sdbpGNSS maps SDBP GNSS ID to gpsprot.GNSS.
var sdbpGNSS = [6]gpsprot.GNSS{
	0: gpsprot.BDS,
	1: gpsprot.GPS,
	2: gpsprot.QZSS,
	3: gpsprot.SBAS,
	4: gpsprot.GAL,
	5: gpsprot.GLO,
}

type sigEntry struct {
	sig gpsprot.Signal
	id  gpsprot.SignalID
}

// sdbpSignal maps [gnssID][signalID] to Signal and SignalID.
// Signal ID 7 is undocumented and used across all constellations for
// satellites known from almanac/ephemeris but not tracked (CN0=0).
// It is not in this table and such entries are skipped.
var sdbpSignal = [6][]sigEntry{
	// BDS (GNSS ID 0)
	0: {
		{gpsprot.SigBDSB1I, gpsprot.SigIDBDSB1I},
		{gpsprot.SigBDSB1C, gpsprot.SigIDBDSB1C},
		{gpsprot.SigBDSB2I, gpsprot.SigIDBDSB2I},
		{gpsprot.SigBDSB2a, gpsprot.SigIDBDSB2a},
		{gpsprot.SigBDSB2b, gpsprot.SigIDBDSB2b},
		{gpsprot.SigBDSB3I, gpsprot.SigIDBDSB3I},
	},
	// GPS (GNSS ID 1)
	1: {
		{gpsprot.SigGPSL1CA, gpsprot.SigIDGPSL1CA},
		{gpsprot.SigGPSL1C, gpsprot.SigIDGPSL1C},
		{gpsprot.SigGPSL2C, gpsprot.SigIDGPSL2CM},
		{gpsprot.SigGPSL2P, gpsprot.SigIDGPSL2P},
		{gpsprot.SigGPSL5, gpsprot.SigIDGPSL5I},
	},
	// QZSS (GNSS ID 2)
	2: {
		{gpsprot.SigQZSSL1CA, gpsprot.SigIDQZSSL1CA},
		{gpsprot.SigQZSSL1C, gpsprot.SigIDQZSSL1CD},
		{gpsprot.SigQZSSL2C, gpsprot.SigIDQZSSL2CM},
		{0, ""}, // L2P - no gpsprot equivalent
		{gpsprot.SigQZSSL5, gpsprot.SigIDQZSSL5I},
	},
	// SBAS (GNSS ID 3)
	3: {
		{gpsprot.SigSBASL1CA, gpsprot.SigIDGPSL1CA},
	},
	// Galileo (GNSS ID 4)
	4: {
		{gpsprot.SigGALE1, gpsprot.SigIDGALE1},
		{gpsprot.SigGALE5a, gpsprot.SigIDGALE5a},
		{gpsprot.SigGALE5b, gpsprot.SigIDGALE5b},
	},
	// GLONASS (GNSS ID 5)
	5: {
		{gpsprot.SigGLOL1, gpsprot.SigIDGLOL1},
		{gpsprot.SigGLOL2, gpsprot.SigIDGLOL2},
	},
}

const datSATUsedFlag = 1 << 7

// satsDatSAT converts DatSAT to SatellitesMsg.
func satsDatSAT(m *sdbpbin.DatSAT) *gpsprot.SatellitesMsg {
	if len(m.Sats) == 0 {
		return nil
	}
	msg := &gpsprot.SatellitesMsg{
		NativeMsgID:  "DAT-SAT",
		UsedValidity: gpsprot.SatelliteUsedSignal,
	}
	for i := range m.Sats {
		s := &m.Sats[i]
		if int(s.GNSSID) >= len(sdbpGNSS) {
			continue
		}
		gnss := sdbpGNSS[s.GNSSID]
		sigs := sdbpSignal[s.GNSSID]
		if int(s.SignalID) >= len(sigs) || sigs[s.SignalID].id == "" {
			continue
		}
		se := sigs[s.SignalID]
		used := s.Flags&datSATUsedFlag != 0
		msg.SVs = append(msg.SVs, gpsprot.SVInfo{
			ID: gpsprot.SVID{GNSS: gnss, Num: uint8(s.SatID)},
			LookAngles: &gpsprot.LookAngles{
				Azimuth:   int16(s.Azim),
				Elevation: s.Elev,
			},
			Signals: []gpsprot.SignalInfo{{
				ID:   se.id,
				CN0:  s.CN0,
				Used: used,
			}},
			Used: used,
		})
	}
	if len(msg.SVs) == 0 {
		return nil
	}
	return msg
}
