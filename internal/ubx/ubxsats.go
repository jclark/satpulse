package ubx

import (
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

func satellitesNavSat(u *bin.NavSat) *gpsprot.SatellitesMsg {
	info := make([]gpsprot.SVInfo, u.NumSVs)
	for i, usv := range u.SVs {
		info[i] = gpsprot.SVInfo{
			SVID: gpsprot.SVID{GNSS: idToGNSS(usv.GNSSID), PRN: int16(usv.SVID)},
			CNO:   usv.CNO,
			Azimuth: usv.Azim,
			Elevation: usv.Elev,
		}
	}
	return &gpsprot.SatellitesMsg{
		NavEpoch: iTOWEpoch(u.ITOW),
		Info: info,
		Tag: Tag,
		NativeMsgID: "UBX-NAV-SAT",
	}
}
