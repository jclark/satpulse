package ubx

import (
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
		svid := gpsprot.SVID{GNSS: idToGNSS(usv.GNSSID), PRN: int16(usv.SVID)}
		if svid.GNSS == gpsprot.GLO && svid.PRN == 255 {
			// UBX uses 255 for unknown GLONASS SVID (NMEA uses null)
			svid.PRN = gpsprot.GLOUnknown
		}
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

func svInfoSVID(uSVID byte) gpsprot.SVID {
	g := gpsprot.GPS
	prn := int16(uSVID)
	if uSVID >= 120 && uSVID <= 158 {
		g = gpsprot.SBAS
	} else if uSVID >= 193 && uSVID <= 197 {
		g = gpsprot.QZSS
		prn -= 192
	} else if uSVID >= 65 && uSVID <= 96 {
		g = gpsprot.GLO
		prn -= 64
	} else if uSVID == 255 {
		g = gpsprot.GLO
		prn = gpsprot.GLOUnknown
	}
	return gpsprot.SVID{GNSS: g, PRN: prn}
}
