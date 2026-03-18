package sdbp

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

// gnssMap maps sdbpbin.GNSSID to gpsprot.GNSS.
var gnssMap = map[sdbpbin.GNSSID]gpsprot.GNSS{
	sdbpbin.BDS:  gpsprot.BDS,
	sdbpbin.GPS:  gpsprot.GPS,
	sdbpbin.QZSS: gpsprot.QZSS,
	sdbpbin.SBAS: gpsprot.SBAS,
	sdbpbin.GAL:  gpsprot.GAL,
	sdbpbin.GLO:  gpsprot.GLO,
}

// signalIDMap maps sdbpbin.GNSSID and sdbpbin.SignalID to gpsprot.SignalID.
// Signal ID 7 is undocumented and used across all constellations for
// satellites known from almanac/ephemeris but not tracked (CN0=0).
// It is not in this table and such entries are skipped.
var signalIDMap = map[sdbpbin.GNSSID]map[sdbpbin.SignalID]gpsprot.SignalID{
	sdbpbin.BDS: {
		sdbpbin.SigBDSB1I: gpsprot.SigIDBDSB1I,
		sdbpbin.SigBDSB1C: gpsprot.SigIDBDSB1C,
		sdbpbin.SigBDSB2I: gpsprot.SigIDBDSB2I,
		sdbpbin.SigBDSB2a: gpsprot.SigIDBDSB2a,
		sdbpbin.SigBDSB2b: gpsprot.SigIDBDSB2b,
		sdbpbin.SigBDSB3I: gpsprot.SigIDBDSB3I,
	},
	sdbpbin.GPS: {
		sdbpbin.SigGPSL1CA: gpsprot.SigIDGPSL1CA,
		sdbpbin.SigGPSL1C:  gpsprot.SigIDGPSL1C,
		sdbpbin.SigGPSL2C:  gpsprot.SigIDGPSL2CM,
		sdbpbin.SigGPSL2P:  gpsprot.SigIDGPSL2P,
		sdbpbin.SigGPSL5:   gpsprot.SigIDGPSL5I,
	},
	sdbpbin.QZSS: {
		sdbpbin.SigQZSSL1CA: gpsprot.SigIDQZSSL1CA,
		sdbpbin.SigQZSSL1C:  gpsprot.SigIDQZSSL1CD,
		sdbpbin.SigQZSSL2C:  gpsprot.SigIDQZSSL2CM,
		// SigQZSSL2P: not a real signal (protocol doc mirrors GPS numbering)
		sdbpbin.SigQZSSL5: gpsprot.SigIDQZSSL5I,
	},
	sdbpbin.SBAS: {
		sdbpbin.SigSBASL1CA: gpsprot.SigIDGPSL1CA,
	},
	sdbpbin.GAL: {
		sdbpbin.SigGALE1:  gpsprot.SigIDGALE1,
		sdbpbin.SigGALE5a: gpsprot.SigIDGALE5a,
		sdbpbin.SigGALE5b: gpsprot.SigIDGALE5b,
	},
	sdbpbin.GLO: {
		sdbpbin.SigGLOG1: gpsprot.SigIDGLOL1,
		sdbpbin.SigGLOG2: gpsprot.SigIDGLOL2,
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
		gnss, ok := gnssMap[s.GNSSID]
		if !ok {
			continue
		}
		sigMap := signalIDMap[s.GNSSID]
		sigID, ok := sigMap[s.SignalID]
		if !ok {
			continue
		}
		used := s.Flags&datSATUsedFlag != 0
		msg.SVs = append(msg.SVs, gpsprot.SVInfo{
			ID: gpsprot.SVID{GNSS: gnss, Num: s.SatID},
			LookAngles: &gpsprot.LookAngles{
				Azimuth:   int16(s.Azim),
				Elevation: s.Elev,
			},
			Signals: []gpsprot.SignalInfo{{
				ID:   sigID,
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
