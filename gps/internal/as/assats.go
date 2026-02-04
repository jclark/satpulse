package as

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// NAV-SVINFO SVID to RINEX SVID Mapping
//
// Verified by correlating NAV-SVINFO messages with NMEA GSV sentences using
// azimuth/elevation matching from packet captures.
//
// Verified ranges:
//   SVID 1-32     -> GPS PRN 1-32        (Num = SVID)
//   SVID 65-96    -> GLONASS slot 1-32   (Num = SVID - 64)
//   SVID 193-199  -> QZSS PRN 1-7        (Num = SVID - 192)
//   SVID 201-263  -> BeiDou PRN 1-63     (Num = SVID - 200)
//   SVID 301-336  -> Galileo PRN 1-36    (Num = SVID - 300)
//
// SBAS is unverified (no SBAS satellites in capture). Plausible ranges based on asnmea.go:
//   SVID 33-64    -> SBAS PRN 120-151    (NMEA-style: Num = SVID + 87)
//   SVID 120-158  -> SBAS PRN 120-158    (Native: Num = SVID)
//
// NavIC/IRNSS is unverified. Plausible ranges based on asnmea.go:
//   SVID 401-414  -> NavIC PRN 1-14      (NMEA-style: Num = SVID - 400)
//   SVID 901-914  -> NavIC PRN 1-14      (Allystar extended: Num = SVID - 900)

// navSVInfoRange defines an SVID range for NAV-SVINFO messages.
type navSVInfoRange struct {
	minSVID uint16
	maxSVID uint16
	gnss    gpsprot.GNSS
	offset  int16 // Num = SVID + offset (can be negative)
}

// navSVInfoRanges defines the SVID mapping for Allystar NAV-SVINFO messages.
var navSVInfoRanges = []navSVInfoRange{
	{1, 32, gpsprot.GPS, 0},         // GPS PRN 1-32
	{33, 64, gpsprot.SBAS, 87},      // SBAS NMEA-style (unverified)
	{65, 96, gpsprot.GLO, -64},      // GLONASS slot 1-32
	{120, 158, gpsprot.SBAS, 0},     // SBAS native PRN (unverified)
	{193, 199, gpsprot.QZSS, -192},  // QZSS PRN 1-7
	{201, 263, gpsprot.BDS, -200},   // BeiDou PRN 1-63
	{301, 336, gpsprot.GAL, -300},   // Galileo PRN 1-36
	{401, 414, gpsprot.NAVIC, -400}, // NavIC NMEA-style (unverified)
	{901, 914, gpsprot.NAVIC, -900}, // NavIC Allystar extended (unverified)
}

// asSVID converts an Allystar NAV-SVINFO SVID to a gpsprot.SVID.
func asSVID(svid uint16) gpsprot.SVID {
	for _, r := range navSVInfoRanges {
		if svid >= r.minSVID && svid <= r.maxSVID {
			num := uint8(int16(svid) + r.offset)
			return gpsprot.SVID{GNSS: r.gnss, Num: num}
		}
	}
	return gpsprot.SVID{}
}

// satellitesNavSVInfo converts asbin.NavSVInfo to gpsprot.SatellitesMsg.
func satellitesNavSVInfo(m *asbin.NavSVInfo) *gpsprot.SatellitesMsg {
	svs := make([]gpsprot.SVInfo, 0, len(m.Sats))
	for _, asv := range m.Sats {
		if asv.Quality < asbin.NavSVInfoQualityCodeLocked {
			continue
		}
		svid := asSVID(asv.Svid)
		if !svid.IsValid() {
			continue
		}
		svs = append(svs, gpsprot.SVInfo{
			ID: svid,
			Signals: []gpsprot.SignalInfo{{
				CN0: asv.Cno,
			}},
			LookAngles: &gpsprot.LookAngles{
				Azimuth:   asv.Azim,
				Elevation: asv.Elev,
			},
			Used: asv.Flags&asbin.NavSVInfoFlagUsed != 0,
		})
	}
	return &gpsprot.SatellitesMsg{
		SVs:          svs,
		Tag:          Tag,
		NativeMsgID:  "NAV-SVINFO",
		UsedValidity: gpsprot.SatelliteUsedSV,
	}
}
