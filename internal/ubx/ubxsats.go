package ubx

import (
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

func satellitesNavSat(u *bin.NavSat) *gpsprot.SatellitesMsg {
	// This is the minimum quality signal we require to include in the SatellitesMsg.
	// With CodeLocked, we have a CN0 of > 0 and it's reasonably stable.
	const minQuality = bin.NavSatQualityCodeLocked
	info := make([]gpsprot.SVInfo, 0, u.NumSVs)
	for _, usv := range u.SVs {
		if usv.Flags&bin.NavSatQuality < minQuality {
			continue
		}
		svid := gpsprot.SVID{GNSS: idToGNSS(usv.GNSSID), PRN: int16(usv.SVID)}
		if svid.GNSS == gpsprot.GLO && svid.PRN == 255 {
			// UBX uses 255 for unknown GLONASS SVID (NMEA uses null)
			svid.PRN = gpsprot.GLOUnknown
		}
		info = append(info, gpsprot.SVInfo{
			SVID:      svid,
			CNO:       usv.CNO,
			Azimuth:   usv.Azim,
			Elevation: usv.Elev,
		})
	}
	return &gpsprot.SatellitesMsg{
		NavEpoch:    iTOWEpoch(u.ITOW),
		Info:        info,
		Tag:         Tag,
		NativeMsgID: "UBX-NAV-SAT",
	}
}
